package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

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
