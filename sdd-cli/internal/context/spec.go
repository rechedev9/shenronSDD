package context

import (
	"fmt"
	"io"
)

// AssembleSpec builds context for the spec phase.
// Includes: proposal.md, cumulative summary, project stack, sdd-spec SKILL.md.
func AssembleSpec(w io.Writer, p *Params) error {
	skill, err := loadSkill(p.SkillsPath, "sdd-spec")
	if err != nil {
		return err
	}

	proposal, err := loadArtifact(p.ChangeDir, "proposal.md")
	if err != nil {
		return fmt.Errorf("spec requires proposal artifact: %w", err)
	}

	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "CHANGE", fmt.Sprintf(
		"Name: %s\nDescription: %s",
		p.ChangeName, p.Description,
	))

	// Cumulative context so spec isn't written blind.
	writeSectionStr(w, "PROJECT", projectContext(p))

	if summary := buildSummary(p.ChangeDir, p); summary != "" {
		writeSectionStr(w, "PIPELINE CONTEXT", summary)
	}

	writeSection(w, "PROPOSAL", proposal)

	return nil
}
