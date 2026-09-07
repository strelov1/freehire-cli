package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newAutoApplyCmd is the `auto-apply` command group: everything reachable by a full-scope
// API key downstream of an attempt already queued from the website. Starting a NEW
// attempt (POST /jobs/:slug/auto-apply) is deliberately cookie-only on the server — a
// fresh attempt can end in a real submitted application, and the browser is the only
// place a candidate can watch it happen and undo it — so there is no `run`/`start`
// subcommand here, and there will not be one: queue a job for auto-apply from
// freehire.me itself, then use this group to watch it through.
func newAutoApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto-apply",
		Short: "Track and review an auto-apply attempt already queued on freehire.me",
		Long: "Acts on an attempt already queued from the website — starting a new one is " +
			"cookie-only on the server (a real application can go out, and the browser is " +
			"the only place you can watch it and undo it), so there is no command for " +
			"that here.\n\n" +
			"`status` shows where a tracked job's attempt stands and gives you the queue " +
			"id the other two take. `tailor` (re)starts writing the CV for it. `review " +
			"--approve`/`--decline` records your decision once a tailored CV and its " +
			"resolved preview are ready.",
	}
	cmd.AddCommand(newAutoApplyStatusCmd(), newAutoApplyTailorCmd(), newAutoApplyReviewCmd())
	return cmd
}

// autoApplyPreviewField is one field of what an approved attempt would submit.
type autoApplyPreviewField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// autoApplyPreview is the resolved_preview object — present only when Status is
// pending_review. Pending is left as raw JSON: only its count is shown, so its own
// field shape (a server-internal detail) never has to be mirrored here.
type autoApplyPreview struct {
	Fields  []autoApplyPreviewField `json:"fields"`
	Pending []json.RawMessage       `json:"pending"`
}

// autoApplyUnmapped is one required question the attempt could not answer — present only
// when Status is blocked.
type autoApplyUnmapped struct {
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Reason   string `json:"reason"`
}

// autoApplyStatusLine is the `auto_apply` field embedded in GET /me/tracking/:slug, null
// when the job has no attempt at all.
type autoApplyStatusLine struct {
	Status          string              `json:"status"`
	QueueID         int64               `json:"queue_id"`
	ResolvedPreview *autoApplyPreview   `json:"resolved_preview"`
	Unmapped        []autoApplyUnmapped `json:"unmapped"`
}

// trackedApplication is the subset of GET /me/tracking/:slug this group reads.
type trackedApplication struct {
	AutoApply *autoApplyStatusLine `json:"auto_apply"`
}

func newAutoApplyStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <job-slug>",
		Short: "Show the live auto-apply attempt for a tracked job",
		Long: "Print where a job's auto-apply attempt stands: tailoring (no CV yet), " +
			"pending_review (shows what would be submitted — your decision is next), " +
			"approved (queued for unattended submission), blocked (shows the unanswered " +
			"question), declined, or failed. Says so, rather than erroring, when the job " +
			"has no attempt at all.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := strings.TrimSpace(args[0])
			if slug == "" {
				return fmt.Errorf("a job slug is required")
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.AutoApplyStatus(cmd.Context(), slug)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var app trackedApplication
			if err := json.Unmarshal(data, &app); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if app.AutoApply == nil {
				fmt.Fprintf(out, "No auto-apply attempt for %s yet — queue one from freehire.me.\n", slug)
				return nil
			}
			printAutoApplyStatus(out, *app.AutoApply)
			return nil
		},
	}
}

// printAutoApplyStatus renders a status line's human-readable form, plus whatever detail
// and next step its status implies.
func printAutoApplyStatus(out io.Writer, s autoApplyStatusLine) {
	fmt.Fprintf(out, "status:   %s\n", s.Status)
	fmt.Fprintf(out, "queue id: %d\n", s.QueueID)
	switch s.Status {
	case "pending_review":
		fmt.Fprintln(out, "\nWould submit:")
		for _, f := range s.ResolvedPreview.Fields {
			fmt.Fprintf(out, "  %-30s %s\n", f.Label, f.Value)
		}
		if n := len(s.ResolvedPreview.Pending); n > 0 {
			fmt.Fprintf(out, "  (%d field(s) resolve at submit time)\n", n)
		}
		fmt.Fprintf(out, "\nNext: freehire auto-apply review %d --approve|--decline\n", s.QueueID)
	case "blocked":
		fmt.Fprintln(out, "\nUnanswered questions:")
		for _, u := range s.Unmapped {
			fmt.Fprintf(out, "  %s — %s\n", u.Label, u.Reason)
		}
	case "tailoring":
		fmt.Fprintf(out, "\nNext: freehire auto-apply tailor %d\n", s.QueueID)
	}
}

func newAutoApplyTailorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tailor <queue-id>",
		Short: "Start or continue tailoring the CV for a queued auto-apply attempt",
		Long: "Copy the queue id from `freehire auto-apply status <job-slug>`. Idempotent: " +
			"calling it again continues the same tailoring session rather than starting a " +
			"second one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := autoApplyQueueID(args[0])
			if err != nil {
				return err
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.TailorAutoApplyQueueEntry(cmd.Context(), id)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Tailoring started for queue entry %d\nNext: freehire auto-apply status <job-slug>\n", id)
			return nil
		},
	}
}

func newAutoApplyReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review <queue-id>",
		Short: "Approve or decline a tailored CV before it is submitted",
		Long: "Record your decision on a queued attempt's tailored CV: --approve queues it " +
			"for unattended submission, --decline parks it for good. A decision can be " +
			"recorded only once — the server refuses a second call as a 409.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := autoApplyQueueID(args[0])
			if err != nil {
				return err
			}
			approve, _ := cmd.Flags().GetBool("approve")
			decline, _ := cmd.Flags().GetBool("decline")
			decision, err := autoApplyDecision(approve, decline)
			if err != nil {
				return err
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.ReviewAutoApplyQueueEntry(cmd.Context(), id, decision)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Queue entry %d: %s\n", id, decision)
			return nil
		},
	}
	cmd.Flags().Bool("approve", false, "approve the tailored CV for unattended submission")
	cmd.Flags().Bool("decline", false, "decline the tailored CV")
	return cmd
}

// autoApplyDecision reads exactly one of --approve/--decline into the wire value the
// server expects.
func autoApplyDecision(approve, decline bool) (string, error) {
	switch {
	case approve && decline:
		return "", fmt.Errorf("pass one of --approve or --decline, not both")
	case approve:
		return "approved", nil
	case decline:
		return "declined", nil
	default:
		return "", fmt.Errorf("pass --approve or --decline")
	}
}

// autoApplyQueueID parses the positional queue-id argument. Unlike a CV id (opaque, never
// validated client-side), a queue id is a plain integer the API itself hands out as one —
// rejecting an obviously-wrong value here is a clearer error than a 404 for a typo.
func autoApplyQueueID(arg string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(arg), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid queue id %q — copy it from `freehire auto-apply status <job-slug>`", arg)
	}
	return id, nil
}
