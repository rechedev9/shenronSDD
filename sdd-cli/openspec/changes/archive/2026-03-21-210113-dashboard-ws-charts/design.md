# Design: Dashboard WebSocket + ECharts

See detailed design: docs/superpowers/specs/2026-03-21-dashboard-ws-charts-design.md

## Architecture
- DB-poll hub (hub.go) polls SQLite + filesystem every 1s
- WebSocket broadcast to connected clients with diff-based deltas
- ECharts (vendored, embedded) for 5 chart types
- Tabbed layout, lazy chart initialization

## Key Decisions
- coder/websocket (pure Go, no CGO)
- verify_results table (separate from verify_events)
- No broker-driven push (CLI commands are separate processes)
