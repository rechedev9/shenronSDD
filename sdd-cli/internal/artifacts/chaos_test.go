package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// TestChaosConcurrentWritePending writes pending artifacts from many goroutines
// to different phases simultaneously.
func TestChaosConcurrentWritePending(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	phases := []state.Phase{
		state.PhaseExplore, state.PhasePropose, state.PhaseSpec,
		state.PhaseDesign, state.PhaseTasks,
	}

	var wg sync.WaitGroup
	wg.Add(len(phases) * 10)

	for _, ph := range phases {
		for i := 0; i < 10; i++ {
			go func(p state.Phase, n int) {
				defer wg.Done()
				content := []byte(fmt.Sprintf("# Content %s iteration %d\n\n## Current State\ntest\n\n## Relevant Files\ntest\n", p, n))
				if err := WritePending(dir, p, content); err != nil {
					t.Errorf("WritePending(%s, %d): %v", p, n, err)
				}
			}(ph, i)
		}
	}

	wg.Wait()

	// All pending files should exist (last writer wins).
	for _, ph := range phases {
		if !PendingExists(dir, ph) {
			t.Errorf("pending %s should exist", ph)
		}
	}
}

// TestChaosConcurrentPromoteDifferentPhases promotes different phases concurrently.
func TestChaosConcurrentPromoteDifferentPhases(t *testing.T) {
	t.Parallel()

	phases := []struct {
		phase   state.Phase
		content string
	}{
		{state.PhaseExplore, "## Current State\ntest\n\n## Relevant Files\ntest"},
		{state.PhasePropose, "## Intent\ntest\n\n## Scope\ntest"},
		{state.PhaseDesign, "## Architecture\ntest"},
		{state.PhaseTasks, "- [ ] task one"},
	}

	var wg sync.WaitGroup
	wg.Add(len(phases))

	for _, pp := range phases {
		go func(p state.Phase, content string) {
			defer wg.Done()
			dir := t.TempDir()
			WritePending(dir, p, []byte(content))
			promoted, err := Promote(dir, p, false)
			if err != nil {
				t.Errorf("Promote(%s): %v", p, err)
				return
			}
			if _, err := os.Stat(promoted); err != nil {
				t.Errorf("promoted file %s missing: %v", promoted, err)
			}
		}(pp.phase, pp.content)
	}

	wg.Wait()
}

// TestChaosConcurrentValidate validates many phases concurrently.
func TestChaosConcurrentValidate(t *testing.T) {
	t.Parallel()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			// Alternate between valid and invalid content.
			if n%2 == 0 {
				err := Validate(state.PhaseExplore, []byte("## Current State\nok\n## Relevant Files\nok"))
				if err != nil {
					t.Errorf("valid explore should pass: %v", err)
				}
			} else {
				err := Validate(state.PhaseExplore, []byte("no headings"))
				if err == nil {
					t.Error("invalid explore should fail")
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestChaosConcurrentList lists artifacts while writing them.
func TestChaosConcurrentList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var wg sync.WaitGroup
	wg.Add(3)

	// Writer: creates artifact files.
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			os.WriteFile(filepath.Join(dir, "exploration.md"), []byte(fmt.Sprintf("explore %d", i)), 0o644)
			os.WriteFile(filepath.Join(dir, "proposal.md"), []byte(fmt.Sprintf("propose %d", i)), 0o644)
		}
	}()

	// Reader 1: lists artifacts.
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_, _ = List(dir)
		}
	}()

	// Reader 2: lists pending.
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_, _ = ListPending(dir)
		}
	}()

	wg.Wait()
}
