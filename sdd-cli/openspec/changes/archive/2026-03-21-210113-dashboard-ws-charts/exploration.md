# Exploration: dashboard-ws-charts

**Change:** dashboard-ws-charts
**Phase:** explore
**Date:** 2026-03-21

## 1. Current State

### Dashboard architecture

The dashboard is a self-contained `internal/dashboard` package with a single
`Server` struct. It serves three HTMX fragment endpoints that the browser polls
every 3 seconds:

- `GET /fragments/kpi` → renders `kpi.html` (Go template, server-side)
- `GET /fragments/pipelines` → renders `pipelines.html`
- `GET /fragments/errors` → renders `errors.html`
- `GET /static/htmx.min.js` → serves the ~50 KB vendored HTMX file

`base.html` is a full-page shell that contains three `<div>` elements with
`hx-get` / `hx-trigger="load, every 3s"` attributes. All rendering is
server-side Go templates. The browser holds no state beyond what HTMX manages
transparently.

There is no persistent connection. Each 3-second tick spawns three independent
HTTP requests. The only data flow is pull (browser → server).

### Current `MetricsReader` interface (server.go:25)

```go
type MetricsReader interface {
    TokenSummary(ctx context.Context) (*store.TokenStats, error)
    PhaseTokensByChange(ctx context.Context) ([]store.ChangeTokens, error)
    RecentErrors(ctx context.Context, limit int) ([]store.ErrorRow, error)
}
```

`*store.Store` satisfies this interface today. The dashboard command
(`cmd_dashboard.go`) opens the store and passes it directly to `dashboard.New`.

### Store schema (store.go:109)

Two tables exist today:

```sql
phase_events (id, timestamp, change, phase, bytes, tokens, cached, duration_ms)
verify_events (id, timestamp, change, command_name, command, exit_code,
               error_lines TEXT, fingerprint)
```

`phase_events` has all the columns needed for `TokenHistory`, `PhaseDurations`,
and `CacheHistory` queries. `verify_events` captures failure detail (error
lines, fingerprint) but only records failures — it has no concept of a passing
run. A new `verify_results` table is required for the verify chart.

### Event broker limitation

`events.Broker` is an in-process pub/sub bus. Each `sdd` CLI invocation
(e.g., `sdd verify foo`, `sdd write foo explore`) creates a fresh ephemeral
broker that lives only for the duration of that OS process. The dashboard server
runs in its own long-lived process. There is no mechanism to share a broker
across process boundaries. SQLite (WAL mode, safe for concurrent readers) is the
only shared persistent medium.

### cmd_verify.go flow (cmd_verify.go:17)

Three logical paths:
1. **Smart-skip** (line 42): if source files unchanged since last PASS, exits
   early returning JSON `{skipped: true}`. Does not touch the store at all.
2. **Normal run + pass**: runs `verify.Run`, writes report. Emits nothing to
   the store.
3. **Normal run + fail**: runs `verify.Run`, writes report. Opens store via
   `tryOpenStore`, wires broker via `newBroker`, emits `VerifyFailed` event.
   The `store.RegisterSubscribers` wiring causes `InsertVerifyEvent` to fire for
   each failed command.

No path currently writes to the store for passing runs. The verify chart
requires both pass and fail data, so `verify_results` must be populated on all
three paths.

### cmd_dashboard.go flow (cmd_dashboard.go:20)

Opens the store → creates `dashboard.Server` → blocks on
`srv.ListenAndServe(ctx, addr)`. No background goroutines beyond the HTTP
server's internal ones. Adding `hub.Run(ctx)` requires starting it before
`ListenAndServe`.

### Template and static asset inventory

| File | Lines | Role | Fate |
|------|-------|------|------|
| `templates/base.html` | 65 | Full-page shell, HTMX wiring | Full rewrite |
| `templates/kpi.html` | 18 | KPI fragment | Delete |
| `templates/pipelines.html` | 31 | Pipeline table fragment | Delete |
| `templates/errors.html` | 29 | Error table fragment | Delete |
| `static/htmx.min.js` | 1 (min) | HTMX library (~50 KB) | Delete |

## 2. Relevant Files

