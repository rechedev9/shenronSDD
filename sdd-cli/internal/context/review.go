package context

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// AssembleReview builds context for the review phase.
// Includes: spec files, design.md, git diff of changed files, sdd-review SKILL.md.
// Optionally includes AGENTS.md / CLAUDE.md if present.
func AssembleReview(w io.Writer, p *Params) error {
	skill, err := loadSkill(p.SkillsPath, "sdd-review")
	if err != nil {
		return err
	}

	specs, err := loadSpecs(p.ChangeDir)
	if err != nil {
		return fmt.Errorf("review requires spec artifacts: %w", err)
	}

	design, err := loadArtifact(p.ChangeDir, "design.md")
	if err != nil {
		return fmt.Errorf("review requires design artifact: %w", err)
	}

	tasks, err := loadArtifact(p.ChangeDir, "tasks.md")
	if err != nil {
		return fmt.Errorf("review requires tasks artifact: %w", err)
	}

	// Get git diff of uncommitted changes.
	diff, err := gitDiff(p.ProjectDir)
	if err != nil {
		diff = fmt.Sprintf("(git diff unavailable: %v)", err)
	}

	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "CHANGE", fmt.Sprintf(
		"Name: %s\nDescription: %s",
		p.ChangeName, p.Description,
	))

	writeSection(w, "SPECIFICATIONS", []byte(specs))
	writeSection(w, "DESIGN", design)
	writeSection(w, "TASKS", tasks)
	writeSectionStr(w, "GIT DIFF", diff)

	// Load project rules if present (AGENTS.md or CLAUDE.md).
	if rules, err := loadProjectRules(p.ProjectDir); err == nil {
		writeSection(w, "PROJECT RULES", rules)
	}

	return nil
}

// gitDiff runs git diff and returns staged + unstaged changes.
func gitDiff(projectDir string) (string, error) {
	// Unstaged changes.
	cmd := exec.Command("git", "diff")
	cmd.Dir = projectDir
	unstaged, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}

	// Staged changes.
	cmd = exec.Command("git", "diff", "--cached")
	cmd.Dir = projectDir
	staged, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git diff --cached: %w", err)
	}

	var parts []string
	if len(staged) > 0 {
		parts = append(parts, "=== STAGED ===\n"+string(staged))
	}
	if len(unstaged) > 0 {
		parts = append(parts, "=== UNSTAGED ===\n"+string(unstaged))
	}
	if len(parts) == 0 {
		return "(no changes)", nil
	}
	return strings.Join(parts, "\n"), nil
}

// loadProjectRules tries to load AGENTS.md or CLAUDE.md from the project root.
func loadProjectRules(projectDir string) ([]byte, error) {
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		data, err := loadArtifact(projectDir, name)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("no project rules file found")
}
