package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/artifacts"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	sddctx "github.com/rechedev9/shenronSDD/sdd-cli/internal/context"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/verify"
)

// staleThreshold is the duration after which a change is considered abandoned.
// Changes inactive longer than this are flagged as stale.
const staleThreshold = 24 * time.Hour

func runInit(args []string, stdout io.Writer, stderr io.Writer) error {
	force := false
	projectDir := "."

	for _, arg := range args {
		switch {
		case arg == "--force" || arg == "-f":
			force = true
		case !strings.HasPrefix(arg, "-"):
			projectDir = arg
		default:
			return errs.Usage(fmt.Sprintf("unknown flag: %s", arg))
		}
	}

	// Resolve to absolute path.
	abs, err := resolveDir(projectDir)
	if err != nil {
		return errs.WriteError(stderr, "init", err)
	}

	result, err := config.Init(abs, force)
	if err != nil {
		return errs.WriteError(stderr, "init", err)
	}

	// JSON output on stdout for machine consumption.
	out := struct {
		Command    string         `json:"command"`
		Status     string         `json:"status"`
		ConfigPath string         `json:"config_path"`
		Dirs       []string       `json:"dirs"`
		Config     *config.Config `json:"config"`
	}{
		Command:    "init",
		Status:     "success",
		ConfigPath: result.ConfigPath,
		Dirs:       result.Dirs,
		Config:     result.Config,
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))
	return nil
}

func resolveDir(dir string) (string, error) {
	abs, err := os.Getwd()
	if dir != "." {
		abs, err = filepath.Abs(dir)
	}
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	return abs, nil
}

func runNew(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 2 {
		return errs.Usage("usage: sdd new <name> <description>")
	}

	name := args[0]
	description := args[1]

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "new", fmt.Errorf("get working directory: %w", err))
	}

	// Load config.
	configPath := filepath.Join(cwd, "openspec", "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return errs.WriteError(stderr, "new", fmt.Errorf("load config (run 'sdd init' first): %w", err))
	}

	// Create change directory.
	changeDir := filepath.Join(cwd, "openspec", "changes", name)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		return errs.WriteError(stderr, "new", fmt.Errorf("create change dir: %w", err))
	}

	// Create initial state.
	st := state.NewState(name, description)
	statePath := filepath.Join(changeDir, "state.json")
	if err := state.Save(st, statePath); err != nil {
		return errs.WriteError(stderr, "new", err)
	}

	// Capture git HEAD for diff support. Non-fatal: not all projects use git.
	if sha, err := gitHeadSHA(cwd); err == nil {
		st.BaseRef = sha
		_ = state.Save(st, statePath) // best-effort re-save
	}

	// Run explore assembler to stdout.
	p := &sddctx.Params{
		ChangeDir:   changeDir,
		ChangeName:  name,
		Description: description,
		ProjectDir:  cwd,
		Config:      cfg,
		SkillsPath:  cfg.SkillsPath,
		Stderr:      stderr,
	}

	if err := sddctx.Assemble(stdout, state.PhaseExplore, p); err != nil {
		// Non-fatal: context assembly failure doesn't block change creation.
		fmt.Fprintf(stderr, "warning: explore context assembly failed: %v\n", err)
	}

	return nil
}

func runContext(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd context <name> [phase]")
	}

	name := args[0]

	changeDir, err := resolveChangeDir(name)
	if err != nil {
		return errs.WriteError(stderr, "context", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "context", fmt.Errorf("get working directory: %w", err))
	}

	// Load config.
	configPath := filepath.Join(cwd, "openspec", "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return errs.WriteError(stderr, "context", fmt.Errorf("load config: %w", err))
	}

	// Load state.
	statePath := filepath.Join(changeDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		return errs.WriteError(stderr, "context", fmt.Errorf("load state: %w", err))
	}

	p := &sddctx.Params{
		ChangeDir:   changeDir,
		ChangeName:  st.Name,
		Description: st.Description,
		ProjectDir:  cwd,
		Config:      cfg,
		SkillsPath:  cfg.SkillsPath,
		Stderr:      stderr,
	}

	// Explicit phase arg → single assembly.
	if len(args) >= 2 {
		phase := state.Phase(args[1])
		if err := sddctx.Assemble(stdout, phase, p); err != nil {
			return errs.WriteError(stderr, "context", err)
		}
		return nil
	}

	// Auto-resolve: check if multiple phases are ready (spec+design parallel window).
	ready := st.ReadyPhases()
	if len(ready) == 0 {
		return errs.WriteError(stderr, "context", fmt.Errorf("no phases ready (pipeline complete or blocked)"))
	}
	if len(ready) > 1 {
		// Concurrent assembly for parallel phases (spec+design).
		if err := sddctx.AssembleConcurrent(stdout, ready, p); err != nil {
			return errs.WriteError(stderr, "context", err)
		}
		return nil
	}

	// Single phase ready.
	if err := sddctx.Assemble(stdout, ready[0], p); err != nil {
		return errs.WriteError(stderr, "context", err)
	}
	return nil
}

