package artifacts

import (
	"errors"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		phase   state.Phase
		content string
		wantErr bool
	}{
		// explore
		{"explore valid", state.PhaseExplore, "## Current State\nfoo\n## Relevant Files\nbar", false},
		{"explore missing current state", state.PhaseExplore, "## Relevant Files\nbar", true},
		{"explore missing relevant files", state.PhaseExplore, "## Current State\nfoo", true},
		{"explore empty", state.PhaseExplore, "", true},

		// propose
		{"propose valid", state.PhasePropose, "## Intent\nadd feature\n## Scope\nlogin only", false},
		{"propose missing intent", state.PhasePropose, "## Scope\nlogin only", true},
		{"propose missing scope", state.PhasePropose, "## Intent\nadd feature", true},

		// spec
		{"spec valid", state.PhaseSpec, "## Requirements\n- must login", false},
		{"spec no heading", state.PhaseSpec, "just plain text", true},

		// design
		{"design valid", state.PhaseDesign, "## Architecture\ncomponent diagram", false},
		{"design no heading", state.PhaseDesign, "no headings here", true},

		// tasks
		{"tasks valid", state.PhaseTasks, "- [ ] Create LoginPage\n- [ ] Add OAuth", false},
		{"tasks completed", state.PhaseTasks, "- [x] Done task", false},
		{"tasks no checkbox", state.PhaseTasks, "just a list\n- item one", true},

		// apply
		{"apply valid", state.PhaseApply, "- [x] Task done\n- [ ] Task pending", false},
		{"apply no checkbox", state.PhaseApply, "no tasks here", true},

		// review — needs heading + file:line + verdict
		{"review valid", state.PhaseReview, "## Analysis\nReviewed main.go:42\nVerdict: PASS", false},
		{"review FAIL verdict", state.PhaseReview, "## Analysis\nChecked server.go:10\nFAIL", false},
		{"review APPROVED", state.PhaseReview, "## Summary\nSee handler.go:5\nAPPROVED", false},
		{"review REJECTED", state.PhaseReview, "## Summary\nSee handler.go:5\nREJECTED", false},
		{"review no file ref", state.PhaseReview, "## Analysis\nLooks good\nPASS", true},
		{"review no verdict", state.PhaseReview, "## Analysis\nmain.go:42\nlooks fine", true},
		{"review no heading", state.PhaseReview, "main.go:42\nPASS", true},
		{"review empty", state.PhaseReview, "", true},

		// verify — no rules
		{"verify empty", state.PhaseVerify, "", false},
		{"verify anything", state.PhaseVerify, "whatever content", false},

		// clean
		{"clean valid", state.PhaseClean, "## Dead Code\nnone found", false},
		{"clean no heading", state.PhaseClean, "clean report", true},

		// archive — no rules
		{"archive empty", state.PhaseArchive, "", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(tt.phase, []byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%s) error = %v, wantErr %v", tt.phase, err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Errorf("error should wrap ErrValidation, got: %v", err)
			}
		})
	}
}

func TestValidateUnknownPhase(t *testing.T) {
	t.Parallel()
	err := Validate("nonexistent", []byte("anything"))
	if err != nil {
		t.Errorf("unknown phase should return nil, got: %v", err)
	}
}
