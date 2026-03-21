package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sddctx "github.com/rechedev9/shenronSDD/sdd-cli/internal/context"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/events"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
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
