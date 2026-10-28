package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// Job alerts are served by the signed-in MCP server's tools, not by REST: that is the
// one place holding their whole rule (which channels an account can receive on, the
// match ledger behind `updates`). Each subcommand is one tool call, and its refusals
// arrive as the server's own sentences ("nothing was changed: …"), printed as is.

func newAlertsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alerts",
		Short: "Manage job alerts: saved searches delivered by email, Telegram, the app or a webhook",
		Long: `Manage job alerts: saved searches over any search filter whose new postings are
delivered on the channels your account has connected (email, Telegram, the
mobile app, a webhook). Delivery timing is account-wide and set on the website.

Run the same filters through ` + "`freehire search`" + ` first to see what an alert
would catch.`,
	}
	cmd.AddCommand(newAlertsListCmd(), newAlertsAddCmd(), newAlertsUpdateCmd(),
		newAlertsPausedCmd("pause", true), newAlertsPausedCmd("resume", false),
		newAlertsRemoveCmd(), newAlertsUpdatesCmd())
	return cmd
}

// callAlertTool calls one alert tool as the signed-in user and returns its result.
func callAlertTool(cmd *cobra.Command, tool string, args map[string]any) (json.RawMessage, error) {
	c, _, err := authedClient(cmd)
	if err != nil {
		return nil, err
	}
	data, err := c.CallTool(cmd.Context(), tool, args)
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 401 {
		return nil, fmt.Errorf("job alerts need a signed-in account: run `freehire auth login` (%w)", err)
	}
	return data, err
}

type alertChannel struct {
	Channel string `json:"channel"`
	Active  bool   `json:"active"`
	Pending int64  `json:"pending"`
}

type alert struct {
	ID       int64          `json:"id"`
	Name     string         `json:"name"`
	Filter   string         `json:"filter"`
	Paused   bool           `json:"paused"`
	Channels []alertChannel `json:"channels"`
}

func printAlert(cmd *cobra.Command, a alert) {
	chans := make([]string, 0, len(a.Channels))
	for _, c := range a.Channels {
		chans = append(chans, c.Channel)
	}
	state := ""
	if a.Paused {
		state = "  [paused]"
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "#%d  %s%s  → %s\n", a.ID, a.Name, state, strings.Join(chans, ", "))
	if a.Filter != "" {
		fmt.Fprintf(out, "     %s\n", a.Filter)
	}
}

// warnDropped reports the filter values an alert tool dropped as not understood. It
// keeps them off the alert rather than letting them narrow it to nothing, so the alert
// that was saved is wider than the one asked for.
func warnDropped(cmd *cobra.Command, dropped []string) {
	for _, v := range dropped {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: dropped unrecognised filter value %s — see `freehire facets`\n", v)
	}
}

func newAlertsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List your job alerts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := callAlertTool(cmd, "list_subscriptions", map[string]any{})
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var res struct {
				Alerts   []alert `json:"alerts"`
				Schedule struct {
					Frequency string `json:"digest_frequency"`
				} `json:"delivery_schedule"`
			}
			if err := json.Unmarshal(data, &res); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(res.Alerts) == 0 {
				fmt.Fprintln(out, "no job alerts — create one with `freehire alerts add`")
				return nil
			}
			for _, a := range res.Alerts {
				printAlert(cmd, a)
			}
			if res.Schedule.Frequency != "" {
				fmt.Fprintf(out, "\ndelivery: %s (account-wide)\n", res.Schedule.Frequency)
			}
			return nil
		},
	}
}

// addAlertSearchFlags registers the search part of an alert: search's own filter
// flags plus a salary ceiling and the channels to deliver on.
func addAlertSearchFlags(cmd *cobra.Command) {
	addFacetFlags(cmd)
	cmd.Flags().StringArray("skills", nil, "filter by skill, e.g. go, react (repeatable)")
	cmd.Flags().Int("salary-max", 0, "maximum salary")
	cmd.Flags().String("name", "", "alert name (1-100 characters)")
	cmd.Flags().StringArray("channel", nil, "deliver on: email|telegram|push|webhook (repeatable; must be connected)")
}

