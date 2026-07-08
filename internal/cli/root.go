// Package cli implements the freehire command-line interface (cobra commands
// over the internal API client and credential store).
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
	"github.com/strelov1/freehire-cli/internal/config"
)

// Execute runs the CLI; it prints any error to stderr and exits non-zero. It is
// the binary's entry point.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "freehire",
		Short: "Search and track jobs from the terminal via the freehire API",
		Long: "freehire is a CLI over the freehire API. Authenticate once with " +
			"`freehire auth login`, then search, open, and apply to jobs. Pass --json " +
			"for machine-readable output (handy for agents).",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("json", false, "output raw JSON from the API")
	root.PersistentFlags().String("api-url", "", "override the API base URL")
	root.AddCommand(newAuthCmd(), newSearchCmd(), newJobCmd(), newApplyCmd(),
		newSaveCmd(), newUnsaveCmd(), newMyCmd(), newStageCmd(), newNoteCmd(),
		newCompanyCmd(), newJobsCmd(), newSubmitCmd(), newSubmissionsCmd(),
		newMarketFitCmd())
	return root
}

// authedClient builds an API client from stored/env credentials, applying the
// --api-url override. It also returns the effective base URL (for display).
func authedClient(cmd *cobra.Command) (*client.Client, string, error) {
	r, err := config.Resolve(os.Getenv)
	if err != nil {
		return nil, "", err
	}
	base := r.APIURL
	if f, _ := cmd.Flags().GetString("api-url"); f != "" {
		base = f
	}
	return client.New(base, r.Token, nil), base, nil
}

func wantJSON(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// printJSON pretty-prints raw API data to stdout, falling back to the raw bytes
// if it is not indentable JSON.
func printJSON(cmd *cobra.Command, data json.RawMessage) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), buf.String())
}

// jobRow is the subset of a job shown in a search-results row.
type jobRow struct {
	PublicSlug  string `json:"public_slug"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	CompanySlug string `json:"company_slug"`
	Location    string `json:"location"`
}

// jobDetail adds the fields shown for a single job.
type jobDetail struct {
	jobRow
	URL         string `json:"url"`
	Description string `json:"description"`
}
