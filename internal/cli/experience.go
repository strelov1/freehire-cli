package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
)

// newExperienceCmd is the `experience` command group. Reading, adding and correcting all
// take a key — the same full-scope key that already reaches `cv edit` and `apply`.
//
// Removing does not, and that is the server's rule rather than a missing command: DELETE on
// either kind, and the merge that folds two achievements together, stay cookie-only. The
// bank has no undo, and deleting an employment cascades to every achievement under it, so
// destruction is left where the candidate can see it happen.
func newExperienceCmd() *cobra.Command {
	experience := &cobra.Command{
		Use:   "experience",
		Short: "Read, add to and correct your experience bank",
		Long: "The durable record of what you've actually done — employments and the " +
			"evidence attached to them — that CVs are tailored from. `list` shows the " +
			"whole bank; `employments add` and `atoms add` record new entries; " +
			"`employments update` and `atoms update` correct one.\n\n" +
			"An atom added here is stamped `manual` provenance — you typed it yourself, " +
			"the only honest claim an API call outside a chat can make. Correcting an " +
			"entry with a key does NOT restamp it: an achievement the agent inferred " +
			"stays inferred, and so stays off the CV. Only your own edit on the site " +
			"turns it into something you assert.\n\n" +
			"Deleting is on the site only.",
	}
	experience.AddCommand(newExperienceListCmd(), newExperienceEmploymentsCmd(), newExperienceAtomsCmd())
	return experience
}

// bankAtom and bankEmployment are the writable fields of a banked entry as `experience list`
// serves them. They exist because the server's update is a whole-row REPLACE: to change one
// field the command has to send back everything else exactly as it stands.
//
// Provenance is deliberately absent. It is not a field any caller sends — the server decides
// it from the credential — and reading it here only to echo it back would invite a future
// change to start trusting the client with it.
type bankAtom struct {
	ID           string   `json:"id"`
	EmploymentID string   `json:"employment_id"`
	Claim        string   `json:"claim"`
	Context      string   `json:"context"`
	Metrics      []string `json:"metrics"`
	Skills       []string `json:"skills"`
}

type bankEmployment struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Company string `json:"company"`
	// Name carries the place label for a project, which the API serves under `name` rather
	// than `company`. Both map to the same stored field.
	Name     string     `json:"name"`
	Role     string     `json:"role"`
	Location string     `json:"location"`
	Start    string     `json:"start"`
	End      string     `json:"end"`
	Current  bool       `json:"current"`
	Summary  string     `json:"summary"`
	Link     string     `json:"link"`
	Stack    []string   `json:"stack"`
	Atoms    []bankAtom `json:"atoms"`
}

// label returns the place's name whichever key the API served it under.
func (e bankEmployment) label() string {
	if e.Company != "" {
		return e.Company
	}
	return e.Name
}

type bankView struct {
	Employments []bankEmployment `json:"employments"`
	Unplaced    []bankAtom       `json:"unplaced"`
}

// findAtom returns the banked achievement with this id, and the employment it sits under.
// Atoms are grouped by place in the response rather than each carrying a back-reference in
// every version of the API, so the grouping is the more reliable source of the two.
func (b bankView) findAtom(id string) (bankAtom, bool) {
	for _, e := range b.Employments {
		for _, a := range e.Atoms {
			if a.ID == id {
				a.EmploymentID = e.ID
				return a, true
			}
		}
	}
	for _, a := range b.Unplaced {
		if a.ID == id {
			a.EmploymentID = ""
			return a, true
		}
	}
	return bankAtom{}, false
}

func (b bankView) findEmployment(id string) (bankEmployment, bool) {
	for _, e := range b.Employments {
		if e.ID == id {
			return e, true
		}
	}
	return bankEmployment{}, false
}

// fetchBank reads the caller's whole bank, which every correction starts from.
func fetchBank(cmd *cobra.Command, c *client.Client) (bankView, error) {
	data, err := c.ListExperience(cmd.Context())
	if err != nil {
		return bankView{}, err
	}
	var b bankView
	if err := json.Unmarshal(data, &b); err != nil {
		return bankView{}, err
	}
	return b, nil
}

