package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database for SDD telemetry.
type Store struct {
	db *sql.DB
}

// PhaseEvent records a single phase execution.
type PhaseEvent struct {
	Timestamp  time.Time
	Change     string
	Phase      string
	Bytes      int
	Tokens     int
	Cached     bool
	DurationMs int64
}

// VerifyEvent records a single verify command execution.
type VerifyEvent struct {
	Timestamp   time.Time
	Change      string
	CommandName string
	Command     string
	ExitCode    int
	ErrorLines  []string
	Fingerprint string
}

// TokenStats summarises token usage across all phase events.
type TokenStats struct {
	TotalTokens int
	CacheHitPct float64
	ErrorCount  int
}

// ChangeTokens is a per-change token total.
type ChangeTokens struct {
	Change string
	Tokens int
}

// ErrorRow is a single row from verify_events for display.
type ErrorRow struct {
	Timestamp   string
	CommandName string
	Command     string
	ExitCode    int
	Change      string
	Fingerprint string
	FirstLine   string
}

// TokenHistoryRow is a single row for the token usage chart.
type TokenHistoryRow struct {
	Timestamp string
	Change    string
	Phase     string
	Tokens    int
	Cached    bool
}

// PhaseDurationRow is a per-phase average duration.
type PhaseDurationRow struct {
	Phase         string
	AvgDurationMs int64
}

// CacheHistoryRow is a single row for the cache hit/miss chart.
type CacheHistoryRow struct {
	Timestamp string
	Phase     string
	Cached    bool
}

// VerifyHistoryRow is a single row for the verify timeline chart.
type VerifyHistoryRow struct {
	Timestamp   string
	Change      string
	CommandName string
	ExitCode    int
	Passed      bool
}

// VerifyResult records a single verify command result (pass or fail).
type VerifyResult struct {
	Timestamp   time.Time
	Change      string
	CommandName string
	ExitCode    int
	Passed      bool
}

// Open creates the parent directory if needed, opens the SQLite database,
// applies WAL pragmas, and runs schema migrations.
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: mkdir %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}

	// WAL pragmas — must be executed outside a transaction.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(context.Background(), p); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: pragma %q: %w", p, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate creates tables and indexes if they don't already exist.
func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS phase_events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp  TEXT    NOT NULL,
			change     TEXT    NOT NULL,
			phase      TEXT    NOT NULL,
			bytes      INTEGER NOT NULL,
			tokens     INTEGER NOT NULL,
			cached     INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS verify_events (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp    TEXT    NOT NULL,
			change       TEXT    NOT NULL,
			command_name TEXT    NOT NULL,
			command      TEXT    NOT NULL,
			exit_code    INTEGER NOT NULL,
			error_lines  TEXT    NOT NULL,
			fingerprint  TEXT    NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_phase_events_timestamp ON phase_events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_verify_events_timestamp ON verify_events(timestamp)`,
		`CREATE TABLE IF NOT EXISTS verify_results (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp    TEXT    NOT NULL,
			change       TEXT    NOT NULL,
			command_name TEXT    NOT NULL,
			exit_code    INTEGER NOT NULL,
			passed       INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_verify_results_timestamp ON verify_results(timestamp)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store: migrate: %w", err)
		}
	}
	return nil
}

// InsertPhaseEvent stores a phase execution record. Timestamp is stored as
// RFC 3339 TEXT.
func (s *Store) InsertPhaseEvent(ctx context.Context, e PhaseEvent) error {
	cached := 0
	if e.Cached {
		cached = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO phase_events (timestamp, change, phase, bytes, tokens, cached, duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp.Format(time.RFC3339), e.Change, e.Phase,
		e.Bytes, e.Tokens, cached, e.DurationMs,
	)
	if err != nil {
		return fmt.Errorf("store: insert phase event: %w", err)
	}
	return nil
}

// InsertVerifyEvent stores a verify command result. ErrorLines is serialised
// as a JSON array of strings.
func (s *Store) InsertVerifyEvent(ctx context.Context, e VerifyEvent) error {
	lines := e.ErrorLines
	if lines == nil {
		lines = []string{}
	}
	linesJSON, err := json.Marshal(lines)
	if err != nil {
		return fmt.Errorf("store: marshal error lines: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO verify_events (timestamp, change, command_name, command, exit_code, error_lines, fingerprint)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Timestamp.Format(time.RFC3339), e.Change, e.CommandName,
		e.Command, e.ExitCode, string(linesJSON), e.Fingerprint,
	)
	if err != nil {
		return fmt.Errorf("store: insert verify event: %w", err)
	}
	return nil
}

// TokenSummary computes aggregate token stats across all phase events and
// an error count from verify_events. Returns zero values on an empty DB.
func (s *Store) TokenSummary(ctx context.Context) (*TokenStats, error) {
	var stats TokenStats

	var totalTokens sql.NullInt64
	var totalRows sql.NullInt64
	var cachedRows sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(tokens), 0),
		        COUNT(*),
		        COALESCE(SUM(CASE WHEN cached = 1 THEN 1 ELSE 0 END), 0)
		 FROM phase_events`,
	).Scan(&totalTokens, &totalRows, &cachedRows)
	if err != nil {
		return nil, fmt.Errorf("store: token summary: %w", err)
	}

	stats.TotalTokens = int(totalTokens.Int64)
	if totalRows.Int64 > 0 {
		stats.CacheHitPct = float64(cachedRows.Int64) / float64(totalRows.Int64) * 100
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM verify_events`,
	).Scan(&stats.ErrorCount)
	if err != nil {
		return nil, fmt.Errorf("store: token summary error count: %w", err)
	}

	return &stats, nil
}

