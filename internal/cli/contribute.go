package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// resolvedLink is what the intake answers with. PublicSlug is null unless a posting is in the
// catalog; CompanySlug is set only for "tracked".
type resolvedLink struct {
	PublicSlug  *string `json:"public_slug"`
	Status      string  `json:"status"`
	CompanySlug string  `json:"company_slug"`
}

// contributionRow is one recorded contribution as `freehire contributions` prints it. Source
// and board are empty on a review row — a link we could not derive a board from, awaiting a
// maintainer's judgement.
type contributionRow struct {
	URL       string `json:"url"`
	Source    string `json:"source"`
	Board     string `json:"board"`
	Status    string `json:"status"`
	Surface   string `json:"surface"`
	CreatedAt string `json:"created_at"`
}

// newContributeCmd is the `contribute` command: hand freehire a job link. It replaced
// `submit`, which wrote to the moderation queue for hand-authored job cards — a different
// feature that happened to share a verb, and one no agent should be reaching for when what it
// means is "here is a vacancy you don't have".
func newContributeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "contribute <url>",
		Short: "Hand freehire a job link",
		Long: "Give freehire a link to a vacancy or an ATS board. If we already carry the " +
			"posting you get its slug back; if we can read the page, it is imported and you get " +
			"the new posting; and if the company's board is one we don't crawl yet, it is " +
			"queued for onboarding and you earn an AI credit. Track what you've sent with " +
			"`freehire contributions`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, base, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.Contribute(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var r resolvedLink
			if err := json.Unmarshal(data, &r); err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "Sent.")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), contributeMessage(r, base))
			return nil
		},
	}
}

// contributeMessage puts one intake outcome into a line, appending the job and company URLs
// the caller can act on. Pure, so the wording is testable without a server.
func contributeMessage(r resolvedLink, base string) string {
	var msg string
	switch r.Status {
	case "found":
		msg = "Already in the catalogue."
	case "tracked":
		msg = "Added. We already crawl this company, so its other roles will follow on the next crawl."
	case "imported":
		msg = "Added, and this company is new to us — we'll start crawling its board. +1 AI credit."
	default:
		msg = "We couldn't read that page. It's queued for a maintainer to look at — not credited yet."
	}
	var links []string
	if r.PublicSlug != nil && *r.PublicSlug != "" {
		links = append(links, base+"/jobs/"+*r.PublicSlug)
	}
	if r.CompanySlug != "" {
		links = append(links, base+"/companies/"+r.CompanySlug)
	}
	if len(links) == 0 {
		return msg
	}
	return msg + "\n" + strings.Join(links, "\n")
}

// newContributionsCmd lists the caller's own contributions, newest first.
func newContributionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "contributions",
		Short: "List the boards you've contributed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.MyContributions(cmd.Context())
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var rows []contributionRow
			if err := json.Unmarshal(data, &rows); err != nil {
				return err
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No contributions yet.")
				return nil
			}
			for _, r := range rows {
				fmt.Fprintln(cmd.OutOrStdout(), contributionLine(r))
			}
			return nil
		},
	}
}

// contributionLine renders one row. A review row has no board, so it is shown by its URL and
// labelled as awaiting judgement rather than presented as a contributed board.
func contributionLine(r contributionRow) string {
	if r.Status == "review" {
		return fmt.Sprintf("%s · under review · %s", r.URL, r.Surface)
	}
	return fmt.Sprintf("%s (%s) · %s · %s", r.Board, r.Source, r.Status, r.Surface)
}
