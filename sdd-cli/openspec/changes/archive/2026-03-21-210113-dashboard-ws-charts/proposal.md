# Proposal: Dashboard WebSocket + ECharts Upgrade

**Change Name**: dashboard-ws-charts
**Date**: 2026-03-21
**Status**: Proposed

## Intent

Replace the HTMX polling-based dashboard with a WebSocket push architecture and add 5 ECharts visualizations for pipeline observability. The current 3-second HTMX poll cycle creates unnecessary HTTP traffic and cannot efficiently deliver incremental chart data; a DB-poll hub pushing deltas over WebSocket reduces latency to ~1s, simplifies the transport model to a single channel, and enables rich time-series and heatmap visualizations that were impractical with server-rendered HTML fragments.

## Scope

### In Scope

- New WebSocket hub (`internal/dashboard/hub.go`) with 1s DB-poll loop, diff-based delta push, client lifecycle management, and initial snapshot delivery
- 5 ECharts visualizations behind a tab bar: tokens over time (line), phase durations (horizontal bar), cache hit/miss ratio (stacked area), verify timeline (scatter), phase heatmap (grid)
- New `verify_results` SQLite table to capture both pass and fail verify outcomes
- 4 new store query methods: `TokenHistory`, `PhaseDurations`, `CacheHistory`, `VerifyHistory`
- `InsertVerifyResult` calls on all 3 verify paths (smart-skip, pass, fail) in `cmd_verify.go`
- Client-side JS (`dashboard.js`) for WS connection management, DOM updates, and ECharts initialization
- Vendored tree-shaken `echarts.min.js` (~200 KB) embedded via `//go:embed`
- Full rewrite of `base.html` with dark theme preserved, tab bar layout, connection indicator
- Deletion of HTMX infrastructure: `htmx.min.js`, `kpi.html`, `pipelines.html`, `errors.html`, and 3 fragment handler routes
- New Go dependency: `github.com/coder/websocket` v1.8.14
- Tests: new `hub_test.go`, updated `server_test.go` and `store_test.go`

### Out of Scope

- Changes to `events/broker.go` or cross-process event sharing
- Changes to `state/`, `phase/`, `context/`, `artifacts/`, `config/`, or any non-dashboard CLI commands
- JS unit testing framework (verified through Go WS integration tests + manual browser QA)
- Authentication or access control on the WebSocket endpoint
- Multi-server or distributed deployment considerations
- CDN loading of ECharts (vendored and embedded for offline use)

## Approach

**DB-poll hub with WS push.** A `Hub` goroutine polls SQLite and the filesystem on a 1-second interval, diffs against last-known state (struct deep-equal for KPIs/pipelines/errors, sequence-based `WHERE id > lastSeq` for chart rows, deep-equal grid for heatmap), and broadcasts only changed data as typed JSON messages over open WebSocket connections. On connect, clients receive a full snapshot. Dead connections are removed on write error under the same lock that guards the client map.

**ECharts embedded behind tabs.** Five chart containers live in `base.html`, one visible at a time. Charts are lazy-initialized on first tab select and call `resize()` on show. The vendored `echarts.min.js` is tree-shaken to line/bar/scatter/heatmap/area types.

**Verify results pipeline.** A new `verify_results` table records one row per command per verify run (pass and fail, including smart-skip). This is direct DB insertion in `cmd_verify.go`, not event-driven. The existing `verify_events` table and `VerifyFailed` event remain unchanged for the error log.

## Files Affected

