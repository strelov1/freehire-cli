package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// newExperienceCmd is the `experience` command group. Reading takes a key, like `cv`;
// adding takes one too — the server admits it under the same full-scope key that already
// reaches `cv edit` and `apply`, rather than a narrower scope, because no user-facing key
// today can be minted narrower than full anyway. Correcting or removing an existing entry
// is cookie-only on the server and has no command here.
func newExperienceCmd() *cobra.Command {
	experience := &cobra.Command{
		Use:   "experience",
		Short: "Read and add to your experience bank",
		Long: "The durable record of what you've actually done — employments and the " +
			"evidence attached to them — that CVs are tailored from. `list` shows the " +
			"whole bank; `employments add` and `atoms add` record new entries. Every " +
			"atom added this way is stamped `manual` provenance: it means you typed it " +
			"yourself, which is the only honest claim an API call outside a chat can make.",
	}
	experience.AddCommand(newExperienceListCmd(), newExperienceEmploymentsCmd(), newExperienceAtomsCmd())
	return experience
}

func newExperienceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Print your whole experience bank",
		Long: "Print every employment and every achievement attached to it, grouped by " +
			"place; achievements with no place come back under `unplaced`. Output is JSON.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.ListExperience(cmd.Context())
			if err != nil {
				return err
			}
			printJSON(cmd, data)
			return nil
		},
	}
}

func newExperienceEmploymentsCmd() *cobra.Command {
	employments := &cobra.Command{
		Use:   "employments",
		Short: "Manage the places your achievements are attached to",
	}
	employments.AddCommand(newExperienceEmploymentsAddCmd())
	return employments
}

func newExperienceEmploymentsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Record a new employment or project",
		Long: "Record a place where evidence was produced — a job or a side project. " +
			"At least --company or --role is required. Copy the returned id to attach " +
			"achievements to it with `experience atoms add --employment`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			company := mustString(cmd, "company")
			role := mustString(cmd, "role")
			if company == "" && role == "" {
				return fmt.Errorf("give at least --company or --role")
			}
			kind := mustString(cmd, "kind")
			if kind == "" {
				kind = "job"
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.CreateEmployment(cmd.Context(), client.CreateEmploymentParams{
				Kind:     kind,
				Company:  company,
				Role:     role,
				Location: mustString(cmd, "location"),
				Start:    mustString(cmd, "start"),
				End:      mustString(cmd, "end"),
				Current:  mustBool(cmd, "current"),
				Summary:  mustString(cmd, "summary"),
			})
			if err != nil {
				return err
			}
			printJSON(cmd, data)
			return nil
		},
	}
	cmd.Flags().String("kind", "job", `"job" or "project"`)
	cmd.Flags().String("company", "", "company or project name")
	cmd.Flags().String("role", "", "your title or role there")
	cmd.Flags().String("location", "", "free-text location")
	cmd.Flags().String("start", "", `a free-form date as you'd print it, e.g. "Mar 2021"`)
	cmd.Flags().String("end", "", "same format as --start; omit if ongoing")
	cmd.Flags().Bool("current", false, "mark this employment as still ongoing")
	cmd.Flags().String("summary", "", "one line about the place, for context")
	return cmd
}

func newExperienceAtomsCmd() *cobra.Command {
	atoms := &cobra.Command{
		Use:   "atoms",
		Short: "Manage the achievements attached to your employments",
	}
	atoms.AddCommand(newExperienceAtomsAddCmd())
	return atoms
}

func newExperienceAtomsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Record a new achievement",
		Long: "Record one piece of evidence — the sentence a CV bullet would carry. " +
			"--claim is required. A claim already in the bank, under any spelling, is " +
			"refused rather than duplicated. Provenance is always `manual`, regardless " +
			"of anything else about the call.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			claim := mustString(cmd, "claim")
			if claim == "" {
				return fmt.Errorf("--claim is required")
			}
			metrics, _ := cmd.Flags().GetStringArray("metric")
			skills, _ := cmd.Flags().GetStringArray("skill")
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.CreateAtom(cmd.Context(), client.CreateAtomParams{
				Claim:        claim,
				Context:      mustString(cmd, "context"),
				Metrics:      metrics,
				Skills:       skills,
				EmploymentID: mustString(cmd, "employment"),
			})
			if err != nil {
				return err
			}
			printJSON(cmd, data)
			return nil
		},
	}
	cmd.Flags().String("claim", "", "the achievement as one CV-bullet-grade sentence (required)")
	cmd.Flags().String("context", "", "how it was done, in a sentence or two")
	cmd.Flags().StringArray("metric", nil, `a number as stated, e.g. "20s->1s" (repeatable)`)
	cmd.Flags().StringArray("skill", nil, "a canonical skill slug, e.g. go, kubernetes (repeatable)")
	cmd.Flags().String("employment", "", "the employment id this belongs to, from experience list")
	return cmd
}
