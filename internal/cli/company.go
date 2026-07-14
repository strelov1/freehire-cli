package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newCompanyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "company <slug>",
		Short: "Show a company and its open jobs",
		Long: "Show a company and the jobs it currently has open. Use the company " +
			"slug shown by `freehire job <slug>` (or `--company <slug>` on search).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := publicClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.GetCompany(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var v companyView
			if err := json.Unmarshal(data, &v); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s (%s)\n", v.Company.Name, v.Company.Slug)
			for _, j := range v.Jobs {
				fmt.Fprintf(out, "%-44s  %-16s  %s\n",
					trunc(j.Title, 44), trunc(j.Location, 16), j.PublicSlug)
			}
			fmt.Fprintf(out, "\n%d job(s)\n", len(v.Jobs))
			return nil
		},
	}
}

// companyView is the company-detail response: the company plus its open jobs.
type companyView struct {
	Company struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"company"`
	Jobs []jobRow `json:"jobs"`
}
