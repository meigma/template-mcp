# MCP Dev Proxy — Design (TEMPORARY)

> **Status: rough draft for discussion. This document is temporary** — it guides
> the first implementation pass and will be deleted (or folded into the proxy's
> README) once the implementation settles. Do not treat it as durable
> documentation.

## 1. Problem

Developing an MCP server *with* an LLM is awkward: the client (Claude Code)
spawns the server as a stdio subprocess and reads its tool list at session
start. Every code change requires killing the conversation, rebuilding, and
reconnecting — exactly the loop that breaks iterative "LLM works on the MCP
server it is connected to" development.

The fix is a **dev proxy** that sits between the client and the
in-development server:

- The client connects to the proxy **once** and keeps that session for the
  whole dev loop.
- The proxy supervises the real server as a **child process**, watches the
  source tree, rebuilds and restarts the child on change, and re-advertises
  the child's tools to the client via `notifications/tools/list_changed`.

### Verified foundation (do not re-litigate)

- **Empirically confirmed (2026-06-09):** Claude Code 2.1.170 honors
  `tools/list_changed` on a live session — it re-fetched the tool list and
  called a newly **added** tool (returning an unguessable secret) in the next
  turn, with no reconnect. This is the load-bearing fact for the design.
  *Not yet verified:* whether a **same-named tool with a changed schema** is
  refreshed (see §10).
- **Known constraint:** tool changes do not propagate mid-turn. Reloads land
  whenever a build finishes; the client simply *observes* the new tool set at
  its next turn boundary. The proxy does not (and cannot) detect or target
  turn boundaries.
- **Prior art:** `neilopet/mcpmon` (Node) proves the pattern (buffer →
  restart → replay handshake → synthesize `list_changed`). There is no Go
  implementation on the official SDK, and no stdio↔HTTP-hot-reload prior art.
