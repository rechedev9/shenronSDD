package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
)

// fakeMetrics implements MetricsReader for testing.
type fakeMetrics struct {
	stats  *store.TokenStats
	tokens []store.ChangeTokens
	errors []store.ErrorRow
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

func newTestServer(t *testing.T, m MetricsReader) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	changesDir := filepath.Join(dir, "changes")
	os.MkdirAll(changesDir, 0o755)
	return New(m, changesDir), changesDir
}

func TestHandleIndex(t *testing.T) {
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
	if !strings.Contains(body, "htmx.min.js") {
		t.Error("expected body to contain htmx.min.js script reference")
	}
}

func TestHandleKPI(t *testing.T) {
	fm := &fakeMetrics{
		stats: &store.TokenStats{
			TotalTokens: 42000,
			CacheHitPct: 73.5,
			ErrorCount:  3,
		},
	}
	srv, _ := newTestServer(t, fm)
	req := httptest.NewRequest("GET", "/fragments/kpi", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "42000") {
		t.Error("expected body to contain token count 42000")
	}
	if !strings.Contains(body, "74%") {
		t.Error("expected body to contain cache hit pct 74%")
	}
	if !strings.Contains(body, "3") {
		t.Error("expected body to contain error count")
	}
}

func TestHandleKPI_Empty(t *testing.T) {
	srv, _ := newTestServer(t, &fakeMetrics{})
	req := httptest.NewRequest("GET", "/fragments/kpi", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "0") {
		t.Error("expected zero values in empty KPI")
	}
}

func TestHandlePipelines_Empty(t *testing.T) {
	srv, _ := newTestServer(t, &fakeMetrics{})
	req := httptest.NewRequest("GET", "/fragments/pipelines", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No active changes") {
		t.Error("expected empty state message")
	}
}

func TestHandleErrors(t *testing.T) {
	fm := &fakeMetrics{
		errors: []store.ErrorRow{
			{
				Timestamp:   "2026-03-21T10:00:00Z",
				CommandName: "test",
				ExitCode:    1,
				Change:      "my-change",
				Fingerprint: "abc12345678",
				FirstLine:   "FAIL pkg/foo",
			},
		},
	}
	srv, _ := newTestServer(t, fm)
	req := httptest.NewRequest("GET", "/fragments/errors", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "my-change") {
		t.Error("expected body to contain change name")
	}
	if !strings.Contains(body, "FAIL pkg/foo") {
		t.Error("expected body to contain error first line")
	}
	if !strings.Contains(body, "abc12345") {
		t.Error("expected body to contain fingerprint prefix")
	}
}

func TestHandleErrors_Empty(t *testing.T) {
	srv, _ := newTestServer(t, &fakeMetrics{})
	req := httptest.NewRequest("GET", "/fragments/errors", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "No errors recorded") {
		t.Error("expected empty state message")
	}
}

func TestStaticHtmx(t *testing.T) {
	srv, _ := newTestServer(t, &fakeMetrics{})
	req := httptest.NewRequest("GET", "/static/htmx.min.js", nil)
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty htmx.min.js response")
	}
}
