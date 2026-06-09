package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/template-mcp/internal/mcpserver"
)

// newStdioCommand builds the "stdio" subcommand, which serves the MCP server
// over the stdio transport for local clients that spawn the process.
//
// To produce an HTTP-only repository, delete this file and its AddCommand call
// in root.go.
func newStdioCommand(options Options) *cobra.Command {
	return &cobra.Command{
		Use:   "stdio",
		Short: "Serve the MCP server over stdio (local transport)",
		Long: "Serve the MCP server over the stdio transport.\n\n" +
			"The client launches this process and exchanges newline-delimited " +
			"JSON-RPC messages over stdin/stdout.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// CRITICAL: stdout is the JSON-RPC channel for this transport.
			// Nothing in this code path may write to os.Stdout — a stray
			// fmt.Println would corrupt the protocol. The server logs to stderr
			// (see mcpserver.New); keep all diagnostics on stderr too.
			srv := mcpserver.New(mcpserver.BuildInfo{Version: options.Build.Version})
			if err := srv.Run(cmd.Context(), &mcp.StdioTransport{}); err != nil {
				return fmt.Errorf("run stdio server: %w", err)
			}

			return nil
		},
	}
}
