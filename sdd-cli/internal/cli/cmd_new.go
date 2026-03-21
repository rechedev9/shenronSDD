package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	sddctx "github.com/rechedev9/shenronSDD/sdd-cli/internal/context"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

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
