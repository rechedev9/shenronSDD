# Spec: Dashboard WebSocket + ECharts

See detailed spec: docs/superpowers/specs/2026-03-21-dashboard-ws-charts-design.md

## REQ-1: WebSocket Transport
- MUST replace HTMX polling with WebSocket push
- MUST use DB-poll hub (1s interval) with diff-based delta push
- MUST auto-reconnect with exponential backoff

## REQ-2: ECharts Visualizations
- MUST render 5 charts: token usage, phase durations, cache ratio, verify timeline, phase heatmap
- MUST use tabbed layout below KPI cards
- MUST lazy-initialize charts on first tab select

## REQ-3: Verify Results
- MUST record all verify results (pass and fail) in verify_results table
- MUST include smart-skip path results

## REQ-4: Backward Compatibility
- MUST preserve sdd dashboard command interface unchanged
- MUST preserve dark theme styling
