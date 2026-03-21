package errlog

import (
	"fmt"
	"sync"
	"testing"
)

// TestChaosConcurrentRecord records errors from many goroutines simultaneously.
// Tests atomic file writes under contention.
func TestChaosConcurrentRecord(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()

	const goroutines = 10
	const iterations = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				Record(cwd, ErrorEntry{
					Timestamp:   "2026-03-21T00:00:00Z",
					Change:      fmt.Sprintf("change-%d", id),
					CommandName: "test",
					Command:     "go test ./...",
					ExitCode:    1,
					ErrorLines:  []string{fmt.Sprintf("error %d-%d", id, j)},
					Fingerprint: Fingerprint("go test ./...", []string{fmt.Sprintf("error %d-%d", id, j)}),
				})
			}
		}(i)
	}

	wg.Wait()

	// Load and verify — may have lost some writes due to concurrent read-modify-write,
	// but should never corrupt the file.
	log := Load(cwd)
	if log.Version != 1 {
		t.Errorf("version = %d, want 1", log.Version)
	}
	// At minimum some entries should have been written.
	if len(log.Entries) == 0 {
		t.Error("expected at least some entries after concurrent writes")
	}
	// Should not exceed maxEntries.
	if len(log.Entries) > 100 {
		t.Errorf("entries = %d, exceeds max 100", len(log.Entries))
	}
}

// TestChaosConcurrentRecordAndLoad reads and writes the error log simultaneously.
func TestChaosConcurrentRecordAndLoad(t *testing.T) {
	t.Parallel()
	cwd := t.TempDir()

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer.
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			Record(cwd, ErrorEntry{
				Timestamp:   "2026-03-21T00:00:00Z",
				Change:      "rw-test",
				CommandName: "build",
				Command:     "go build",
				ExitCode:    1,
				ErrorLines:  []string{"fail"},
				Fingerprint: Fingerprint("go build", []string{"fail"}),
			})
		}
	}()

	// Reader.
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			log := Load(cwd)
			// Must never panic or return corrupt data.
			_ = log.RecurringFingerprints(3)
		}
	}()

	wg.Wait()
}

// TestChaosFingerprintConcurrent computes fingerprints from many goroutines.
func TestChaosFingerprintConcurrent(t *testing.T) {
	t.Parallel()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				fp := Fingerprint(fmt.Sprintf("cmd-%d", n), []string{fmt.Sprintf("err-%d", j)})
				if len(fp) != 16 {
					t.Errorf("fingerprint length = %d, want 16", len(fp))
				}
			}
		}(i)
	}

	wg.Wait()
}
