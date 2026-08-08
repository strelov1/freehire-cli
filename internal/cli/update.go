package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/selfupdate"
)

// latestRelease and applyUpdate are package vars, not direct calls to
// selfupdate.LatestRelease/selfupdate.Apply, so tests can swap in fakes and
// never hit the network or overwrite a real binary.
var (
	latestRelease = selfupdate.LatestRelease
	applyUpdate   = selfupdate.Apply
)

// newUpdateCmd checks GitHub for a newer freehire release and, unless
// --check is set, downloads and installs it over the running binary.
func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update freehire to the latest release",
		Long: "Checks the latest GitHub release and, unless --check is set, downloads " +
			"and installs it in place of the running binary.",
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check")
			current := Version

			rel, err := latestRelease(cmd.Context())
			if err != nil {
				return fmt.Errorf("checking latest release: %w", err)
			}
			available := selfupdate.IsNewer(current, rel.Tag)
			updated := false

			if available && !checkOnly {
				url, ok := rel.Assets[selfupdate.AssetName()]
				if !ok {
					return fmt.Errorf("no release asset named %s in %s", selfupdate.AssetName(), rel.Tag)
				}
				if err := applyUpdate(cmd.Context(), url); err != nil {
					return err
				}
				updated = true
			}

			if wantJSON(cmd) {
				data, err := json.Marshal(map[string]any{
					"current": current,
					"latest":  rel.Tag,
					"updated": updated,
				})
				if err != nil {
					return err
				}
				printJSON(cmd, data)
				return nil
			}

			if current == "dev" {
				fmt.Fprintln(cmd.OutOrStdout(),
					"Note: this is an unstamped dev build; the version comparison below may be inaccurate.")
			}
			switch {
			case updated:
				fmt.Fprintf(cmd.OutOrStdout(), "Updated: %s -> %s\n", current, rel.Tag)
			case !available:
				fmt.Fprintf(cmd.OutOrStdout(), "Already up to date (%s).\n", current)
			default: // available && checkOnly
				fmt.Fprintf(cmd.OutOrStdout(),
					"Update available: %s -> %s. Run `freehire update` to install.\n", current, rel.Tag)
			}
			return nil
		},
	}
	cmd.Flags().Bool("check", false, "only check for an update, don't install it")
	return cmd
}