func runWrite(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 2 {
		return errs.Usage("usage: sdd write <name> <phase>")
	}

	name := args[0]
	phaseStr := args[1]
	phase := state.Phase(phaseStr)

	// Resolve change directory.
	changeDir, err := resolveChangeDir(name)
	if err != nil {
		return errs.WriteError(stderr, "write", err)
	}

	// Load state.
	statePath := filepath.Join(changeDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		return errs.WriteError(stderr, "write", fmt.Errorf("load state: %w", err))
	}

	// Promote pending artifact.
	promoted, err := artifacts.Promote(changeDir, phase)
	if err != nil {
		return errs.WriteError(stderr, "write", err)
	}

	// Advance state.
	if err := st.Advance(phase); err != nil {
		return errs.WriteError(stderr, "write", fmt.Errorf("advance state: %w", err))
	}

	// Save state.
	if err := state.Save(st, statePath); err != nil {
		return errs.WriteError(stderr, "write", err)
	}

	out := struct {
		Command      string `json:"command"`
		Status       string `json:"status"`
		Change       string `json:"change"`
		Phase        string `json:"phase"`
		PromotedTo   string `json:"promoted_to"`
		CurrentPhase string `json:"current_phase"`
	}{
		Command:      "write",
		Status:       "success",
		Change:       name,
		Phase:        phaseStr,
		PromotedTo:   promoted,
		CurrentPhase: string(st.CurrentPhase),
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))
	return nil
}

func resolveChangeDir(name string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	changeDir := filepath.Join(cwd, "openspec", "changes", name)
	info, err := os.Stat(changeDir)
	if err != nil {
		return "", fmt.Errorf("change directory not found: %s", changeDir)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", changeDir)
	}
	return changeDir, nil
}

func runStatus(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd status <name>")
	}

	name := args[0]

	changeDir, err := resolveChangeDir(name)
	if err != nil {
		return errs.WriteError(stderr, "status", err)
	}

	statePath := filepath.Join(changeDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		return errs.WriteError(stderr, "status", fmt.Errorf("load state: %w", err))
	}

	// Build phase list with statuses.
	type phaseInfo struct {
		Phase  string `json:"phase"`
		Status string `json:"status"`
	}
	phases := make([]phaseInfo, 0, len(state.AllPhases()))
	var completed []string
	for _, p := range state.AllPhases() {
		ps := st.Phases[p]
		phases = append(phases, phaseInfo{Phase: string(p), Status: string(ps)})
		if ps == state.StatusCompleted {
			completed = append(completed, string(p))
		}
	}

	out := struct {
		Command      string      `json:"command"`
		Status       string      `json:"status"`
		Change       string      `json:"change"`
		Description  string      `json:"description"`
		CurrentPhase string      `json:"current_phase"`
		Completed    []string    `json:"completed"`
		Phases       []phaseInfo `json:"phases"`
		IsComplete   bool        `json:"is_complete"`
		UpdatedAt    string      `json:"updated_at"`
		Stale        bool        `json:"stale,omitempty"`
		StaleHours   int         `json:"stale_hours,omitempty"`
	}{
		Command:      "status",
		Status:       "success",
		Change:       st.Name,
		Description:  st.Description,
		CurrentPhase: string(st.CurrentPhase),
		Completed:    completed,
		Phases:       phases,
		IsComplete:   st.IsComplete(),
		UpdatedAt:    st.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		Stale:        st.IsStale(staleThreshold),
		StaleHours:   st.StaleHours(),
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))
	return nil
}