// PhaseTokensByChange returns per-change token totals, grouped by change name.
func (s *Store) PhaseTokensByChange(ctx context.Context) ([]ChangeTokens, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT change, SUM(tokens) FROM phase_events GROUP BY change ORDER BY change`)
	if err != nil {
		return nil, fmt.Errorf("store: phase tokens by change: %w", err)
	}
	defer rows.Close()

	var result []ChangeTokens
	for rows.Next() {
		var ct ChangeTokens
		if err := rows.Scan(&ct.Change, &ct.Tokens); err != nil {
			return nil, fmt.Errorf("store: scan change tokens: %w", err)
		}
		result = append(result, ct)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: change tokens rows: %w", err)
	}
	return result, nil
}

// RecentErrors returns the most recent verify events, newest first.
// It parses the JSON error_lines column to extract FirstLine.
func (s *Store) RecentErrors(ctx context.Context, limit int) ([]ErrorRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, command_name, command, exit_code, change, fingerprint, error_lines
		 FROM verify_events
		 ORDER BY timestamp DESC
		 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: recent errors: %w", err)
	}
	defer rows.Close()

	result := make([]ErrorRow, 0)
	for rows.Next() {
		var r ErrorRow
		var linesJSON string
		if err := rows.Scan(&r.Timestamp, &r.CommandName, &r.Command,
			&r.ExitCode, &r.Change, &r.Fingerprint, &linesJSON); err != nil {
			return nil, fmt.Errorf("store: scan error row: %w", err)
		}
		var lines []string
		if err := json.Unmarshal([]byte(linesJSON), &lines); err != nil {
			return nil, fmt.Errorf("store: unmarshal error lines: %w", err)
		}
		if len(lines) > 0 {
			r.FirstLine = lines[0]
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: error rows: %w", err)
	}
	return result, nil
}

// InsertVerifyResult stores a verify command result (pass/fail).
func (s *Store) InsertVerifyResult(ctx context.Context, r VerifyResult) error {
	passed := 0
	if r.Passed {
		passed = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO verify_results (timestamp, change, command_name, exit_code, passed)
		 VALUES (?, ?, ?, ?, ?)`,
		r.Timestamp.Format(time.RFC3339), r.Change, r.CommandName,
		r.ExitCode, passed,
	)
	if err != nil {
		return fmt.Errorf("store: insert verify result: %w", err)
	}
	return nil
}

// TokenHistory returns token usage rows since the given time.
func (s *Store) TokenHistory(ctx context.Context, since time.Time) ([]TokenHistoryRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, change, phase, tokens, cached
		 FROM phase_events
		 WHERE timestamp > ?
		 ORDER BY id`, since.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store: token history: %w", err)
	}
	defer rows.Close()

	var result []TokenHistoryRow
	for rows.Next() {
		var r TokenHistoryRow
		var cached int
		if err := rows.Scan(&r.Timestamp, &r.Change, &r.Phase, &r.Tokens, &cached); err != nil {
			return nil, fmt.Errorf("store: scan token history: %w", err)
		}
		r.Cached = cached == 1
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: token history rows: %w", err)
	}
	return result, nil
}

// PhaseDurations returns per-phase average duration across all phase events.
func (s *Store) PhaseDurations(ctx context.Context) ([]PhaseDurationRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT phase, AVG(duration_ms) FROM phase_events GROUP BY phase ORDER BY phase`)
	if err != nil {
		return nil, fmt.Errorf("store: phase durations: %w", err)
	}
	defer rows.Close()

	var result []PhaseDurationRow
	for rows.Next() {
		var r PhaseDurationRow
		if err := rows.Scan(&r.Phase, &r.AvgDurationMs); err != nil {
			return nil, fmt.Errorf("store: scan phase duration: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: phase durations rows: %w", err)
	}
	return result, nil
}

// CacheHistory returns cache hit/miss rows since the given time.
func (s *Store) CacheHistory(ctx context.Context, since time.Time) ([]CacheHistoryRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, phase, cached
		 FROM phase_events
		 WHERE timestamp > ?
		 ORDER BY id`, since.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store: cache history: %w", err)
	}
	defer rows.Close()

	var result []CacheHistoryRow
	for rows.Next() {
		var r CacheHistoryRow
		var cached int
		if err := rows.Scan(&r.Timestamp, &r.Phase, &cached); err != nil {
			return nil, fmt.Errorf("store: scan cache history: %w", err)
		}
		r.Cached = cached == 1
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: cache history rows: %w", err)
	}
	return result, nil
}

// VerifyHistory returns verify result rows since the given time.
func (s *Store) VerifyHistory(ctx context.Context, since time.Time) ([]VerifyHistoryRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, change, command_name, exit_code, passed
		 FROM verify_results
		 WHERE timestamp > ?
		 ORDER BY id`, since.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store: verify history: %w", err)
	}
	defer rows.Close()

	var result []VerifyHistoryRow
	for rows.Next() {
		var r VerifyHistoryRow
		var passed int
		if err := rows.Scan(&r.Timestamp, &r.Change, &r.CommandName, &r.ExitCode, &passed); err != nil {
			return nil, fmt.Errorf("store: scan verify history: %w", err)
		}
		r.Passed = passed == 1
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: verify history rows: %w", err)
	}
	return result, nil
}
