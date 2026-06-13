package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
	"github.com/strelov1/freehire-cli/internal/config"
)

func newAuthCmd() *cobra.Command {
	auth := &cobra.Command{Use: "auth", Short: "Manage the stored API key"}
	auth.AddCommand(newAuthLoginCmd(), newAuthStatusCmd(), newAuthLogoutCmd())
	return auth
}

func newAuthLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Validate an API key and store it in ~/.freehire/creds.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			token, _ := cmd.Flags().GetString("token")
			token = strings.TrimSpace(token)
			if token == "" {
				fmt.Fprint(cmd.OutOrStdout(), "API key: ")
				sc := bufio.NewScanner(cmd.InOrStdin())
				if sc.Scan() {
					token = strings.TrimSpace(sc.Text())
				}
			}
			if token == "" {
				return errors.New("no API key provided")
			}

			base := config.DefaultAPIURL
			if v := os.Getenv(config.EnvAPIURL); v != "" {
				base = v
			}
			if f, _ := cmd.Flags().GetString("api-url"); f != "" {
				base = f
			}

			// Validate the key before storing it, so a bad key never lands in creds.
			data, err := client.New(base, token, nil).Me(cmd.Context())
			if err != nil {
				return fmt.Errorf("validating key: %w", err)
			}
			if err := config.Save(config.Creds{Token: token, APIURL: base}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s\n", email(data))
			return nil
		},
	}
	cmd.Flags().String("token", "", "API key (fhk_…); prompted on stdin if omitted")
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether a key is configured and valid",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, base, err := authedClient(cmd)
			if err != nil {
				return err
			}
			data, err := c.Me(cmd.Context())
			if err != nil {
				return fmt.Errorf("key not valid: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Authenticated as %s @ %s\n", email(data), base)
			return nil
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the stored API key",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := config.Remove(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out.")
			return nil
		},
	}
}

// email extracts the email from a user `data` payload, "" when absent.
func email(data []byte) string {
	var u struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(data, &u)
	return u.Email
}