func runList(_ []string, stdout io.Writer, stderr io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "list", fmt.Errorf("get working directory: %w", err))
	}

	type changeInfo struct {
		Name         string `json:"name"`
		CurrentPhase string `json:"current_phase"`
		Description  string `json:"description"`
		IsComplete   bool   `json:"is_complete"`
		Stale        bool   `json:"stale,omitempty"`
	}

	var changes []changeInfo

	changesDir := filepath.Join(cwd, "openspec", "changes")
	entries, err := os.ReadDir(changesDir)
	if err != nil && !os.IsNotExist(err) {
		return errs.WriteError(stderr, "list", fmt.Errorf("read changes directory: %w", err))
	}

	for _, e := range entries {
		if !e.IsDir() || e.Name() == "archive" {
			continue
		}

		statePath := filepath.Join(changesDir, e.Name(), "state.json")
		st, err := state.Load(statePath)
		if err != nil {
			continue // skip entries without valid state
		}

		changes = append(changes, changeInfo{
			Name:         st.Name,
			CurrentPhase: string(st.CurrentPhase),
			Description:  st.Description,
			IsComplete:   st.IsComplete(),
			Stale:        st.IsStale(staleThreshold),
		})
	}

	out := struct {
		Command string       `json:"command"`
		Status  string       `json:"status"`
		Count   int          `json:"count"`
		Changes []changeInfo `json:"changes"`
	}{
		Command: "list",
		Status:  "success",
		Count:   len(changes),
		Changes: changes,
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))
	return nil
}

func runVerify(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd verify <name>")
	}

	name := args[0]

	changeDir, err := resolveChangeDir(name)
	if err != nil {
		return errs.WriteError(stderr, "verify", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "verify", fmt.Errorf("get working directory: %w", err))
	}

	// Load config for commands.
	configPath := filepath.Join(cwd, "openspec", "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return errs.WriteError(stderr, "verify", fmt.Errorf("load config: %w", err))
	}

	// Smart-skip: reuse last verify if no source files changed.
	if skip, _ := shouldSkipVerify(cwd, changeDir); skip {
		fmt.Fprintf(stderr, "sdd: verify skipped — no source changes since last PASS\n")
		out := struct {
			Command    string `json:"command"`
			Status     string `json:"status"`
			Change     string `json:"change"`
			Passed     bool   `json:"passed"`
			Skipped    bool   `json:"skipped,omitempty"`
			ReportPath string `json:"report_path"`
		}{
			Command:    "verify",
			Status:     "success",
			Change:     name,
			Passed:     true,
			Skipped:    true,
			ReportPath: filepath.Join(changeDir, "verify-report.md"),
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(stdout, string(data))
		return nil
	}

	// Build command list from config.
	commands := []verify.CommandSpec{
		{Name: "build", Command: cfg.Commands.Build},
		{Name: "lint", Command: cfg.Commands.Lint},
		{Name: "test", Command: cfg.Commands.Test},
	}

	// Run verification in the project root.
	report, err := verify.Run(cwd, commands, verify.DefaultTimeout, stderr)
	if err != nil {
		return errs.WriteError(stderr, "verify", fmt.Errorf("run verify: %w", err))
	}

	// Write report to change directory.
	if err := verify.WriteReport(report, changeDir); err != nil {
		return errs.WriteError(stderr, "verify", err)
	}

	// JSON output.
	out := struct {
		Command    string `json:"command"`
		Status     string `json:"status"`
		Change     string `json:"change"`
		Passed     bool   `json:"passed"`
		ReportPath string `json:"report_path"`
	}{
		Command:    "verify",
		Status:     "success",
		Change:     name,
		Passed:     report.Passed,
		ReportPath: filepath.Join(changeDir, "verify-report.md"),
	}

	if !report.Passed {
		out.Status = "failed"
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))

	if !report.Passed {
		return fmt.Errorf("verify: %d command(s) failed", report.FailedCount())
	}
	return nil
}

func runArchive(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd archive <name>")
	}

	name := args[0]

	// Resolve change directory.
	changeDir, err := resolveChangeDir(name)
	if err != nil {
		return errs.WriteError(stderr, "archive", err)
	}

	// Load state and verify pipeline is ready for archive.
	statePath := filepath.Join(changeDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		return errs.WriteError(stderr, "archive", fmt.Errorf("load state: %w", err))
	}

	if err := st.CanTransition(state.PhaseArchive); err != nil {
		return errs.WriteError(stderr, "archive", fmt.Errorf("not ready to archive: %w", err))
	}

	// Execute archive.
	result, err := verify.Archive(changeDir)
	if err != nil {
		return errs.WriteError(stderr, "archive", err)
	}

	// JSON output.
	out := struct {
		Command      string `json:"command"`
		Status       string `json:"status"`
		Change       string `json:"change"`
		ArchivePath  string `json:"archive_path"`
		ManifestPath string `json:"manifest_path"`
	}{
		Command:      "archive",
		Status:       "success",
		Change:       name,
		ArchivePath:  result.ArchivePath,
		ManifestPath: result.ManifestPath,
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))
	return nil
}

