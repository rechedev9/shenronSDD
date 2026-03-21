package dashboard

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
)

const defaultLookback = 24 * time.Hour

// wsMessage is the JSON envelope sent over WebSocket.
type wsMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// PhaseStatusRow holds one cell of the pipeline heatmap.
type PhaseStatusRow struct {
	Change string `json:"change"`
	Phase  string `json:"phase"`
	Status string `json:"status"`
}

// changeSnapshot holds the loaded state for a single active change directory.
type changeSnapshot struct {
	dir   string
	state *state.State
}

// Hub manages WebSocket clients and pushes data deltas.
type Hub struct {
	metrics    MetricsReader
	changesDir string

	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}

	// Last-known state for diffing.
	lastKPI        KPIData
	lastPipelines  []PipelineData
	lastErrors     []ErrorData
	lastHeatmap    []PhaseStatusRow
	lastTokenTS    string // max timestamp seen in phase_events
	lastVerifyTS   string // max timestamp seen in verify_results
	lastDurations  []store.PhaseDurationRow
	lastCacheTS    string // tracks cache history watermark independently
}

// NewHub creates a hub. Call Run() to start the poll loop.
func NewHub(m MetricsReader, changesDir string) *Hub {
	return &Hub{
		metrics:    m,
		changesDir: changesDir,
		clients:    make(map[*websocket.Conn]struct{}),
	}
}

// Run starts the 1-second poll loop. Blocks until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.poll(ctx)
		}
	}
}

// HandleWS upgrades an HTTP connection to WebSocket and sends the initial snapshot.
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // local dev tool, accept any origin
	})
	if err != nil {
		slog.Error("ws accept", "error", err)
		return
	}

	ctx := r.Context()

	h.sendSnapshot(ctx, conn)

	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()

	// Keep connection alive — read loop discards incoming messages.
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			break
		}
	}

	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
	_ = conn.Close(websocket.StatusNormalClosure, "") // best-effort close
}

// poll queries DB + filesystem, diffs against last state, broadcasts deltas.
func (h *Hub) poll(ctx context.Context) {
	h.mu.Lock()
	if len(h.clients) == 0 {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	// Single filesystem walk — shared by KPI, pipelines, and heatmap.
	changes := h.loadChanges()

	// KPIs.
	kpi := h.buildKPIFromChanges(ctx, changes)
	if kpi != h.lastKPI {
		h.broadcast(ctx, wsMessage{Type: "kpi", Data: kpi})
		h.lastKPI = kpi
	}

	// Pipelines.
	pipelines := h.buildPipelinesFromChanges(ctx, changes)
	if !reflect.DeepEqual(pipelines, h.lastPipelines) {
		h.broadcast(ctx, wsMessage{Type: "pipelines", Data: pipelines})
		h.lastPipelines = pipelines
	}

	// Errors.
	errors := h.buildErrors(ctx)
	if !reflect.DeepEqual(errors, h.lastErrors) {
		h.broadcast(ctx, wsMessage{Type: "errors", Data: errors})
		h.lastErrors = errors
	}

	// Heatmap.
	heatmap := buildHeatmapFromChanges(changes)
	if !reflect.DeepEqual(heatmap, h.lastHeatmap) {
		h.broadcast(ctx, wsMessage{Type: "chart:heatmap", Data: heatmap})
		h.lastHeatmap = heatmap
	}

	// Chart data — incremental by timestamp.
	tokenSince := h.parseSinceTS(h.lastTokenTS)

	if rows, err := h.metrics.TokenHistory(ctx, tokenSince); err == nil && len(rows) > 0 {
		h.broadcast(ctx, wsMessage{Type: "chart:tokens", Data: rows})
		h.lastTokenTS = rows[len(rows)-1].Timestamp
	}

	if rows, err := h.metrics.PhaseDurations(ctx); err == nil && len(rows) > 0 {
		if !reflect.DeepEqual(rows, h.lastDurations) {
			h.broadcast(ctx, wsMessage{Type: "chart:durations", Data: rows})
			h.lastDurations = rows
		}
	}

	cacheSince := h.parseSinceTS(h.lastCacheTS)
	if rows, err := h.metrics.CacheHistory(ctx, cacheSince); err == nil && len(rows) > 0 {
		h.broadcast(ctx, wsMessage{Type: "chart:cache", Data: rows})
		h.lastCacheTS = rows[len(rows)-1].Timestamp
	}

	verifySince := h.parseSinceTS(h.lastVerifyTS)
	if rows, err := h.metrics.VerifyHistory(ctx, verifySince); err == nil && len(rows) > 0 {
		h.broadcast(ctx, wsMessage{Type: "chart:verify", Data: rows})
		h.lastVerifyTS = rows[len(rows)-1].Timestamp
	}
}

// parseSinceTS converts a stored timestamp to time.Time, falling back to defaultLookback.
func (h *Hub) parseSinceTS(ts string) time.Time {
	if ts == "" {
		return time.Now().Add(-defaultLookback)
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Now().Add(-defaultLookback)
	}
	return t
}

// sendSnapshot sends the full current state to a single client.
func (h *Hub) sendSnapshot(ctx context.Context, conn *websocket.Conn) {
	since := time.Now().Add(-defaultLookback)
	changes := h.loadChanges()

	msgs := []wsMessage{
		{Type: "kpi", Data: h.buildKPIFromChanges(ctx, changes)},
		{Type: "pipelines", Data: h.buildPipelinesFromChanges(ctx, changes)},
		{Type: "errors", Data: h.buildErrors(ctx)},
		{Type: "chart:heatmap", Data: buildHeatmapFromChanges(changes)},
	}

	if rows, err := h.metrics.TokenHistory(ctx, since); err == nil {
		msgs = append(msgs, wsMessage{Type: "chart:tokens", Data: rows})
	}
	if rows, err := h.metrics.PhaseDurations(ctx); err == nil {
		msgs = append(msgs, wsMessage{Type: "chart:durations", Data: rows})
	}
	if rows, err := h.metrics.CacheHistory(ctx, since); err == nil {
		msgs = append(msgs, wsMessage{Type: "chart:cache", Data: rows})
	}
	if rows, err := h.metrics.VerifyHistory(ctx, since); err == nil {
		msgs = append(msgs, wsMessage{Type: "chart:verify", Data: rows})
	}

	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			return
		}
	}
}

