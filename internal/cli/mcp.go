package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/strelov1/freehire-cli/internal/client"
	"github.com/strelov1/freehire-cli/internal/config"
	"github.com/strelov1/freehire-cli/internal/mcpproxy"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run freehire's MCP server over stdio for a local AI host",
		Long: "mcp connects a local MCP host (Claude Code, Claude Desktop, Cursor) to freehire's " +
			"hosted MCP server, signed in as the account `freehire auth login` stored — the same " +
			"tools, prompts and instructions as the remote connector, with no second sign-in. " +
			"Without a stored sign-in it serves the public catalogue tools only. The server also " +
			"tells the agent this CLI is on its shell, for results that belong on disk.\n\n" +
			"  claude mcp add freehire -- freehire mcp",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := config.ResolveOptional(os.Getenv)
			if err != nil {
				return err
			}
			base := r.APIURL
			if f, _ := cmd.Flags().GetString("api-url"); f != "" {
				base = f
			}
			relay := &mcpproxy.Relay{
				Endpoint:  strings.TrimRight(base, "/") + "/api/v1/mcp",
				Token:     r.Token,
				UserAgent: client.UserAgent,
			}
			return relay.Run(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
}
