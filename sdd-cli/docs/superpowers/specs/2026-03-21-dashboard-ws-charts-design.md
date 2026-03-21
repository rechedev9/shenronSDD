# Dashboard Upgrade: WebSocket + ECharts

**Date:** 2026-03-21
**Status:** Draft

## Summary

Replace HTMX polling with WebSocket push (DB-poll driven) and add 5 ECharts
visualizations behind a tab bar. Drop htmx.min.js, add echarts.min.js +
dashboard.js. One new Go file (hub.go), moderate changes to server.go and
store.go, full rewrite of base.html. Adds a `verify_results` table so the
verify chart shows both passes and failures.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Charts | All 5: tokens over time, phase durations, cache ratio, verify timeline, phase heatmap | Full observability into the pipeline |
| Chart library | Apache ECharts (~200KB tree-shaken) | Heatmap, area, scatter, line, bar all native. One dependency. |
| Real-time transport | WebSocket with DB-poll push | CLI commands run as separate processes with ephemeral brokers — events can't cross process boundaries. SQLite (WAL mode) is the shared medium. Hub polls DB on 1s interval, diffs, pushes deltas over WS. |
| WS library | `github.com/coder/websocket` | Pure Go, zero external deps, actively maintained. Replaces archived gorilla/websocket. |
| Layout | Tabs below KPI cards | Compact, one chart visible at a time. No scroll fatigue. |
| HTMX | Fully replaced | One transport. Drop 50KB. Simpler mental model. |

## Architecture

### Why DB-Poll, Not Broker-Driven

Each `sdd` CLI invocation (e.g., `sdd verify`, `sdd context`) is a separate
OS process that creates its own ephemeral `events.Broker`. The dashboard runs
as a long-lived HTTP server in a different process. There is no shared broker
across processes. The only shared state is SQLite (WAL mode, safe for
concurrent readers) and the filesystem (`state.json`, artifacts).

The hub polls SQLite + filesystem on a 1-second interval, diffs against
last-known state, and pushes only the deltas over WebSocket. This gives
~1s latency (vs 3s HTMX polling today) and works reliably across process
boundaries.

### WebSocket Hub (`internal/dashboard/hub.go`, ~250 LOC)

```go
type Hub struct {
    clients    map[*websocket.Conn]struct{}
    mu         sync.Mutex
    metrics    MetricsReader
    changesDir string

    // Last-known state for diffing
    lastKPI       KPIData
    lastPipelines []PipelineData
    lastErrors    []ErrorData
    lastTokenSeq  int64  // max phase_events.id seen
    lastVerifySeq int64  // max verify_results.id seen
}
```

**Lifecycle:**

1. `NewHub(metrics, changesDir)` — creates hub
2. `hub.Run(ctx)` — starts a 1s ticker goroutine that:
   a. Queries DB for KPIs, pipelines, errors, new chart rows
   b. Scans filesystem for state.json changes (heatmap)
   c. Diffs against last-known state
   d. Broadcasts only changed/new data as typed JSON messages
   e. Updates last-known state
3. `hub.HandleWS(w, r)` — upgrades connection, sends full initial snapshot
   (KPIs + pipelines + errors + chart history from DB), adds client to map
4. `broadcast()` iterates clients under lock, writes to each, removes dead
   connections

**Diff strategy:**
- KPIs: deep-equal struct comparison; push full KPI message on any change
- Pipelines: deep-equal slice comparison; push full pipelines on any change
- Errors: deep-equal slice comparison; push full errors on any change
- Chart data: sequence-based — query `WHERE id > lastSeenId`, push new rows
  only. Lightweight and incremental.
- Heatmap: deep-equal grid comparison; push full grid on any change

**Message format:**

```json
{"type": "kpi", "data": {"activeChanges": 4, "totalTokens": 12000, "cacheHitPct": 72, "errorCount": 3}}
{"type": "pipelines", "data": [{"name": "foo", "currentPhase": "spec", "completed": 3, "total": 10, "tokens": 500, "progressPct": 30, "status": "ok"}]}
{"type": "errors", "data": [{"timestamp": "...", "commandName": "test", "exitCode": 1, "change": "foo", "fingerprint": "ab12cd34", "firstLine": "..."}]}
{"type": "chart:tokens", "data": [{"timestamp": "...", "change": "...", "phase": "...", "tokens": 500, "cached": false}]}
{"type": "chart:durations", "data": [{"phase": "spec", "avgDurationMs": 1200}]}
{"type": "chart:cache", "data": [{"timestamp": "...", "phase": "...", "cached": true}]}
{"type": "chart:verify", "data": [{"timestamp": "...", "change": "...", "commandName": "test", "exitCode": 0, "passed": true}]}
{"type": "chart:heatmap", "data": [{"change": "foo", "phase": "spec", "status": "completed"}]}
```

