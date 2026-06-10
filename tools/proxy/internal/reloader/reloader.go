package reloader

import (
	"errors"
	"log/slog"
)

// Options configures a Reloader for [New].
type Options struct {
	// Watcher streams raw source-change events for the core to debounce.
	// Required.
	Watcher Watcher

	// Builder produces the unique child artifact for each reload cycle.
	// Required.
	Builder Builder

	// Upstream spawns and health-gates child MCP sessions from built
	// artifacts. Required.
	Upstream Upstream

	// Logger receives orchestration diagnostics. Nil selects a no-op
	// logger.
	//
	// WARNING: a logger must never write to [os.Stdout]. Stdout is the
	// JSON-RPC protocol channel on both of the proxy's hops; stderr is
	// always safe.
	Logger *slog.Logger

	// Clock supplies debounce and backoff timers. Nil selects real time.
	Clock Clock
}

// Reloader is the dev proxy's core orchestrator.
//
// It owns the watch, debounce, build, health-gate, swap cycle that keeps a
// persistent downstream MCP session pointed at a disposable child server.
// This skeleton carries construction and wiring only; the orchestration loop
// lands in a later milestone.
type Reloader struct {
	watcher  Watcher
	builder  Builder
	upstream Upstream
	logger   *slog.Logger
	clock    Clock

	frontend Frontend
}

// New constructs a Reloader from the provided Options.
//
// The Watcher, Builder, and Upstream ports are required; New returns an
// error when any of them is nil. A nil Logger selects a no-op logger and a
// nil Clock selects real time. The returned Reloader still needs its
// client-facing side wired via [Reloader.SetFrontend] before it can run.
func New(options Options) (*Reloader, error) {
	if options.Watcher == nil {
		return nil, errors.New("watcher is required")
	}
	if options.Builder == nil {
		return nil, errors.New("builder is required")
	}
	if options.Upstream == nil {
		return nil, errors.New("upstream is required")
	}

	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	clock := options.Clock
	if clock == nil {
		clock = systemClock{}
	}

	return &Reloader{
		watcher:  options.Watcher,
		builder:  options.Builder,
		upstream: options.Upstream,
		logger:   logger,
		clock:    clock,
	}, nil
}

// SetFrontend wires the client-facing side the core reconciles tool sets
// against. It must be called before the orchestration loop runs.
//
// SetFrontend exists to resolve the construction cycle between the core and
// the downstream adapter: build the core first, construct the downstream
// adapter with the core's router (a [CallToolFunc]), then hand the adapter
// back here. The Frontend never needs the core type, only the CallToolFunc.
func (r *Reloader) SetFrontend(frontend Frontend) {
	r.frontend = frontend
}
