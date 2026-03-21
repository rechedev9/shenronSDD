package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestChaosConcurrentWrites hammers InsertPhaseEvent + InsertVerifyResult from
// a realistic number of goroutines (2 writers + 2 readers). In production,
// sdd verify writes while the dashboard hub reads.
func TestChaosConcurrentWrites(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)

	const goroutines = 3
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				err := db.InsertPhaseEvent(context.Background(), PhaseEvent{
					Timestamp:  time.Now().UTC(),
					Change:     "chaos-test",
					Phase:      "explore",
					Bytes:      100,
					Tokens:     25,
					Cached:     j%2 == 0,
					DurationMs: int64(j),
				})
				if err != nil {
					t.Errorf("goroutine %d: insert phase event: %v", id, err)
				}
			}
		}(i)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				err := db.InsertVerifyResult(context.Background(), VerifyResult{
					Timestamp:   time.Now().UTC(),
					Change:      "chaos-test",
					CommandName: "build",
					ExitCode:    0,
					Passed:      true,
				})
				if err != nil {
					t.Errorf("goroutine %d: insert verify result: %v", id, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all rows were written.
	stats, err := db.TokenSummary(context.Background())
	if err != nil {
		t.Fatalf("token summary: %v", err)
	}
	wantTokens := goroutines * iterations * 25
	if stats.TotalTokens != wantTokens {
		t.Errorf("total tokens = %d, want %d", stats.TotalTokens, wantTokens)
	}

	rows, err := db.VerifyHistory(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("verify history: %v", err)
	}
	wantRows := goroutines * iterations
	if len(rows) != wantRows {
		t.Errorf("verify rows = %d, want %d", len(rows), wantRows)
	}
}

// TestChaosConcurrentReadsWrites reads and writes simultaneously.
func TestChaosConcurrentReadsWrites(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(4)

	// Writer 1: phase events.
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = db.InsertPhaseEvent(ctx, PhaseEvent{
				Timestamp: time.Now().UTC(), Change: "rw-test",
				Phase: "spec", Bytes: 50, Tokens: 10, DurationMs: 5,
			})
		}
	}()

	// Writer 2: verify results.
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = db.InsertVerifyResult(ctx, VerifyResult{
				Timestamp: time.Now().UTC(), Change: "rw-test",
				CommandName: "test", ExitCode: 0, Passed: true,
			})
		}
	}()

	// Reader 1: token summary.
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = db.TokenSummary(ctx)
		}
	}()

	// Reader 2: phase durations.
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = db.PhaseDurations(ctx)
		}
	}()

	wg.Wait()
}

func openTestDB(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chaos.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
