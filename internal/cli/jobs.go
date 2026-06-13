package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search jobs by keyword",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			limit, _ := cmd.Flags().GetInt("limit")
			offset, _ := cmd.Flags().GetInt("offset")
			res, err := c.Search(cmd.Context(), strings.Join(args, " "), limit, offset)
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, res.Data)
				return nil
			}
			var rows []jobRow
			if err := json.Unmarshal(res.Data, &rows); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, r := range rows {
				fmt.Fprintf(out, "%-40s  %-20s  %-16s  %s\n",
					trunc(r.Title, 40), trunc(r.Company, 20), trunc(r.Location, 16), r.PublicSlug)
			}
			fmt.Fprintf(out, "\n%d of %d shown\n", len(rows), res.Total)
			return nil
		},
	}
	cmd.Flags().Int("limit", 20, "maximum results")
	cmd.Flags().Int("offset", 0, "results offset for paging")
	return cmd
}

func newJobCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "job <slug>",
		Short: "Show a job's content by its slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.GetJob(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			var j jobDetail
			if err := json.Unmarshal(data, &j); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s\n%s · %s\n%s\n\n%s\n", j.Title, j.Company, j.Location, j.URL, j.Description)
			return nil
		},
	}
}

func newApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply <slug>",
		Short: "Mark a job as applied for your account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, _, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.Apply(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Marked applied: %s\n", args[0])
			return nil
		},
	}
}

// trunc shortens s to at most n bytes, appending an ellipsis when it cuts.
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
