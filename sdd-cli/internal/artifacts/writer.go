package artifacts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// WritePending writes content to .pending/{phase}.md in the change directory.
func WritePending(changeDir string, phase state.Phase, data []byte) error {
	pendingDir := filepath.Join(changeDir, ".pending")
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		return fmt.Errorf("create .pending directory: %w", err)
	}

	filename := PendingFileName(phase)
	path := filepath.Join(pendingDir, filename)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write pending %s: %w", filename, err)
	}
	return nil
}

// PendingPath returns the path to the pending artifact for a phase.
func PendingPath(changeDir string, phase state.Phase) string {
	return filepath.Join(changeDir, ".pending", PendingFileName(phase))
}

// PendingExists reports whether a pending artifact exists for the given phase.
func PendingExists(changeDir string, phase state.Phase) bool {
	_, err := os.Stat(PendingPath(changeDir, phase))
	return err == nil
}
