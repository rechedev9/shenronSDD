package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidTransitions(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*State) // complete prerequisite phases
		target Phase
	}{
		{"explore first", nil, PhaseExplore},
		{"propose after explore", func(s *State) { s.Phases[PhaseExplore] = StatusCompleted }, PhasePropose},
		{"spec after propose", func(s *State) {
			s.Phases[PhaseExplore] = StatusCompleted
			s.Phases[PhasePropose] = StatusCompleted
		}, PhaseSpec},
		{"design after propose", func(s *State) {
			s.Phases[PhaseExplore] = StatusCompleted
			s.Phases[PhasePropose] = StatusCompleted
		}, PhaseDesign},
		{"tasks after spec+design", func(s *State) {
			s.Phases[PhaseExplore] = StatusCompleted
			s.Phases[PhasePropose] = StatusCompleted
			s.Phases[PhaseSpec] = StatusCompleted
			s.Phases[PhaseDesign] = StatusCompleted
		}, PhaseTasks},
		{"apply after tasks", func(s *State) {
			completeUpTo(s, PhaseTasks)
		}, PhaseApply},
		{"review after apply", func(s *State) {
			completeUpTo(s, PhaseApply)
		}, PhaseReview},
		{"verify after review", func(s *State) {
			completeUpTo(s, PhaseReview)
		}, PhaseVerify},
		{"clean after verify", func(s *State) {
			completeUpTo(s, PhaseVerify)
		}, PhaseClean},
		{"archive after clean", func(s *State) {
			completeUpTo(s, PhaseClean)
		}, PhaseArchive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewState("test", "test")
			if tt.setup != nil {
				tt.setup(s)
			}
			if err := s.CanTransition(tt.target); err != nil {
				t.Errorf("CanTransition(%s) = %v, want nil", tt.target, err)
			}
		})
	}
}

func TestInvalidTransitions(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*State)
		target    Phase
		wantErr   error
	}{
		{"propose without explore", nil, PhasePropose, ErrPrerequisitesNotMet},
		{"spec without propose", func(s *State) {
			s.Phases[PhaseExplore] = StatusCompleted
		}, PhaseSpec, ErrPrerequisitesNotMet},
		{"tasks without spec", func(s *State) {
			s.Phases[PhaseExplore] = StatusCompleted
			s.Phases[PhasePropose] = StatusCompleted
			s.Phases[PhaseDesign] = StatusCompleted
		}, PhaseTasks, ErrPrerequisitesNotMet},
		{"tasks without design", func(s *State) {
			s.Phases[PhaseExplore] = StatusCompleted
			s.Phases[PhasePropose] = StatusCompleted
			s.Phases[PhaseSpec] = StatusCompleted
		}, PhaseTasks, ErrPrerequisitesNotMet},
		{"apply without tasks", func(s *State) {
			completeUpTo(s, PhaseDesign)
		}, PhaseApply, ErrPrerequisitesNotMet},
		{"already completed", func(s *State) {
			s.Phases[PhaseExplore] = StatusCompleted
		}, PhaseExplore, ErrAlreadyCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewState("test", "test")
			if tt.setup != nil {
				tt.setup(s)
			}
			err := s.CanTransition(tt.target)
			if err == nil {
				t.Fatalf("CanTransition(%s) = nil, want error", tt.target)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CanTransition(%s) error = %v, want %v", tt.target, err, tt.wantErr)
			}
		})
	}
}

func TestAdvance(t *testing.T) {
	s := NewState("feat", "desc")

	// Walk the full pipeline.
	steps := []Phase{
		PhaseExplore, PhasePropose, PhaseSpec, PhaseDesign,
		PhaseTasks, PhaseApply, PhaseReview, PhaseVerify,
		PhaseClean, PhaseArchive,
	}
	for _, step := range steps {
		if err := s.Advance(step); err != nil {
			t.Fatalf("Advance(%s) = %v", step, err)
		}
		if s.Phases[step] != StatusCompleted {
			t.Errorf("after Advance(%s): status = %q, want completed", step, s.Phases[step])
		}
	}
	if !s.IsComplete() {
		t.Error("expected IsComplete() = true after all phases")
	}
}

func TestAdvanceParallelSpecDesign(t *testing.T) {
	s := NewState("feat", "desc")
	s.Phases[PhaseExplore] = StatusCompleted
	s.Phases[PhasePropose] = StatusCompleted

	// Design first, then spec — both valid after propose.
	if err := s.Advance(PhaseDesign); err != nil {
		t.Fatalf("Advance(design) = %v", err)
	}
	// After completing design, spec should still be the next ready (or design depending on order).
	if err := s.Advance(PhaseSpec); err != nil {
		t.Fatalf("Advance(spec) = %v", err)
	}
	// Now tasks should be ready.
	if s.CurrentPhase != PhaseTasks {
		t.Errorf("current phase = %q, want tasks", s.CurrentPhase)
	}
}

