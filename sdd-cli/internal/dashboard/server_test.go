package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
)

// fakeMetrics implements MetricsReader for testing.
type fakeMetrics struct {
	stats         *store.TokenStats
	tokens        []store.ChangeTokens
	errors        []store.ErrorRow
	tokenHistory  []store.TokenHistoryRow
	durations     []store.PhaseDurationRow
	cacheHistory  []store.CacheHistoryRow
	verifyHistory []store.VerifyHistoryRow
}

func (f *fakeMetrics) TokenSummary(_ context.Context) (*store.TokenStats, error) {
	if f.stats == nil {
		return &store.TokenStats{}, nil
	}
	return f.stats, nil
}

func (f *fakeMetrics) PhaseTokensByChange(_ context.Context) ([]store.ChangeTokens, error) {
	return f.tokens, nil
}

func (f *fakeMetrics) RecentErrors(_ context.Context, _ int) ([]store.ErrorRow, error) {
	return f.errors, nil
}

func (f *fakeMetrics) TokenHistory(_ context.Context, _ time.Time) ([]store.TokenHistoryRow, error) {
	return f.tokenHistory, nil
}

func (f *fakeMetrics) PhaseDurations(_ context.Context) ([]store.PhaseDurationRow, error) {
	return f.durations, nil
}

func (f *fakeMetrics) CacheHistory(_ context.Context, _ time.Time) ([]store.CacheHistoryRow, error) {
	return f.cacheHistory, nil
}

func (f *fakeMetrics) VerifyHistory(_ context.Context, _ time.Time) ([]store.VerifyHistoryRow, error) {
	return f.verifyHistory, nil
}

func newTestServer(t *testing.T, m MetricsReader) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	changesDir := filepath.Join(dir, "changes")
	_ = os.MkdirAll(changesDir, 0o755)
	return New(m, changesDir), changesDir
}

func TestHandleIndex(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, &fakeMetrics{})
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "SHENRON") {
		t.Error("expected body to contain SHENRON")
	}
	if !strings.Contains(body, "echarts.min.js") {
		t.Error("expected body to contain echarts.min.js script reference")
	}
	if !strings.Contains(body, "dashboard.js") {
		t.Error("expected body to contain dashboard.js script reference")
	}
}

func TestHandleIndex_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, &fakeMetrics{})
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestStaticECharts(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, &fakeMetrics{})
	req := httptest.NewRequest("GET", "/static/echarts.min.js", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty echarts.min.js response")
	}
}

func TestStaticDashboardJS(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, &fakeMetrics{})
	req := httptest.NewRequest("GET", "/static/dashboard.js", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty dashboard.js response")
	}
}

func TestWSRouteExists(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, &fakeMetrics{})
	// Verify /ws route is registered (non-WS request gets a protocol error, not 404).
	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	// WebSocket upgrade will fail without proper headers, but it shouldn't be 404.
	if w.Code == http.StatusNotFound {
		t.Error("expected /ws route to be registered, got 404")
	}
}

func TestHubCreated(t *testing.T) {
	t.Parallel()
	srv, _ := newTestServer(t, &fakeMetrics{})
	if srv.Hub() == nil {
		t.Error("expected Hub() to return non-nil hub")
	}
}