// filterFlags are every flag that contributes to an alert's facet filters.
func filterFlags() []string {
	names := []string{"remote", "visa", "facet", "skills"}
	for _, f := range namedFacets {
		names = append(names, f.flag)
	}
	return names
}

// alertSearchArgs turns the flags into the tools' search arguments. With onlyChanged
// (update), a part is sent only when one of its flags was given, because a sent part
// REPLACES the alert's: the filter flags together replace every facet filter, and a
// salary bound of 0 clears it.
func alertSearchArgs(cmd *cobra.Command, onlyChanged bool) (map[string]any, error) {
	args := map[string]any{}
	facets, err := facetsFromFlags(cmd)
	if err != nil {
		return nil, err
	}
	if skills, _ := cmd.Flags().GetStringArray("skills"); len(skills) > 0 {
		facets["skills"] = append(facets["skills"], skills...)
	}
	// The salary floor travels beside the filters, not in them.
	facets.Del("salary_min")

	if onlyChanged {
		// An update sends filters only when asked to touch them: given, they replace
		// the alert's whole set, so an empty set (--clear-filters) clears it.
		touched, _ := cmd.Flags().GetBool("clear-filters")
		for _, f := range filterFlags() {
			touched = touched || cmd.Flags().Changed(f)
		}
		if touched {
			args["filters"] = map[string][]string(facets)
		}
	} else if len(facets) > 0 {
		args["filters"] = map[string][]string(facets)
	}
	for _, f := range []struct{ flag, arg string }{{"salary-min", "salary_min"}, {"salary-max", "salary_max"}} {
		v, _ := cmd.Flags().GetInt(f.flag)
		if v > 0 || onlyChanged && cmd.Flags().Changed(f.flag) {
			args[f.arg] = v
		}
	}
	if name, _ := cmd.Flags().GetString("name"); name != "" || onlyChanged && cmd.Flags().Changed("name") {
		args["name"] = name
	}
	if chans, _ := cmd.Flags().GetStringArray("channel"); len(chans) > 0 {
		args["channels"] = chans
	}
	return args, nil
}

func newAlertsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [query]",
		Short: "Create a job alert from a query and/or search filters",
		Long: `Create a job alert from a query and/or search filters. The search must narrow
the catalogue — an alert on everything is refused.

  freehire alerts add --skills go --remote --region eu
  freehire alerts add 'title:"data engineer"' --country DE --name "Data DE" --channel telegram

Without --channel it delivers on email when alert mail is on, otherwise on the
first connected channel.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs, err := alertSearchArgs(cmd, false)
			if err != nil {
				return err
			}
			if q := strings.TrimSpace(strings.Join(args, " ")); q != "" {
				toolArgs["query"] = q
			}
			data, err := callAlertTool(cmd, "subscribe", toolArgs)
			if err != nil {
				return err
			}
			return printAlertResult(cmd, data, "created")
		},
	}
	addAlertSearchFlags(cmd)
	return cmd
}

func newAlertsUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Change a job alert; only the parts you pass change",
		Long: `Change a job alert. Only the parts you pass change:

  --name                 renames it
  --query                replaces the text query ("" clears it)
  any filter flag        REPLACES all its facet filters (--clear-filters clears them)
  --salary-min/max       replaces that bound (0 clears it)
  --channel              the complete set of channels to deliver on

Filters only the website sets (exclusions, match-all modes) are kept.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := alertID(args[0])
			if err != nil {
				return err
			}
			toolArgs, err := alertSearchArgs(cmd, true)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("query") {
				q, _ := cmd.Flags().GetString("query")
				toolArgs["query"] = q
			}
			if len(toolArgs) == 0 {
				return errors.New("nothing to change: pass --name, --query, a filter flag, a salary bound or --channel")
			}
			toolArgs["id"] = id
			data, err := callAlertTool(cmd, "update_subscription", toolArgs)
			if err != nil {
				return err
			}
			return printAlertResult(cmd, data, "updated")
		},
	}
	addAlertSearchFlags(cmd)
	cmd.Flags().String("query", "", "new text query (\"\" clears it)")
	cmd.Flags().Bool("clear-filters", false, "remove every facet filter (the query must still narrow the search)")
	return cmd
}

