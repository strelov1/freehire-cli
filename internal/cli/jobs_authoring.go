package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// newJobsCmd is the `jobs` command group for moderator-authored postings. These
// require the moderator role on the server; a non-moderator key gets a 403.
func newJobsCmd() *cobra.Command {
	jobs := &cobra.Command{
		Use:   "jobs",
		Short: "Create and edit job postings (moderator)",
		Long: "Create and edit hand-curated job postings. These require the moderator " +
			"role on your account; a regular key is rejected with 403.",
	}
	jobs.AddCommand(newJobsAddCmd(), newJobsEditCmd())
	return jobs
}

func newJobsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a job posting",
		Long: "Create a hand-curated job posting. --url, --title and --company are " +
			"required. The URL is the dedup key, so re-running with the same URL updates " +
			"the existing posting instead of creating a duplicate.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			p := client.CreateJobParams{
				URL:         mustString(cmd, "url"),
				Title:       mustString(cmd, "title"),
				Company:     mustString(cmd, "company"),
				Location:    mustString(cmd, "location"),
				Description: mustString(cmd, "description"),
				Remote:      mustBool(cmd, "remote"),
			}
			if cmd.Flags().Changed("posted-at") {
				v := mustString(cmd, "posted-at")
				p.PostedAt = &v
			}
			data, err := c.CreateJob(cmd.Context(), p)
			if err != nil {
				return err
			}
			return printJobResult(cmd, data, "Job created")
		},
	}
	cmd.Flags().String("url", "", "canonical posting URL (required, dedup key)")
	cmd.Flags().String("title", "", "job title (required)")
	cmd.Flags().String("company", "", "company name (required)")
	cmd.Flags().String("location", "", "free-text location")
	cmd.Flags().String("description", "", "job description")
	cmd.Flags().Bool("remote", false, "mark the job remote")
	cmd.Flags().String("posted-at", "", "posting date (RFC3339)")
	return cmd
}

func newJobsEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <slug>",
		Short: "Edit a job posting",
		Long: "Partially update a hand-curated job, addressed by its public slug. Only the " +
			"flags you pass are changed; the URL identity cannot be edited. Editing the " +
			"location/description/company re-derives the search facets.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			var p client.EditJobParams
			changed := false
			setStr := func(flag string, dst **string) {
				if cmd.Flags().Changed(flag) {
					v := mustString(cmd, flag)
					*dst = &v
					changed = true
				}
			}
			setStr("title", &p.Title)
			setStr("company", &p.Company)
			setStr("location", &p.Location)
			setStr("description", &p.Description)
			setStr("posted-at", &p.PostedAt)
			if cmd.Flags().Changed("remote") {
				v := mustBool(cmd, "remote")
				p.Remote = &v
				changed = true
			}
			if !changed {
				return fmt.Errorf("nothing to update: pass at least one field flag")
			}

			data, err := c.EditJob(cmd.Context(), args[0], p)
			if err != nil {
				return err
			}
			return printJobResult(cmd, data, "Job updated")
		},
	}
	cmd.Flags().String("title", "", "new job title")
	cmd.Flags().String("company", "", "new company name")
	cmd.Flags().String("location", "", "new free-text location")
	cmd.Flags().String("description", "", "new job description")
	cmd.Flags().Bool("remote", false, "set the remote flag")
	cmd.Flags().String("posted-at", "", "posting date (RFC3339)")
	return cmd
}

// printJobResult renders the create/edit response: raw JSON under --json, else a one
// line confirmation with the public slug from the returned job.
func printJobResult(cmd *cobra.Command, data json.RawMessage, verb string) error {
	if wantJSON(cmd) {
		printJSON(cmd, data)
		return nil
	}
	var j jobRow
	if err := json.Unmarshal(data, &j); err != nil {
		// The write succeeded; only the convenience decode failed. Show the raw data.
		fmt.Fprintf(cmd.OutOrStdout(), "%s.\n", verb)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (%s)\n", verb, j.PublicSlug, j.Title)
	return nil
}

// mustString / mustBool read a flag whose existence is guaranteed by registration,
// dropping the always-nil error for readable call sites.
func mustString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

func mustBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}