Chart data messages are arrays (batched new rows), not single objects.

### Verify Results Table

Currently `cmd_verify.go` only records failures in `verify_events`. The
verify chart needs both pass and fail data. Add:

- **New table `verify_results`** in `store.go` migration:
  ```sql
  CREATE TABLE IF NOT EXISTS verify_results (
      id           INTEGER PRIMARY KEY AUTOINCREMENT,
      timestamp    TEXT    NOT NULL,
      change       TEXT    NOT NULL,
      command_name TEXT    NOT NULL,
      exit_code    INTEGER NOT NULL,
      passed       INTEGER NOT NULL
  )
  ```
- **`cmd_verify.go`**: after every verify run (both pass and fail, including
  the smart-skip path), open the store and insert one row per command result
  into `verify_results`. This is direct DB insertion, not event-driven — same
  pattern as the existing store usage in `cmd_verify.go`.
- **Existing `verify_events`** table and `VerifyFailed` event remain
  unchanged — they continue to record detailed failure info (error_lines,
  fingerprint) for the error table.
- **`VerifyHistory`** query reads from `verify_results`, not `verify_events`.

### Store — New Query Methods (`internal/store/store.go`)

New methods on `Store` (added to `MetricsReader` interface alongside the
existing 3 methods which remain unchanged):

```go
TokenHistory(ctx context.Context, since time.Time) ([]TokenHistoryRow, error)
PhaseDurations(ctx context.Context) ([]PhaseDurationRow, error)
CacheHistory(ctx context.Context, since time.Time) ([]CacheHistoryRow, error)
VerifyHistory(ctx context.Context, since time.Time) ([]VerifyHistoryRow, error)
```

**Query sources:**
- `TokenHistory` → `SELECT timestamp, change, phase, tokens, cached FROM phase_events WHERE timestamp > ? ORDER BY id`
- `PhaseDurations` → `SELECT phase, AVG(duration_ms) FROM phase_events GROUP BY phase`
- `CacheHistory` → `SELECT timestamp, phase, cached FROM phase_events WHERE timestamp > ? ORDER BY id`
- `VerifyHistory` → `SELECT timestamp, change, command_name, exit_code, passed FROM verify_results WHERE timestamp > ? ORDER BY id`

Row types:

- `TokenHistoryRow`: Timestamp, Change, Phase, Tokens, Cached
- `PhaseDurationRow`: Phase, AvgDurationMs
- `CacheHistoryRow`: Timestamp, Phase, Cached
- `VerifyHistoryRow`: Timestamp, Change, CommandName, ExitCode, Passed

The `since` parameter bounds queries to keep initial payloads small (default:
24 hours).

`PhaseStatusGrid` (heatmap data) is derived from filesystem `state.json`
files, not DB. Lives as a standalone function in `hub.go`.

### Client-Side (`static/dashboard.js`, ~300-400 LOC)

Three responsibilities:

**1. WS connection manager**
- Connects to `ws://host:port/ws`
- Dispatches by `type` field to appropriate handler
- Auto-reconnect: exponential backoff (1s → 2s → 4s, cap 10s)
- Connection indicator in header: green=connected, yellow=reconnecting,
  red=disconnected

**2. DOM updaters**
- `handleKPI(data)` — updates 4 card values via `textContent`
- `handlePipelines(data)` — rebuilds pipeline table tbody
- `handleErrors(data)` — rebuilds error table tbody

**3. ECharts managers** — one per chart, lazy-initialized on first tab select:
- `TokenChart` — line chart, incremental `appendData`
- `DurationChart` — horizontal bar, full replace on update
- `CacheChart` — stacked area (hit vs miss), incremental
- `VerifyChart` — scatter (x=time, y=command, color=pass/fail), incremental
- `HeatmapChart` — grid (x=phase, y=change, color=status), full replace

### Tab Behavior