- **SDK facts (go-sdk v1.6.1, verified in source):**
  - `(*mcp.Server).AddTool` **replaces** an existing same-named tool in place
    and always emits `tools/list_changed` (`server.go:280` "add replaces
    existing tools"); notifications are coalesced by a short debounce.
    `RemoveTools(names ...string)` is the removal API.
  - `(*mcp.Server).AddTool` **panics** when the input schema is nil,
    non-object, or unmarshalable; the output schema is **optional** and
    validated (object-typed, marshalable) only when present
    (`server.go:247-276`). Child tool definitions are untrusted input and
    MUST be validated before registration (§4, §5).
  - The proxy must register passthrough tools via the **non-generic**
    `(*mcp.Server).AddTool` with a raw `mcp.ToolHandler`. The generic
    `mcp.AddTool[In,Out]` used elsewhere in this template validates and
    defaults arguments against Go types — wrong for a proxy, which must
    forward arguments byte-for-byte and let the child do the validating.
  - `mcp.CommandTransport` runs a child over stdin/stdout with spec-correct
    shutdown: close stdin → SIGTERM → SIGKILL; `TerminateDuration` is
    configurable (default 5s — too slow for a dev loop; we set it short).
    It does **not** wire the child's stderr; the proxy sets `cmd.Stderr`.
  - `mcp.ClientOptions` exposes `ToolListChangedHandler` (and prompt/
    resource/logging/progress handlers) for the proxy's upstream client.
  - The SDK client **advertises `roots` by default** and answers
    `roots/list` itself with an empty list (`client.go:471`) — and the
    dispatch table registers that handler **unconditionally**
    (`client.go:906`), so disabling the capability via `ClientCapabilities{}`
    changes what is advertised but not what is answered. Making roots fail
    loudly requires receiving middleware that rejects the method.
  - `(*Server).AddTool` only **logs** an invalid tool name and registers it
    anyway (`server.go:239`); name validity and duplicates are the caller's
    problem.
  - `mcp.ServerOptions.Capabilities`: any non-nil field overrides inferred
    capabilities — this is how the proxy advertises a superset envelope.
  - `mcp.NewInMemoryTransports` enables full in-process integration tests.

## 2. Scope

**v1 (this design): stdio downstream, stdio child, tools only.**

| In scope (v1) | Out of scope (v1), open for later |
|---|---|
| stdio downstream (client ⇄ proxy) | Streamable HTTP downstream (the port exists; see §8) |
| stdio child via `CommandTransport` | HTTP upstream child |
| Tool forwarding + `tools/list_changed` reconciliation | Prompts/resources forwarding (stubbed; superset capability already advertised) |
| Child **runtime** tool changes (child emits its own `list_changed`) | Server→client passthrough of sampling/elicitation/roots (child gets a loud error, §5) |
| Watch → debounce → build → health-gate → swap | Progress passthrough (progress tokens are **stripped** from forwarded calls in v1) |
| Cancellation passthrough (downstream cancel → ctx → child call) | Multi-child aggregation |
| Child crash supervision (restart with backoff) | Metrics (slog only in v1; metrics stay opt-in per house style) |
| Child stderr passthrough | Forwarding child `instructions` (§10) |

**v1 fidelity gaps (explicit):** a child that uses sampling or elicitation
gets errors (the proxy's upstream client has no handlers). Roots needs
explicit handling on **both** axes: the upstream client sets
`Capabilities: &mcp.ClientCapabilities{}` so roots are not advertised, AND
its receiving middleware (`Client.AddReceivingMiddleware`) **rejects**
`roots/list` with an unsupported-method error — capability opt-out alone is
insufficient because the SDK's built-in handler answers `[]` regardless of
what was advertised (`client.go:906`). The middleware also logs all
sampling/elicitation/roots traffic loudly; the gap is always visible on
stderr, never silent.

**Prompts/resources under the superset envelope:** the envelope means the
downstream SDK server answers `prompts/list`/`resources/list` with empty
lists in v1 — which would *silently* hide a child that actually has prompts
or resources. Two deliberate mitigations: (1) the health gate inspects the
child's advertised capabilities, and if the child exposes prompts or
resources the proxy logs a prominent warning that v1 does not forward them;
(2) forwarding lands in v1.1 (§10). Narrowing the envelope to tools-only is
NOT the fix — capabilities freeze at the downstream initialize, so dropping
prompts/resources from the envelope would make a child's first prompt
permanently invisible without a client reconnect (the drift §3 exists to
prevent).
Child MCP `logging` notifications: the proxy re-sends the downstream client's
last `logging/setLevel` to each new child and forwards child log messages
downstream (`LoggingMessageHandler` → downstream session log) — cheap and
keeps the Logging capability honest; child stderr remains the primary debug
channel.

**Placement:** `tools/proxy` as a **nested Go module**
(`github.com/meigma/template-mcp/tools/proxy`). Rationale: the proxy is dev
tooling — its dependencies (`fsnotify`) must not leak into the template's
`go.mod` that consumers inherit, and GoReleaser/release workflows must not
ship it. It can incubate here and be extracted to its own repo if it proves
broadly useful.

Nested-module wiring (decided, not open): add `proxy: 'tools/proxy'` to
`.moon/workspace.yml` projects, give the proxy its own `moon.yml` with
format/lint/build/test tasks (run from `tools/proxy`, reusing the root
`.golangci.yml` with the module-path-appropriate local-prefix override), and
add `proxy:check`-equivalent deps to `root:check` so CI covers it. The
nested module also carries its own `.mockery.yaml` (the root module has no
mockery setup today).

## 3. Architecture

Hexagonal, mirroring the template's own seam (transport-agnostic core, thin
transport adapters). The core package never constructs transports, processes,
or watchers; those live in adapters behind ports. (The core does import the
`mcp` package for its data types — `*mcp.Tool`, call params/results — which
is deliberate: those types *are* the domain vocabulary.)

```
tools/proxy/
  DESIGN.md                    (this file — temporary)
  go.mod                       nested module; not part of template releases
  .mockery.yaml                mockery config for the port mocks
  cmd/mcp-devproxy/main.go     thin entrypoint: signal ctx, exit code
  internal/cli/                cobra root + flags (mirrors template cli shape)
  internal/reloader/           THE HEXAGON: orchestrator + reconciler + buffer
                               (pure logic; ports + mcp types + injected clock)
  internal/upstream/           driven adapter: go-sdk mcp.Client + CommandTransport
  internal/downstream/         driving adapter: go-sdk mcp.Server + stdio transport
                               (Streamable HTTP slots in here later, §8)
  internal/watch/              driven adapter: fsnotify file watcher
  internal/build/              driven adapter: exec'd build command (go build)
```

### Ports (defined in `internal/reloader`)

Interfaces sit at external boundaries only, per house style. Sketch — names
and shapes are first-pass, expected to move during implementation:

```go
// Watcher reports source-change events. Implementations send raw events;
// debouncing/coalescing is core logic (it must be unit-testable).
type Watcher interface {
    // Watch streams change events until ctx is cancelled.
    Watch(ctx context.Context) (<-chan ChangeEvent, error)
}

// Builder produces a runnable child artifact from the source tree.
type Builder interface {
    // Build runs one build. A non-nil error means "keep the old child".
    // BuildResult carries the unique artifact path for this cycle and any
    // compile output for surfacing to the developer.
    Build(ctx context.Context) (BuildResult, error)
}

// Upstream spawns and supervises one child MCP session at a time.
type Upstream interface {
    // Start launches the artifact, connects, initializes, health-gates it
    // (ListTools under timeout), and VALIDATES every advertised tool
    // definition (schema present, object-typed, marshalable — the
    // downstream AddTool panics otherwise). Invalid tools fail the gate.
    // A non-nil error means "keep the old child".
    Start(ctx context.Context, artifact string) (ChildSession, error)
}

// ChildSession is one live child MCP connection.
type ChildSession interface {
    Tools() []*mcp.Tool                 // validated snapshot from the health gate
    CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
    ToolsChanged() <-chan []*mcp.Tool   // child emitted its own list_changed (re-listed + validated)
    Done() <-chan struct{}              // closed when the child dies unexpectedly
    Close() error
}

// Frontend is the client-facing side the core drives. The downstream adapter
// implements it on top of mcp.Server: removed tools via RemoveTools, added
// and changed tools via a single replacing (*Server).AddTool each — which is
// what emits the (coalesced) list_changed. No Remove+Add dance: AddTool
// replaces in place, the notification has no payload, and clients refetch
// the same final list either way; a Remove+Add would only open a window in
// which the tool transiently does not exist.
type Frontend interface {
    // Reconcile makes the advertised tool set match tools, wiring each
    // tool's handler to call. It returns an error instead of panicking on a
    // definition that slipped past validation. A no-op diff makes no
    // AddTool/RemoveTools calls and emits nothing.
    Reconcile(tools []*mcp.Tool, call CallToolFunc) error
}

// CallToolFunc routes one forwarded tool call; the core provides its router
// method, which targets the current ChildSession (or the swap buffer).
//
// Forwarding conversion (v1): the router constructs fresh CallToolParams
// carrying only Name and the raw Arguments bytes (byte-for-byte, no
// validation or defaulting). Meta is dropped entirely — including the
// progressToken, which lives in _meta (protocol.go GetProgressToken):
// forwarding it would invite progress notifications the proxy does not
// relay in v1 (§2). Cancellation still propagates via ctx.
type CallToolFunc func(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
```

Construction order (resolves the chicken-and-egg between core and
downstream): build the core first (`reloader.New(reloader.Options{Watcher,
Builder, Upstream, Logger, Clock})` — nil `Logger` → no-op, nil `Clock` →
real time, both per house convention); construct the downstream adapter with
the core's router method value; then `core.SetFrontend(frontend)` before
`Run`. The Frontend never needs the core type, only the `CallToolFunc`.

**Logging passthrough bypasses the core** (the core orchestrates reloads,
not log lines). The downstream adapter observes `logging/setLevel` via
`Server.AddReceivingMiddleware` and exposes the last-known level; the
upstream adapter is constructed with a log-forwarding callback (its
`LoggingMessageHandler` → the downstream session's `Log`, which the SDK
gates on the client's level) and applies the last-known level to each new
child via `ClientSession.SetLoggingLevel` after connect. The cli package
wires the two adapters together at construction.

**Stale-view gating is downstream-adapter-local.** A post-swap `tools/call`
carries only a name and raw arguments — no generation — so the proxy cannot
tell from the request alone whether the client issued it from a stale cached
tool list. The adapter therefore tracks generations itself: `Reconcile`
bumps a per-tool generation for every changed tool and records tombstones
for removed names, and the downstream server's receiving middleware records,
per session, the generation at that session's most recent `tools/list`
response. A `tools/call` whose target tool changed after the session's last
list — or whose name is a tombstone — is intercepted and answered with the
friendly stale-reload error ("tool changed by dev reload; list refreshes
next turn") instead of dispatching. The moment the client re-lists (which
Claude Code does on `list_changed` — verified), calls flow normally. This is
what makes the §5 stale-view row implementable for same-name schema changes,
and it upgrades removed-tool calls from the SDK's raw "unknown tool" error
to the friendly one.

### Data flow

```
Claude Code ⇄ stdio ⇄ [downstream: mcp.Server]      (persistent session)
                              │ CallToolFunc / Reconcile
                        [reloader: orchestrator]
                              │ CallTool / Start / Close
                      [upstream: mcp.Client] ⇄ stdio ⇄ child server  (disposable)
                              ▲
        [watch: fsnotify] → [debounce] → [build: go build -o <unique path>]
```

Two key identities:

- **Downstream session is persistent.** It is initialized once by the real
  client; its capabilities are frozen at that initialize (spec rule), which
  is why the proxy advertises a **superset capability envelope** up front
  (`Capabilities{Tools/Prompts/Resources with ListChanged, Logging}`) even
  though v1 forwards only tools. Without this, a child gaining its first
  prompt later could never surface without a reconnect.
- **Upstream sessions are disposable.** The proxy is itself a real MCP
  client; each new child gets a fresh, normal `initialize` from the proxy
  (no need to replay the downstream client's exact init params — confirm in
  the integration tests; mcpmon replays verbatim, but the proxy terminates
  the downstream session itself, so a proxy-identity handshake should be
  correct).

### Cold start

The proxy serves the downstream session **immediately** with an empty tool
set, then kicks off the first build/health-gate cycle; the first successful
child triggers a normal Reconcile (and thus `list_changed`). This keeps
"client spawns proxy" instant and survives a broken first build (§5). The
pre-first-turn `list_changed` case is covered in the acceptance run (§9).
Consequence: the child's `instructions` are not known at downstream
initialize time, so v1 does not forward instructions (§10).

## 4. Reload lifecycle

States: `SERVING → BUILDING → STARTING(health-gate) → SWAPPING → SERVING`,
with failure edges back to `SERVING` (old child kept).

On a debounced change event (default debounce ~300ms, coalescing bursts):

1. **Build.** `Builder.Build` writes a **unique artifact per cycle** (temp
   path carried in `BuildResult`) — never overwrite the running child's
   binary in place (ETXTBSY on Linux; macOS codesign invalidation can
   SIGKILL the running process). On failure: log the compile error to
   stderr, stay on the old child. The developer's broken save never kills
   the working server.
2. **Start + health-gate.** `Upstream.Start` spawns the new child, connects,
   initializes, issues `ListTools` under a timeout (paginate via the SDK's
   `Tools` iterator), and **validates every tool definition**: input schema
   present, `type:"object"`, marshalable; output schema — optional — held to
   the same shape only when present, so the downstream `AddTool` panic path
   is unreachable without rejecting valid tools that have no structured
   output; tool names valid per the SDK's rules (the SDK only logs invalid
   names and registers them anyway);
   and **no duplicate names** (the reconcile diff keys on name — duplicates
   are ambiguous). Any failure → kill the half-started child, keep the old
   one. The old child keeps serving during build+start, so the swap gap is
   tiny.
3. **Quiesce.** Stop routing new downstream calls (buffer them — bounded,
   with a per-call timeout); wait for in-flight calls on the old child to
   drain, up to a grace timeout. Calls still outstanding after the grace
   period get an error result — non-idempotent calls cannot be transparently
   replayed.
4. **Swap.** Atomically repoint the router to the new `ChildSession`; `Close`
   the old one (short `TerminateDuration`, ~1s for dev). (The new child
   already received the downstream client's last `logging/setLevel` during
   `Start` — see the logging-passthrough wiring in §3.)
5. **Reconcile.** Diff old vs new tool sets: key on name, detect change via
   a **canonical fingerprint of the full wire definition** (canonical-JSON
   marshal of the `mcp.Tool`: name, title, description, input/output
   schemas, annotations — read-only/destructive hints affect client
   behavior — icons, and `_meta`; no fields ignored). Removed →
   `RemoveTools`; added and changed → one replacing `AddTool` each. The SDK
   coalesces the resulting `tools/list_changed`. An identical tool set
   produces zero calls and no notification.
6. **Drain.** Release buffered calls to the new child — **gated on tool
   generation**: every call records, at ingress, a fingerprint of the tool
   definition the client could see when it issued the call. A buffered call
   is forwarded only if the new generation's definition for that tool is
   identical; if the tool was removed or changed by the swap, the call gets
   the proxy's stale-reload error instead (§5). A non-idempotent call issued
   against old semantics must never silently execute on new code.

Additional supervisor behaviors:

- **Serialized swaps.** A change event during `BUILDING/STARTING/SWAPPING`
  cancels-and-supersedes: finish or abort the current cycle, then run once
  more. Never two children racing for the same downstream session.
- **Crash supervision.** `ChildSession.Done()` firing in `SERVING` triggers
  an immediate restart cycle (skip build — the last good artifact is intact)
  with exponential backoff. `Done()` firing during an in-flight cycle is
  noted and otherwise ignored: the cycle already replaces the child.
- **Child runtime tool changes.** `ToolsChanged()` delivering a new
  (validated) snapshot in `SERVING` triggers Reconcile directly — no process
  restart. (This is how the 2026-06-09 verification server behaved.)

## 5. Failure handling summary

| Failure | Behavior |
|---|---|
| Build fails | Keep old child; log compile output; stay `SERVING` |
| New child fails init/health-gate (incl. malformed tool definitions) | Kill it; keep old child |
| **First** build/child fails (no old child) | Serve empty tool set; log loudly; retry with backoff |
| Old child hangs on Close | `CommandTransport` escalates SIGTERM→SIGKILL (short terminate duration) |
| Call in flight during swap | Drain up to grace timeout, then error that call |
| Call arrives mid-swap | Buffered (bounded queue + per-call timeout); drained to the new child only if the tool's definition is unchanged, else stale-reload error (§4.6) |
| Buffer overflow / timeout | Error the excess/expired calls; never block the downstream session |
| Stale client view post-swap (mid-turn): call names a removed/changed tool | Intercepted by per-session stale-view gating (§3): friendly stale-reload error until the session re-lists |
| Child crashes while serving | Auto-restart with backoff (skip build); downstream session survives |
| Child crashes during an in-flight cycle | Note it; the cycle's swap already replaces the child |
| Child uses sampling/elicitation/roots | Child gets an error; proxy logs the gap loudly (v1 fidelity gap, §2) |
| Watcher burst (save storms) | Debounce + cancel-and-supersede |
| Client exits / proxy signaled (possibly mid-swap) | Cancel any in-flight cycle; `Close` **all** children (serving + candidate — `SWAPPING` briefly owns two); exit cleanly, no orphans |

## 6. CLI shape

Mirrors the template's cobra/viper conventions (env prefix `MCP_DEVPROXY_*`,
flag-name constants). The child command is positional argv after `--`;
`--watch` is a repeatable directory flag (fsnotify watches directories — the
watch adapter handles recursion; these are paths, not Go package patterns):

