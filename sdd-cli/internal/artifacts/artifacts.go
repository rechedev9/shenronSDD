package artifacts

import (
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// ArtifactFileName maps each phase to its final artifact filename.
// Spec is special — it's a directory, not a single file.
var ArtifactFileName = map[state.Phase]string{
	state.PhaseExplore: "exploration.md",
	state.PhasePropose: "proposal.md",
	state.PhaseSpec:    "specs",
	state.PhaseDesign:  "design.md",
	state.PhaseTasks:   "tasks.md",
	state.PhaseApply:   "tasks.md", // apply updates tasks.md (marks tasks done)
	state.PhaseReview:  "review-report.md",
	state.PhaseVerify:  "verify-report.md",
	state.PhaseClean:   "clean-report.md",
	state.PhaseArchive: "archive-manifest.md",
}

// PendingFileName returns the filename used in .pending/ for a phase.
func PendingFileName(phase state.Phase) string {
	return string(phase) + ".md"
}