func TestAdvanceInvalid(t *testing.T) {
	s := NewState("feat", "desc")
	err := s.Advance(PhaseApply)
	if err == nil {
		t.Fatal("expected error advancing to apply from fresh state")
	}
	// Phase should still be pending.
	if s.Phases[PhaseApply] != StatusPending {
		t.Error("apply should remain pending after invalid advance")
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := NewState("auth", "Add auth")
	original.Phases[PhaseExplore] = StatusCompleted
	original.CurrentPhase = PhasePropose

	if err := Save(original, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("name = %q, want %q", loaded.Name, original.Name)
	}
	if loaded.CurrentPhase != original.CurrentPhase {
		t.Errorf("current phase = %q, want %q", loaded.CurrentPhase, original.CurrentPhase)
	}
	if loaded.Phases[PhaseExplore] != StatusCompleted {
		t.Errorf("explore status = %q, want completed", loaded.Phases[PhaseExplore])
	}
}

func TestSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := NewState("test", "test")
	if err := Save(s, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify no .tmp file remains.
	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("temp file %s should not exist after save", tmp)
	}

	// Verify the file is valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var check State
	if err := json.Unmarshal(data, &check); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "state.json")

	s := NewState("test", "test")
	if err := Save(s, path); err != nil {
		t.Fatalf("Save with nested dirs: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file should exist at %s", path)
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load("/nonexistent/path/state.json")
	if err == nil {
		t.Fatal("expected error loading missing file")
	}
}

func TestLoadCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error loading corrupt JSON")
	}
	if !errors.Is(err, ErrCorruptState) {
		t.Errorf("error = %v, want ErrCorruptState", err)
	}
}

func TestLoadMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Phases: map[Phase]PhaseStatus{}}
	for _, p := range AllPhases() {
		s.Phases[p] = StatusPending
	}
	data, _ := json.Marshal(s)
	os.WriteFile(path, data, 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !errors.Is(err, ErrCorruptState) {
		t.Errorf("error = %v, want ErrCorruptState", err)
	}
}

func TestLoadMissingPhase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := &State{Name: "test", Phases: map[Phase]PhaseStatus{
		PhaseExplore: StatusPending,
		// Missing other phases.
	}}
	data, _ := json.Marshal(s)
	os.WriteFile(path, data, 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing phases")
	}
	if !errors.Is(err, ErrCorruptState) {
		t.Errorf("error = %v, want ErrCorruptState", err)
	}
}

func TestRecoverFromArtifacts(t *testing.T) {
	dir := t.TempDir()

	// Create some artifact files.
	os.WriteFile(filepath.Join(dir, "exploration.md"), []byte("# Exploration"), 0o644)
	os.WriteFile(filepath.Join(dir, "proposal.md"), []byte("# Proposal"), 0o644)
	os.WriteFile(filepath.Join(dir, "design.md"), []byte("# Design"), 0o644)
	os.MkdirAll(filepath.Join(dir, "specs"), 0o755)
	os.WriteFile(filepath.Join(dir, "specs", "auth-spec.md"), []byte("# Spec"), 0o644)

	s := Recover("feat", "desc", dir)

	if s.Phases[PhaseExplore] != StatusCompleted {
		t.Error("explore should be recovered as completed")
	}
	if s.Phases[PhasePropose] != StatusCompleted {
		t.Error("propose should be recovered as completed")
	}
	if s.Phases[PhaseSpec] != StatusCompleted {
		t.Error("spec should be recovered as completed")
	}
	if s.Phases[PhaseDesign] != StatusCompleted {
		t.Error("design should be recovered as completed")
	}
	if s.Phases[PhaseTasks] != StatusPending {
		t.Errorf("tasks should be pending, got %q", s.Phases[PhaseTasks])
	}
	if s.CurrentPhase != PhaseTasks {
		t.Errorf("current phase = %q, want tasks", s.CurrentPhase)
	}
}

func TestRecoverEmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := Recover("feat", "desc", dir)

	if s.CurrentPhase != PhaseExplore {
		t.Errorf("current phase = %q, want explore", s.CurrentPhase)
	}
	for _, p := range AllPhases() {
		if s.Phases[p] != StatusPending {
			t.Errorf("phase %s = %q, want pending", p, s.Phases[p])
		}
	}
}

func TestRecoverEmptySpecsDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "specs"), 0o755) // empty specs dir

	s := Recover("feat", "desc", dir)
	if s.Phases[PhaseSpec] != StatusPending {
		t.Errorf("spec should remain pending for empty specs dir, got %q", s.Phases[PhaseSpec])
	}
}

func TestIsComplete(t *testing.T) {
	s := NewState("test", "test")
	if s.IsComplete() {
		t.Error("fresh state should not be complete")
	}

	for _, p := range AllPhases() {
		s.Phases[p] = StatusCompleted
	}
	if !s.IsComplete() {
		t.Error("all-completed state should be complete")
	}
}

func TestIsCompleteWithSkipped(t *testing.T) {
	s := NewState("test", "test")
	for _, p := range AllPhases() {
		s.Phases[p] = StatusCompleted
	}
	s.Phases[PhaseClean] = StatusSkipped
	if !s.IsComplete() {
		t.Error("completed+skipped should count as complete")
	}
}

// completeUpTo marks all phases up to and including target as completed.
func completeUpTo(s *State, target Phase) {
	order := []Phase{
		PhaseExplore, PhasePropose, PhaseSpec, PhaseDesign,
		PhaseTasks, PhaseApply, PhaseReview, PhaseVerify,
		PhaseClean, PhaseArchive,
	}
	for _, p := range order {
		s.Phases[p] = StatusCompleted
		if p == target {
			return
		}
	}
}
