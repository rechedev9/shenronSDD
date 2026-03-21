package artifacts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

var ErrNoPending = errors.New("no pending artifact")

// Promote moves .pending/{phase}.md to its final location in the change directory.
// For spec phase, the pending file is moved into the specs/ directory.
// If force is false, content is validated against phase-specific rules before promotion.
func Promote(changeDir string, phase state.Phase, force bool) (string, error) {
	src := PendingPath(changeDir, phase)

	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf("%w: %s (expected at %s)", ErrNoPending, phase, src)
	}

	finalName, ok := ArtifactFileName(phase)
	if !ok {
		return "", fmt.Errorf("no artifact mapping for phase: %s", phase)
	}

	var dst string
	if phase == state.PhaseSpec {
		// Spec artifacts go into specs/ directory.
		specsDir := filepath.Join(changeDir, "specs")
		if err := os.MkdirAll(specsDir, 0o755); err != nil {
			return "", fmt.Errorf("create specs directory: %w", err)
		}
		// Use the pending filename as the spec file name inside specs/.
		dst = filepath.Join(specsDir, PendingFileName(phase))
	} else {
		dst = filepath.Join(changeDir, finalName)
	}

	// Read, validate, write to destination, remove source (cross-device safe).
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("read pending: %w", err)
	}
	if !force {
		if err := Validate(phase, data); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", fmt.Errorf("write promoted: %w", err)
	}
	if err := os.Remove(src); err != nil {
		// Non-fatal — artifact was promoted, just cleanup failed.
		return dst, nil
	}

	return dst, nil
}