func newAlertsPausedCmd(use string, paused bool) *cobra.Command {
	short := "Pause a job alert on every channel (keeps it for later)"
	if !paused {
		short = "Resume a paused job alert"
	}
	return &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := alertID(args[0])
			if err != nil {
				return err
			}
			data, err := callAlertTool(cmd, "update_subscription", map[string]any{"id": id, "paused": paused})
			if err != nil {
				return err
			}
			if paused {
				return printAlertResult(cmd, data, "paused")
			}
			return printAlertResult(cmd, data, "resumed")
		},
	}
}

func newAlertsRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Stop a job alert on every channel, or on the one named",
		Long: `Stop a job alert on every channel, or only on --channel. The saved search stays
on your account as a bookmark. To stop it for a while, use pause instead.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := alertID(args[0])
			if err != nil {
				return err
			}
			toolArgs := map[string]any{"id": id}
			if ch, _ := cmd.Flags().GetString("channel"); ch != "" {
				toolArgs["channel"] = ch
			}
			data, err := callAlertTool(cmd, "unsubscribe", toolArgs)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var res struct {
				Stopped []string `json:"stopped_channels"`
			}
			if err := json.Unmarshal(data, &res); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "stopped alert #%d on %s\n", id, strings.Join(res.Stopped, ", "))
			return nil
		},
	}
	cmd.Flags().String("channel", "", "stop only this channel")
	return cmd
}

func newAlertsUpdatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "updates [id]",
		Short: "List the new postings your job alerts matched, newest first",
		Long: `List the new postings your job alerts matched, newest first — still-open
postings only, all alerts or the one given. An alert only matches postings that
appear after it was created, so a new one starts empty.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if len(args) == 1 {
				id, err := alertID(args[0])
				if err != nil {
					return err
				}
				toolArgs["id"] = id
			}
			if v, _ := cmd.Flags().GetString("since"); v != "" {
				toolArgs["since"] = v
			}
			if v, _ := cmd.Flags().GetInt("days"); v > 0 {
				toolArgs["days"] = v
			}
			if v, _ := cmd.Flags().GetInt("limit"); v > 0 {
				toolArgs["limit"] = v
			}
			data, err := callAlertTool(cmd, "subscription_updates", toolArgs)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var res struct {
				Since    string `json:"since"`
				Postings []struct {
					Title    string   `json:"title"`
					Company  string   `json:"company"`
					Location string   `json:"location"`
					URL      string   `json:"url"`
					Alerts   []string `json:"alerts"`
				} `json:"postings"`
				Truncated bool `json:"truncated"`
			}
			if err := json.Unmarshal(data, &res); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(res.Postings) == 0 {
				fmt.Fprintf(out, "no matches since %s\n", calendarDay(res.Since))
				return nil
			}
			for _, p := range res.Postings {
				fmt.Fprintf(out, "%-40s  %-20s  %-16s  %s\n",
					trunc(p.Title, 40), trunc(p.Company, 20), trunc(p.Location, 16), p.URL)
			}
			if res.Truncated {
				fmt.Fprintln(out, "\nmore matches than shown — raise --limit or narrow --days")
			}
			return nil
		},
	}
	cmd.Flags().String("since", "", "only matches at or after this RFC 3339 time")
	cmd.Flags().Int("days", 0, "look back this many days (default 7, max 30)")
	cmd.Flags().Int("limit", 0, "how many postings")
	return cmd
}

// printAlertResult prints the alert a write returned, after any values it dropped.
func printAlertResult(cmd *cobra.Command, data json.RawMessage, verb string) error {
	var res struct {
		Alert    alert    `json:"alert"`
		Dropped  []string `json:"ignored_params"`
		Outcomes []struct {
			Channel string `json:"channel"`
			Status  string `json:"status"`
		} `json:"channels_outcome"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return err
	}
	warnDropped(cmd, res.Dropped)
	if wantJSON(cmd) {
		printJSON(cmd, data)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", verb)
	printAlert(cmd, res.Alert)
	for _, o := range res.Outcomes {
		if o.Status == "already_subscribed" {
			fmt.Fprintf(cmd.OutOrStdout(), "     (%s was already delivering this search)\n", o.Channel)
		}
	}
	return nil
}

func alertID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid alert id %q — see `freehire alerts list`", s)
	}
	return id, nil
}
