package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// facetsResult is the /jobs/facets response: per-facet value→count distributions
// and numeric min/max stats, under an optional filter.
type facetsResult struct {
	Total  int64                       `json:"total"`
	Facets map[string]map[string]int64 `json:"facets"`
	Stats  map[string]facetStat        `json:"stats"`
}

type facetStat struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

func newFacetsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "facets [filters]",
		Short: "List the market's facet values and skills — what you can filter by",
		Long: "facets lists the live values of every filter (category, seniority, " +
			"region, country, skills, …) with a vacancy count each, so you know which " +
			"values to pass to search and market-fit. Narrow it with any filter flag " +
			"(e.g. --category backend) to see the values relevant to that slice. Pass " +
			"--json for the full machine-readable distribution.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			filter, err := facetsFromFlags(cmd)
			if err != nil {
				return err
			}
			data, err := c.Facets(cmd.Context(), filter)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var r facetsResult
			if err := json.Unmarshal(data, &r); err != nil {
				return err
			}
			top, _ := cmd.Flags().GetInt("top")
			printFacets(cmd, r, top)
			return nil
		},
	}
	addFacetFlags(cmd)
	cmd.Flags().Int("top", 15, "max values shown per facet (human output)")
	return cmd
}

// printFacets renders each facet's values (count-descending, capped at top) as a
// one-line summary, followed by the numeric stat ranges.
func printFacets(cmd *cobra.Command, r facetsResult, top int) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Market: %d open vacancies\n\n", r.Total)

	names := make([]string, 0, len(r.Facets))
	for name := range r.Facets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		values := topValues(r.Facets[name], top)
		if len(values) == 0 {
			continue
		}
		parts := make([]string, len(values))
		for i, v := range values {
			parts[i] = fmt.Sprintf("%s (%d)", v.name, v.count)
		}
		line := strings.Join(parts, " · ")
		if more := len(r.Facets[name]) - len(values); more > 0 {
			line += fmt.Sprintf(" … +%d more", more)
		}
		fmt.Fprintf(out, "%-16s %s\n", name+":", line)
	}

	if len(r.Stats) > 0 {
		fmt.Fprintln(out)
		statNames := make([]string, 0, len(r.Stats))
		for name := range r.Stats {
			statNames = append(statNames, name)
		}
		sort.Strings(statNames)
		for _, name := range statNames {
			s := r.Stats[name]
			fmt.Fprintf(out, "%-16s %g–%g\n", name+":", s.Min, s.Max)
		}
	}
}

// facetValue is one facet value with its vacancy count.
type facetValue struct {
	name  string
	count int64
}

// topValues sorts a facet's values by count descending (ties broken by name) and
// returns at most top of them.
func topValues(dist map[string]int64, top int) []facetValue {
	vs := make([]facetValue, 0, len(dist))
	for name, count := range dist {
		vs = append(vs, facetValue{name, count})
	}
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].count != vs[j].count {
			return vs[i].count > vs[j].count
		}
		return vs[i].name < vs[j].name
	})
	if top > 0 && len(vs) > top {
		vs = vs[:top]
	}
	return vs
}
