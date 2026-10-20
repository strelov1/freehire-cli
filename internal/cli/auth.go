package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/strelov1/freehire-cli/internal/client"
	"github.com/strelov1/freehire-cli/internal/config"
	"github.com/strelov1/freehire-cli/internal/oauthlogin"
)

func newAuthCmd() *cobra.Command {
	auth := &cobra.Command{Use: "auth", Short: "Manage the stored API key"}
	auth.AddCommand(newAuthLoginCmd(), newAuthStatusCmd(), newAuthLogoutCmd())
	return auth
}

func newAuthLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in via the browser, or with --token, storing a token in ~/.freehire/creds.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			base := config.DefaultAPIURL
			if v := os.Getenv(config.EnvAPIURL); v != "" {
				base = v
			}
			if f, _ := cmd.Flags().GetString("api-url"); f != "" {
				base = f
			}

			useOAuth, _ := cmd.Flags().GetBool("oauth")
			token, _ := cmd.Flags().GetString("token")
			token = strings.TrimSpace(token)
			switch loginMethodFor(useOAuth, token != "", isTerminal(cmd.InOrStdin())) {
			case loginViaBrowser:
				t, err := oauthlogin.Login(cmd.Context(), http.DefaultClient, base, openBrowser, cmd.OutOrStdout())
				if err != nil {
					return fmt.Errorf("sign in: %w", err)
				}
				token = t
			case loginViaPipedKey:
				sc := bufio.NewScanner(cmd.InOrStdin())
				if sc.Scan() {
					token = strings.TrimSpace(sc.Text())
				}
				if token == "" {
					return errors.New("no API key on stdin: pass --token fhk_…, or run in a terminal to sign in via the browser")
				}
			}

			// Validate before storing, so a bad credential never lands in creds.
			data, err := client.New(base, token, nil).Me(cmd.Context())
			if err != nil {
				return fmt.Errorf("validating credential: %w", err)
			}
			if err := config.Save(config.Creds{Token: token, APIURL: base}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s\n", email(data))
			return nil
		},
	}
	cmd.Flags().String("token", "", "API key (fhk_…); without it a terminal signs in via the browser, and piped stdin is read as the key")
	// --oauth is the default now; kept so the instructions that spelled it out still run.
	cmd.Flags().Bool("oauth", false, "sign in via the browser (the default in a terminal)")
	_ = cmd.Flags().MarkHidden("oauth")
	return cmd
}

type loginMethod int

const (
	loginViaToken loginMethod = iota
	loginViaBrowser
	loginViaPipedKey
)

// loginMethodFor picks how `auth login` obtains a credential. A person at a
// terminal gets the browser sign-in with nothing to paste; a script keeps the
// two ways it always had — --token, or the key piped on stdin — and is never
// sent to wait on a browser nobody will open.
func loginMethodFor(oauthFlag, hasToken, interactive bool) loginMethod {
	switch {
	case oauthFlag:
		return loginViaBrowser
	case hasToken:
		return loginViaToken
	case interactive:
		return loginViaBrowser
	default:
		return loginViaPipedKey
	}
}

// isTerminal reports whether r is a terminal — a person typing, not a pipe, a
// file or a test's buffer. It asks the terminal driver rather than checking for
// a character device: /dev/null is one too, and a CI job run with </dev/null
// would otherwise open a browser and wait on it.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
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
