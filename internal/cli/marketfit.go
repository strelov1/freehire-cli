package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// coverageResult is the subset of the market-coverage response shown for humans.
type coverageResult struct {
	Total             int64           `json:"total"`
	Covered           int64           `json:"covered"`
	CoveragePercent   int             `json:"coverage_percent"`
	MustHaveTotal     int             `json:"must_have_total"`
	MustHaveCovered   int             `json:"must_have_covered"`
	StackMatchPercent int             `json:"stack_match_percent"`
	Gaps              []coverageGap   `json:"gaps"`
	Skills            []coverageSkill `json:"skills"`
}

type coverageGap struct {
	Name          string `json:"name"`
	NewVacancies  int64  `json:"new_vacancies"`
	UnlockPercent int    `json:"unlock_percent"`
}

type coverageSkill struct {
	Name            string `json:"name"`
	MarketFrequency int    `json:"market_frequency"`
	MustHave        bool   `json:"must_have"`
	Status          string `json:"status"`
}

func newMarketFitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "market-fit --skills <skill,...> [filters]",
		Short: "Score your skills against the live market for a filter",
		Long: "market-fit measures how much of the live open-vacancy market your " +
			"skills reach, for a role you narrow with facet filters. Pass your skills " +
			"with --skills (comma-separated or repeated); one skill probes that one " +
			"skill's demand. It reports coverage, must-have skills you hold, and the " +
			"missing skills that would unlock the most new vacancies. Pass --json for " +
			"the raw response.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			rawSkills, _ := cmd.Flags().GetStringSlice("skills")
			skills := cleanStrings(rawSkills)
			if len(skills) == 0 {
				return fmt.Errorf("at least one --skills value is required")
			}
			facets, err := facetsFromFlags(cmd)
			if err != nil {
				return err
			}
			data, err := c.Coverage(cmd.Context(), client.CoverageParams{Skills: skills, Facets: facets})
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var r coverageResult
			if err := json.Unmarshal(data, &r); err != nil {
				return err
			}
			printCoverage(cmd, r)
			return nil
		},
	}
	cmd.Flags().StringSlice("skills", nil, "your skills, comma-separated or repeated (e.g. --skills go,react)")
	addFacetFlags(cmd)
	return cmd
}

// printCoverage renders the coverage result as a short human report: the headline
// coverage, the must-have and stack-match summary, then the biggest missing-skill
// wins.
func printCoverage(cmd *cobra.Command, r coverageResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Coverage: %d%% (%d of %d vacancies list ≥1 of your skills)\n",
		r.CoveragePercent, r.Covered, r.Total)
	fmt.Fprintf(out, "Must-have skills held: %d of %d   ·   stack match: %d%%\n",
		r.MustHaveCovered, r.MustHaveTotal, r.StackMatchPercent)

	if len(r.Gaps) > 0 {
		fmt.Fprintln(out, "\nBiggest gaps (missing skill → new vacancies unlocked):")
		for _, g := range r.Gaps {
			fmt.Fprintf(out, "  %-24s  +%d  (%d%%)\n", trunc(g.Name, 24), g.NewVacancies, g.UnlockPercent)
		}
	}
}

// cleanStrings trims each value and drops the empties.
func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
