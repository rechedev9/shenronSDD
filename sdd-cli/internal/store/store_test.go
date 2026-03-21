package store

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir + "/test.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpen_CreatesSchema(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	// Query sqlite_master for our tables.
	rows, err := s.db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	tables := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	if !tables["phase_events"] {
		t.Error("missing table phase_events")
	}
	if !tables["verify_events"] {
		t.Error("missing table verify_events")
	}
}

func TestOpen_WALMode(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	var mode string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestInsertPhaseEvent_Roundtrip(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	events := []PhaseEvent{
		{Timestamp: time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC), Change: "feat-a", Phase: "explore", Bytes: 1024, Tokens: 256, Cached: true, DurationMs: 100},
		{Timestamp: time.Date(2026, 3, 21, 10, 1, 0, 0, time.UTC), Change: "feat-a", Phase: "propose", Bytes: 2048, Tokens: 512, Cached: false, DurationMs: 200},
		{Timestamp: time.Date(2026, 3, 21, 10, 2, 0, 0, time.UTC), Change: "feat-b", Phase: "design", Bytes: 512, Tokens: 128, Cached: false, DurationMs: 50},
	}
	for _, e := range events {
		if err := s.InsertPhaseEvent(ctx, e); err != nil {
			t.Fatalf("InsertPhaseEvent: %v", err)
		}
	}

	stats, err := s.TokenSummary(ctx)
	if err != nil {
		t.Fatalf("TokenSummary: %v", err)
	}
	if stats.TotalTokens != 896 {
		t.Errorf("TotalTokens = %d, want 896", stats.TotalTokens)
	}
	// 1 of 3 cached → 33.33%
	if math.Abs(stats.CacheHitPct-33.333333) > 0.01 {
		t.Errorf("CacheHitPct = %f, want ~33.33", stats.CacheHitPct)
	}
}

func TestTokenSummary_Empty(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	stats, err := s.TokenSummary(ctx)
	if err != nil {
		t.Fatalf("TokenSummary: %v", err)
	}
	if stats.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0", stats.TotalTokens)
	}
	if stats.CacheHitPct != 0 {
		t.Errorf("CacheHitPct = %f, want 0", stats.CacheHitPct)
	}
	if stats.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", stats.ErrorCount)
	}
}

func TestPhaseTokensByChange(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	events := []PhaseEvent{
		{Timestamp: time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC), Change: "feat-a", Phase: "explore", Bytes: 1024, Tokens: 256, Cached: false, DurationMs: 100},
		{Timestamp: time.Date(2026, 3, 21, 10, 1, 0, 0, time.UTC), Change: "feat-a", Phase: "propose", Bytes: 2048, Tokens: 512, Cached: false, DurationMs: 200},
		{Timestamp: time.Date(2026, 3, 21, 10, 2, 0, 0, time.UTC), Change: "feat-b", Phase: "design", Bytes: 512, Tokens: 128, Cached: false, DurationMs: 50},
	}
	for _, e := range events {
		if err := s.InsertPhaseEvent(ctx, e); err != nil {
			t.Fatalf("InsertPhaseEvent: %v", err)
		}
	}

	rows, err := s.PhaseTokensByChange(ctx)
	if err != nil {
		t.Fatalf("PhaseTokensByChange: %v", err)
	}

	m := make(map[string]int)
	for _, r := range rows {
		m[r.Change] = r.Tokens
	}
	if m["feat-a"] != 768 {
		t.Errorf("feat-a tokens = %d, want 768", m["feat-a"])
	}
	if m["feat-b"] != 128 {
		t.Errorf("feat-b tokens = %d, want 128", m["feat-b"])
	}
}