// anyFlagChanged reports whether the caller named at least one field to change. A correction
// that names nothing is a write with no content: refusing it is more useful than sending the
// row back to the server unchanged and reporting success.
func anyFlagChanged(cmd *cobra.Command, names ...string) bool {
	for _, n := range names {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

// overlayString returns the flag's value when the caller named it, and the banked value
// otherwise — the rule that keeps a whole-row replace from erasing what nobody mentioned.
func overlayString(cmd *cobra.Command, name, current string) string {
	if cmd.Flags().Changed(name) {
		return mustString(cmd, name)
	}
	return current
}

func overlayStrings(cmd *cobra.Command, name string, current []string) []string {
	if !cmd.Flags().Changed(name) {
		return current
	}
	v, _ := cmd.Flags().GetStringArray(name)
	return v
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
	employments.AddCommand(newExperienceEmploymentsAddCmd(), newExperienceEmploymentsUpdateCmd())
	return employments
}

// employmentFlagNames are the fields `employments update` can change; `add` offers the same
// set, so a correction can reach anything the create could set.
var employmentFlagNames = []string{"kind", "company", "role", "location", "start", "end", "current", "summary"}

func newExperienceEmploymentsUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <employment-id>",
		Short: "Correct a place you already recorded",
		Long: "Change one or more fields of an employment. Take the id from " +
			"`experience list`.\n\n" +
			"Only the flags you pass change; everything else is carried over from what is " +
			"banked. That is not cosmetic — the API replaces the whole entry, so the " +
			"command reads it first and sends it back with your changes applied.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return fmt.Errorf("an employment id is required — take one from `freehire experience list`")
			}
			if !anyFlagChanged(cmd, employmentFlagNames...) {
				return fmt.Errorf("name at least one field to change, e.g. --company or --end")
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			bank, err := fetchBank(cmd, c)
			if err != nil {
				return err
			}
			cur, ok := bank.findEmployment(id)
			if !ok {
				return fmt.Errorf("no employment %s in your bank — check the id with `freehire experience list`", id)
			}
			current := cur.Current
			if cmd.Flags().Changed("current") {
				current = mustBool(cmd, "current")
			}
			data, err := c.UpdateEmployment(cmd.Context(), id, client.CreateEmploymentParams{
				Kind:     overlayString(cmd, "kind", cur.Kind),
				Company:  overlayString(cmd, "company", cur.label()),
				Role:     overlayString(cmd, "role", cur.Role),
				Location: overlayString(cmd, "location", cur.Location),
				Start:    overlayString(cmd, "start", cur.Start),
				End:      overlayString(cmd, "end", cur.End),
				Current:  current,
				Summary:  overlayString(cmd, "summary", cur.Summary),
				Stack:    cur.Stack,
				Link:     cur.Link,
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
	cmd.Flags().String("end", "", "same format as --start; pass empty to mark it ongoing")
	cmd.Flags().Bool("current", false, "mark this employment as still ongoing")
	cmd.Flags().String("summary", "", "one line about the place, for context")
	return cmd
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
	atoms.AddCommand(newExperienceAtomsAddCmd(), newExperienceAtomsUpdateCmd())
	return atoms
}

// atomFlagNames are the fields `atoms update` can change.
var atomFlagNames = []string{"claim", "context", "metric", "skill", "employment"}

func newExperienceAtomsUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <atom-id>",
		Short: "Correct an achievement you already recorded",
		Long: "Change one or more fields of a banked achievement — most often a typo in " +
			"--claim, which cannot be fixed by re-adding it (a claim already in the bank " +
			"comes back as a conflict). Take the id from `experience list`.\n\n" +
			"Only the flags you pass change; --metric and --skill REPLACE the whole list " +
			"when given, since there is no sensible way to address one item of it from a " +
			"flag. Everything you do not name is carried over — the API replaces the whole " +
			"entry, so the command reads it first and sends it back with your changes.\n\n" +
			"Correcting with a key does not change who is held to have said it: an " +
			"achievement the agent inferred stays inferred, and stays uncitable on a CV. " +
			"Only your own edit on the site turns it into your assertion.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return fmt.Errorf("an achievement id is required — take one from `freehire experience list`")
			}
			if !anyFlagChanged(cmd, atomFlagNames...) {
				return fmt.Errorf("name at least one field to change, e.g. --claim or --metric")
			}
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			bank, err := fetchBank(cmd, c)
			if err != nil {
				return err
			}
			cur, ok := bank.findAtom(id)
			if !ok {
				return fmt.Errorf("no achievement %s in your bank — check the id with `freehire experience list`", id)
			}
			data, err := c.UpdateAtom(cmd.Context(), id, client.CreateAtomParams{
				Claim:        overlayString(cmd, "claim", cur.Claim),
				Context:      overlayString(cmd, "context", cur.Context),
				Metrics:      overlayStrings(cmd, "metric", cur.Metrics),
				Skills:       overlayStrings(cmd, "skill", cur.Skills),
				EmploymentID: overlayString(cmd, "employment", cur.EmploymentID),
			})
			if err != nil {
				return err
			}
			printJSON(cmd, data)
			return nil
		},
	}
	cmd.Flags().String("claim", "", "the achievement as one CV-bullet-grade sentence")
	cmd.Flags().String("context", "", "how it was done, in a sentence or two")
	cmd.Flags().StringArray("metric", nil, `a number as stated, e.g. "20s->1s" (repeatable; replaces the list)`)
	cmd.Flags().StringArray("skill", nil, "a canonical skill slug, e.g. go (repeatable; replaces the list)")
	cmd.Flags().String("employment", "", "move it to this employment id, from experience list")
	return cmd
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
