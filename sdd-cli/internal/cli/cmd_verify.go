package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/errlog"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/events"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/verify"
)

func runVerify(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 1 {
		return errs.Usage("usage: sdd verify <name> [--force]")
	}

	name := args[0]
	force := false
	for _, arg := range args[1:] {
		switch arg {
		case "--force", "-f":
			force = true
		}
	}

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

		// Record smart-skip as passing results for dashboard charts.
		if vdb := tryOpenStore(cwd); vdb != nil {
			for _, cmd := range []string{"build", "lint", "test"} {
				_ = vdb.InsertVerifyResult(context.Background(), store.VerifyResult{
					Timestamp:   time.Now().UTC(),
					Change:      name,
					CommandName: cmd,
					ExitCode:    0,
					Passed:      true,
				})
			}
			vdb.Close()
		}

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

	// Early stopping: warn about recurring error patterns.
	if !force {
		if matches := checkRecurringFailures(cwd, name); len(matches) > 0 {
			fmt.Fprintf(stderr, "sdd verify: %d error pattern(s) recur 3+ times for %q:\n", len(matches), name)
			for fp, count := range matches {
				fmt.Fprintf(stderr, "  fingerprint %s — seen %d times\n", fp, count)
			}
			fmt.Fprintf(stderr, "Investigate before retrying. Use --force to run anyway.\n")
			return fmt.Errorf("verify: recurring failures detected (use --force to override)")
		}
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

	// Open store once for verify results + error collection.
	db := tryOpenStore(cwd)
	if db != nil {
		defer db.Close()
	}

	// Record all verify results (pass and fail) for dashboard charts.
	if db != nil {
		for _, r := range report.Results {
			_ = db.InsertVerifyResult(context.Background(), store.VerifyResult{
				Timestamp:   time.Now().UTC(),
				Change:      name,
				CommandName: r.Name,
				ExitCode:    r.ExitCode,
				Passed:      r.Passed,
			})
		}
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

// checkRecurringFailures returns fingerprints that recur 3+ times globally
// and match recent failures for the given change. Returns nil if no matches.
func checkRecurringFailures(cwd, changeName string) map[string]int {
	log := errlog.Load(cwd)
	recurring := log.RecurringFingerprints(3)
	if len(recurring) == 0 {
		return nil
	}

	// Collect fingerprints from this change's recent failures.
	var changeFingerprints []string
	for i := len(log.Entries) - 1; i >= 0; i-- {
		e := log.Entries[i]
		if e.Change == changeName {
			changeFingerprints = append(changeFingerprints, e.Fingerprint)
		}
		if len(changeFingerprints) >= 10 {
			break
		}
	}

	matches := make(map[string]int)
	for _, fp := range changeFingerprints {
		if count, ok := recurring[fp]; ok {
			matches[fp] = count
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}
