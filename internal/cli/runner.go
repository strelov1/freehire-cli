package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/runner"
)

// newRunnerCmd is `freehire runner`: keep a harness available to the freehire
// assistant on this machine.
//
// The point is where the model credential lives. The server holds the session,
// the journal and the UI; the harness — and the credential it authenticates
// with — stay here. Nothing about the model provider is sent to the server.
func newRunnerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runner",
		Short: "Run the assistant's coding harness on this machine (beta)",
		Long: "Connect this machine to the freehire assistant and run its coding " +
			"harness locally, so your model-provider credential never leaves it.\n\n" +
			"The connection is long-lived and idle until the assistant needs a " +
			"session. Sessions you start with this device selected run here; " +
			"everything else is unaffected. Stop with Ctrl-C.\n\n" +
			"The harness binary (claude-code-acp) must be installed and logged in.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			token := runner.ResolveToken(mustString(cmd, "token"))
			if token == "" {
				return errors.New(
					"no runner token: pass --token or set ROY_RUNNER_TOKEN")
			}
			deviceID, err := runner.DeviceID()
			if err != nil {
				return fmt.Errorf("resolving device id: %w", err)
			}

			// Ctrl-C ends the connection, which tears down any harness this
			// device is running — they are useless once the server cannot
			// reach them.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			err = runner.Run(ctx, runner.Options{
				ServerURL: mustString(cmd, "server"),
				Token:     token,
				DeviceID:  deviceID,
				Out:       cmd.ErrOrStderr(),
			})
			// A clean Ctrl-C is not a failure.
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(cmd.ErrOrStderr(), "runner stopped")
				return nil
			}
			return err
		},
	}
	cmd.Flags().String("server", "https://agent.freehire.dev",
		"assistant server base URL")
	cmd.Flags().String("token", "",
		"session token for this device (or set ROY_RUNNER_TOKEN)")
	return cmd
}
