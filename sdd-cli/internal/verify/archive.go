package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ArchiveResult holds the outcome of an archive operation.
type ArchiveResult struct {
	ArchivePath  string `json:"archive_path"`
	ManifestPath string `json:"manifest_path"`
}

// Archive moves changeDir into openspec/changes/archive/{timestamp}-{name}/.
func Archive(changeDir string) (*ArchiveResult, error) {
	name := filepath.Base(changeDir)
	changesDir := filepath.Dir(changeDir)
	archiveParent := filepath.Join(changesDir, "archive")

	if err := os.MkdirAll(archiveParent, 0o755); err != nil {
		return nil, fmt.Errorf("create archive directory: %w", err)
	}

	stamp := time.Now().UTC().Format("2006-01-02-150405")
	archiveName := stamp + "-" + name
	archivePath := filepath.Join(archiveParent, archiveName)

	if err := os.Rename(changeDir, archivePath); err != nil {
		return nil, fmt.Errorf("move change to archive: %w", err)
	}

	manifestPath := filepath.Join(archivePath, "archive-manifest.md")
	if err := writeManifest(archivePath, name, manifestPath); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	return &ArchiveResult{
		ArchivePath:  archivePath,
		ManifestPath: manifestPath,
	}, nil
}

// writeManifest creates archive-manifest.md listing all archived artifacts.
func writeManifest(archivePath, changeName, manifestPath string) error {
	entries, err := os.ReadDir(archivePath)
	if err != nil {
		return fmt.Errorf("read archive directory: %w", err)
	}

	var b strings.Builder
	b.WriteString("# Archive Manifest\n\n")
	b.WriteString(fmt.Sprintf("**Change:** %s\n", changeName))
	b.WriteString(fmt.Sprintf("**Archived:** %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	b.WriteString("## Artifacts\n\n")

	phaseArtifacts := map[string]bool{
		"exploration.md":   true,
		"proposal.md":      true,
		"design.md":        true,
		"tasks.md":         true,
		"review-report.md": true,
		"verify-report.md": true,
		"clean-report.md":  true,
	}

	specCount := 0
	completed := 0
	for _, e := range entries {
		name := e.Name()
		if name == "archive-manifest.md" || (e.IsDir() && name == ".pending") {
			continue
		}
		if e.IsDir() && name == "specs" {
			specEntries, _ := os.ReadDir(filepath.Join(archivePath, "specs"))
			specCount = len(specEntries)
			b.WriteString(fmt.Sprintf("- `specs/` (%d files)\n", specCount))
			continue
		}
		b.WriteString(fmt.Sprintf("- `%s`\n", name))
		if phaseArtifacts[name] {
			completed++
		}
	}
	if specCount > 0 {
		completed++ // spec phase
	}

	// Summary section.
	b.WriteString("\n## Summary\n\n")
	b.WriteString(fmt.Sprintf("- **Completed phases:** %d\n", completed))
	b.WriteString(fmt.Sprintf("- **Spec files:** %d\n", specCount))

	// Atomic write: temp file + rename.
	tmp := manifestPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write archive manifest: %w", err)
	}
	if err := os.Rename(tmp, manifestPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename archive manifest: %w", err)
	}
	return nil
}