func runDiff(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd diff <name>")
	}

	name := args[0]

	changeDir, err := resolveChangeDir(name)
	if err != nil {
		return errs.WriteError(stderr, "diff", err)
	}

	statePath := filepath.Join(changeDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		return errs.WriteError(stderr, "diff", fmt.Errorf("load state: %w", err))
	}

	if st.BaseRef == "" {
		return errs.WriteError(stderr, "diff", fmt.Errorf("base_ref not recorded; change was created before diff support"))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "diff", fmt.Errorf("get working directory: %w", err))
	}

	files, err := gitDiffFiles(cwd, st.BaseRef)
	if err != nil {
		return errs.WriteError(stderr, "diff", fmt.Errorf("git diff: %w", err))
	}

	out := struct {
		Command string   `json:"command"`
		Status  string   `json:"status"`
		Change  string   `json:"change"`
		BaseRef string   `json:"base_ref"`
		Files   []string `json:"files"`
		Count   int      `json:"count"`
	}{
		Command: "diff",
		Status:  "success",
		Change:  name,
		BaseRef: st.BaseRef,
		Files:   files,
		Count:   len(files),
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))
	return nil
}

// gitHeadSHA returns the current HEAD SHA in dir.
func gitHeadSHA(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitDiffFiles returns files changed between ref and the working tree.
func gitDiffFiles(dir, ref string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("exec git: %w", err)
	}
	raw := strings.TrimRight(string(out), "\n")
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
}

// shouldSkipVerify returns true if verify can be skipped because:
// 1. verify-report.md exists and contains PASSED
// 2. No source files (excluding openspec/) changed since HEAD
// Returns (false, nil) on any error — never skips when unsure.
func shouldSkipVerify(cwd, changeDir string) (bool, error) {
	// Check existing report is PASSED.
	reportPath := filepath.Join(changeDir, "verify-report.md")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return false, nil // no report → can't skip
	}
	if !strings.Contains(string(data), "**Status:** PASSED") {
		return false, nil // last run failed → must re-verify
	}

	// Check for source file changes.
	files, err := gitDiffFiles(cwd, "HEAD")
	if err != nil {
		return false, nil // git error → don't skip
	}

	// Filter out openspec/ files — those aren't source code.
	for _, f := range files {
		if !strings.HasPrefix(f, "openspec/") {
			return false, nil // source file changed → must verify
		}
	}

	return true, nil // no source changes + last verify passed → skip
}

func runHealth(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd health <name>")
	}

	name := args[0]

	changeDir, err := resolveChangeDir(name)
	if err != nil {
		return errs.WriteError(stderr, "health", err)
	}

	statePath := filepath.Join(changeDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		return errs.WriteError(stderr, "health", fmt.Errorf("load state: %w", err))
	}

	// Count completed phases.
	var completed int
	for _, p := range state.AllPhases() {
		if st.Phases[p] == state.StatusCompleted {
			completed++
		}
	}

	// Load pipeline metrics.
	pm := sddctx.LoadPipelineMetrics(changeDir)

	// Build warnings.
	var warnings []string
	if st.IsStale(staleThreshold) {
		warnings = append(warnings, fmt.Sprintf("change inactive for %d hours", st.StaleHours()))
	}

	// Check if last verify failed.
	reportPath := filepath.Join(changeDir, "verify-report.md")
	if data, err := os.ReadFile(reportPath); err == nil {
		if strings.Contains(string(data), "**Status:** FAILED") {
			warnings = append(warnings, "last verify FAILED")
		}
	}

	out := struct {
		Command      string   `json:"command"`
		Status       string   `json:"status"`
		Change       string   `json:"change"`
		CurrentPhase string   `json:"current_phase"`
		Completed    int      `json:"completed"`
		TotalPhases  int      `json:"total_phases"`
		CacheHits    int      `json:"cache_hits"`
		CacheMisses  int      `json:"cache_misses"`
		TotalTokens  int      `json:"total_tokens"`
		Stale        bool     `json:"stale,omitempty"`
		StaleHours   int      `json:"stale_hours,omitempty"`
		Warnings     []string `json:"warnings,omitempty"`
	}{
		Command:      "health",
		Status:       "success",
		Change:       st.Name,
		CurrentPhase: string(st.CurrentPhase),
		Completed:    completed,
		TotalPhases:  len(state.AllPhases()),
		CacheHits:    pm.CacheHits,
		CacheMisses:  pm.CacheMisses,
		TotalTokens:  pm.TotalTokens,
		Stale:        st.IsStale(staleThreshold),
		StaleHours:   st.StaleHours(),
		Warnings:     warnings,
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))
	return nil
}
