package dashboard

import (
	"context"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// MetricsReader is the consumer-defined interface for reading telemetry data.
type MetricsReader interface {
	TokenSummary(ctx context.Context) (*store.TokenStats, error)
	PhaseTokensByChange(ctx context.Context) ([]store.ChangeTokens, error)
	RecentErrors(ctx context.Context, limit int) ([]store.ErrorRow, error)
}

// KPIData holds the data for the KPI cards fragment.
type KPIData struct {
	ActiveChanges int
	TotalTokens   int
	CacheHitPct   float64
	ErrorCount    int
}

// PipelineData holds the data for a single pipeline row.
type PipelineData struct {
	Name         string
	CurrentPhase string
	Completed    int
	Total        int
	Tokens       int
	ProgressPct  int
	Status       string // "ok", "warn", "error"
}

// ErrorData holds the data for a single error row.
type ErrorData struct {
	Timestamp   string
	CommandName string
	ExitCode    int
	Change      string
	Fingerprint string
	FirstLine   string
}

// Server serves the dashboard UI over HTTP.
type Server struct {
	metrics    MetricsReader
	changesDir string
	templates  *template.Template
	httpServer *http.Server
}

// New creates a dashboard server.
func New(m MetricsReader, changesDir string) *Server {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	return &Server{
		metrics:    m,
		changesDir: changesDir,
		templates:  tmpl,
	}
}

// ListenAndServe blocks until ctx is cancelled, then gracefully shuts down.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	s.httpServer = &http.Server{Addr: addr, Handler: s.routes()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		s.httpServer.Shutdown(shutdownCtx)
	}()
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Static files (htmx.min.js).
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Full page.
	mux.HandleFunc("/", s.handleIndex)

	// htmx fragments.
	mux.HandleFunc("/fragments/kpi", s.handleKPI)
	mux.HandleFunc("/fragments/pipelines", s.handlePipelines)
	mux.HandleFunc("/fragments/errors", s.handleErrors)

	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.templates.ExecuteTemplate(w, "base.html", nil)
}

func (s *Server) handleKPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := KPIData{}

	// Count active changes by scanning changesDir for dirs with state.json.
	if entries, err := os.ReadDir(s.changesDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || e.Name() == "archive" {
				continue
			}
			statePath := filepath.Join(s.changesDir, e.Name(), "state.json")
			if _, err := os.Stat(statePath); err == nil {
				data.ActiveChanges++
			}
		}
	}

	// Read token summary from store.
	if stats, err := s.metrics.TokenSummary(r.Context()); err == nil {
		data.TotalTokens = stats.TotalTokens
		data.CacheHitPct = stats.CacheHitPct
		data.ErrorCount = stats.ErrorCount
	}

	s.templates.ExecuteTemplate(w, "kpi.html", data)
}

func (s *Server) handlePipelines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Build token lookup from store.
	tokenMap := make(map[string]int)
	if ct, err := s.metrics.PhaseTokensByChange(r.Context()); err == nil {
		for _, c := range ct {
			tokenMap[c.Change] = c.Tokens
		}
	}

	allPhases := state.AllPhases()
	total := len(allPhases)

	var pipelines []PipelineData

	entries, err := os.ReadDir(s.changesDir)
	if err != nil {
		// Render empty state on error.
		s.templates.ExecuteTemplate(w, "pipelines.html", pipelines)
		return
	}

	for _, e := range entries {
		if !e.IsDir() || e.Name() == "archive" {
			continue
		}
		changeDir := filepath.Join(s.changesDir, e.Name())
		statePath := filepath.Join(changeDir, "state.json")
		st, err := state.Load(statePath)
		if err != nil {
			continue
		}

		completed := 0
		for _, p := range allPhases {
			if st.Phases[p] == state.StatusCompleted {
				completed++
			}
		}

		pct := 0
		if total > 0 {
			pct = completed * 100 / total
		}

		status := "ok"
		// Check if verify-report.md contains FAILED.
		reportPath := filepath.Join(changeDir, "verify-report.md")
		if data, err := os.ReadFile(reportPath); err == nil {
			if strings.Contains(string(data), "FAILED") {
				status = "error"
			}
		}
		// Check staleness.
		if status == "ok" && st.IsStale(24*time.Hour) {
			status = "warn"
		}

		pipelines = append(pipelines, PipelineData{
			Name:         st.Name,
			CurrentPhase: string(st.CurrentPhase),
			Completed:    completed,
			Total:        total,
			Tokens:       tokenMap[st.Name],
			ProgressPct:  pct,
			Status:       status,
		})
	}

	s.templates.ExecuteTemplate(w, "pipelines.html", pipelines)
}

func (s *Server) handleErrors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	rows, err := s.metrics.RecentErrors(r.Context(), 20)
	if err != nil {
		// Render empty state on error.
		s.templates.ExecuteTemplate(w, "errors.html", []ErrorData(nil))
		return
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

	s.templates.ExecuteTemplate(w, "errors.html", data)
}
