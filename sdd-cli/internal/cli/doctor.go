package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	sddctx "github.com/rechedev9/shenronSDD/sdd-cli/internal/context"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/errlog"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// CheckResult holds the outcome of a single diagnostic check.
type CheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func checkConfig(configPath string) (CheckResult, *config.Config) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return CheckResult{Name: "config", Status: "fail", Message: err.Error()}, nil
	}
	if cfg.Version != 0 && cfg.Version != config.ConfigVersion {
		msg := fmt.Sprintf("config version %d, expected %d", cfg.Version, config.ConfigVersion)
		return CheckResult{Name: "config", Status: "warn", Message: msg}, cfg
	}
	return CheckResult{Name: "config", Status: "pass", Message: fmt.Sprintf("config.yaml v%d loaded", cfg.Version)}, cfg
}

func checkCache(changesDir string, cfg *config.Config) CheckResult {
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return CheckResult{Name: "cache", Status: "warn", Message: "cannot read changes directory"}
	}
	skillsPath := ""
	if cfg != nil {
		skillsPath = cfg.SkillsPath
	}
	total := 0
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "archive" {
			continue
		}
		changeDir := filepath.Join(changesDir, e.Name())
		n, _ := sddctx.CheckCacheIntegrity(changeDir, skillsPath)
		total += n
	}
	if total > 0 {
		return CheckResult{Name: "cache", Status: "warn", Message: fmt.Sprintf("%d stale cache entry(s)", total)}
	}
	return CheckResult{Name: "cache", Status: "pass", Message: "all cache entries current"}
}

func checkOrphanedPending(changesDir string) CheckResult {
	entries, err := os.ReadDir(changesDir)
	if err != nil {
		return CheckResult{Name: "orphaned_pending", Status: "pass"}
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "archive" {
			continue
		}
		changeDir := filepath.Join(changesDir, e.Name())
		pendingDir := filepath.Join(changeDir, ".pending")
		pfiles, err := os.ReadDir(pendingDir)
		if err != nil {
			continue
		}
		for _, pf := range pfiles {
			if pf.IsDir() || !strings.HasSuffix(pf.Name(), ".md") {
				continue
			}
			phase := strings.TrimSuffix(pf.Name(), ".md")
			promoted := filepath.Join(changeDir, phase+".md")
			if _, err := os.Stat(promoted); err == nil {
				count++
			}
		}
	}
	if count > 0 {
		return CheckResult{Name: "orphaned_pending", Status: "warn", Message: fmt.Sprintf("%d orphaned .pending file(s)", count)}
	}
	return CheckResult{Name: "orphaned_pending", Status: "pass"}
}

func checkSkillsPath(cfg *config.Config) CheckResult {
	if cfg == nil {
		return CheckResult{Name: "skills_path", Status: "warn", Message: "skipped: config unavailable"}
	}
	if cfg.SkillsPath == "" {
		return CheckResult{Name: "skills_path", Status: "warn", Message: "no skills_path configured — using embedded prompts"}
	}
	if _, err := os.Stat(cfg.SkillsPath); err != nil {
		return CheckResult{Name: "skills_path", Status: "fail", Message: fmt.Sprintf("skills directory not found: %s", cfg.SkillsPath)}
	}
	phases := state.AllPhases()
	present := 0
	for _, p := range phases {
		skillPath := filepath.Join(cfg.SkillsPath, "sdd-"+string(p), "SKILL.md")
		if _, err := os.Stat(skillPath); err == nil {
			present++
		}
	}
	msg := fmt.Sprintf("%d/%d SKILL.md files present", present, len(phases))
	if present < len(phases) {
		return CheckResult{Name: "skills_path", Status: "warn", Message: msg}
	}
	return CheckResult{Name: "skills_path", Status: "pass", Message: msg}
}