// broadcast sends a message to all connected clients, removing dead ones.
func (h *Hub) broadcast(ctx context.Context, msg wsMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.clients {
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			_ = conn.Close(websocket.StatusGoingAway, "") // best-effort close dead client
			delete(h.clients, conn)
		}
	}
}

// loadChanges walks changesDir once and loads state.json for each active change.
func (h *Hub) loadChanges() []changeSnapshot {
	entries, err := os.ReadDir(h.changesDir)
	if err != nil {
		return nil
	}

	var changes []changeSnapshot
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "archive" {
			continue
		}
		changeDir := filepath.Join(h.changesDir, e.Name())
		statePath := filepath.Join(changeDir, "state.json")
		st, err := state.Load(statePath)
		if err != nil {
			continue
		}
		changes = append(changes, changeSnapshot{dir: changeDir, state: st})
	}
	return changes
}

// buildKPIFromChanges computes KPI data using pre-loaded changes.
func (h *Hub) buildKPIFromChanges(ctx context.Context, changes []changeSnapshot) KPIData {
	data := KPIData{ActiveChanges: len(changes)}

	if stats, err := h.metrics.TokenSummary(ctx); err == nil {
		data.TotalTokens = stats.TotalTokens
		data.CacheHitPct = stats.CacheHitPct
		data.ErrorCount = stats.ErrorCount
	}

	return data
}

// buildPipelinesFromChanges computes pipeline data using pre-loaded changes.
func (h *Hub) buildPipelinesFromChanges(ctx context.Context, changes []changeSnapshot) []PipelineData {
	tokenMap := make(map[string]int)
	if ct, err := h.metrics.PhaseTokensByChange(ctx); err == nil {
		for _, c := range ct {
			tokenMap[c.Change] = c.Tokens
		}
	}

	allPhases := state.AllPhases()
	total := len(allPhases)
	var pipelines []PipelineData

	for _, ch := range changes {
		completed := 0
		for _, p := range allPhases {
			if ch.state.Phases[p] == state.StatusCompleted {
				completed++
			}
		}

		pct := 0
		if total > 0 {
			pct = completed * 100 / total
		}

		status := "ok"
		reportPath := filepath.Join(ch.dir, "verify-report.md")
		if data, err := os.ReadFile(reportPath); err == nil {
			if strings.Contains(string(data), "**Status:** FAILED") {
				status = "error"
			}
		}
		if status == "ok" && ch.state.IsStale(defaultLookback) {
			status = "warn"
		}

		pipelines = append(pipelines, PipelineData{
			Name:         ch.state.Name,
			CurrentPhase: string(ch.state.CurrentPhase),
			Completed:    completed,
			Total:        total,
			Tokens:       tokenMap[ch.state.Name],
			ProgressPct:  pct,
			Status:       status,
		})
	}

	return pipelines
}

// buildErrors fetches recent errors from the store.
func (h *Hub) buildErrors(ctx context.Context) []ErrorData {
	rows, err := h.metrics.RecentErrors(ctx, 20)
	if err != nil {
		return nil
	}

	var data []ErrorData
	for _, r := range rows {
		fp := r.Fingerprint
		if len(fp) > 8 {
			fp = fp[:8]
		}
		ts := r.Timestamp
		if len(ts) > 19 {
			ts = ts[:19]
		}
		data = append(data, ErrorData{
			Timestamp:   ts,
			CommandName: r.CommandName,
			ExitCode:    r.ExitCode,
			Change:      r.Change,
			Fingerprint: fp,
			FirstLine:   r.FirstLine,
		})
	}

	return data
}

// buildHeatmapFromChanges builds the phase status grid from pre-loaded changes.
func buildHeatmapFromChanges(changes []changeSnapshot) []PhaseStatusRow {
	allPhases := state.AllPhases()
	var grid []PhaseStatusRow

	for _, ch := range changes {
		for _, p := range allPhases {
			status := string(ch.state.Phases[p])
			if status == "" {
				status = string(state.StatusPending)
			}
			grid = append(grid, PhaseStatusRow{
				Change: ch.state.Name,
				Phase:  string(p),
				Status: status,
			})
		}
	}

	return grid
}
