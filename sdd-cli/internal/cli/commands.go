package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/artifacts"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	sddctx "github.com/rechedev9/shenronSDD/sdd-cli/internal/context"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/dashboard"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/errlog"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/events"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/verify"
)

// newBroker creates and wires a broker with default subscribers.
// db may be nil — SQLite subscribers are skipped when nil.
func newBroker(stderr io.Writer, verbosity int, db *store.Store) *events.Broker {
	broker := events.NewBroker()
	sddctx.RegisterSubscribers(broker, stderr, verbosity)
	store.RegisterSubscribers(broker, db)
	return broker
}

// tryOpenStore opens the SQLite store best-effort. Returns nil if unavailable.
func tryOpenStore(cwd string) *store.Store {
	path := filepath.Join(cwd, "openspec", ".cache", "sdd.db")
	db, err := store.Open(path)
	if err != nil {
		return nil
	}
	return db
}

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
	args, verbosity := ParseVerbosityFlags(args)
	jsonOut := false
	var positional []string
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOut = true
		case !strings.HasPrefix(arg, "-"):
			positional = append(positional, arg)
		default:
			return errs.Usage(fmt.Sprintf("unknown flag: %s", arg))
		}
	}

	if len(positional) < 2 {
		return errs.Usage("usage: sdd new <name> <description> [--json]")
	}

	name := positional[0]
	description := positional[1]

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

	if jsonOut {
		out := struct {
			Command      string `json:"command"`
			Status       string `json:"status"`
			Change       string `json:"change"`
			Description  string `json:"description"`
			ChangeDir    string `json:"change_dir"`
			CurrentPhase string `json:"current_phase"`
		}{
			Command:      "new",
			Status:       "success",
			Change:       name,
			Description:  description,
			ChangeDir:    changeDir,
			CurrentPhase: string(state.PhaseExplore),
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(stdout, string(data))
		return nil
	}

	// Run explore assembler to stdout.
	db := tryOpenStore(cwd)
	if db != nil {
		defer db.Close()
	}
	broker := newBroker(stderr, int(verbosity), db)
	p := &sddctx.Params{
		ChangeDir:   changeDir,
		ChangeName:  name,
		Description: description,
		ProjectDir:  cwd,
		Config:      cfg,
		SkillsPath:  cfg.SkillsPath,
		Stderr:      stderr,
		Verbosity:   int(verbosity),
		Broker:      broker,
	}

	if err := sddctx.Assemble(stdout, state.PhaseExplore, p); err != nil {
		// Non-fatal: context assembly failure doesn't block change creation.
		slog.Warn("explore context assembly failed", "error", err)
	}

	return nil
}

func runContext(args []string, stdout io.Writer, stderr io.Writer) error {
	args, verbosity := ParseVerbosityFlags(args)
	jsonOut := false
	var positional []string
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOut = true
		case !strings.HasPrefix(arg, "-"):
			positional = append(positional, arg)
		default:
			return errs.Usage(fmt.Sprintf("unknown flag: %s", arg))
		}
	}

	if len(positional) < 1 {
		return errs.Usage("usage: sdd context <name> [phase] [--json]")
	}

	name := positional[0]

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

	db := tryOpenStore(cwd)
	if db != nil {
		defer db.Close()
	}
	broker := newBroker(stderr, int(verbosity), db)
	p := &sddctx.Params{
		ChangeDir:   changeDir,
		ChangeName:  st.Name,
		Description: st.Description,
		ProjectDir:  cwd,
		Config:      cfg,
		SkillsPath:  cfg.SkillsPath,
		Stderr:      stderr,
		Verbosity:   int(verbosity),
		Broker:      broker,
	}

	// Choose target writer: buffer for JSON mode, stdout otherwise.
	var target io.Writer
	var buf *bytes.Buffer
	if jsonOut {
		buf = &bytes.Buffer{}
		target = buf
	} else {
		target = stdout
	}

	// Determine phase and assemble.
	var phase string
	if len(positional) >= 2 {
		// Explicit phase arg → single assembly.
		ph, err := state.ResolvePhase(positional[1])
		if err != nil {
			return errs.WriteError(stderr, "context", err)
		}
		phase = positional[1]
		if err := sddctx.Assemble(target, ph, p); err != nil {
			return errs.WriteError(stderr, "context", err)
		}
	} else {
		// Auto-resolve: check if multiple phases are ready (spec+design parallel window).
		ready := st.ReadyPhases()
		if len(ready) == 0 {
			return errs.WriteError(stderr, "context", fmt.Errorf("no phases ready (pipeline complete or blocked)"))
		}
		if len(ready) > 1 {
			// Concurrent assembly for parallel phases (spec+design).
			var names []string
			for _, r := range ready {
				names = append(names, string(r))
			}
			phase = strings.Join(names, "+")
			if err := sddctx.AssembleConcurrent(target, ready, p); err != nil {
				return errs.WriteError(stderr, "context", err)
			}
		} else {
			phase = string(ready[0])
			if err := sddctx.Assemble(target, ready[0], p); err != nil {
				return errs.WriteError(stderr, "context", err)
			}
		}
	}

	if jsonOut {
		content := buf.String()
		out := struct {
			Command string `json:"command"`
			Status  string `json:"status"`
			Change  string `json:"change"`
			Phase   string `json:"phase"`
			Context string `json:"context"`
			Bytes   int    `json:"bytes"`
			Tokens  int    `json:"tokens"`
		}{
			Command: "context",
			Status:  "success",
			Change:  name,
			Phase:   phase,
			Context: content,
			Bytes:   len(content),
			Tokens:  len(content) / 4,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(stdout, string(data))
	}

	return nil
}