```sh
mcp-devproxy \
  --build "go build -o {{artifact}} ./cmd/template-mcp" \
  --watch cmd --watch internal \
  [--debounce 300ms] [--quiesce 5s] [--terminate 1s] \
  -- {{artifact}} stdio
```

`{{artifact}}` is substituted with the cycle's unique build artifact (§4.1).
Claude Code config then points at the proxy instead of the server:

```json
{"mcpServers": {"dev": {"command": "mcp-devproxy", "args": ["--build", "...", "--watch", "cmd", "--", "..."]}}}
```

Zero-config defaults for this template's layout are a convenience layer in
`internal/cli`, kept apart from the generic flags so extraction to a
standalone repo stays clean.

## 7. Observability

- **stdout is the protocol channel on BOTH hops** (downstream to client,
  upstream to child). All proxy logging goes to **stderr** via `slog` —
  identical discipline to the template server, same failure mode if broken.
- **Child stderr is forwarded to proxy stderr** (set `cmd.Stderr` on the
  `exec.Cmd`; `CommandTransport` does not do this for us). The developer
  sees their server's logs as if it ran directly.
- Core packages accept `*slog.Logger`, nil → no-op, per house style.
- No metrics in v1; if added later they are opt-in and OpenMetrics-shaped.

## 8. Adding networked transports later (explicitly open)

