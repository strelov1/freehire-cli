package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search jobs by keyword, with optional facet filters",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			limit, _ := cmd.Flags().GetInt("limit")
			offset, _ := cmd.Flags().GetInt("offset")

			facets, err := facetsFromFlags(cmd)
			if err != nil {
				return err
			}
			// --skills is a market filter here (jobs listing the skill), so it joins
			// the facet params — unlike market-fit, where --skills is the measured set.
			if skills, _ := cmd.Flags().GetStringArray("skills"); len(skills) > 0 {
				facets["skills"] = append(facets["skills"], skills...)
			}

			res, err := c.Search(cmd.Context(), client.SearchParams{
				Query:  strings.Join(args, " "),
				Limit:  limit,
				Offset: offset,
				Facets: facets,
			})
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, res.Data)
				return nil
			}
			var rows []jobRow
			if err := json.Unmarshal(res.Data, &rows); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, r := range rows {
				fmt.Fprintf(out, "%-40s  %-20s  %-16s  %s\n",
					trunc(r.Title, 40), trunc(r.Company, 20), trunc(r.Location, 16), r.PublicSlug)
			}
			fmt.Fprintf(out, "\n%d of %d shown\n", len(rows), res.Total)
			return nil
		},
	}
	cmd.Flags().Int("limit", 20, "maximum results")
	cmd.Flags().Int("offset", 0, "results offset for paging")
	addFacetFlags(cmd)
	cmd.Flags().StringArray("skills", nil, "filter by skill, e.g. go, react (repeatable)")
	return cmd
}

func newJobCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "job <slug>",
		Short: "Show a job's content by its slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.GetJob(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var j jobDetail
			if err := json.Unmarshal(data, &j); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s\n%s (%s) · %s\n%s\n\n%s\n",
				j.Title, j.Company, j.CompanySlug, j.Location, j.URL, j.Description)
			return nil
		},
	}
}

func newApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <slug>",
		Short: "Mark a job as applied for your account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.Apply(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Marked applied: %s\n", args[0])
			return nil
		},
	}
}

// trunc shortens s to at most n bytes, appending an ellipsis when it cuts.
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func newSaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "save <slug>",
		Short: "Bookmark a job for later",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.Save(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Saved: %s\n", args[0])
			return nil
		},
	}
}

func newUnsaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unsave <slug>",
		Short: "Remove a job's bookmark",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.Unsave(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Unsaved: %s\n", args[0])
			return nil
		},
	}
}

func newMyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "my",
		Short: "List your tracked jobs (viewed / saved / applied)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			filter, _ := cmd.Flags().GetString("filter")
			limit, _ := cmd.Flags().GetInt("limit")
			offset, _ := cmd.Flags().GetInt("offset")
			res, err := c.MyJobs(cmd.Context(), filter, limit, offset)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, res.Data)
				return nil
			}
			var rows []myJobRow
			if err := json.Unmarshal(res.Data, &rows); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, r := range rows {
				note := ""
				if r.Notes != nil && *r.Notes != "" {
					note = "  — " + trunc(*r.Notes, 40)
				}
				fmt.Fprintf(out, "%-40s  %-20s  %-10s  %s%s\n",
					trunc(r.Job.Title, 40), trunc(r.Job.Company, 20), r.status(), r.Job.PublicSlug, note)
			}
			fmt.Fprintf(out, "\n%d of %d shown\n", len(rows), res.Total)
			return nil
		},
	}
	cmd.Flags().String("filter", "all", "all | viewed | saved | applied")
	cmd.Flags().Int("limit", 20, "maximum results")
	cmd.Flags().Int("offset", 0, "results offset for paging")
	return cmd
}

// myJobRow is one row of the `my` listing: the job plus the caller's interaction
// timestamps, application stage, and notes.
type myJobRow struct {
	Job       jobRow  `json:"job"`
	SavedAt   *string `json:"saved_at"`
	AppliedAt *string `json:"applied_at"`
	Stage     *string `json:"stage"`
	Notes     *string `json:"notes"`
}

// state renders the interaction as a short tag for the human listing.
func (r myJobRow) state() string {
	switch {
	case r.AppliedAt != nil:
		return "applied"
	case r.SavedAt != nil:
		return "saved"
	default:
		return "viewed"
	}
}

// status is the most specific label for the row: the application stage when the
// user has set one, otherwise the coarse interaction state.
func (r myJobRow) status() string {
	if r.Stage != nil && *r.Stage != "" {
		return *r.Stage
	}
	return r.state()
}
