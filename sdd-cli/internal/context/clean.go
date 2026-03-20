package context

import (
	"fmt"
	"io"
)

// AssembleClean builds context for the clean phase.
// Includes: verify-report.md, tasks.md, design.md, specs, cumulative summary,
// sdd-clean SKILL.md.
func AssembleClean(w io.Writer, p *Params) error {
	skill, err := loadSkill(p.SkillsPath, "sdd-clean")
	if err != nil {
		return err
	}

	verifyReport, err := loadArtifact(p.ChangeDir, "verify-report.md")
	if err != nil {
		return fmt.Errorf("clean requires verify-report artifact: %w", err)
	}

	tasks, err := loadArtifact(p.ChangeDir, "tasks.md")
	if err != nil {
		return fmt.Errorf("clean requires tasks artifact: %w", err)
	}

	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "CHANGE", fmt.Sprintf(
		"Name: %s\nDescription: %s",
		p.ChangeName, p.Description,
	))

	// Cumulative context so cleanup has full picture.
	if summary := buildSummary(p.ChangeDir, p); summary != "" {
		writeSectionStr(w, "PIPELINE CONTEXT", summary)
	}

	writeSection(w, "VERIFY REPORT", verifyReport)
	writeSection(w, "TASKS", tasks)

	// Design and specs — clean needs to know what was intended to justify removals.
	if design, err := loadArtifact(p.ChangeDir, "design.md"); err == nil {
		writeSection(w, "DESIGN", design)
	}
	if specs, err := loadSpecs(p.ChangeDir); err == nil {
		writeSection(w, "SPECIFICATIONS", []byte(specs))
	}

	return nil
}
