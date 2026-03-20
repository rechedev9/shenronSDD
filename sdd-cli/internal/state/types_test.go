package state

import "testing"

func TestNewState(t *testing.T) {
	s := NewState("add-auth", "Add authentication module")

	if s.Name != "add-auth" {
		t.Errorf("name = %q, want %q", s.Name, "add-auth")
	}
	if s.Description != "Add authentication module" {
		t.Errorf("description = %q, want %q", s.Description, "Add authentication module")
	}
	if s.CurrentPhase != PhaseExplore {
		t.Errorf("current phase = %q, want %q", s.CurrentPhase, PhaseExplore)
	}
	if s.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}

	allPhases := AllPhases()
	if len(s.Phases) != len(allPhases) {
		t.Errorf("phase count = %d, want %d", len(s.Phases), len(allPhases))
	}
	for _, p := range allPhases {
		status, ok := s.Phases[p]
		if !ok {
			t.Errorf("phase %q missing from state", p)
			continue
		}
		if status != StatusPending {
			t.Errorf("phase %q status = %q, want %q", p, status, StatusPending)
		}
	}
}

func TestAllPhasesOrder(t *testing.T) {
	phases := AllPhases()
	expected := []Phase{
		PhaseExplore, PhasePropose, PhaseSpec, PhaseDesign,
		PhaseTasks, PhaseApply, PhaseReview, PhaseVerify,
		PhaseClean, PhaseArchive,
	}
	if len(phases) != len(expected) {
		t.Fatalf("phase count = %d, want %d", len(phases), len(expected))
	}
	for i, p := range phases {
		if p != expected[i] {
			t.Errorf("phase[%d] = %q, want %q", i, p, expected[i])
		}
	}
}
