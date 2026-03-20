// Package verify runs build, lint, and test commands as a quality gate.
package verify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// DefaultTimeout is the per-command timeout.
const DefaultTimeout = 5 * time.Minute

// CommandResult captures the outcome of a single command execution.
type CommandResult struct {
	Name     string        `json:"name"`
	Command  string        `json:"command"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration"`
	ExitCode int           `json:"exit_code"`
	Output   string        `json:"output"` // combined stdout+stderr
	TimedOut bool          `json:"timed_out"`
}

// ErrorLines returns the first n lines from a failed command's output.
func (r *CommandResult) ErrorLines(n int) []string {
	if r.Passed || r.Output == "" {
		return nil
	}
	// SplitN avoids allocating slices for all lines when only n are needed.
	lines := strings.SplitN(strings.TrimRight(r.Output, "\n"), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return lines
}

// Report is the aggregate result of all verification commands.
type Report struct {
	Timestamp time.Time        `json:"timestamp"`
	Passed    bool             `json:"passed"`
	Results   []*CommandResult `json:"results"`
}

// FailedCount returns the number of failed commands.
func (r *Report) FailedCount() int {
	count := 0
	for _, res := range r.Results {
		if !res.Passed {
			count++
		}
	}
	return count
}

// CommandSpec defines a single command to run.
type CommandSpec struct {
	Name    string // human label: "build", "test", "lint"
	Command string // shell command: "go test ./..."
}

// Run executes each command sequentially in workDir, stopping on first failure.
// Empty command strings are skipped. Progress is reported to progress writer
// (nil = silent).
func Run(workDir string, commands []CommandSpec, timeout time.Duration, progress io.Writer) (*Report, error) {
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	report := &Report{
		Timestamp: time.Now().UTC(),
		Passed:    true,
	}

	for _, spec := range commands {
		if spec.Command == "" {
			continue
		}

		if progress != nil {
			fmt.Fprintf(progress, "sdd: verify %s...\n", spec.Name)
		}

		result := runOne(workDir, spec, timeout)
		report.Results = append(report.Results, result)

		if progress != nil {
			if result.Passed {
				fmt.Fprintf(progress, "sdd: verify %s: ok (%s)\n", spec.Name, result.Duration.Round(time.Millisecond))
			} else if result.TimedOut {
				fmt.Fprintf(progress, "sdd: verify %s: TIMEOUT (%s)\n", spec.Name, timeout)
			} else {
				fmt.Fprintf(progress, "sdd: verify %s: FAILED (exit %d)\n", spec.Name, result.ExitCode)
			}
		}

		if !result.Passed {
			report.Passed = false
			break // stop on first failure
		}
	}

	return report, nil
}

// runOne executes a single command with timeout.
// Uses process groups so timeout kills the entire process tree (sh + children).
func runOne(workDir string, spec CommandSpec, timeout time.Duration) *CommandResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "sh", "-c", spec.Command)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Kill the process group on context cancellation so child processes die too.
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	elapsed := time.Since(start)

	result := &CommandResult{
		Name:     spec.Name,
		Command:  spec.Command,
		Duration: elapsed,
		Output:   buf.String(),
	}

	if err == nil {
		result.Passed = true
		result.ExitCode = 0
		return result
	}

	// Check timeout.
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Passed = false
		result.ExitCode = -1
		result.Output = fmt.Sprintf("command timed out after %s\n%s", timeout, buf.String())
		return result
	}

	// Non-zero exit.
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.ExitCode = -1
	}
	result.Passed = false
	return result
}

// maxErrorLines is the number of error lines included in failure reports.
const maxErrorLines = 30

// WriteReport writes verify-report.md into changeDir.
func WriteReport(report *Report, changeDir string) error {
	path := filepath.Join(changeDir, "verify-report.md")

	var buf bytes.Buffer
	buf.WriteString("# Verify Report\n\n")
	buf.WriteString(fmt.Sprintf("**Timestamp:** %s\n\n", report.Timestamp.Format(time.RFC3339)))

	if report.Passed {
		buf.WriteString("**Status:** PASSED\n\n")
		buf.WriteString("All commands passed.\n\n")
	} else {
		buf.WriteString(fmt.Sprintf("**Status:** FAILED (%d command(s) failed)\n\n", report.FailedCount()))
	}

	for _, res := range report.Results {
		icon := "PASS"
		if !res.Passed {
			icon = "FAIL"
		}
		buf.WriteString(fmt.Sprintf("## %s — %s\n\n", res.Name, icon))
		buf.WriteString(fmt.Sprintf("- **Command:** `%s`\n", res.Command))
		buf.WriteString(fmt.Sprintf("- **Duration:** %s\n", res.Duration.Round(time.Millisecond)))
		buf.WriteString(fmt.Sprintf("- **Exit code:** %d\n", res.ExitCode))

		if res.TimedOut {
			buf.WriteString("- **Timed out:** yes\n")
		}

		if !res.Passed {
			lines := res.ErrorLines(maxErrorLines)
			if len(lines) > 0 {
				buf.WriteString("\n**Error output:**\n\n```\n")
				for i, line := range lines {
					buf.WriteString(fmt.Sprintf("%3d: %s\n", i+1, line))
				}
				buf.WriteString("```\n")
			}
		}
		buf.WriteString("\n")
	}

	// Atomic write: temp file + rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write verify report: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename verify report: %w", err)
	}
	return nil
}
