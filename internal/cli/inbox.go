package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// signalVocabulary lists the classification labels `inbox triage` accepts, for the
// command's help. The API validates the value (it is the source of truth), so an
// unknown signal still reaches the server and surfaces its error — this list is
// guidance only and cannot drift the enforcement.
var signalVocabulary = []string{
	"acknowledgement", "screening", "interview_invitation", "assessment",
	"offer", "rejection", "info_request", "incomplete_application", "other",
}

// inboxRow is the subset of an inbox message shown in CLI output. BodyText is
// present only when --body was passed.
type inboxRow struct {
	ID           int64  `json:"id"`
	Source       string `json:"source"`
	FromName     string `json:"from_name"`
	FromAddr     string `json:"from_addr"`
	Subject      string `json:"subject"`
	Snippet      string `json:"snippet"`
	BodyText     string `json:"body_text"`
	ReceivedAt   string `json:"received_at"`
	Read         bool   `json:"read"`
	StatusSignal string `json:"status_signal"`
	LinkedSlug   string `json:"linked_slug"`
}

// newInboxCmd is the `inbox` group: read the caller's mail and record an agent's
// triage of it. freehire does not fetch mail here — the caller's own mail client
// does that, and `inbox push` is where its output enters the inbox.
func newInboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Read and triage your job-application mail",
		Long: "Read the mail linked to your job search and record what each message is.\n\n" +
			"freehire fetches no mail here: your own client (himalaya, mbsync, anything\n" +
			"IMAP) does that, and `inbox push` hands the result over. `inbox list\n" +
			"--unclassified --body` is then the agent's work queue, and `inbox triage`\n" +
			"records its verdict and moves the application's stage.",
	}
	cmd.AddCommand(
		newInboxListCmd(), newInboxReadCmd(), newInboxPushCmd(), newInboxTriageCmd(),
		newInboxLinkCmd(), newInboxUnlinkCmd(), newInboxReadAllCmd(),
		newInboxDeleteCmd(), newInboxRestoreCmd(),
	)
	return cmd
}

// inboxFilterFlags registers the listing filters shared by `list` and `read-all`.
func inboxFilterFlags(cmd *cobra.Command) {
	cmd.Flags().String("source", "", "narrow to one account: gmail, hosted, or external")
	cmd.Flags().String("status", "", "narrow to one classified label ("+strings.Join(signalVocabulary, ", ")+")")
	cmd.Flags().String("query", "", "match subject, sender, or body")
}

func inboxFilters(cmd *cobra.Command) client.InboxParams {
	return client.InboxParams{
		Source: mustString(cmd, "source"),
		Status: mustString(cmd, "status"),
		Query:  mustString(cmd, "query"),
	}
}

func newInboxListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your mail, newest first",
		Long: "List your mail. --unclassified is the agent's work queue: messages\n" +
			"nothing has judged yet. --body returns each message's readable text inline,\n" +
			"so a whole page can be classified in one call — and, unlike `inbox read`,\n" +
			"it marks nothing read. Pages carrying bodies are capped at 50.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			p := inboxFilters(cmd)
			p.Unread = mustBool(cmd, "unread")
			p.Unclassified = mustBool(cmd, "unclassified")
			p.WithBody = mustBool(cmd, "body")
			p.Limit, _ = cmd.Flags().GetInt("limit")
			p.Offset, _ = cmd.Flags().GetInt("offset")
			res, err := c.Inbox(cmd.Context(), p)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, res.Data)
				return nil
			}
			var rows []inboxRow
			if err := json.Unmarshal(res.Data, &rows); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, r := range rows {
				fmt.Fprintf(out, "%d\t%s\t%s\t%s%s\n",
					r.ID, r.Source, sender(r), trunc(r.Subject, 60), inboxRowSuffix(r))
			}
			fmt.Fprintf(out, "\n%d of %d\n", len(rows), res.Total)
			return nil
		},
	}
	inboxFilterFlags(cmd)
	cmd.Flags().Bool("unread", false, "only messages you have not opened")
	cmd.Flags().Bool("unclassified", false, "only messages awaiting triage (the agent's work queue)")
	cmd.Flags().Bool("body", false, "include each message's readable body (does not mark them read)")
	cmd.Flags().Int("limit", 20, "page size (capped at 50 with --body)")
	cmd.Flags().Int("offset", 0, "page offset")
	return cmd
}

