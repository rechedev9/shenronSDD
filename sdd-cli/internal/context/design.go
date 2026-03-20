package context

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// AssembleDesign builds context for the design phase.
// Includes: spec files (MUST/SHOULD requirements), proposal.md, sdd-design SKILL.md.
func AssembleDesign(w io.Writer, p *Params) error {
	skill, err := loadSkill(p.SkillsPath, "sdd-design")
	if err != nil {
		return err
	}

	proposal, err := loadArtifact(p.ChangeDir, "proposal.md")
	if err != nil {
		return fmt.Errorf("design requires proposal artifact: %w", err)
	}

	// Load spec files from specs/ directory.
	specs, err := loadSpecs(p.ChangeDir)
	if err != nil {
		return fmt.Errorf("design requires spec artifacts: %w", err)
	}

	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "CHANGE", fmt.Sprintf(
		"Name: %s\nDescription: %s",
		p.ChangeName, p.Description,
	))

	// Cumulative context so design decisions are grounded.
	writeSectionStr(w, "PROJECT", projectContext(p))

	if summary := buildSummary(p.ChangeDir, p); summary != "" {
		writeSectionStr(w, "PIPELINE CONTEXT", summary)
	}

	writeSection(w, "PROPOSAL", proposal)
	writeSection(w, "SPECIFICATIONS", []byte(specs))

	return nil
}

// loadSpecs reads all .md files from the specs/ directory, concatenated.
func loadSpecs(changeDir string) (string, error) {
	specsDir := filepath.Join(changeDir, "specs")
	entries, err := os.ReadDir(specsDir)
	if err != nil {
		return "", fmt.Errorf("read specs directory: %w", err)
	}

	var parts []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specsDir, e.Name()))
		if err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("### %s\n\n%s", e.Name(), string(data)))
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("no spec files found in %s", specsDir)
	}

	return strings.Join(parts, "\n\n"), nil
}