| File | Action | Scope |
|------|--------|-------|
| `internal/dashboard/hub.go` | New | ~250 LOC. Hub struct, poll loop, WS handler, broadcast, diff, snapshot. |
| `internal/dashboard/hub_test.go` | New | ~200 LOC. WS upgrade, fan-out, dead client cleanup, diff logic tests. |
| `internal/dashboard/server.go` | Modify | Add hub field, `/ws` route. Remove 3 fragment handlers. Extend `MetricsReader` with 4 new methods. |
| `internal/dashboard/server_test.go` | Modify | Remove fragment tests, add `/ws` upgrade test, update `fakeMetrics` for new interface. |
| `internal/dashboard/templates/base.html` | Rewrite | Dark theme preserved. HTMX divs replaced with chart containers, tab bar. Load echarts + dashboard.js. |
| `internal/dashboard/templates/kpi.html` | Delete | Rendered client-side. |
| `internal/dashboard/templates/pipelines.html` | Delete | Rendered client-side. |
| `internal/dashboard/templates/errors.html` | Delete | Rendered client-side. |
| `internal/dashboard/static/htmx.min.js` | Delete | Replaced by WS. |
| `internal/dashboard/static/echarts.min.js` | New | ~200 KB vendored tree-shaken ECharts bundle. |
| `internal/dashboard/static/dashboard.js` | New | ~300-400 LOC. WS client, DOM updaters, ECharts managers, tab logic. |
| `internal/store/store.go` | Modify | Add `verify_results` table migration, `InsertVerifyResult`, 4 new query methods + row types. |
| `internal/store/store_test.go` | Modify | Tests for new query methods, `InsertVerifyResult` round-trip, migration. |
| `internal/cli/cmd_verify.go` | Modify | Insert `verify_results` rows on all 3 paths (smart-skip, pass, fail). |
| `internal/cli/cmd_dashboard.go` | Modify | Start `hub.Run(ctx)` goroutine before `ListenAndServe`. |
| `go.mod` / `go.sum` | Modify | Add `github.com/coder/websocket` v1.8.14. |

## Dependencies

| Dependency | Version | Rationale |
|------------|---------|-----------|
| `github.com/coder/websocket` | v1.8.14 | Pure Go, zero transitive deps, no CGO. Actively maintained successor to nhooyr.io/websocket. |
| Apache ECharts (vendored JS) | ~5.5 | Tree-shaken to ~200 KB. Line, bar, scatter, heatmap, area chart types. Embedded in binary via `//go:embed`. |

## Risk Assessment

| Risk | Level | Mitigation |
|------|-------|------------|
| Concurrent write to dead WS connections | Medium | Remove dead clients under same lock as client map; `coder/websocket` errors are deterministic and synchronous. |
| Smart-skip path in cmd_verify.go gains DB dependency | Medium | Use existing `tryOpenStore` best-effort pattern; inserts are skipped if store unavailable. |
| `server_test.go` fakeMetrics interface drift | Low | All test files updated atomically with `MetricsReader` extension. |
| ECharts bundle size (~200 KB added to binary) | Low | Acceptable for local dev tool. Verify post-build. |
| SQLite WAL concurrent readers | Low | Already in WAL mode with `busy_timeout=5000`. Hub queries are read-only and short. |
| Schema migration on existing DBs | Low | `CREATE TABLE IF NOT EXISTS` is idempotent. No data loss. |
| ECharts tab resize | Low | `chart.resize()` called on tab show. Standard ECharts pattern. |

## Rollback Plan

1. Revert the commit(s) introducing the change.
2. Run `go mod tidy` to remove `github.com/coder/websocket`.
3. The `verify_results` table persists harmlessly in existing SQLite DBs (no migration needed to remove it; it is simply unused).
4. `make check` confirms clean revert.

The dashboard reverts to HTMX polling with no data loss. The `verify_results` table is additive and does not affect existing tables or queries.

## Success Criteria

- `make check` passes (fmt + lint + test) with all new and updated tests green.
- `sdd dashboard` serves the upgraded UI on `:8811` with WebSocket connection indicator showing green.
- All 5 charts render with data from `phase_events` and `verify_results` tables.
- Tab switching shows/hides charts correctly with proper `resize()` behavior.
- Running `sdd verify` (pass, fail, smart-skip) inserts rows into `verify_results` visible in the verify chart within ~1s.
- Running `sdd write` / `sdd context` triggers token/cache chart updates within ~1s.
- Binary size increase is under 300 KB (ECharts bundle + WS library).
- No changes to any package outside the listed files; `events/`, `state/`, `phase/`, `context/`, `artifacts/` remain untouched.