The downstream adapter is the seam — the same one the template itself uses
(`stdio.go` vs `http.go`). To add a Streamable HTTP downstream:

- `internal/downstream` gains an HTTP variant: `StreamableHTTPHandler` +
  the same hardening the template's `http` command bakes in (Origin
  verification wrap, loopback default, fail-closed non-loopback bind).
- The proxy owns the `Mcp-Session-Id`, so the session id survives child
  swaps; the unsolved-anywhere part is bridging open SSE streams across a
  swap (possibly requiring the SDK `EventStore` for resumability) — this is
  why HTTP is deferred, not because the hexagon resists it.
- Nothing in `internal/reloader`, `internal/upstream`, `internal/watch`, or
  `internal/build` changes.

## 9. Testing strategy (per house go-testing rules)

- **Unit (reloader core, the bulk):** mockery mocks for `Watcher`,
  `Builder`, `Upstream`, `ChildSession`, `Frontend`. Table-driven, testify
  `assert`/`require`, shared `testContext` struct, injected fake clock (no
  real sleeps). Behaviors to prove:
  - build failure keeps the old child serving;
  - health-gate failure (malformed tool definition, invalid tool name, or
    duplicate names) keeps the old child;
  - successful cycle swaps, closes the old child, reconciles, drains buffer;
  - calls during swap are buffered then served by the new child when the
    tool definition is unchanged; buffered calls to a removed/changed tool
    get the stale-reload error (generation gating);
  - post-swap stale-view gating: a call to a changed tool from a session
    that has not re-listed gets the friendly error; after the session
    re-lists, the same call dispatches (downstream-adapter test);
  - fingerprint sensitivity: an annotations-only change (e.g. readOnlyHint)
    counts as changed;
  - buffer overflow errors excess calls; buffered calls time out if the
    swap stalls;
  - in-flight calls past the grace timeout get errors;
  - an identical tool set produces zero Frontend calls (no notification);
  - overlapping change events serialize (cancel-and-supersede, single final
    cycle);
  - crash in `SERVING` restarts with backoff; crash mid-cycle is absorbed;
  - cold start serves empty, then reconciles on first healthy child;
  - `ToolsChanged()` triggers reconcile without a restart;
  - shutdown closes both children when signaled mid-swap.