func runWrite(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 2 {
		return errs.Usage("usage: sdd write <name> <phase>")
	}

	name := args[0]
	phaseStr := args[1]
	phase, err := state.ResolvePhase(phaseStr)
	if err != nil {
		return errs.WriteError(stderr, "write", err)
	}

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

	cwd, _ := os.Getwd()
	db := tryOpenStore(cwd)
	if db != nil {
		defer db.Close()
	}
	broker := newBroker(stderr, 0, db)
	prevPhase := string(st.CurrentPhase)

	// Promote pending artifact.
	promoted, err := artifacts.Promote(changeDir, phase)
	if err != nil {
		return errs.WriteError(stderr, "write", err)
	}

	broker.Emit(events.Event{
		Type: events.ArtifactPromoted,
		Payload: events.ArtifactPromotedPayload{
			Change:     name,
			Phase:      string(phase),
			PromotedTo: promoted,
		},
	})

	// Advance state.
	if err := st.Advance(phase); err != nil {
		return errs.WriteError(stderr, "write", fmt.Errorf("advance state: %w", err))
	}

	// Save state.
	if err := state.Save(st, statePath); err != nil {
		return errs.WriteError(stderr, "write", err)
	}

	broker.Emit(events.Event{
		Type: events.StateAdvanced,
		Payload: events.StateAdvancedPayload{
			Change:    name,
			FromPhase: prevPhase,
			ToPhase:   string(st.CurrentPhase),
		},
	})

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
		slog.Info("verify skipped", "reason", "no source changes since last PASS")
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

	// Emit VerifyFailed event for error collection.
	if !report.Passed {
		db := tryOpenStore(cwd)
		if db != nil {
			defer db.Close()
		}
		broker := newBroker(stderr, 0, db)
		var failedCmds []events.VerifyFailedCommand
		for _, r := range report.Results {
			if !r.Passed {
				failedCmds = append(failedCmds, events.VerifyFailedCommand{
					Name:       r.Name,
					Command:    r.Command,
					ExitCode:   r.ExitCode,
					ErrorLines: r.ErrorLines(5),
				})
			}
		}
		broker.Emit(events.Event{
			Type:    events.VerifyFailed,
			Payload: events.VerifyFailedPayload{Change: name, Results: failedCmds},
		})
	}

	if !report.Passed {
		return fmt.Errorf("verify: %d command(s) failed", report.FailedCount())
	}
	return nil
}

func runArchive(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd archive <name> [--force]")
	}

	name := args[0]
	force := false
	for _, arg := range args[1:] {
		switch arg {
		case "--force", "-f":
			force = true
		}
	}

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
		if !force {
			return errs.WriteError(stderr, "archive", fmt.Errorf("not ready to archive: %w", err))
		}
		slog.Warn("archive --force: skipping prerequisite check", "error", err)
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

func runErrors(args []string, stdout io.Writer, stderr io.Writer) error {
	jsonOut := false
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOut = true
		default:
			return errs.Usage(fmt.Sprintf("unknown flag: %s", arg))
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "errors", fmt.Errorf("get working directory: %w", err))
	}

	log := errlog.Load(cwd)

	if jsonOut {
		type errorGroup struct {
			Fingerprint string   `json:"fingerprint"`
			Count       int      `json:"count"`
			Command     string   `json:"command"`
			LastSeen    string   `json:"last_seen"`
			ErrorLines  []string `json:"error_lines"`
		}
		groups := make(map[string]*errorGroup)
		for _, e := range log.Entries {
			g, ok := groups[e.Fingerprint]
			if !ok {
				g = &errorGroup{
					Fingerprint: e.Fingerprint,
					Command:     e.Command,
					ErrorLines:  e.ErrorLines,
				}
				groups[e.Fingerprint] = g
			}
			g.Count++
			if e.Timestamp > g.LastSeen {
				g.LastSeen = e.Timestamp
				g.ErrorLines = e.ErrorLines
			}
		}

		sorted := make([]*errorGroup, 0, len(groups))
		for _, g := range groups {
			sorted = append(sorted, g)
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Count > sorted[j].Count
		})

		out := struct {
			Command string        `json:"command"`
			Status  string        `json:"status"`
			Total   int           `json:"total"`
			Groups  []*errorGroup `json:"groups"`
		}{
			Command: "errors",
			Status:  "success",
			Total:   len(log.Entries),
			Groups:  sorted,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(stdout, string(data))
		return nil
	}

	if len(log.Entries) == 0 {
		fmt.Fprintln(stdout, "sdd errors: no recorded errors")
		return nil
	}

	counts := log.RecurringFingerprints(1)
	fmt.Fprintf(stdout, "sdd errors: %d entries, %d unique patterns\n\n", len(log.Entries), len(counts))
	start := 0
	if len(log.Entries) > 10 {
		start = len(log.Entries) - 10
	}
	for _, e := range log.Entries[start:] {
		fp := e.Fingerprint
		if len(fp) > 8 {
			fp = fp[:8]
		}
		fmt.Fprintf(stdout, "  %s  %-8s  exit=%d  %s  [%s]\n",
			e.Timestamp[:19], e.CommandName, e.ExitCode, e.Change, fp)
	}
	return nil
}

