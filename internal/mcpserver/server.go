// Package mcpserver builds the transport-agnostic MCP server for this template.
//
// The server defined here knows nothing about how it is connected to a client:
// the same *mcp.Server is driven by the stdio and http subcommands in
// internal/cli. Keeping transport concerns out of this package is the seam that
// lets a consumer keep one transport and delete the other without ever touching
// the server or its tools.
package mcpserver

import (
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BuildInfo describes linker-injected build metadata surfaced to MCP clients.
type BuildInfo struct {
	// Version is the release version reported in the server implementation info.
	Version string
}

// New constructs the template MCP server and registers its tools.
//
// The server logs to [os.Stderr]. This is deliberate and required: the stdio
// transport reserves [os.Stdout] for the JSON-RPC message stream, so any log
// written to stdout would corrupt the protocol. Logging to stderr is correct
// for both transports, so it is wired uniformly here.
//
// New is transport-agnostic; callers choose a transport when they run the
// returned server (see internal/cli).
func New(build BuildInfo) *mcp.Server {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "template-mcp",
		Title:   "Meigma MCP server template",
		Version: build.Version,
	}, &mcp.ServerOptions{
		Logger: logger,
	})

	registerRandomInt(srv)

	return srv
}
