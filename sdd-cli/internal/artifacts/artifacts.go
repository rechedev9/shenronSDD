package artifacts

import (
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/phase"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// ArtifactFileName returns the canonical artifact filename for a phase.
func ArtifactFileName(ph state.Phase) (string, bool) {
	desc, ok := phase.DefaultRegistry.Get(string(ph))
	if !ok {
		return "", false
	}
	return desc.ArtifactFile, true
}

// PendingFileName returns the filename used in .pending/ for a phase.
func PendingFileName(phase state.Phase) string {
	return string(phase) + ".md"
}
