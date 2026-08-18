package cli

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// namedFacet binds a convenience CLI flag to the API facet param it fills. These
// are the high-traffic filters exposed as first-class flags on the search and
// market-fit commands; the long tail of the facet vocabulary is reachable via the
// generic --facet key=value flag.
type namedFacet struct{ flag, param, usage string }

var namedFacets = []namedFacet{
	{"region", "regions", "region: global|ru|cis|central_asia|eu|us"},
	{"country", "countries", "ISO-3166 country code, e.g. BR, US"},
	{"city", "cities", "city slug"},
	{"company", "company_slug", "company slug"},
	{"category", "category", "role category: backend|frontend|fullstack|devops|ml_ai|qa|..."},
	{"role", "role", "role facet, e.g. senior_backend"},
	{"seniority", "seniority", "seniority: intern|junior|middle|senior|staff|principal|lead|c_level"},
	{"employment-type", "employment_type", "employment type, e.g. full_time, contract"},
	{"english-level", "english_level", "English level, e.g. a2, b1, b2, c1"},
	// Exclusion is a first-class part of a real search — "contract work, but not
	// the Python ones" — and every facet supports the `<param>_exclude`
	// convention. Only skills get a named flag: it is the exclusion people
	// actually reach for, and the generic --facet covers the rest
	// (--facet company_type_exclude=outstaff) without a flag per facet.
	{"exclude-skill", "skills_exclude", "skill to filter OUT, e.g. python"},
}

// addFacetFlags registers the shared market-filter flags on a command: the named
// facets (each repeatable, OR within a facet), the remote shortcut, the salary
// floor, the visa toggle, and the generic --facet escape hatch for any other facet
// param. It deliberately does NOT register --skills: that flag means different
// things per command (a market filter for search, the measured skill set for
// market-fit), so each command owns its own --skills flag.
func addFacetFlags(cmd *cobra.Command) {
	for _, f := range namedFacets {
		cmd.Flags().StringArray(f.flag, nil, "filter by "+f.usage+" (repeatable)")
	}
	cmd.Flags().Bool("remote", false, "only remote jobs (work_mode=remote)")
	cmd.Flags().Int("salary-min", 0, "minimum salary (enrichment.salary_min)")
	cmd.Flags().Bool("visa", false, "only jobs offering visa sponsorship")
	cmd.Flags().StringArray("facet", nil, "arbitrary facet as key=value, e.g. --facet source=greenhouse (repeatable)")
}

// warnIgnored reports the query params the API did not read, on stderr.
//
// stderr, not stdout: --json output is piped into jq or an agent's parser, and a
// warning mixed into it would break the parse. Staying silent instead is worse
// than either — an unread filter widens the result set rather than failing the
// request, so the caller gets a bigger, wronger answer with nothing to show that
// anything went astray.
func warnIgnored(cmd *cobra.Command, ignored []client.IgnoredParam) {
	for _, p := range ignored {
		hint := ""
		if p.DidYouMean != "" {
			hint = fmt.Sprintf(" — did you mean %q?", p.DidYouMean)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: ignored unknown filter %q%s\n", p.Param, hint)
	}
}

// facetsFromFlags collects the shared market-filter flags into API query params.
// An invalid --facet (no "=") is a usage error.
func facetsFromFlags(cmd *cobra.Command) (url.Values, error) {
	facets := url.Values{}
	for _, f := range namedFacets {
		if vs, _ := cmd.Flags().GetStringArray(f.flag); len(vs) > 0 {
			facets[f.param] = append(facets[f.param], vs...)
		}
	}
	if remote, _ := cmd.Flags().GetBool("remote"); remote {
		facets.Set("work_mode", "remote")
	}
	if v, _ := cmd.Flags().GetInt("salary-min"); v > 0 {
		facets.Set("salary_min", strconv.Itoa(v))
	}
	if visa, _ := cmd.Flags().GetBool("visa"); visa {
		facets.Set("visa_sponsorship", "true")
	}
	raw, _ := cmd.Flags().GetStringArray("facet")
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --facet %q, want key=value", kv)
		}
		facets.Add(k, v)
	}
	return facets, nil
}