func runDump(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd dump <name>")
	}

	name := args[0]

	changeDir, err := resolveChangeDir(name)
	if err != nil {
		return errs.WriteError(stderr, "dump", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "dump", fmt.Errorf("get working directory: %w", err))
	}

	// Load state.
	statePath := filepath.Join(changeDir, "state.json")
	st, err := state.Load(statePath)
	if err != nil {
		return errs.WriteError(stderr, "dump", fmt.Errorf("load state: %w", err))
	}

	// Load config.
	configPath := filepath.Join(cwd, "openspec", "config.yaml")
	cfg, err := config.Load(configPath)
	if err != nil {
		return errs.WriteError(stderr, "dump", fmt.Errorf("load config: %w", err))
	}

	// List artifacts.
	arts, err := artifacts.List(changeDir)
	if err != nil {
		return errs.WriteError(stderr, "dump", fmt.Errorf("list artifacts: %w", err))
	}

	pending, err := artifacts.ListPending(changeDir)
	if err != nil {
		return errs.WriteError(stderr, "dump", fmt.Errorf("list pending: %w", err))
	}

	// Load pipeline metrics.
	pm := sddctx.LoadPipelineMetrics(changeDir)

	// Read cache hash files.
	cacheKeys := make(map[string]string)
	cacheDir := filepath.Join(changeDir, ".cache")
	hashFiles, err := filepath.Glob(filepath.Join(cacheDir, "*.hash"))
	if err == nil {
		for _, hf := range hashFiles {
			base := strings.TrimSuffix(filepath.Base(hf), ".hash")
			raw, err := os.ReadFile(hf)
			if err != nil {
				continue
			}
			cacheKeys[base] = strings.TrimSpace(string(raw))
		}
	}

	out := struct {
		Command   string                   `json:"command"`
		Change    string                   `json:"change"`
		State     *state.State             `json:"state"`
		Config    *config.Config           `json:"config"`
		Artifacts []artifacts.ArtifactInfo `json:"artifacts"`
		Pending   []artifacts.ArtifactInfo `json:"pending"`
		Metrics   *sddctx.PipelineMetrics  `json:"metrics"`
		CacheKeys map[string]string        `json:"cache_keys"`
	}{
		Command:   "dump",
		Change:    name,
		State:     st,
		Config:    cfg,
		Artifacts: arts,
		Pending:   pending,
		Metrics:   pm,
		CacheKeys: cacheKeys,
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))
	return nil
}

func runDashboard(args []string, stdout io.Writer, stderr io.Writer) error {
	port := "8811"
	for i, arg := range args {
		switch {
		case (arg == "--port" || arg == "-p") && i+1 < len(args):
			port = args[i+1]
		}
	}

	p, err := strconv.Atoi(port)
	if err != nil || p < 1024 || p > 65535 {
		return errs.Usage(fmt.Sprintf("invalid port: %s (must be 1024-65535)", port))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "dashboard", fmt.Errorf("get working directory: %w", err))
	}
	dbPath := filepath.Join(cwd, "openspec", ".cache", "sdd.db")
	changesDir := filepath.Join(cwd, "openspec", "changes")

	db, err := store.Open(dbPath)
	if err != nil {
		return errs.WriteError(stderr, "dashboard", fmt.Errorf("open store: %w", err))
	}
	defer db.Close()

	srv := dashboard.New(db, changesDir)
	addr := "0.0.0.0:" + port

	out := struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		URL     string `json:"url"`
	}{
		Command: "dashboard",
		Status:  "running",
		URL:     "http://" + addr,
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("dashboard started", "url", "http://"+addr)
	return srv.ListenAndServe(ctx, addr)
}
