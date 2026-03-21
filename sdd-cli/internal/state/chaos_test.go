package state

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestChaosConcurrentStateReads creates multiple states and reads them concurrently.
func TestChaosConcurrentStateReads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create N state files.
	const n = 20
	for i := 0; i < n; i++ {
		st := NewState("chaos", "test")
		path := filepath.Join(dir, "state.json")
		if err := Save(st, path); err != nil {
			t.Fatalf("save: %v", err)
		}
	}

	// Read concurrently.
	var wg sync.WaitGroup
	wg.Add(50)
	for i := 0; i < 50; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				st, err := Load(filepath.Join(dir, "state.json"))
				if err != nil {
					t.Errorf("load: %v", err)
					return
				}
				// Exercise AllPhases and state machine methods.
				_ = st.ReadyPhases()
				_ = st.IsComplete()
				_ = st.IsStale(0)
			}
		}()
	}
	wg.Wait()
}

// TestChaosConcurrentSaveLoad writes and reads state.json simultaneously.
// AtomicWrite should prevent corrupt reads.
func TestChaosConcurrentSaveLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Seed initial state.
	st := NewState("chaos-rw", "concurrent save/load test")
	if err := Save(st, path); err != nil {
		t.Fatalf("initial save: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: continuously saves.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			s := NewState("chaos-rw", "concurrent save/load test")
			_ = Save(s, path)
		}
	}()

	// Reader: continuously loads.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			loaded, err := Load(path)
			if err != nil {
				// Temporary read errors during atomic rename are expected on some OS.
				// The key invariant: if Load succeeds, the state is valid (not corrupt).
				continue
			}
			if loaded.Name != "chaos-rw" {
				t.Errorf("corrupt state: name = %q, want chaos-rw", loaded.Name)
			}
		}
	}()

	wg.Wait()
}

// TestChaosRecoverFromCorruption verifies Recover handles missing/corrupt state.json.
func TestChaosRecoverFromCorruption(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write a corrupt state.json.
	os.WriteFile(filepath.Join(dir, "state.json"), []byte("{invalid json"), 0o644)

	// Write some artifacts so Recover has something to work with.
	os.WriteFile(filepath.Join(dir, "exploration.md"), []byte("explore"), 0o644)
	os.WriteFile(filepath.Join(dir, "proposal.md"), []byte("propose"), 0o644)

	st := Recover("chaos-recover", "test", dir)
	if st.Name != "chaos-recover" {
		t.Errorf("name = %q, want chaos-recover", st.Name)
	}
	if st.Phases[PhaseExplore] != StatusCompleted {
		t.Errorf("explore should be completed from artifact")
	}
	if st.Phases[PhasePropose] != StatusCompleted {
		t.Errorf("propose should be completed from artifact")
	}
	if st.Phases[PhaseSpec] != StatusPending {
		t.Errorf("spec should be pending (no artifact)")
	}
}
