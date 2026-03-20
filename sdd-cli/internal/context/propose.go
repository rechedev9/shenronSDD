package context

import (
	"fmt"
	"io"
)

// AssemblePropose builds context for the propose phase.
// Includes: exploration.md, project context, file tree, sdd-propose SKILL.md.
func AssemblePropose(w io.Writer, p *Params) error {
	skill, err := loadSkill(p.SkillsPath, "sdd-propose")
	if err != nil {
		return err
	}

	exploration, err := loadArtifact(p.ChangeDir, "exploration.md")
	if err != nil {
		return fmt.Errorf("propose requires exploration artifact: %w", err)
	}

	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "CHANGE", fmt.Sprintf(
		"Name: %s\nDescription: %s",
		p.ChangeName, p.Description,
	))

	// Carry forward project context that explore had but propose was losing.
	writeSectionStr(w, "PROJECT", projectContext(p))

	fileTree, err := gitFileTree(p.ProjectDir)
	if err == nil {
		writeSectionStr(w, "FILE TREE", fileTree)
	}

	writeSection(w, "EXPLORATION", exploration)

	return nil
}
