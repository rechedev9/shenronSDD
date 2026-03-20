package artifacts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// Read reads a promoted artifact for a given phase from the change directory.
func Read(changeDir string, phase state.Phase) ([]byte, error) {
	name, ok := ArtifactFileName[phase]
	if !ok {
		return nil, fmt.Errorf("no artifact defined for phase: %s", phase)
	}

	path := filepath.Join(changeDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read artifact %s: %w", name, err)
	}
	return data, nil
}

// ReadFile reads an arbitrary file relative to the change directory.
func ReadFile(changeDir, filename string) ([]byte, error) {
	path := filepath.Join(changeDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", filename, err)
	}
	return data, nil
}