func checkBuildTools(cfg *config.Config) CheckResult {
	if cfg == nil {
		return CheckResult{Name: "build_tools", Status: "warn", Message: "skipped: config unavailable"}
	}
	cmds := []string{cfg.Commands.Build, cfg.Commands.Test, cfg.Commands.Lint, cfg.Commands.Format}
	var missing []string
	seen := map[string]bool{}
	for _, cmd := range cmds {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		bin := strings.Fields(cmd)[0]
		if seen[bin] {
			continue
		}
		seen[bin] = true
		if _, err := exec.LookPath(bin); err != nil {
			missing = append(missing, bin)
		}
	}
	if len(missing) > 0 {
		return CheckResult{Name: "build_tools", Status: "fail", Message: fmt.Sprintf("not in PATH: %s", strings.Join(missing, ", "))}
	}
	return CheckResult{Name: "build_tools", Status: "pass", Message: "all build commands found"}
}

func checkErrors(cwd string) CheckResult {
	log := errlog.Load(cwd)
	if len(log.Entries) == 0 {
		return CheckResult{Name: "errors", Status: "pass", Message: "no recorded errors"}
	}
	recurring := log.RecurringFingerprints(3)
	if len(recurring) > 0 {
		return CheckResult{
			Name:    "errors",
			Status:  "warn",
			Message: fmt.Sprintf("%d recurring error pattern(s); run 'sdd errors' for details", len(recurring)),
		}
	}
	return CheckResult{
		Name:    "errors",
		Status:  "pass",
		Message: fmt.Sprintf("%d error(s) recorded, no recurring patterns", len(log.Entries)),
	}
}

func checkPprof() CheckResult {
	val := os.Getenv("SDD_PPROF")
	if val == "" {
		return CheckResult{Name: "pprof", Status: "pass", Message: "SDD_PPROF not set (no profiling)"}
	}
	return CheckResult{Name: "pprof", Status: "pass", Message: fmt.Sprintf("SDD_PPROF=%s", val)}
}

func runDoctor(args []string, stdout io.Writer, stderr io.Writer) error {
	jsonOut := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOut = true
		default:
			return errs.Usage(fmt.Sprintf("unknown flag: %s", arg))
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "doctor", fmt.Errorf("get working directory: %w", err))
	}

	configPath := filepath.Join(cwd, "openspec", "config.yaml")
	changesDir := filepath.Join(cwd, "openspec", "changes")

	configResult, cfg := checkConfig(configPath)
	checks := []CheckResult{
		configResult,
		checkCache(changesDir, cfg),
		checkOrphanedPending(changesDir),
		checkSkillsPath(cfg),
		checkBuildTools(cfg),
		checkErrors(cwd),
		checkPprof(),
	}

	if jsonOut {
		out := struct {
			Command string        `json:"command"`
			Status  string        `json:"status"`
			Checks  []CheckResult `json:"checks"`
		}{
			Command: "doctor",
			Status:  aggregateStatus(checks),
			Checks:  checks,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(stdout, string(data))
	} else {
		printDoctorTable(stdout, checks)
	}

	failCount := 0
	for _, c := range checks {
		if c.Status == "fail" {
			failCount++
		}
	}
	if failCount > 0 {
		return fmt.Errorf("doctor: %d check(s) failed", failCount)
	}
	return nil
}

func aggregateStatus(checks []CheckResult) string {
	worst := "pass"
	for _, c := range checks {
		switch c.Status {
		case "fail":
			return "fail"
		case "warn":
			worst = "warn"
		}
	}
	return worst
}

func printDoctorTable(w io.Writer, checks []CheckResult) {
	maxName := 0
	for _, c := range checks {
		if len(c.Name) > maxName {
			maxName = len(c.Name)
		}
	}
	fmt.Fprintln(w, "sdd doctor")
	for _, c := range checks {
		if c.Message != "" {
			fmt.Fprintf(w, "  %-*s  %-4s  %s\n", maxName, c.Name, c.Status, c.Message)
		} else {
			fmt.Fprintf(w, "  %-*s  %s\n", maxName, c.Name, c.Status)
		}
	}
}