| File | Absolute Path | Relevance |
|------|---------------|-----------|
| `server.go` | `internal/dashboard/server.go` | Primary target: add hub field, remove 3 fragment handlers, add `/ws` route |
| `server_test.go` | `internal/dashboard/server_test.go` | 8 tests covering fragment endpoints + htmx static — all need updating |
| `base.html` | `internal/dashboard/templates/base.html` | Full rewrite to WS + ECharts layout |
| `store.go` | `internal/store/store.go` | Add `verify_results` table + 5 new methods |
| `store_test.go` | `internal/store/store_test.go` | Extend with 6+ new test cases |
| `cmd_verify.go` | `internal/cli/cmd_verify.go` | Add `verify_results` writes on all 3 code paths |
| `cmd_dashboard.go` | `internal/cli/cmd_dashboard.go` | Start hub goroutine before `ListenAndServe` |
| `broker.go` | `internal/events/broker.go` | Read-only reference — no changes needed |
| `types.go` | `internal/state/types.go` | `Phase`, `PhaseStatus` constants used for heatmap grid |
| `go.mod` | `go.mod` | Add `github.com/coder/websocket` |

New files to create:

| File | Absolute Path | Role |
|------|---------------|------|
| `hub.go` | `internal/dashboard/hub.go` | WS hub: poll loop, diff, broadcast, initial snapshot |
| `hub_test.go` | `internal/dashboard/hub_test.go` | Hub tests: upgrade, fan-out, dead client cleanup, diff logic |
| `echarts.min.js` | `internal/dashboard/static/echarts.min.js` | Vendored ECharts (~200 KB tree-shaken) |
| `dashboard.js` | `internal/dashboard/static/dashboard.js` | WS client, DOM updaters, ECharts managers |

## 3. Dependency Map

```
cmd_dashboard.go
    └── dashboard.New(db, changesDir)          [dashboard/server.go]
            └── hub.NewHub(metrics, changesDir) [dashboard/hub.go — NEW]
                    ├── store.TokenHistory      [store/store.go — NEW]
                    ├── store.PhaseDurations    [store/store.go — NEW]
                    ├── store.CacheHistory      [store/store.go — NEW]
                    ├── store.VerifyHistory     [store/store.go — NEW]
                    ├── store.TokenSummary      [existing]
                    ├── store.PhaseTokensByChange [existing]
                    ├── store.RecentErrors      [existing]
                    └── state.Load / AllPhases  [state/types.go + state/state.go]

cmd_verify.go
    └── store.InsertVerifyResult               [store/store.go — NEW]
            (on smart-skip, pass, and fail paths)

browser
    └── WebSocket /ws → hub.HandleWS           [dashboard/hub.go — NEW]
            broadcasts typed JSON messages
    └── GET /static/echarts.min.js             [dashboard/static/ — NEW]
    └── GET /static/dashboard.js               [dashboard/static/ — NEW]
```

Packages with no changes: `context/`, `phase/`, `artifacts/`, `config/`,
`fsutil/`, `csync/`, `sddlog/`, `errlog/`, `events/`, all other `cli/cmd_*.go`.

## 4. Data Flow

### Current (HTMX polling)

```
Browser                     Server                          SQLite
  │                            │                              │
  ├──GET /fragments/kpi──────► │──TokenSummary()───────────► │
  │ ◄──200 HTML KPI fragment── │ ◄──TokenStats─────────────── │
  │   (every 3s)               │                              │
  ├──GET /fragments/pipelines─► │──os.ReadDir(changesDir)      │
  │ ◄──200 HTML table───────── │  state.Load per entry        │
  │                            │                              │
  ├──GET /fragments/errors───► │──RecentErrors()──────────── ► │
  │ ◄──200 HTML table───────── │ ◄──[]ErrorRow──────────────── │
```

### Proposed (WebSocket push)

```
Browser                     Hub goroutine (1s tick)          SQLite / FS
  │                            │                              │
  ├──WS connect /ws──────────► │                              │
  │ ◄──snapshot (all types)─── │─initial DB queries──────────►│
  │                            │                              │
  │  [1s tick fires]           │─KPIs, pipelines, errors────► FS+DB
  │                            │─chart rows (WHERE id>last)──►│
  │                            │─diff against lastKnown       │
  │ ◄──{type:"kpi", data:...}──│  (only changed types sent)   │
  │ ◄──{type:"chart:tokens"}───│                              │
  │                            │                              │
  │  [JS dispatches by type]   │                              │
  │   handleKPI(data)          │                              │
  │   TokenChart.appendData()  │                              │
```

