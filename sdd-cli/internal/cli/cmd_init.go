package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
)

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