// sender renders a message's sender, preferring the display name.
func sender(r inboxRow) string {
	if r.FromName != "" {
		return trunc(r.FromName, 24)
	}
	return trunc(r.FromAddr, 24)
}

// inboxRowSuffix appends the triage state a row carries, when it has any.
func inboxRowSuffix(r inboxRow) string {
	var parts []string
	if r.StatusSignal != "" {
		parts = append(parts, r.StatusSignal)
	}
	if r.LinkedSlug != "" {
		parts = append(parts, "→ "+r.LinkedSlug)
	}
	if len(parts) == 0 {
		return ""
	}
	return "\t[" + strings.Join(parts, " ") + "]"
}

func newInboxReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read <id>",
		Short: "Read one message in full",
		Long: "Read one message in full. This marks it read — when sweeping a backlog " +
			"use `inbox list --body` instead, which does not.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := authedEmail(cmd, args[0])
			if err != nil {
				return err
			}
			data, err := c.GetEmail(cmd.Context(), id)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var r inboxRow
			if err := json.Unmarshal(data, &r); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "From: %s <%s>\nSubject: %s\nDate: %s\n\n%s\n",
				r.FromName, r.FromAddr, r.Subject, r.ReceivedAt, r.BodyText)
			return nil
		},
	}
}

func newInboxPushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Upload mail your own client fetched",
		Long: "Upload a batch of messages your own mail client fetched, as JSON on stdin\n" +
			"or from --file. Accepts either a bare array or {\"messages\": [...]}.\n\n" +
			"Each message needs an external_id — its Message-ID header — which is the\n" +
			"deduplication key: re-pushing the same id updates that message instead of\n" +
			"storing a copy, so a nightly re-sync is safe. At most 100 per call.\n\n" +
			"Fields: external_id (required), thread_id, from_addr, from_name, subject,\n" +
			"body_text, body_html, received_at (RFC3339).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			raw, err := readBatch(cmd, mustString(cmd, "file"))
			if err != nil {
				return err
			}
			msgs, err := decodeBatch(raw)
			if err != nil {
				return err
			}
			res, err := c.PushEmails(cmd.Context(), msgs)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pushed: %d new, %d updated\n", res.Inserted, res.Updated)
			return nil
		},
	}
	cmd.Flags().String("file", "", "read the batch from a file instead of stdin")
	return cmd
}

// readBatch reads the push payload from a file or stdin.
func readBatch(cmd *cobra.Command, path string) ([]byte, error) {
	if path != "" {
		return os.ReadFile(path)
	}
	return io.ReadAll(cmd.InOrStdin())
}

// decodeBatch accepts either a bare JSON array of messages or the wrapped
// {"messages": [...]} shape, so a caller can pipe `jq` output straight in.
func decodeBatch(raw []byte) ([]client.IngestMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, errors.New("no messages: pass JSON on stdin or use --file")
	}
	if strings.HasPrefix(trimmed, "[") {
		var msgs []client.IngestMessage
		if err := json.Unmarshal(raw, &msgs); err != nil {
			return nil, fmt.Errorf("decode messages: %w", err)
		}
		return msgs, nil
	}
	var wrapped struct {
		Messages []client.IngestMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("decode messages: %w", err)
	}
	return wrapped.Messages, nil
}

func newInboxTriageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "triage <id> <signal>",
		Short: "Record what a message is, and optionally which application it belongs to",
		Long: "Record an agent's verdict for one message: what it is, and — with --slug —\n" +
			"which of your applications it belongs to. Both are written together, and a\n" +
			"forward signal moves that application's stage.\n\n" +
			"Omitting --slug classifies without touching the link; clearing a link is\n" +
			"`inbox unlink`.\n\nSignals: " + strings.Join(signalVocabulary, ", "),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := authedEmail(cmd, args[0])
			if err != nil {
				return err
			}
			p := client.TriageParams{Signal: args[1], Slug: mustString(cmd, "slug")}
			if cmd.Flags().Changed("confidence") {
				v, _ := cmd.Flags().GetFloat64("confidence")
				p.Confidence = &v
			}
			data, err := c.Triage(cmd.Context(), id, p)
			if err != nil {
				return err
			}
			done := fmt.Sprintf("Triaged %d: %s", id, p.Signal)
			if p.Slug != "" {
				done += " → " + p.Slug
			}
			reportEmail(cmd, data, done)
			return nil
		},
	}
	cmd.Flags().String("slug", "", "link the message to this application")
	cmd.Flags().Float64("confidence", 0, "your confidence in the verdict, 0–1")
	return cmd
}

func newInboxLinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link <id> <slug>",
		Short: "Attach a message to one of your applications",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := authedEmail(cmd, args[0])
			if err != nil {
				return err
			}
			data, err := c.LinkEmail(cmd.Context(), id, args[1])
			if err != nil {
				return err
			}
			reportEmail(cmd, data, fmt.Sprintf("Linked %d → %s", id, args[1]))
			return nil
		},
	}
}

func newInboxUnlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink <id>",
		Short: "Clear a message's application link (its classification stays)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := authedEmail(cmd, args[0])
			if err != nil {
				return err
			}
			data, err := c.UnlinkEmail(cmd.Context(), id)
			if err != nil {
				return err
			}
			reportEmail(cmd, data, fmt.Sprintf("Unlinked %d", id))
			return nil
		},
	}
}

func newInboxReadAllCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read-all",
		Short: "Mark every unread message matching the filters as read",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			marked, err := c.MarkAllRead(cmd.Context(), inboxFilters(cmd))
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Marked %d read\n", marked)
			return nil
		},
	}
	inboxFilterFlags(cmd)
	return cmd
}

func newInboxDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Soft-delete a message (restore with `inbox restore`)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return emailAction(cmd, args[0], "Deleted", func(c *client.Client, id int64) error {
				return c.DeleteEmail(cmd.Context(), id)
			})
		},
	}
}

func newInboxRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <id>",
		Short: "Undo a soft-delete",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return emailAction(cmd, args[0], "Restored", func(c *client.Client, id int64) error {
				return c.RestoreEmail(cmd.Context(), id)
			})
		},
	}
}

// emailAction runs a per-message action that returns no payload and reports it.
func emailAction(cmd *cobra.Command, arg, verb string, do func(*client.Client, int64) error) error {
	c, id, err := authedEmail(cmd, arg)
	if err != nil {
		return err
	}
	if err := do(c, id); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %d\n", verb, id)
	return nil
}

// authedEmail resolves the two things every per-message command needs: an
// authenticated client and a validated message id.
func authedEmail(cmd *cobra.Command, arg string) (*client.Client, int64, error) {
	c, _, err := authedClient(cmd)
	if err != nil {
		return nil, 0, err
	}
	id, err := parseEmailID(arg)
	if err != nil {
		return nil, 0, err
	}
	return c, id, nil
}

// reportEmail prints the API payload when --json was passed, and the human line
// otherwise — the shared tail of the commands that return the changed message.
func reportEmail(cmd *cobra.Command, data json.RawMessage, human string) {
	if wantJSON(cmd) {
		printJSON(cmd, data)
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), human)
}

// parseEmailID turns the CLI's message-id argument into an int64, rejecting
// anything else up front rather than sending it to the API as a 404.
func parseEmailID(arg string) (int64, error) {
	id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid message id %q: expected a positive number from `inbox list`", arg)
	}
	return id, nil
}
