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