5 `<div>` containers, one visible at a time. Tab click shows target, calls
`chart.resize()`. Charts initialized lazily on first view.

### Layout (base.html)

```
┌──────────────────────────────────────────┐
│ SHENRON SDD                    ● LIVE    │
├──────────────────────────────────────────┤
│ [Active: 4] [Tokens: 12k] [Cache: 72%] [Errors: 3] │  ← KPI cards
├──────────────────────────────────────────┤
│ TOKENS | DURATIONS | CACHE | VERIFY | HEATMAP │  ← tab bar
├──────────────────────────────────────────┤
│                                          │
│            [active chart]                │  ← one chart at a time
│                                          │
├──────────────────────────────────────────┤
│ Pipeline table                           │
├──────────────────────────────────────────┤
│ Error table                              │
└──────────────────────────────────────────┘
```

Dark theme preserved: `#1a1a2e` bg, `#16213e` cards, cyan/green/purple/red
accents. ECharts theme config matches.

## File Changes

| File | Action | Scope |
|------|--------|-------|
| `internal/dashboard/hub.go` | **New** | ~250 LOC. Hub, poll loop, WS handler, broadcast, initial snapshot, diff logic. |
| `internal/dashboard/server.go` | **Modify** | Add hub field. `New()` signature stays `New(m MetricsReader, changesDir string)` — hub is created internally. Add `/ws` route, remove 3 fragment handlers. Move KPI/pipeline/error computation logic into hub snapshot builder. |
| `internal/dashboard/templates/base.html` | **Rewrite** | Dark theme preserved. HTMX divs → chart containers + tab bar. Load echarts + dashboard.js. |
| `internal/dashboard/templates/kpi.html` | **Delete** | Rendered client-side now. |
| `internal/dashboard/templates/pipelines.html` | **Delete** | Rendered client-side now. |
| `internal/dashboard/templates/errors.html` | **Delete** | Rendered client-side now. |
| `internal/dashboard/static/htmx.min.js` | **Delete** | Replaced by WS. |
| `internal/dashboard/static/echarts.min.js` | **New** | ~200KB tree-shaken (line, bar, scatter, heatmap, area). Vendored and embedded via `//go:embed` — no CDN, works offline. |
| `internal/dashboard/static/dashboard.js` | **New** | ~300-400 LOC. WS client, DOM updaters, ECharts managers, tab logic. |
| `internal/store/store.go` | **Modify** | Add `verify_results` table migration, 4 new query methods + row types, `InsertVerifyResult` method. |
| `internal/cli/cmd_verify.go` | **Modify** | Insert rows into `verify_results` after every verify run (pass and fail, including smart-skip path). Direct DB write, not event-driven. |
| `internal/cli/cmd_dashboard.go` | **Modify** | Start hub poll loop via `hub.Run(ctx)` after creating the server. |
| `go.mod` | **Modify** | Add `github.com/coder/websocket`. |

**No changes to:** `events/broker.go`, `store/subscribers.go`, `state/`,
`phase/`, `context/`, other `cli/cmd_*.go` files.

## New Dependency

`github.com/coder/websocket` v1.8.14 — pure Go, zero transitive deps, no CGO.
Actively maintained by Coder (formerly nhooyr.io/websocket).

## Testing

**`hub_test.go`** (new, ~200 LOC):
- WS upgrade + initial snapshot delivery (assert all message types present)
- Broadcast fan-out (2 clients, insert DB row, both receive delta after tick)
- Dead client cleanup (connect, close, poll tick, no panic)
- Diff logic: insert DB row, verify only delta is pushed (not full re-send)
- Mock MetricsReader for deterministic data

**`server_test.go`** (update existing):
- Remove fragment endpoint tests (`/fragments/kpi`, etc.)
- Keep index test, add `/ws` upgrade test
- Update `New()` calls to match new signature

**`store_test.go`** (extend):
- Test 4 new query methods with known data
- Test `since` parameter filtering
- Test empty DB → empty slices
- Test `InsertVerifyResult` + `VerifyHistory` round-trip
- Test `verify_results` table migration

No JS unit tests — verified through Go WS integration tests + manual browser
QA.

## Migration Path

This is a breaking change to the dashboard UI but not to any CLI command
interface. The `sdd dashboard` command keeps the same flags and behavior.
Users see the upgraded UI on next page load.
