// Command child is the E2E test fixture the dev proxy builds and supervises:
// a minimal MCP stdio server whose tool set comes from a JSON manifest given
// as its first argument, so editing the manifest in the watched tree is
// indistinguishable from a source change once the proxy rebuilds and respawns
// the binary. At startup it appends its pid to the file given as its second
// argument, letting the tests prove that swapped-out children die and that
// shutdown leaves no orphans.
//
// It lives under testdata so the module's ./... wildcards (build, lint, test)
// never touch it; the E2E test builds it by explicit path. It compiles inside
// this module against the module's own go.mod, which is what keeps the build
// offline: every required module is already in the local cache because the
// test binary itself was just built from the same graph.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: %s <tools-manifest.json> <pid-file>", os.Args[0])
	}

	tools, err := readManifest(os.Args[1])
	if err != nil {
		return err
	}
	if err := appendPid(os.Args[2]); err != nil {
		return err
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "e2e-child", Version: "0.0.1"}, nil)
	for _, name := range tools {
		server.AddTool(
			&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}},
			func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: "ok-" + name}},
				}, nil
			},
		)
	}
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

// readManifest parses the JSON array of tool names to serve.
func readManifest(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tools manifest: %w", err)
	}
	var tools []string
	if err := json.Unmarshal(raw, &tools); err != nil {
		return nil, fmt.Errorf("parse tools manifest: %w", err)
	}
	return tools, nil
}

// appendPid records this process in the pid file, one pid per line, oldest
// first.
func appendPid(path string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open pid file: %w", err)
	}
	if _, err := fmt.Fprintln(file, os.Getpid()); err != nil {
		return fmt.Errorf("record pid: %w", err)
	}
	return file.Close()
}
