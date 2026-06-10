package upstream_test

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/meigma/template-mcp/tools/proxy/internal/reloader"
	"github.com/meigma/template-mcp/tools/proxy/internal/upstream"
)

// waitTimeout bounds every asynchronous wait in this suite.
const waitTimeout = 5 * time.Second

// tick is the polling interval for Eventually/Never-style waits.
const tick = 10 * time.Millisecond

// logRecorder is a [slog.Handler] that captures log messages for assertions.
type logRecorder struct {
	mu       sync.Mutex
	messages []string
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *logRecorder) Handle(_ context.Context, record slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, record.Message)
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }

func (r *logRecorder) WithGroup(string) slog.Handler { return r }

func (r *logRecorder) contains(substr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, message := range r.messages {
		if strings.Contains(message, substr) {
			return true
		}
	}
	return false
}

// fakeChild is a real in-process mcp.Server standing in for the built child
// binary; the upstream adapter connects to it through the transport seam.
type fakeChild struct {
	server  *mcp.Server
	session atomic.Pointer[mcp.ServerSession]
}

func newFakeChild(toolNames ...string) *fakeChild {
	child := &fakeChild{
		server: mcp.NewServer(&mcp.Implementation{Name: "fake-child", Version: "0.0.1"}, nil),
	}
	for _, name := range toolNames {
		child.addTool(name)
	}
	return child
}

// addTool registers a valid passthrough tool whose result identifies it.
func (c *fakeChild) addTool(name string) {
	c.server.AddTool(
		&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok-" + name}}}, nil
		},
	)
}

// factory returns a TransportFactory that connects the fake child server to
// one end of an in-memory transport pair and hands the adapter the other
// end (servers must be connected before clients).
func (c *fakeChild) factory(t *testing.T) upstream.TransportFactory {
	return func(string) (mcp.Transport, error) {
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		session, err := c.server.Connect(t.Context(), serverTransport, nil)
		if err != nil {
			return nil, err
		}
		c.session.Store(session)
		return clientTransport, nil
	}
}

// serverSession returns the fake child's server-side session captured by the
// factory during Start.
func (c *fakeChild) serverSession(t *testing.T) *mcp.ServerSession {
	t.Helper()

	session := c.session.Load()
	require.NotNil(t, session, "expected the transport factory to have connected the fake child")
	return session
}

// testContext bundles the fake child, the adapter under test, and its
// captured logs.
type testContext struct {
	child *fakeChild
	logs  *logRecorder
	up    *upstream.Upstream
}

// newTestContext wires options to the fake child's transport seam and a log
// recorder, then constructs the adapter.
func newTestContext(t *testing.T, child *fakeChild, options upstream.Options) *testContext {
	t.Helper()

	logs := &logRecorder{}
	options.Transport = child.factory(t)
	options.Logger = slog.New(logs)

	up, err := upstream.New(options)
	require.NoError(t, err, "construct upstream")
	return &testContext{child: child, logs: logs, up: up}
}

// start runs Start against the fake child and registers cleanup.
func (tc *testContext) start(t *testing.T) reloader.ChildSession {
	t.Helper()

	session, err := tc.up.Start(t.Context(), "unused-artifact")
	require.NoError(t, err, "start child")
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

// awaitSnapshot polls ToolsChanged until a snapshot containing wantTool
// arrives, keeping the most recent one.
func awaitSnapshot(t *testing.T, session reloader.ChildSession, wantTool string) []*mcp.Tool {
	t.Helper()

	var snapshot []*mcp.Tool
	require.Eventually(t, func() bool {
		select {
		case latest := <-session.ToolsChanged():
			snapshot = latest
		default:
		}
		return slices.Contains(toolNames(snapshot), wantTool)
	}, waitTimeout, tick, "expected a validated snapshot containing %q on ToolsChanged", wantTool)
	return snapshot
}

// serveToolList intercepts tools/list on the fake child to serve definitions
// its own AddTool validation would refuse to register.
func serveToolList(tools []*mcp.Tool) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				return &mcp.ListToolsResult{Tools: tools}, nil
			}
			return next(ctx, method, req)
		}
	}
}

