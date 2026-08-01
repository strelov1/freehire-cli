package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ghostSignal is the served signal a posting carries: a hedged level, the criteria
// that produced it, and the denominator of the scale. It mirrors the API's
// jobview.Ghost, and the pointers mirror it deliberately — the payload OMITS a
// contributor count below the anonymity gate, where a count of one would identify
// that applicant to the employer, so an absent count must stay absent rather than
// decode to a zero this file could accidentally print.
type ghostSignal struct {
	Level         string   `json:"level"`
	Criteria      []string `json:"criteria"`
	CriteriaTotal int      `json:"criteria_total"`
	Contributors  *int     `json:"contributors"`
	ATSCheckedAt  *string  `json:"ats_checked_at"`
}

// ghostLevels are the two levels that say anything, worded as the website words
// them. An unrecognised level renders NOTHING: a line beside a job that names a
// level this build cannot explain is worse than no line.
var ghostLevels = map[string]string{
	"possible": "Possibly inactive",
	"likely":   "Likely inactive",
}

// ghostCriteria mirrors the API's criterion vocabulary, in the order it reports them
// (structural first, then outcome). `label` states a criterion that fired; `short`
// names one that did not, for the summary line — four sentences do not survive being
// joined by commas.
var ghostCriteria = []struct{ code, label, short string }{
	{"evergreen_posting", "Posting behaves as evergreen", "how the posting behaves"},
	{"ats_absent", "Not on the company's own careers board", "the company's own board"},
	{"silent_applications", "Applications here went unanswered", "applications here"},
	{"user_reports", "People reported no response", "reports from people"},
}

// lines renders the signal as the block `job` prints, or nil when there is nothing
// to say (no signal, or a level this build does not recognise).
//
// The unfired criteria are accounted for rather than dropped, because that is what
// tells a reader WHY the level is not higher instead of leaving them to guess how
// serious it is. And they are reported as not OBSERVED, never as "no data": a
// criterion can fail to fire because it was checked and found clear, and the payload
// cannot tell the two apart.
func (g *ghostSignal) lines() []string {
	if g == nil {
		return nil
	}
	label, ok := ghostLevels[g.Level]
	if !ok {
		return nil
	}

	fired := make(map[string]bool, len(g.Criteria))
	for _, c := range g.Criteria {
		fired[c] = true
	}

	out := []string{fmt.Sprintf("%s — %d of %d checks fired", label, len(g.Criteria), g.CriteriaTotal)}
	var unobserved []string
	known := make(map[string]bool, len(ghostCriteria))
	for _, c := range ghostCriteria {
		known[c.code] = true
		if !fired[c.code] {
			unobserved = append(unobserved, c.short)
			continue
		}
		line := "  · " + c.label
		if detail := g.detailFor(c.code); detail != "" {
			line += " (" + detail + ")"
		}
		out = append(out, line)
	}
	// A criterion the server has and this build does not still earns a line, by its
	// raw code. Dropping it would understate a signal the payload already carries,
	// and the scale's numerator would then disagree with the rows beneath it.
	for _, c := range g.Criteria {
		if !known[c] {
			out = append(out, "  · "+c)
		}
	}
	if len(unobserved) > 0 {
		out = append(out, "  Not observed: "+strings.Join(unobserved, ", ")+".")
	}
	return out
}

// detailFor is the fact behind a fired criterion, phrased as a continuation of its
// line. A criterion whose firing IS the whole fact gets none.
func (g *ghostSignal) detailFor(code string) string {
	switch code {
	case "ats_absent":
		if g.ATSCheckedAt == nil {
			return "checked against the company board"
		}
		return "checked " + ghostDay(*g.ATSCheckedAt)
	case "silent_applications", "user_reports":
		if g.Contributors == nil {
			return "reported"
		}
		return fmt.Sprintf("from %d people", *g.Contributors)
	}
	return ""
}

// ghostDay reduces a served timestamp to its calendar day, leaving anything it
// cannot parse untouched — an odd-looking date is better than a dropped one.
func ghostDay(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format(appliedOnLayout)
}

// appliedOnLayout is a plain calendar date: the day the reporter states they
// applied, matching what the API accepts.
const appliedOnLayout = "2006-01-02"

// newGhostCmd is the evidence channel behind the signal `job` prints: one person
// stating they applied to a posting and were never answered, and withdrawing that
// statement.
//
// `ghost` names the CHANNEL, not the claim. What a caller files is observable — a
// date, and an answer that never came — and what the CLI prints back never upgrades
// that into a statement about an employer's intent. An agent relaying any of this to
// a person must keep the same distinction.
func newGhostCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ghost",
		Short: "Report a posting that never answered your application",
		Long: "The evidence channel behind the \"possibly inactive\" signal shown by " +
			"`freehire job`. A report says you applied on a given day and were never " +
			"answered; it reaches no moderator and closes nothing. It needs a verified " +
			"email address, and you can file once per posting.",
	}
	cmd.AddCommand(newGhostReportCmd(), newGhostRetractCmd())
	return cmd
}

func newGhostReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report <slug>",
		Short: "State that you applied to a posting and were never answered",
		Long: "File your claim about a posting: you applied on --applied-on and nobody " +
			"ever came back. It is one of four checks behind the posting's signal, and " +
			"it starts counting once the application has been silent for about three " +
			"weeks. Withdraw it with `freehire ghost retract`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appliedOn, err := appliedOnFlag(cmd)
			if err != nil {
				return err
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.ReportGhost(cmd.Context(), args[0], appliedOn)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Filed: applied %s, never answered — %s\n"+
					"That is one check of four. The posting is marked once a second check "+
					"fires, and the stronger wording needs a second person.\n",
				appliedOn, args[0])
			return nil
		},
	}
	cmd.Flags().String("applied-on", "", "the day you applied, YYYY-MM-DD")
	return cmd
}

func newGhostRetractCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "retract <slug>",
		Short: "Withdraw your report about a posting",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			if err := c.RetractGhostReport(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Withdrawn: %s\n", args[0])
			return nil
		},
	}
}

// appliedOnFlag reads --applied-on and checks it is a plain calendar date.
//
// The date is REQUIRED rather than defaulted to today. Defaulting would let an agent
// file a claim carrying a date the person never stated, and the date is the whole
// substance of the claim — it is what decides whether the silence has matured. Only
// the FORMAT is checked here: the window (not in the future, not older than a year)
// is the server's rule, and a second copy of it would drift.
func appliedOnFlag(cmd *cobra.Command) (string, error) {
	v, _ := cmd.Flags().GetString("applied-on")
	if v == "" {
		return "", errors.New("--applied-on is required: the day you applied, e.g. --applied-on 2026-06-01")
	}
	if _, err := time.Parse(appliedOnLayout, v); err != nil {
		return "", fmt.Errorf("--applied-on must be a date like 2026-06-01, got %q", v)
	}
	return v, nil
}
