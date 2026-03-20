package context

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// AssembleExplore builds context for the explore phase.
// Includes: file tree (via git ls-files), config summary, sdd-explore SKILL.md.
func AssembleExplore(w io.Writer, p *Params) error {
	// Load skill.
	skill, err := loadSkill(p.SkillsPath, "sdd-explore")
	if err != nil {
		return err
	}

	// Get file tree.
	fileTree, err := gitFileTree(p.ProjectDir)
	if err != nil {
		// Fallback: note that git ls-files failed.
		fileTree = fmt.Sprintf("(git ls-files unavailable: %v)", err)
	}

	// Write assembled context.
	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "PROJECT", fmt.Sprintf(
		"Name: %s\nLanguage: %s\nBuild Tool: %s\nManifests: %s",
		p.Config.ProjectName,
		p.Config.Stack.Language,
		p.Config.Stack.BuildTool,
		strings.Join(p.Config.Stack.Manifests, ", "),
	))

	if p.Description != "" {
		writeSectionStr(w, "CHANGE", fmt.Sprintf(
			"Name: %s\nDescription: %s",
			p.ChangeName, p.Description,
		))
	}

	writeSectionStr(w, "FILE TREE", fileTree)

	// Load actual manifest contents for dependency/version context.
	if manifests := loadManifestContents(p.ProjectDir, p.Config.Stack.Manifests); manifests != "" {
		writeSectionStr(w, "MANIFESTS", manifests)
	}

	return nil
}

// gitFileTree runs git ls-files and returns the output.
func gitFileTree(projectDir string) (string, error) {
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-files: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