- **Integration (in-process):** real go-sdk wiring over
  `mcp.NewInMemoryTransports`: a real `mcp.Client` (standing in for Claude)
  connected to the proxy's downstream `mcp.Server`; a real in-process
  `mcp.Server` as the fake child behind the Upstream port (the port takes a
  transport-producing seam precisely so tests can inject in-memory children).
  Drive a simulated reload and assert the client's `ToolListChangedHandler`
  fires and a subsequent `ListTools`/`CallTool` sees the new tool, with
  arguments passed through byte-for-byte (proving the non-generic AddTool
  passthrough does not validate/default).
- **E2E (slow, few):** real `CommandTransport` child + real `go build` in a
  `t.TempDir()` scratch module. The scratch module must build **offline**:
  give it a `replace` directive pointing at the locally cached go-sdk (or
  pre-build the child fixture once per run) so tests never hit the network.
  One or two tests only — they exist to prove the exec/fsnotify adapters,
  not the orchestration logic.
- **Acceptance (manual, documented):** the tmux + Claude Code procedure from
  the 2026-06-09 verification, extended with two scenarios: (a) the original
  added-tool flow, (b) a **schema-only change** to a same-named tool, and
  (c) a cold start where the first `list_changed` fires before the first
  turn. Re-run when Claude Code majors change.