func TestInsertVerifyEvent_Roundtrip(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	ev := VerifyEvent{
		Timestamp:   time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
		Change:      "feat-a",
		CommandName: "build",
		Command:     "go build ./...",
		ExitCode:    1,
		ErrorLines:  []string{"main.go:10: undefined: foo", "main.go:20: too many args"},
		Fingerprint: "abc123",
	}
	if err := s.InsertVerifyEvent(ctx, ev); err != nil {
		t.Fatalf("InsertVerifyEvent: %v", err)
	}

	errs, err := s.RecentErrors(ctx, 10)
	if err != nil {
		t.Fatalf("RecentErrors: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("len(errs) = %d, want 1", len(errs))
	}
	if errs[0].Change != "feat-a" {
		t.Errorf("Change = %q, want %q", errs[0].Change, "feat-a")
	}
	if errs[0].FirstLine != "main.go:10: undefined: foo" {
		t.Errorf("FirstLine = %q, want %q", errs[0].FirstLine, "main.go:10: undefined: foo")
	}
	if errs[0].Fingerprint != "abc123" {
		t.Errorf("Fingerprint = %q, want %q", errs[0].Fingerprint, "abc123")
	}

	// Verify ErrorCount in TokenSummary reflects verify_events.
	stats, err := s.TokenSummary(ctx)
	if err != nil {
		t.Fatalf("TokenSummary: %v", err)
	}
	if stats.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", stats.ErrorCount)
	}
}

func TestRecentErrors_Empty(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	errs, err := s.RecentErrors(ctx, 10)
	if err != nil {
		t.Fatalf("RecentErrors: %v", err)
	}
	if errs == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(errs) != 0 {
		t.Errorf("len(errs) = %d, want 0", len(errs))
	}
}

func TestRecentErrors_Ordering(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	times := []time.Time{
		time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 21, 11, 0, 0, 0, time.UTC),
	}
	for i, ts := range times {
		ev := VerifyEvent{
			Timestamp:   ts,
			Change:      "feat-a",
			CommandName: "build",
			Command:     "go build ./...",
			ExitCode:    1,
			ErrorLines:  []string{ts.Format(time.RFC3339)},
			Fingerprint: "fp" + string(rune('0'+i)),
		}
		if err := s.InsertVerifyEvent(ctx, ev); err != nil {
			t.Fatalf("InsertVerifyEvent: %v", err)
		}
	}

	errs, err := s.RecentErrors(ctx, 10)
	if err != nil {
		t.Fatalf("RecentErrors: %v", err)
	}
	if len(errs) != 3 {
		t.Fatalf("len(errs) = %d, want 3", len(errs))
	}

	// Newest first: 12:00, 11:00, 10:00
	want := []string{
		"2026-03-21T12:00:00Z",
		"2026-03-21T11:00:00Z",
		"2026-03-21T10:00:00Z",
	}
	for i, w := range want {
		if errs[i].Timestamp != w {
			t.Errorf("errs[%d].Timestamp = %q, want %q", i, errs[i].Timestamp, w)
		}
	}
}

// TestInsertVerifyEvent_EmptyErrorLines verifies JSON encoding of empty/nil slices.
func TestInsertVerifyEvent_EmptyErrorLines(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	ev := VerifyEvent{
		Timestamp:   time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
		Change:      "feat-a",
		CommandName: "test",
		Command:     "go test ./...",
		ExitCode:    0,
		ErrorLines:  nil,
		Fingerprint: "",
	}
	if err := s.InsertVerifyEvent(ctx, ev); err != nil {
		t.Fatalf("InsertVerifyEvent: %v", err)
	}

	errs, err := s.RecentErrors(ctx, 10)
	if err != nil {
		t.Fatalf("RecentErrors: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("len = %d, want 1", len(errs))
	}
	if errs[0].FirstLine != "" {
		t.Errorf("FirstLine = %q, want empty", errs[0].FirstLine)
	}
}

// Verify JSON round-trip fidelity of error_lines.
func TestInsertVerifyEvent_JSONFidelity(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ctx := context.Background()

	lines := []string{"error: pkg/foo.go:1 syntax error", "note: did you mean?"}
	ev := VerifyEvent{
		Timestamp:   time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC),
		Change:      "feat-c",
		CommandName: "build",
		Command:     "go build",
		ExitCode:    1,
		ErrorLines:  lines,
		Fingerprint: "xyz",
	}
	if err := s.InsertVerifyEvent(ctx, ev); err != nil {
		t.Fatalf("InsertVerifyEvent: %v", err)
	}

	// Read raw JSON from DB.
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT error_lines FROM verify_events LIMIT 1`).Scan(&raw)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	var got []string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0] != lines[0] || got[1] != lines[1] {
		t.Errorf("error_lines = %v, want %v", got, lines)
	}
}
