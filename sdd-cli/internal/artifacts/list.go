package artifacts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// ArtifactInfo describes a single artifact on disk.
type ArtifactInfo struct {
	Phase    state.Phase `json:"phase"`
	Filename string      `json:"filename"`
	Path     string      `json:"path"`
	Size     int64       `json:"size"`
}

// List returns all existing artifacts in the change directory.
func List(changeDir string) ([]ArtifactInfo, error) {
	var result []ArtifactInfo

	for _, phase := range state.AllPhases() {
		name, ok := ArtifactFileName(phase)
		if !ok {
			continue
		}

		path := filepath.Join(changeDir, name)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		if info.IsDir() {
			// For spec directory, list contents.
			entries, err := os.ReadDir(path)
			if err != nil || len(entries) == 0 {
				continue
			}
			for _, e := range entries {
				eInfo, err := e.Info()
				if err != nil {
					continue
				}
				result = append(result, ArtifactInfo{
					Phase:    phase,
					Filename: e.Name(),
					Path:     filepath.Join(path, e.Name()),
					Size:     eInfo.Size(),
				})
			}
		} else {
			result = append(result, ArtifactInfo{
				Phase:    phase,
				Filename: name,
				Path:     path,
				Size:     info.Size(),
			})
		}
	}

	return result, nil
}

// ListPending returns pending artifacts in the .pending/ directory.
func ListPending(changeDir string) ([]ArtifactInfo, error) {
	pendingDir := filepath.Join(changeDir, ".pending")
	entries, err := os.ReadDir(pendingDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read .pending directory: %w", err)
	}

	var result []ArtifactInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, ArtifactInfo{
			Filename: e.Name(),
			Path:     filepath.Join(pendingDir, e.Name()),
			Size:     info.Size(),
		})
	}
	return result, nil
}
