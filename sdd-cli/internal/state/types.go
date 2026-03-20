package state

import "time"

// Phase represents a single SDD pipeline phase.
type Phase string

const (
	PhaseExplore Phase = "explore"
	PhasePropose Phase = "propose"
	PhaseSpec    Phase = "spec"
	PhaseDesign  Phase = "design"
	PhaseTasks   Phase = "tasks"
	PhaseApply   Phase = "apply"
	PhaseReview  Phase = "review"
	PhaseVerify  Phase = "verify"
	PhaseClean   Phase = "clean"
	PhaseArchive Phase = "archive"
)

// PhaseStatus tracks the completion state of a single phase.
type PhaseStatus string

const (
	StatusPending    PhaseStatus = "pending"
	StatusInProgress PhaseStatus = "in_progress"
	StatusCompleted  PhaseStatus = "completed"
	StatusSkipped    PhaseStatus = "skipped"
)

// State is the persisted state for a single SDD change.
type State struct {
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	CurrentPhase Phase                 `json:"current_phase"`
	Phases       map[Phase]PhaseStatus `json:"phases"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	BaseRef      string                `json:"base_ref,omitempty"`
}

// NewState creates a fresh state for a new change.
func NewState(name, description string) *State {
	now := time.Now().UTC()
	phases := make(map[Phase]PhaseStatus, 10)
	for _, p := range AllPhases() {
		phases[p] = StatusPending
	}
	return &State{
		Name:         name,
		Description:  description,
		CurrentPhase: PhaseExplore,
		Phases:       phases,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// IsStale reports whether the change hasn't been updated within threshold.
// Completed changes are never stale.
func (s *State) IsStale(threshold time.Duration) bool {
	if s.IsComplete() {
		return false
	}
	return time.Since(s.UpdatedAt) > threshold
}

// StaleHours returns how many hours since the last update, rounded down.
func (s *State) StaleHours() int {
	return int(time.Since(s.UpdatedAt).Hours())
}

// AllPhases returns the ordered pipeline phases.
func AllPhases() []Phase {
	return []Phase{
		PhaseExplore,
		PhasePropose,
		PhaseSpec,
		PhaseDesign,
		PhaseTasks,
		PhaseApply,
		PhaseReview,
		PhaseVerify,
		PhaseClean,
		PhaseArchive,
	}
}
