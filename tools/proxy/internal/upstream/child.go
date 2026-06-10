package upstream

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/meigma/template-mcp/tools/proxy/internal/reloader"
)

// childSession is one live child MCP connection, implementing the
// reloader.ChildSession port on top of an mcp.ClientSession.
type childSession struct {
	session *mcp.ClientSession

	// tools is the validated snapshot taken by the health gate; immutable
	// once Start returns.
	tools []*mcp.Tool

	// toolsCh delivers re-listed, validated snapshots after the child emits
	// its own tools/list_changed. Capacity 1, written latest-wins.
	toolsCh chan []*mcp.Tool

	// done closes when the child dies unexpectedly; an intentional Close
	// never closes it.
	done chan struct{}

	// ready closes once session is set: the happens-before barrier letting
	// notification handlers use the session.
	ready chan struct{}

	// closing marks an intentional Close before the connection drops so the
	// done watcher never reports it as a crash.
	closing atomic.Bool

	logHandler    func(context.Context, *mcp.LoggingMessageParams)
	relistTimeout time.Duration
	logger        *slog.Logger
}

// Tools returns the validated tool snapshot taken by the health gate.
func (s *childSession) Tools() []*mcp.Tool { return s.tools }

// CallTool forwards one tool call to the child.
func (s *childSession) CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	return s.session.CallTool(ctx, params)
}

// ToolsChanged delivers a re-listed and validated tool snapshot each time
// the child emits its own tools/list_changed.
func (s *childSession) ToolsChanged() <-chan []*mcp.Tool { return s.toolsCh }

// Done is closed when the child dies unexpectedly.
func (s *childSession) Done() <-chan struct{} { return s.done }

// Close terminates the child session and, through the transport, its
// process. An intentional close is never reported as a crash on Done.
func (s *childSession) Close() error {
	s.closing.Store(true)
	return s.session.Close()
}

// watchDone turns the connection ending into crash detection for the core's
// supervisor: done closes unless the shutdown was an intentional Close.
func (s *childSession) watchDone() {
	_ = s.session.Wait()
	if !s.closing.Load() {
		close(s.done)
	}
}

// onToolListChanged re-lists the child's tools and publishes the validated
// snapshot. The synchronous re-list on the notification goroutine is safe:
// JSON-RPC responses bypass the handler queue, so the list call completes
// while this handler blocks, and the serialization of subsequent child
// notifications behind it is desirable ordering. A snapshot that fails
// validation is dropped loudly and the previously advertised set stays.
func (s *childSession) onToolListChanged(ctx context.Context, _ *mcp.ToolListChangedRequest) {
	select {
	case <-s.ready:
	default:
		// Pre-gate notification: the health gate's own list runs later and
		// captures the final set.
		return
	}

	listCtx, cancel := context.WithTimeout(ctx, s.relistTimeout)
	defer cancel()

	tools, err := listTools(listCtx, s.session)
	if err != nil {
		s.logger.ErrorContext(ctx,
			"re-listing tools after the child's tools/list_changed failed; keeping the advertised set",
			"error", err)
		return
	}
	if err := reloader.ValidateTools(tools); err != nil {
		s.logger.ErrorContext(ctx,
			"child runtime tool change failed validation; keeping the advertised set",
			"error", err)
		return
	}
	s.publish(tools)
}

// onLoggingMessage forwards one child log notification to the configured
// handler. It needs no session, so it safely runs pre-ready.
func (s *childSession) onLoggingMessage(ctx context.Context, req *mcp.LoggingMessageRequest) {
	if handler := s.logHandler; handler != nil {
		handler(ctx, req.Params)
	}
}

// publish replaces any unread snapshot with the newest one and never blocks.
// The core only consumes ToolsChanged while the child is the serving one, so
// a blocking send here would wedge the child's notification handling.
func (s *childSession) publish(tools []*mcp.Tool) {
	for {
		select {
		case s.toolsCh <- tools:
			return
		default:
		}
		select {
		case <-s.toolsCh:
		default:
		}
	}
}
