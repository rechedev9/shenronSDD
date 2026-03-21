package dashboard

import (
	"context"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"time"

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
	TokenHistory(ctx context.Context, since time.Time) ([]store.TokenHistoryRow, error)
	PhaseDurations(ctx context.Context) ([]store.PhaseDurationRow, error)
	CacheHistory(ctx context.Context, since time.Time) ([]store.CacheHistoryRow, error)
	VerifyHistory(ctx context.Context, since time.Time) ([]store.VerifyHistoryRow, error)
}

// KPIData holds the data for the KPI cards.
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
	hub        *Hub
	templates  *template.Template
	httpServer *http.Server
}

// New creates a dashboard server with a WebSocket hub.
func New(m MetricsReader, changesDir string) *Server {
	tmpl := template.Must(template.ParseFS(templateFS, "templates/*.html"))
	hub := NewHub(m, changesDir)
	return &Server{
		hub:       hub,
		templates: tmpl,
	}
}

// Hub returns the server's WebSocket hub for starting the poll loop.
func (s *Server) Hub() *Hub {
	return s.hub
}

// ListenAndServe blocks until ctx is cancelled, then gracefully shuts down.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	s.httpServer = &http.Server{Addr: addr, Handler: s.routes()}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx) // best-effort graceful shutdown
	}()
	err := s.httpServer.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Static files (echarts.min.js, dashboard.js).
	staticSub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Full page.
	mux.HandleFunc("/", s.handleIndex)

	// WebSocket endpoint.
	mux.HandleFunc("/ws", s.hub.HandleWS)

	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = s.templates.ExecuteTemplate(w, "base.html", nil) // best-effort template render
}
