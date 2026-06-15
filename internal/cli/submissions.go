package cli

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// submissionRow is the subset of a submission shown in CLI output. submitter_email is
// present only on the moderator queue; review_reason only on a rejected submission.
type submissionRow struct {
	ID             int64  `json:"id"`
	Status         string `json:"status"`
	Title          string `json:"title"`
	Company        string `json:"company"`
	ReviewReason   string `json:"review_reason"`
	SubmitterEmail string `json:"submitter_email"`
}

// newSubmitCmd is the top-level `submit` command: any authenticated user queues a
// vacancy for moderation. It mirrors `jobs add` but writes to the submission queue
// instead of the live catalogue, and needs no special role.
func newSubmitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Submit a job for moderation",
		Long: "Submit a vacancy for review. --url, --title and --company are required. " +
			"A moderator approves it before it appears in the catalogue; track it with " +
			"`freehire submissions`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.Submit(cmd.Context(), client.CreateJobParams{
				URL:         mustString(cmd, "url"),
				Source:      mustString(cmd, "source"),
				Title:       mustString(cmd, "title"),
				Company:     mustString(cmd, "company"),
				Location:    mustString(cmd, "location"),
				Description: mustString(cmd, "description"),
				Remote:      mustBool(cmd, "remote"),
			})
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var s submissionRow
			if err := json.Unmarshal(data, &s); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Submitted for review.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Submitted for review: %s at %s (%s)\n", s.Title, s.Company, s.Status)
			return nil
		},
	}
	cmd.Flags().String("url", "", "canonical posting URL (required, dedup key)")
	cmd.Flags().String("source", "", "real origin of the posting (default \"manual\"); e.g. greenhouse")
	cmd.Flags().String("title", "", "job title (required)")
	cmd.Flags().String("company", "", "company name (required)")
	cmd.Flags().String("location", "", "free-text location")
	cmd.Flags().String("description", "", "job description")
	cmd.Flags().Bool("remote", false, "mark the job remote")
	return cmd
}

// newSubmissionsCmd is the `submissions` group. With no subcommand it lists the
// caller's own submissions; `pending`, `approve`, and `reject` are the moderator
// review actions (a non-moderator key gets a 403).
func newSubmissionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "submissions",
		Short: "List your submissions; review the queue (moderator)",
		Long: "With no subcommand, list your own submissions and their review status. " +
			"The pending/approve/reject subcommands review the queue and require the " +
			"moderator role.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.MySubmissions(cmd.Context())
			if err != nil {
				return err
			}
			return printSubmissionList(cmd, data, false)
		},
	}
	cmd.AddCommand(newSubmissionsPendingCmd(), newSubmissionsApproveCmd(), newSubmissionsRejectCmd())
	return cmd
}

func newSubmissionsPendingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pending",
		Short: "List submissions awaiting review (moderator)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.PendingSubmissions(cmd.Context())
			if err != nil {
				return err
			}
			return printSubmissionList(cmd, data, true)
		},
	}
}

func newSubmissionsApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a submission, minting a live job (moderator)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid submission id %q", args[0])
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.ApproveSubmission(cmd.Context(), id)
			if err != nil {
				return err
			}
			return printSubmissionResult(cmd, data, "Approved", id)
		},
	}
}

func newSubmissionsRejectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a submission with an optional reason (moderator)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid submission id %q", args[0])
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.RejectSubmission(cmd.Context(), id, mustString(cmd, "reason"))
			if err != nil {
				return err
			}
			return printSubmissionResult(cmd, data, "Rejected", id)
		},
	}
	cmd.Flags().String("reason", "", "optional rejection reason")
	return cmd
}

// printSubmissionList renders a submission slice: raw JSON under --json, else one line
// per submission. showSubmitter adds the submitter email column (the moderator queue).
func printSubmissionList(cmd *cobra.Command, data json.RawMessage, showSubmitter bool) error {
	if wantJSON(cmd) {
		printJSON(cmd, data)
		return nil
	}
	var rows []submissionRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(rows) == 0 {
		fmt.Fprintln(out, "No submissions.")
		return nil
	}
	for _, s := range rows {
		if showSubmitter {
			fmt.Fprintf(out, "%-6d  %-10s  %-32s  %s\n", s.ID, s.Status, s.Title, s.SubmitterEmail)
		} else {
			line := fmt.Sprintf("%-6d  %-10s  %-32s  %s", s.ID, s.Status, s.Title, s.Company)
			if s.Status == "rejected" && s.ReviewReason != "" {
				line += "  — " + s.ReviewReason
			}
			fmt.Fprintln(out, line)
		}
	}
	return nil
}

// printSubmissionResult renders an approve/reject response: raw JSON under --json, else a
// one-line confirmation carrying the id (and title when present).
func printSubmissionResult(cmd *cobra.Command, data json.RawMessage, verb string, id int64) error {
	if wantJSON(cmd) {
		printJSON(cmd, data)
		return nil
	}
	out := cmd.OutOrStdout()
	var s submissionRow
	if err := json.Unmarshal(data, &s); err != nil || s.Title == "" {
		fmt.Fprintf(out, "%s submission %d.\n", verb, id)
		return nil
	}
	fmt.Fprintf(out, "%s submission %d: %s\n", verb, id, s.Title)
	return nil
}