### Verify results write path (new)

```
sdd verify <name>                               SQLite
  │                                               │
  ├── smart-skip path:                            │
  │     tryOpenStore → InsertVerifyResult         │
  │     (passed=true, exit_code=0 for each cmd)  │
  │                                               │
  ├── normal run, pass:                           │
  │     tryOpenStore → InsertVerifyResult         │
  │     (passed=true, exit_code=0 for each cmd)  │
  │                                               │
  └── normal run, fail:                           │
        tryOpenStore → InsertVerifyResult         │
        (one row per cmd result, pass or fail)   │
        + existing VerifyFailed event emission   │
```

## 5. Risk Assessment

### R1 — WebSocket upgrade in Go stdlib HTTP server: LOW

`http.Server` supports WebSocket upgrades natively via `github.com/coder/websocket`
`Accept()`. The library handles the HTTP → WS handshake, write serialization,
and ping/pong. No special server configuration needed. Standard pattern.

### R2 — Concurrent write to dead WS connections: MEDIUM

The broadcast loop must handle clients that have disconnected between the tick
and the write. `github.com/coder/websocket` write errors on dead connections
are deterministic and synchronous. The hub must remove dead clients under
the same lock that guards the client map to avoid write races. The design
accounts for this with a remove-on-error pattern.

### R3 — SQLite WAL concurrent readers: LOW

The store is already opened in WAL mode with `busy_timeout=5000`. The hub's
1s tick queries are read-only and short. The CLI processes write-then-close.
Concurrent readers and a single writer in WAL mode is SQLite's intended use
case. No contention risk.

### R4 — `go:embed` with new static files: LOW

The `//go:embed static/*` directive in `server.go` is already present and
covers all files in `static/`. Adding `echarts.min.js` and `dashboard.js`
requires no code change to the embed — they are picked up automatically.
Deleting `htmx.min.js` requires removing the `TestStaticHtmx` test that
asserts on its presence.

### R5 — ECharts bundle size: LOW-MEDIUM

A full ECharts bundle is ~1 MB. Tree-shaken to the 5 chart types needed
(line, bar, scatter, heatmap, area) the spec estimates ~200 KB. This is
vendored and embedded in the binary, increasing binary size by ~200 KB.
Acceptable for a local dev dashboard tool. Binary size should be verified
post-build.

### R6 — `verify_results` schema migration on existing DBs: LOW

The `migrate()` function uses `CREATE TABLE IF NOT EXISTS`, so the new table
is added idempotently on first open of an existing DB. No data loss. Existing
`verify_events` and `phase_events` rows are unaffected.

### R7 — smart-skip path in cmd_verify.go: MEDIUM

The smart-skip path today does zero DB interaction. Adding
`tryOpenStore` + `InsertVerifyResult` on the skip path requires that the store
open succeeds (which may fail in test or unusual environments). The same
best-effort pattern already used for the failure path (`tryOpenStore` returns
nil on error, inserts are skipped) applies here. The skip path must not fail
if the store is unavailable.

### R8 — server_test.go and fakeMetrics interface drift: LOW

The `fakeMetrics` struct in `server_test.go` implements `MetricsReader`. When
4 new query methods are added to the interface, the existing `fakeMetrics`
will fail to compile. All test files in the package must be updated together.
The hub's `hub_test.go` will use a separate mock. Straightforward but all
tests must move atomically.

### R9 — JS reconnect loop vs server shutdown: LOW

On `sdd dashboard` exit (Ctrl-C), `ctx.Done()` fires, the HTTP server does a
3-second graceful shutdown, and all WS connections are closed. The browser's
exponential-backoff reconnect loop will then retry indefinitely. This is
acceptable behavior for a dev tool — the user closed the server intentionally.

### R10 — Tab resize event for ECharts: LOW

ECharts requires explicit `chart.resize()` on container size change. The tab
click handler must call `resize()` when showing a previously-hidden chart.
Without this, ECharts renders at 0×0 on first view. This is a known ECharts
initialization pattern and is accounted for in the design.