// requireChildKilled asserts the fake child's session was closed — the
// health gate must kill a half-started child it rejects.
func requireChildKilled(t *testing.T, child *fakeChild) {
	t.Helper()

	session := child.serverSession(t)
	closed := make(chan struct{})
	go func() {
		_ = session.Wait()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(waitTimeout):
		t.Fatal("expected the half-started child session to be closed after a gate failure")
	}
}

func TestUpstreamStartHappyPath(t *testing.T) {
	t.Parallel()

	tc := newTestContext(t, newFakeChild("alpha", "beta"), upstream.Options{})
	session := tc.start(t)

	assert.ElementsMatch(t, []string{"alpha", "beta"}, toolNames(session.Tools()),
		"expected the health-gate snapshot to carry the child's tools")

	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "alpha"})
	require.NoError(t, err, "forward a tool call to the child")
	require.Len(t, result.Content, 1, "expected the child's result content")
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "expected text content from the fake child")
	assert.Equal(t, "ok-alpha", text.Text, "expected the call to reach the child's handler")
}

func TestUpstreamStartHealthGateFailures(t *testing.T) {
	t.Parallel()

	objectSchema := map[string]any{"type": "object"}

	tests := []struct {
		name    string
		tools   []*mcp.Tool
		wantErr string
	}{
		{
			name:    "missing input schema",
			tools:   []*mcp.Tool{{Name: "broken"}},
			wantErr: "missing input schema",
		},
		{
			name:    "non-object input schema",
			tools:   []*mcp.Tool{{Name: "broken", InputSchema: map[string]any{"type": "array"}}},
			wantErr: `not "object"`,
		},
		{
			name:    "invalid tool name",
			tools:   []*mcp.Tool{{Name: "bad name", InputSchema: objectSchema}},
			wantErr: "invalid character",
		},
		{
			name: "duplicate tool names",
			tools: []*mcp.Tool{
				{Name: "dup", InputSchema: objectSchema},
				{Name: "dup", InputSchema: objectSchema},
			},
			wantErr: "duplicate tool name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			child := newFakeChild()
			child.server.AddReceivingMiddleware(serveToolList(tt.tools))
			tc := newTestContext(t, child, upstream.Options{})

			_, err := tc.up.Start(t.Context(), "unused-artifact")

			require.Error(t, err, "expected the health gate to reject the child")
			require.ErrorContains(t, err, "health gate", "expected the gate to identify itself")
			require.ErrorContains(t, err, tt.wantErr, "expected the rejection to say why")
			requireChildKilled(t, child)
		})
	}
}

func TestUpstreamStartHealthGateTimeout(t *testing.T) {
	t.Parallel()

	child := newFakeChild("alpha")
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	child.server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				select {
				case <-ctx.Done():
				case <-release:
				}
			}
			return next(ctx, method, req)
		}
	})
	tc := newTestContext(t, child, upstream.Options{HealthTimeout: 100 * time.Millisecond})

	start := time.Now()
	_, err := tc.up.Start(t.Context(), "unused-artifact")
	elapsed := time.Since(start)

	require.Error(t, err, "expected the health gate to time out on an unresponsive child")
	require.ErrorContains(t, err, "health gate", "expected the gate to identify itself")
	assert.Less(t, elapsed, 3*time.Second,
		"expected Start to fail within the health timeout, not hang on the child")
}

func TestUpstreamChildRuntimeToolChange(t *testing.T) {
	t.Parallel()

	child := newFakeChild("alpha")
	tc := newTestContext(t, child, upstream.Options{})
	session := tc.start(t)

	child.addTool("gamma")
	snapshot := awaitSnapshot(t, session, "gamma")
	assert.ElementsMatch(t, []string{"alpha", "gamma"}, toolNames(snapshot),
		"expected the re-listed snapshot to carry the full new set")

	// A burst with no reader: latest-wins publishing must absorb the extra
	// snapshot instead of wedging the child's notification handling.
	child.addTool("delta")
	child.addTool("epsilon")
	snapshot = awaitSnapshot(t, session, "epsilon")
	assert.Contains(t, toolNames(snapshot), "delta",
		"expected the final snapshot to include every tool from the burst")
}

