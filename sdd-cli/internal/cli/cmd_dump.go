package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/artifacts"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	sddctx "github.com/rechedev9/shenronSDD/sdd-cli/internal/context"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

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
