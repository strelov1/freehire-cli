package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// statusResult is the /status response: the overall verdict, when the pipeline
// last added a job, and a per-provider health rollup.
type statusResult struct {
	Overall        string                 `json:"overall"`
	GeneratedAt    string                 `json:"generated_at"`
	LastJobAddedAt *string                `json:"last_job_added_at"`
	Providers      []statusProviderResult `json:"providers"`
}

// statusProviderResult is one provider's board counts and derived status.
type statusProviderResult struct {
	Provider      string `json:"provider"`
	Status        string `json:"status"`
	TotalBoards   int64  `json:"total_boards"`
	HealthyBoards int64  `json:"healthy_boards"`
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check whether the freehire ingest pipeline is healthy",
		Long: "status reports the public ingest-fleet health: an overall " +
			"operational/degraded/down verdict, when the pipeline last added a " +
			"job, and which providers (if any) are not operational. Exits " +
			"non-zero when overall is not \"operational\", so it can gate a " +
			"cron or monitoring check.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := publicClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.Status(cmd.Context())
			if err != nil {
				return err
			}
			var s statusResult
			if err := json.Unmarshal(data, &s); err != nil {
				return err
			}
			if wantJSON(cmd) {
				printJSON(cmd, data)
			} else {
				printStatus(cmd, s)
			}
			if s.Overall != "operational" {
				return fmt.Errorf("status: %s", s.Overall)
			}
			return nil
		},
	}
}

// printStatus renders the overall verdict, freshness signal, and any
// non-operational providers — the fleet-wide detail is not worth dumping in
// the common (healthy) case.
func printStatus(cmd *cobra.Command, s statusResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Status: %s\n", s.Overall)
	fmt.Fprintf(out, "Generated at: %s\n", s.GeneratedAt)
	if s.LastJobAddedAt != nil {
		fmt.Fprintf(out, "Last job added: %s\n", *s.LastJobAddedAt)
	} else {
		fmt.Fprintln(out, "Last job added: never")
	}

	var problems []statusProviderResult
	for _, p := range s.Providers {
		if p.Status != "operational" {
			problems = append(problems, p)
		}
	}
	if len(problems) == 0 {
		return
	}
	fmt.Fprintln(out, "\nProblem providers:")
	for _, p := range problems {
		fmt.Fprintf(out, "  %s: %s (%d/%d healthy)\n", p.Provider, p.Status, p.HealthyBoards, p.TotalBoards)
	}
}
