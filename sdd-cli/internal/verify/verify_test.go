package verify

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun_AllPass(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	commands := []CommandSpec{
		{Name: "build", Command: "echo build-ok"},
		{Name: "lint", Command: "echo lint-ok"},
		{Name: "test", Command: "echo test-ok"},
	}

	report, err := Run(dir, commands, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Passed {
		t.Fatal("expected report.Passed to be true")
	}
	if len(report.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(report.Results))
	}
	for _, r := range report.Results {
		if !r.Passed {
			t.Errorf("expected %s to pass", r.Name)
		}
		if r.ExitCode != 0 {
			t.Errorf("expected exit code 0 for %s, got %d", r.Name, r.ExitCode)
		}
	}
}

func TestRun_OneFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	commands := []CommandSpec{
		{Name: "build", Command: "echo build-ok"},
		{Name: "lint", Command: "echo 'lint error: unused var' >&2; exit 1"},
		{Name: "test", Command: "echo test-ok"},
	}

	report, err := Run(dir, commands, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Passed {
		t.Fatal("expected report.Passed to be false")
	}
	// Should stop after lint failure — test never runs.
	if len(report.Results) != 2 {
		t.Fatalf("expected 2 results (stopped on failure), got %d", len(report.Results))
	}
	if !report.Results[0].Passed {
		t.Error("expected build to pass")
	}
	if report.Results[1].Passed {
		t.Error("expected lint to fail")
	}
	if report.Results[1].ExitCode != 1 {
		t.Errorf("expected exit code 1 for lint, got %d", report.Results[1].ExitCode)
	}
}

func TestRun_Timeout(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	commands := []CommandSpec{
		{Name: "hang", Command: "sleep 60"},
	}

	report, err := Run(dir, commands, 500*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Passed {
		t.Fatal("expected report.Passed to be false")
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	r := report.Results[0]
	if !r.TimedOut {
		t.Error("expected TimedOut to be true")
	}
	if r.Passed {
		t.Error("expected command to fail on timeout")
	}
}

func TestRun_SkipsEmptyCommands(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	commands := []CommandSpec{
		{Name: "build", Command: ""},
		{Name: "test", Command: "echo ok"},
	}

	report, err := Run(dir, commands, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Passed {
		t.Fatal("expected report.Passed to be true")
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result (empty skipped), got %d", len(report.Results))
	}
}

func TestErrorLines(t *testing.T) {
	t.Parallel()

	r := &CommandResult{
		Passed: false,
		Output: "line1\nline2\nline3\nline4\nline5\n",
	}

	lines := r.ErrorLines(3)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line1" || lines[2] != "line3" {
		t.Errorf("unexpected lines: %v", lines)
	}

	// Passed command returns nil.
	r2 := &CommandResult{Passed: true, Output: "something"}
	if r2.ErrorLines(5) != nil {
		t.Error("expected nil for passed command")
	}
}

func TestWriteReport_Pass(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	report := &Report{
		Timestamp: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Passed:    true,
		Results: []*CommandResult{
			{Name: "build", Command: "go build ./...", Passed: true, Duration: 2 * time.Second, ExitCode: 0},
			{Name: "test", Command: "go test ./...", Passed: true, Duration: 5 * time.Second, ExitCode: 0},
		},
	}

	if err := WriteReport(report, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "verify-report.md"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "PASSED") {
		t.Error("expected report to contain PASSED")
	}
	if !strings.Contains(content, "All commands passed") {
		t.Error("expected report to contain 'All commands passed'")
	}
	if !strings.Contains(content, "build — PASS") {
		t.Error("expected report to contain 'build — PASS'")
	}
}

func TestWriteReport_Fail(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	report := &Report{
		Timestamp: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Passed:    false,
		Results: []*CommandResult{
			{Name: "build", Command: "go build ./...", Passed: true, Duration: 2 * time.Second, ExitCode: 0},
			{Name: "lint", Command: "golangci-lint run", Passed: false, Duration: 3 * time.Second, ExitCode: 1, Output: "main.go:10: unused variable\nmain.go:20: missing error check\n"},
		},
	}

	if err := WriteReport(report, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "verify-report.md"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "FAILED") {
		t.Error("expected report to contain FAILED")
	}
	if !strings.Contains(content, "lint — FAIL") {
		t.Error("expected report to contain 'lint — FAIL'")
	}
	if !strings.Contains(content, "unused variable") {
		t.Error("expected report to contain error output")
	}
}

func TestRun_ProgressOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	commands := []CommandSpec{
		{Name: "build", Command: "echo build-ok"},
		{Name: "lint", Command: "echo 'fail' >&2; exit 1"},
	}

	var progress bytes.Buffer
	_, err := Run(dir, commands, 30*time.Second, &progress)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := progress.String()
	if !strings.Contains(out, "sdd: verify build...") {
		t.Error("missing build start progress")
	}
	if !strings.Contains(out, "sdd: verify build: ok") {
		t.Error("missing build ok progress")
	}
	if !strings.Contains(out, "sdd: verify lint...") {
		t.Error("missing lint start progress")
	}
	if !strings.Contains(out, "sdd: verify lint: FAILED (exit 1)") {
		t.Error("missing lint failed progress")
	}
}

func TestArchive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Set up: openspec/changes/my-feature/ with some artifacts.
	changeDir := filepath.Join(root, "openspec", "changes", "my-feature")
	specsDir := filepath.Join(changeDir, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write some artifacts.
	artifacts := map[string]string{
		"state.json":       `{"name":"my-feature"}`,
		"exploration.md":   "# Exploration",
		"proposal.md":      "# Proposal",
		"design.md":        "# Design",
		"tasks.md":         "# Tasks",
		"verify-report.md": "# Verify Report\n\nPASSED",
	}
	for name, content := range artifacts {
		if err := os.WriteFile(filepath.Join(changeDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Spec file.
	if err := os.WriteFile(filepath.Join(specsDir, "api.md"), []byte("# API Spec"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run archive.
	result, err := Archive(changeDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original directory should be gone.
	if _, err := os.Stat(changeDir); !os.IsNotExist(err) {
		t.Error("expected change directory to be moved")
	}

	// Archive directory should exist.
	if _, err := os.Stat(result.ArchivePath); err != nil {
		t.Fatalf("archive directory not found: %v", err)
	}

	// Check it's under archive/ with timestamp prefix.
	archiveBase := filepath.Base(result.ArchivePath)
	if !strings.HasSuffix(archiveBase, "-my-feature") {
		t.Errorf("expected archive name to end with -my-feature, got %s", archiveBase)
	}

	// Artifacts should be preserved.
	for name := range artifacts {
		path := filepath.Join(result.ArchivePath, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected artifact %s to exist in archive: %v", name, err)
		}
	}

	// Manifest should exist.
	manifestData, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest := string(manifestData)

	if !strings.Contains(manifest, "my-feature") {
		t.Error("expected manifest to contain change name")
	}
	if !strings.Contains(manifest, "specs/") {
		t.Error("expected manifest to list specs directory")
	}
	if !strings.Contains(manifest, "exploration.md") {
		t.Error("expected manifest to list exploration.md")
	}
}
