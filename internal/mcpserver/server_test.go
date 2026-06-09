package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestServerEndToEnd connects an MCP client to the server over the SDK's
// in-memory transport pair and exercises the full tools/list and tools/call
// path, asserting the random_int tool is discoverable and returns structured
// output within the requested range.
func TestServerEndToEnd(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	srv := New(BuildInfo{Version: "test"})
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	// tools/list: random_int must be present with a non-null input schema.
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var randomIntTool *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "random_int" {
			randomIntTool = tool
			break
		}
	}
	if randomIntTool == nil {
		t.Fatal("tools/list did not include random_int")
	}
	if randomIntTool.InputSchema == nil {
		t.Fatal("random_int InputSchema is nil, want a JSON Schema object")
	}

	// tools/call: the structured Value must fall within the requested range.
	const wantMin, wantMax = 3, 7
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "random_int",
		Arguments: map[string]any{"min": wantMin, "max": wantMax},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.IsError {
		t.Fatalf("call tool returned IsError, content: %+v", result.Content)
	}

	// StructuredContent round-trips through JSON; decode it into the typed shape.
	var out randomIntOutput
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal structured content %q: %v", raw, err)
	}
	if out.Value < wantMin || out.Value > wantMax {
		t.Fatalf("random_int value = %d, want within [%d, %d]", out.Value, wantMin, wantMax)
	}
}

// TestServerEndToEndToolError verifies the tool-error convention: an invalid
// range yields a CallToolResult with IsError set, not a protocol error.
func TestServerEndToEndToolError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	srv := New(BuildInfo{Version: "test"})
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "random_int",
		Arguments: map[string]any{"min": 10, "max": 1},
	})
	if err != nil {
		t.Fatalf("call tool returned a protocol error, want tool-level error: %v", err)
	}
	if !result.IsError {
		t.Fatal("call tool result IsError = false, want true for min > max")
	}
}