## 10. Open questions

1. **Binary/module name.** `mcp-devproxy` is the working name — confirm.
2. **Schema-only refresh (unverified).** Does Claude Code refresh the cached
   definition of a *same-named* tool after `list_changed`? The 2026-06-09
   test only proved tool **addition**. Settle via acceptance scenario (b)
   before relying on schema iteration in the dev loop. Note: the stale-view
   gate (§3) opens as soon as the session re-lists; whether the client then
   *applies* the refreshed schema is this question — if it re-lists but
   keeps a stale cached schema, old-shape arguments still reach the new
   child, whose own validation rejects them.
3. **Status surface.** stderr-only, or also a proxy-injected
   `devproxy_status` tool (build state, last error) the LLM itself can call?
   mcpmon-style injected tools are handy but pollute the tool list.
4. **Upstream init params.** Fresh proxy-identity initialize vs replaying
   the downstream client's params verbatim — settle empirically in the
   integration tests.
5. **Instructions forwarding.** Cold-start-first (§3) means child
   instructions can't be in the downstream initialize. Acceptable for v1
   (template server sets none)? Alternative — block downstream init on the
   first child — trades startup robustness for instruction fidelity.
6. **Prompts/resources forwarding** in v1.1 — the superset envelope already
   advertises them; forwarding is mostly mechanical.
7. **Defaults** for debounce/quiesce/terminate timings — tune on the
   template's own dev loop.

## 11. Non-goals

- Production traffic. This is a development tool; it deliberately trades
  strict transparency (e.g., erroring in-flight calls on swap) for loop
  speed.
- Aggregating multiple servers, auth, or transport bridging — existing
  proxies (mcp-proxy, supergateway) already cover those.
- Hot code swap inside the Go process (plugins). Process restart behind a
  stable session is the reliable mechanism.
