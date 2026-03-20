package context

import (
	"fmt"
	"io"
	"strings"
)

// AssembleApply builds context for the apply phase.
// Includes: tasks.md (current incomplete task only), design.md, spec files,
// sdd-apply SKILL.md.
func AssembleApply(w io.Writer, p *Params) error {
	skill, err := loadSkill(p.SkillsPath, "sdd-apply")
	if err != nil {
		return err
	}

	tasksRaw, err := loadArtifact(p.ChangeDir, "tasks.md")
	if err != nil {
		return fmt.Errorf("apply requires tasks artifact: %w", err)
	}

	design, err := loadArtifact(p.ChangeDir, "design.md")
	if err != nil {
		return fmt.Errorf("apply requires design artifact: %w", err)
	}

	specs, err := loadSpecs(p.ChangeDir)
	if err != nil {
		return fmt.Errorf("apply requires spec artifacts: %w", err)
	}

	// Extract the current incomplete task to minimize context.
	currentTask := extractCurrentTask(string(tasksRaw))
	completedSummary := extractCompletedTasks(string(tasksRaw))

	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "CHANGE", fmt.Sprintf(
		"Name: %s\nDescription: %s",
		p.ChangeName, p.Description,
	))

	// Cumulative context so apply knows what's already been done.
	if summary := buildSummary(p.ChangeDir, p); summary != "" {
		writeSectionStr(w, "PIPELINE CONTEXT", summary)
	}

	writeSectionStr(w, "COMPLETED TASKS", completedSummary)
	writeSectionStr(w, "CURRENT TASK", currentTask)
	writeSection(w, "DESIGN", design)
	writeSection(w, "SPECIFICATIONS", []byte(specs))

	return nil
}

// extractCurrentTask finds the first incomplete task section in tasks.md.
// Returns the section header + all tasks in that section (both complete and incomplete).
// If no incomplete task exists, returns the full content.
func extractCurrentTask(tasks string) string {
	lines := strings.Split(tasks, "\n")
	firstIncomplete := -1

	// Find the first unchecked task.
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "- [ ]") {
			firstIncomplete = i
			break
		}
	}

	if firstIncomplete == -1 {
		return tasks
	}

	// Walk back to find the section header.
	start := firstIncomplete
	for j := firstIncomplete - 1; j >= 0; j-- {
		if strings.HasPrefix(lines[j], "#") {
			start = j
			break
		}
	}

	// Walk forward to find the next section header (##).
	end := len(lines)
	for i := firstIncomplete + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "##") {
			end = i
			break
		}
	}

	return strings.Join(lines[start:end], "\n")
}
