package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Transition graph for the SDD pipeline.
//
//   explore ──→ propose ──→ spec  ──┐
//                           design ─┤ (spec + design are parallel after propose)
//                                   ↓
//                                 tasks ──→ apply ──→ review ──→ verify ──→ clean ──→ archive
//
// spec and design both require propose completed.
// tasks requires BOTH spec AND design completed.
// All other transitions are strictly sequential.

// validNextPhases maps each phase to the set of phases it can transition to.
var validNextPhases = map[Phase][]Phase{
	PhaseExplore: {PhasePropose},
	PhasePropose: {PhaseSpec, PhaseDesign},
	PhaseSpec:    {PhaseTasks},
	PhaseDesign:  {PhaseTasks},
	PhaseTasks:   {PhaseApply},
	PhaseApply:   {PhaseReview},
	PhaseReview:  {PhaseVerify},
	PhaseVerify:  {PhaseClean},
	PhaseClean:   {PhaseArchive},
	PhaseArchive: {},
}

// prerequisites maps each phase to the phases that must be completed before it can start.
var prerequisites = map[Phase][]Phase{
	PhaseExplore: {},
	PhasePropose: {PhaseExplore},
	PhaseSpec:    {PhasePropose},
	PhaseDesign:  {PhasePropose},
	PhaseTasks:   {PhaseSpec, PhaseDesign},
	PhaseApply:   {PhaseTasks},
	PhaseReview:  {PhaseApply},
	PhaseVerify:  {PhaseReview},
	PhaseClean:   {PhaseVerify},
	PhaseArchive: {PhaseClean},
}

var (
	ErrInvalidTransition = errors.New("invalid phase transition")
	ErrPrerequisitesNotMet = errors.New("prerequisites not met")
	ErrAlreadyCompleted    = errors.New("phase already completed")
	ErrCorruptState        = errors.New("corrupt state file")
)

// CanTransition reports whether moving from the current state to the target phase is valid.
func (s *State) CanTransition(target Phase) error {
	if s.Phases[target] == StatusCompleted {
		return fmt.Errorf("%w: %s already completed", ErrAlreadyCompleted, target)
	}

	// Check prerequisites are met.
	for _, req := range prerequisites[target] {
		if s.Phases[req] != StatusCompleted {
			return fmt.Errorf("%w: %s requires %s completed (currently %s)", ErrPrerequisitesNotMet, target, req, s.Phases[req])
		}
	}

	return nil
}

// Advance marks the current target phase as completed and sets CurrentPhase to the next logical phase.
func (s *State) Advance(completed Phase) error {
	if err := s.CanTransition(completed); err != nil {
		return err
	}

	s.Phases[completed] = StatusCompleted
	s.UpdatedAt = time.Now().UTC()

	// Find the next pending phase that has all prerequisites met.
	s.CurrentPhase = s.nextReady()
	return nil
}

// nextReady returns the first phase in pipeline order whose prerequisites are all completed
// and which is still pending. Returns "" if nothing is ready (pipeline done).
func (s *State) nextReady() Phase {
	for _, p := range AllPhases() {
		if s.Phases[p] != StatusPending {
			continue
		}
		ready := true
		for _, req := range prerequisites[p] {
			if s.Phases[req] != StatusCompleted {
				ready = false
				break
			}
		}
		if ready {
			return p
		}
	}
	return "" // pipeline complete
}

// ReadyPhases returns all pending phases whose prerequisites are met.
// Unlike nextReady (which returns the first), this returns all — enabling
// parallel assembly of spec+design when both are ready after propose.
func (s *State) ReadyPhases() []Phase {
	var ready []Phase
	for _, p := range AllPhases() {
		if s.Phases[p] != StatusPending {
			continue
		}
		allMet := true
		for _, req := range prerequisites[p] {
			if s.Phases[req] != StatusCompleted {
				allMet = false
				break
			}
		}
		if allMet {
			ready = append(ready, p)
		}
	}
	return ready
}

// IsComplete reports whether all phases are completed.
func (s *State) IsComplete() bool {
	for _, p := range AllPhases() {
		if s.Phases[p] != StatusCompleted && s.Phases[p] != StatusSkipped {
			return false
		}
	}
	return true
}

// Save writes the state to path atomically (write .tmp, rename).
//
// Resume invariant: if a session crashes mid-pipeline, the next session calls
// Load() to get the last persisted state. nextReady() computes which phase is
// next from completed phases — no manual inspection needed. If state.json is
// corrupt or missing, Recover() rebuilds state from on-disk artifacts.
// This is the incomplete-batch resume pattern.
func Save(s *State, path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("rename temp state: %w", err)
	}
	return nil
}

// Load reads state from path. Returns the state or an error.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptState, err)
	}

	if err := validate(&s); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptState, err)
	}

	return &s, nil
}

// validate checks that a loaded state has all required fields.
func validate(s *State) error {
	if s.Name == "" {
		return errors.New("missing name")
	}
	if s.Phases == nil {
		return errors.New("missing phases map")
	}
	for _, p := range AllPhases() {
		if _, ok := s.Phases[p]; !ok {
			return fmt.Errorf("missing phase: %s", p)
		}
	}
	return nil
}

// Recover attempts to rebuild state from existing artifact files in changeDir.
// It marks phases as completed if their expected artifact exists on disk.
func Recover(name, description, changeDir string) *State {
	s := NewState(name, description)

	// Map of phase → expected artifact filename.
	artifacts := map[Phase]string{
		PhaseExplore: "exploration.md",
		PhasePropose: "proposal.md",
		PhaseSpec:    "specs",   // directory
		PhaseDesign:  "design.md",
		PhaseTasks:   "tasks.md",
		PhaseReview:  "review-report.md",
		PhaseVerify:  "verify-report.md",
	}

	for phase, artifact := range artifacts {
		path := filepath.Join(changeDir, artifact)
		if info, err := os.Stat(path); err == nil {
			// For directories (specs), check it's non-empty.
			if info.IsDir() {
				entries, _ := os.ReadDir(path)
				if len(entries) == 0 {
					continue
				}
			}
			s.Phases[phase] = StatusCompleted
		}
	}

	s.CurrentPhase = s.nextReady()
	s.UpdatedAt = time.Now().UTC()
	return s
}