func TestUpstreamChildDeathClosesDone(t *testing.T) {
	t.Parallel()

	child := newFakeChild("alpha")
	tc := newTestContext(t, child, upstream.Options{})
	session := tc.start(t)

	require.NoError(t, child.serverSession(t).Close(), "kill the fake child")

	require.Eventually(t, func() bool {
		select {
		case <-session.Done():
			return true
		default:
			return false
		}
	}, waitTimeout, tick, "expected Done to close when the child dies unexpectedly")
}

func TestUpstreamCloseDoesNotSignalDone(t *testing.T) {
	t.Parallel()

	child := newFakeChild("alpha")
	tc := newTestContext(t, child, upstream.Options{})
	session := tc.start(t)

	require.NoError(t, session.Close(), "close the child session")

	assert.Never(t, func() bool {
		select {
		case <-session.Done():
			return true
		default:
			return false
		}
	}, 300*time.Millisecond, tick, "expected an intentional Close never to report a crash on Done")
}

func TestUpstreamRootsListRejected(t *testing.T) {
	t.Parallel()

	child := newFakeChild("alpha")
	tc := newTestContext(t, child, upstream.Options{})
	tc.start(t)

	_, err := child.serverSession(t).ListRoots(t.Context(), nil)

	require.Error(t, err, "expected roots/list to be rejected, not answered with an empty list")
	require.ErrorContains(t, err, "does not support roots", "expected the explicit fidelity-gap error")
	assert.True(t, tc.logs.contains("roots/list"), "expected a loud log about the roots fidelity gap")
}

func TestUpstreamLoggingPassthrough(t *testing.T) {
	t.Parallel()

	child := newFakeChild("alpha")
	received := make(chan *mcp.LoggingMessageParams, 8)
	tc := newTestContext(t, child, upstream.Options{
		LogHandler:    func(_ context.Context, params *mcp.LoggingMessageParams) { received <- params },
		LevelProvider: func() mcp.LoggingLevel { return "warning" },
	})
	tc.start(t)

	session := child.serverSession(t)
	require.NoError(t, session.Log(t.Context(), &mcp.LoggingMessageParams{Level: "info", Data: "below"}),
		"log below the replayed level")
	require.NoError(t, session.Log(t.Context(), &mcp.LoggingMessageParams{Level: "error", Data: "above"}),
		"log above the replayed level")

	select {
	case params := <-received:
		assert.Equal(t, mcp.LoggingLevel("error"), params.Level,
			"expected only the message at or above the replayed level to be forwarded")
	case <-time.After(waitTimeout):
		t.Fatal("expected the error-level message to reach the log handler — was the level replayed?")
	}
	assert.Empty(t, received,
		"expected the info-level message to be gated by the replayed logging level")
}

func TestUpstreamWarnsOnUnforwardedCapabilities(t *testing.T) {
	t.Parallel()

	child := newFakeChild("alpha")
	child.server.AddPrompt(&mcp.Prompt{Name: "greet"},
		func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{}, nil
		})
	tc := newTestContext(t, child, upstream.Options{})

	tc.start(t)

	assert.True(t, tc.logs.contains("forwards tools only"),
		"expected a prominent warning that v1 does not forward the child's prompts/resources")
}

func TestNewValidation(t *testing.T) {
	t.Parallel()

	_, err := upstream.New(upstream.Options{})
	require.Error(t, err, "expected New to require an argv template or a transport factory")

	_, err = upstream.New(upstream.Options{Argv: []string{"./child", "stdio"}})
	require.NoError(t, err, "expected an argv template alone to satisfy New")

	_, err = upstream.New(upstream.Options{Transport: func(string) (mcp.Transport, error) {
		transport, _ := mcp.NewInMemoryTransports()
		return transport, nil
	}})
	require.NoError(t, err, "expected a transport factory alone to satisfy New")
}