## 6. Approach Comparison

### Approach A: DB-poll hub (chosen)

The hub runs a goroutine that queries SQLite every 1 second, diffs against
last-known state, and pushes deltas over open WebSocket connections.

**Pros:**
- Works reliably across OS process boundaries (CLI vs dashboard server)
- Incremental chart data via sequence-based `WHERE id > lastSeq` queries
- Single transport (WS only), no mixed HTMX+WS complexity
- ~1s update latency vs 3s today

**Cons:**
- Requires an always-running goroutine in the server process
- Hub holds copies of last-known state (~KB, negligible)
- New `verify_results` table required for verify chart data

### Approach B: SSE (Server-Sent Events)

Replace HTMX polling with SSE (`text/event-stream`). No new Go dependency,
built into browsers natively.

**Pros:**
- No external WebSocket library
- Simpler than WS for unidirectional push

**Cons:**
- Still need DB polling on the server side (same architectural requirement)
- SSE does not support binary or bidirectional messages (no blocker here, but
  less flexible)
- ECharts integration is identical either way
- Less standard for this pattern in Go ecosystem; `coder/websocket` is lighter
  than a custom SSE writer

### Approach C: Keep HTMX, add chart iframes or polling fetch

Add charts via separate polling `fetch()` calls from JS, keep HTMX for
KPI/pipeline/error fragments.

**Pros:**
- Minimal server changes
- No new transport

**Cons:**
- Mixed transport (HTMX + fetch) increases mental complexity
- 3s latency unchanged
- 8+ polling intervals firing simultaneously (3 HTMX + 5 chart fetch)
- Does not eliminate htmx.min.js
- Charts would still need JS and ECharts — does not simplify the JS side

## 7. Recommendation

Implement Approach A (DB-poll hub) as specified in the design doc.

**Rationale:** The DB-poll model is the correct architectural fit because the
process-isolation constraint is real and fundamental — CLI commands cannot
share a broker with the dashboard server. SQLite WAL concurrent readers are
the designed solution. The implementation is straightforward: one new Go file
(`hub.go`, ~250 LOC), moderate changes to existing files, and a clean deletion
of the HTMX machinery.

**Implementation order (bottom-up):**

1. `store.go` — add `verify_results` table migration, `InsertVerifyResult`,
   and 4 new query methods + row types. No dependencies on new code.
2. `store_test.go` — extend with tests for all new store methods.
3. `cmd_verify.go` — add `verify_results` writes on all 3 paths (smart-skip,
   pass, fail). Depends on step 1.
4. `hub.go` — new file. Hub struct, `Run` goroutine, `HandleWS`, broadcast,
   diff logic, initial snapshot builder. Depends on step 1 (new query
   methods).
5. `hub_test.go` — new file. Tests for hub. Depends on step 4.
6. `server.go` — add hub field, add `/ws` route, remove 3 fragment handlers,
   extend `MetricsReader` interface with 4 new methods. Depends on steps 1, 4.
7. `server_test.go` — remove fragment tests, add `/ws` upgrade test, update
   `fakeMetrics` to satisfy new interface. Depends on step 6.
8. `static/echarts.min.js` — vendor tree-shaken bundle.
9. `static/dashboard.js` — WS client, DOM updaters, ECharts managers.
10. `templates/base.html` — rewrite. Remove HTMX divs, add chart containers,
    tab bar, load echarts + dashboard.js.
11. Delete `templates/kpi.html`, `templates/pipelines.html`,
    `templates/errors.html`, `static/htmx.min.js`.
12. `cmd_dashboard.go` — start `hub.Run(ctx)` goroutine before
    `ListenAndServe`.
13. `go.mod` / `go.sum` — `go get github.com/coder/websocket`.

Steps 1–3 are independently testable before the WS work begins. Steps 4–7
form the Go WS layer. Steps 8–12 are the client layer. All steps culminate
in `make check` (fmt + lint + test) as the final gate.

**No scope creep:** `events/broker.go`, `state/`, `phase/`, `context/`,
`artifacts/`, `config/`, and all non-dashboard CLI commands remain untouched.
