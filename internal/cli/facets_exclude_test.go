package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestFacetsFromFlags_ExcludeSkill(t *testing.T) {
	// Excluding a skill is the documented way to filter out roles built on a
	// language you do not have; it needs a flag of its own, not the --facet
	// escape hatch nobody discovers.
	cmd := &cobra.Command{}
	addFacetFlags(cmd)
	if err := cmd.ParseFlags([]string{"--exclude-skill", "python", "--exclude-skill", "php"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	facets, err := facetsFromFlags(cmd)
	if err != nil {
		t.Fatalf("facetsFromFlags: %v", err)
	}

	got := facets["skills_exclude"]
	if len(got) != 2 || got[0] != "python" || got[1] != "php" {
		t.Errorf("skills_exclude = %v, want [python php]", got)
	}
}
