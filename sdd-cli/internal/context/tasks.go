package context

import (
	"fmt"
	"io"
)

// AssembleTasks builds context for the tasks phase.
// Includes: spec files, design.md, sdd-tasks SKILL.md.
func AssembleTasks(w io.Writer, p *Params) error {
	skill, err := loadSkill(p.SkillsPath, "sdd-tasks")
	if err != nil {
		return err
	}

	design, err := loadArtifact(p.ChangeDir, "design.md")
	if err != nil {
		return fmt.Errorf("tasks requires design artifact: %w", err)
	}

	specs, err := loadSpecs(p.ChangeDir)
	if err != nil {
		return fmt.Errorf("tasks requires spec artifacts: %w", err)
	}

	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "CHANGE", fmt.Sprintf(
		"Name: %s\nDescription: %s",
		p.ChangeName, p.Description,
	))

	writeSection(w, "SPECIFICATIONS", []byte(specs))
	writeSection(w, "DESIGN", design)

	return nil
}
