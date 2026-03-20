package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// buildSummary scans existing artifacts in changeDir and produces a compact
// cumulative context (~500-800 bytes) that carries key decisions forward
// through the pipeline. Non-fatal: returns empty string if no artifacts exist.
func buildSummary(changeDir string, p *Params) string {
	var sections []string

	sections = append(sections, fmt.Sprintf("Change: %s — %s", p.ChangeName, p.Description))
	sections = append(sections, fmt.Sprintf("Stack: %s (%s)", p.Config.Stack.Language, p.Config.Stack.BuildTool))

	// Extract key lines from each artifact if it exists.
	if data, err := os.ReadFile(filepath.Join(changeDir, "exploration.md")); err == nil {
		if finding := extractFirst(string(data), "##", 3); finding != "" {
			sections = append(sections, "Exploration: "+finding)
		}
	}

	if data, err := os.ReadFile(filepath.Join(changeDir, "proposal.md")); err == nil {
		if intent := extractFirst(string(data), "##", 3); intent != "" {
			sections = append(sections, "Proposal: "+intent)
		}
	}

	if data, err := os.ReadFile(filepath.Join(changeDir, "design.md")); err == nil {
		if decision := extractFirst(string(data), "##", 3); decision != "" {
			sections = append(sections, "Design: "+decision)
		}
	}

	if data, err := os.ReadFile(filepath.Join(changeDir, "review-report.md")); err == nil {
		if verdict := extractFirst(string(data), "Verdict", 1); verdict != "" {
			sections = append(sections, "Review: "+verdict)
		}
	}

	if len(sections) == 0 {
		return ""
	}

	return strings.Join(sections, "\n")
}

// extractFirst finds the first line containing keyword after the first heading,
// then returns up to n non-empty content lines following it.
// Used to pull key decisions from artifacts without loading the entire file.
func extractFirst(content, keyword string, maxLines int) string {
	lines := strings.Split(content, "\n")
	var result []string

	collecting := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if collecting && len(result) > 0 {
				continue // skip blanks inside collected range
			}
			continue
		}

		if !collecting && strings.Contains(trimmed, keyword) {
			collecting = true
			// Don't include the header itself — get content after it.
			continue
		}

		if collecting {
			// Skip sub-headers.
			if strings.HasPrefix(trimmed, "#") {
				if len(result) > 0 {
					break // hit next section, stop
				}
				continue
			}
			result = append(result, trimmed)
			if len(result) >= maxLines {
				break
			}
		}
	}

	return strings.Join(result, " ")
}

// projectContext returns a compact project overview string with stack info.
func projectContext(p *Params) string {
	return fmt.Sprintf(
		"Project: %s\nLanguage: %s\nBuild Tool: %s\nManifests: %s",
		p.Config.ProjectName,
		p.Config.Stack.Language,
		p.Config.Stack.BuildTool,
		strings.Join(p.Config.Stack.Manifests, ", "),
	)
}

// loadManifestContents reads the actual content of detected manifest files.
// Returns a compact summary with versions and dependencies.
func loadManifestContents(projectDir string, manifests []string) string {
	var parts []string
	for _, m := range manifests {
		data, err := os.ReadFile(filepath.Join(projectDir, m))
		if err != nil {
			continue
		}
		// Cap at 2KB per manifest to keep context lean.
		content := string(data)
		if len(content) > 2048 {
			content = content[:2048] + "\n... (truncated)"
		}
		parts = append(parts, fmt.Sprintf("### %s\n\n```\n%s\n```", m, content))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// extractCompletedTasks returns a summary of completed task sections.
func extractCompletedTasks(tasks string) string {
	lines := strings.Split(tasks, "\n")
	var completed []string
	var currentSection string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "##") {
			currentSection = trimmed
			continue
		}
		if strings.HasPrefix(trimmed, "- [x]") {
			task := strings.TrimPrefix(trimmed, "- [x] ")
			if currentSection != "" {
				completed = append(completed, fmt.Sprintf("%s: %s", currentSection, task))
			} else {
				completed = append(completed, task)
			}
		}
	}

	if len(completed) == 0 {
		return "(no tasks completed yet)"
	}
	return strings.Join(completed, "\n")
}
