package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// newProfileCmd shows the caller's saved job-search profile — what an agent should
// read before asking the user what they are looking for.
func newProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "Show your saved job-search profile",
		Long: "Print the profile you saved on freehire: the roles and skills you want, the " +
			"skills you want to avoid, where and how you will work, and your CV's headline and " +
			"experience. Read this before guessing what to search for — the values are the same " +
			"ones the site's own recommendations use. Contact details are not included.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.GetProfile(cmd.Context())
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var v profileView
			if err := json.Unmarshal(data, &v); err != nil {
				return err
			}
			printProfile(cmd, v, len(data) == 0 || string(data) == "null")
			return nil
		},
	}
}

// profileView is the profile wire shape. Contact fields are absent by construction —
// the API serves the CV as its contact-free projection.
type profileView struct {
	Specializations []string `json:"specializations"`
	Skills          []string `json:"skills"`
	ExcludedSkills  []string `json:"excluded_skills"`
	LocationPrefs   *struct {
		WorkModes []string `json:"work_modes"`
		Remote    struct {
			Regions   []string `json:"regions"`
			Countries []string `json:"countries"`
		} `json:"remote"`
		Base struct {
			Country string `json:"country"`
			City    string `json:"city"`
		} `json:"base"`
		Relocation struct {
			Open      bool     `json:"open"`
			Regions   []string `json:"regions"`
			Countries []string `json:"countries"`
			Cities    []string `json:"cities"`
		} `json:"relocation"`
	} `json:"location_preferences"`
	CV *struct {
		Headline   string   `json:"headline"`
		Location   string   `json:"location"`
		Summary    string   `json:"summary"`
		TotalYears int      `json:"total_years"`
		Skills     []string `json:"skills"`
		Experience []struct {
			Title   string `json:"title"`
			Company string `json:"company"`
			Start   string `json:"start"`
			End     string `json:"end"`
		} `json:"experience"`
	} `json:"cv"`
}

func printProfile(cmd *cobra.Command, v profileView, empty bool) {
	out := cmd.OutOrStdout()
	if empty {
		fmt.Fprintln(out, "No profile saved yet.")
		fmt.Fprintln(out, "Create one at https://freehire.me/my/profile — it also drives your")
		fmt.Fprintln(out, "recommendations, saved-search alerts and the fit analysis on every vacancy.")
		return
	}

	fmt.Fprintf(out, "Roles:            %s\n", join(v.Specializations))
	fmt.Fprintf(out, "Skills:           %s\n", join(v.Skills))
	if len(v.ExcludedSkills) > 0 {
		fmt.Fprintf(out, "Avoiding:         %s\n", join(v.ExcludedSkills))
	}

	if p := v.LocationPrefs; p != nil {
		if len(p.WorkModes) > 0 {
			fmt.Fprintf(out, "Work mode:        %s\n", join(p.WorkModes))
		}
		if reach := join(append(append([]string{}, p.Remote.Regions...), p.Remote.Countries...)); reach != "" {
			fmt.Fprintf(out, "Remote reach:     %s\n", reach)
		}
		if base := strings.TrimPrefix(p.Base.City+", "+strings.ToUpper(p.Base.Country), ", "); strings.Trim(base, ", ") != "" {
			fmt.Fprintf(out, "Based in:         %s\n", strings.Trim(base, ", "))
		}
		if p.Relocation.Open {
			targets := join(append(append(append([]string{}, p.Relocation.Regions...), p.Relocation.Countries...), p.Relocation.Cities...))
			if targets == "" {
				targets = "anywhere"
			}
			fmt.Fprintf(out, "Will relocate to: %s\n", targets)
		}
	}

	if cv := v.CV; cv != nil {
		fmt.Fprintln(out)
		if cv.Headline != "" {
			fmt.Fprintf(out, "CV:               %s\n", cv.Headline)
		}
		if cv.TotalYears > 0 {
			fmt.Fprintf(out, "Experience:       %d years\n", cv.TotalYears)
		}
		if len(cv.Skills) > 0 {
			fmt.Fprintf(out, "CV skills:        %s\n", join(cv.Skills))
		}
		for _, e := range cv.Experience {
			when := strings.Trim(e.Start+"–"+e.End, "–")
			fmt.Fprintf(out, "  %-34s %s\n", trunc(strings.Trim(e.Title+" at "+e.Company, " at "), 34), when)
		}
	}
}

// join renders a value set for the human output, dropping empties.
func join(values []string) string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, ", ")
}
