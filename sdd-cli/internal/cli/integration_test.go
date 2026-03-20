package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/artifacts"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// TestEndToEndPipeline exercises the full SDD pipeline in a temp directory:
// init → new → write (explore..review) → verify → write clean → status → list → archive.
func TestEndToEndPipeline(t *testing.T) {
	root := t.TempDir()

	// Create a go.mod so init can detect the Go stack.
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(root)

	// ── Step 1: sdd init ─────────────────────────────────────────
	t.Run("init", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run([]string{"init"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("init failed: %v\nstderr: %s", err, stderr.String())
		}

		// Verify openspec/ exists.
		assertDir(t, filepath.Join(root, "openspec"))
		assertDir(t, filepath.Join(root, "openspec", "changes"))
		assertFile(t, filepath.Join(root, "openspec", "config.yaml"))

		// Parse JSON output.
		var out map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if out["command"] != "init" || out["status"] != "success" {
			t.Errorf("unexpected output: %v", out)
		}
	})

	// Override config with echo-based commands for verify.
	overrideConfig(t, root)

	// ── Step 2: sdd new ──────────────────────────────────────────
	t.Run("new", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run([]string{"new", "test-feature", "add login page"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("new failed: %v\nstderr: %s", err, stderr.String())
		}

		changeDir := filepath.Join(root, "openspec", "changes", "test-feature")
		assertDir(t, changeDir)
		assertFile(t, filepath.Join(changeDir, "state.json"))

		// State should be at explore phase.
		st := loadState(t, changeDir)
		if st.CurrentPhase != state.PhaseExplore {
			t.Errorf("current phase = %s, want explore", st.CurrentPhase)
		}
	})

	changeDir := filepath.Join(root, "openspec", "changes", "test-feature")

	// ── Step 3: Write through planning phases ────────────────────
	planningPhases := []struct {
		phase   state.Phase
		content string
	}{
		{state.PhaseExplore, "# Exploration\n\nFound login-related files."},
		{state.PhasePropose, "# Proposal\n\nAdd login page with OAuth."},
		{state.PhaseSpec, "# Spec\n\n## Requirements\n- OAuth login"},
		{state.PhaseDesign, "# Design\n\n## Architecture\n- LoginPage component"},
		{state.PhaseTasks, "# Tasks\n\n- [ ] Create LoginPage\n- [ ] Add OAuth flow"},
		{state.PhaseApply, "# Tasks\n\n- [x] Create LoginPage\n- [x] Add OAuth flow"},
		{state.PhaseReview, "# Review Report\n\nAll changes look good."},
	}

	for _, pp := range planningPhases {
		t.Run("write_"+string(pp.phase), func(t *testing.T) {
			// Write pending artifact.
			if err := artifacts.WritePending(changeDir, pp.phase, []byte(pp.content)); err != nil {
				t.Fatalf("write pending: %v", err)
			}

			// Promote via sdd write.
			var stdout, stderr bytes.Buffer
			err := Run([]string{"write", "test-feature", string(pp.phase)}, &stdout, &stderr)
			if err != nil {
				t.Fatalf("write %s failed: %v\nstderr: %s", pp.phase, err, stderr.String())
			}

			var out map[string]interface{}
			if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if out["status"] != "success" {
				t.Errorf("write %s: status = %v, want success", pp.phase, out["status"])
			}
		})
	}

	// ── Step 4: sdd status ───────────────────────────────────────
	t.Run("status", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run([]string{"status", "test-feature"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("status failed: %v\nstderr: %s", err, stderr.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if out["current_phase"] != "verify" {
			t.Errorf("current_phase = %v, want verify", out["current_phase"])
		}

		// 7 phases should be completed (explore through review).
		completed, ok := out["completed"].([]interface{})
		if !ok {
			t.Fatalf("completed is not a list: %T", out["completed"])
		}
		if len(completed) != 7 {
			t.Errorf("completed count = %d, want 7", len(completed))
		}
	})

	// ── Step 5: sdd list ─────────────────────────────────────────
	t.Run("list", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run([]string{"list"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("list failed: %v\nstderr: %s", err, stderr.String())
		}

		var out struct {
			Count   int `json:"count"`
			Changes []struct {
				Name         string `json:"name"`
				CurrentPhase string `json:"current_phase"`
			} `json:"changes"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if out.Count != 1 {
			t.Errorf("count = %d, want 1", out.Count)
		}
		if len(out.Changes) != 1 || out.Changes[0].Name != "test-feature" {
			t.Errorf("unexpected changes: %+v", out.Changes)
		}
		if out.Changes[0].CurrentPhase != "verify" {
			t.Errorf("current_phase = %s, want verify", out.Changes[0].CurrentPhase)
		}
	})

	// ── Step 6: sdd verify ───────────────────────────────────────
	t.Run("verify", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run([]string{"verify", "test-feature"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("verify failed: %v\nstderr: %s", err, stderr.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if out["passed"] != true {
			t.Errorf("passed = %v, want true", out["passed"])
		}

		// verify-report.md should exist.
		assertFile(t, filepath.Join(changeDir, "verify-report.md"))
	})

	// Advance state past verify: write verify phase, then write clean.
	t.Run("advance_verify_and_clean", func(t *testing.T) {
		// Verify wrote the report but didn't advance state — we need sdd write verify.
		if err := artifacts.WritePending(changeDir, state.PhaseVerify, []byte("# Verify\nPassed")); err != nil {
			t.Fatalf("write pending verify: %v", err)
		}
		var stdout, stderr bytes.Buffer
		err := Run([]string{"write", "test-feature", "verify"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("write verify failed: %v\nstderr: %s", err, stderr.String())
		}

		// Write clean.
		if err := artifacts.WritePending(changeDir, state.PhaseClean, []byte("# Clean Report\nNo issues.")); err != nil {
			t.Fatalf("write pending clean: %v", err)
		}
		stdout.Reset()
		stderr.Reset()
		err = Run([]string{"write", "test-feature", "clean"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("write clean failed: %v\nstderr: %s", err, stderr.String())
		}
	})

	// ── Step 7: sdd archive ──────────────────────────────────────
	t.Run("archive", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run([]string{"archive", "test-feature"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("archive failed: %v\nstderr: %s", err, stderr.String())
		}

		var out map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if out["status"] != "success" {
			t.Errorf("status = %v, want success", out["status"])
		}

		// Original change dir should be gone.
		if _, err := os.Stat(changeDir); !os.IsNotExist(err) {
			t.Error("expected change directory to be moved to archive")
		}

		// Archive directory should exist.
		archivePath, ok := out["archive_path"].(string)
		if !ok || archivePath == "" {
			t.Fatal("missing archive_path in output")
		}
		assertDir(t, archivePath)

		// Manifest should exist.
		assertFile(t, filepath.Join(archivePath, "archive-manifest.md"))

		// Key artifacts preserved.
		assertFile(t, filepath.Join(archivePath, "exploration.md"))
		assertFile(t, filepath.Join(archivePath, "proposal.md"))
		assertFile(t, filepath.Join(archivePath, "design.md"))
		assertFile(t, filepath.Join(archivePath, "tasks.md"))
	})

	// ── Step 8: list after archive — should be empty ─────────────
	t.Run("list_after_archive", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := Run([]string{"list"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}

		var out struct {
			Count int `json:"count"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if out.Count != 0 {
			t.Errorf("count = %d, want 0 after archive", out.Count)
		}
	})
}

// TestEdgeCaseWrongPhaseWrite tries to write a phase that is not ready.
func TestEdgeCaseWrongPhaseWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(root)

	// Init + new.
	run(t, "init")
	run(t, "new", "feat", "desc")

	changeDir := filepath.Join(root, "openspec", "changes", "feat")

	// Try to write "apply" — should fail (prerequisites not met).
	if err := artifacts.WritePending(changeDir, state.PhaseApply, []byte("# Apply")); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := Run([]string{"write", "feat", "apply"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error writing apply before prerequisites")
	}
	if !strings.Contains(stderr.String(), "prerequisites") {
		t.Errorf("expected prerequisites error, got: %s", stderr.String())
	}
}

// TestEdgeCaseArchiveNotReady tries to archive before pipeline is complete.
func TestEdgeCaseArchiveNotReady(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(root)

	run(t, "init")
	run(t, "new", "feat", "desc")

	var stdout, stderr bytes.Buffer
	err := Run([]string{"archive", "feat"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error archiving before pipeline completes")
	}
	if !strings.Contains(stderr.String(), "not ready") {
		t.Errorf("expected 'not ready' error, got: %s", stderr.String())
	}
}

// ── Helpers ──────────────────────────────────────────────────────

func overrideConfig(t *testing.T, root string) {
	t.Helper()
	cfg := &config.Config{
		ProjectName: "test",
		Stack: config.Stack{
			Language:  "go",
			BuildTool: "go",
			Manifests: []string{"go.mod"},
		},
		Commands: config.Commands{
			Build: "echo build-ok",
			Test:  "echo test-ok",
			Lint:  "echo lint-ok",
		},
		SkillsPath: filepath.Join(root, "skills"), // nonexistent — context assembly is non-fatal
	}
	configPath := filepath.Join(root, "openspec", "config.yaml")
	if err := config.Save(cfg, configPath); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

func loadState(t *testing.T, changeDir string) *state.State {
	t.Helper()
	st, err := state.Load(filepath.Join(changeDir, "state.json"))
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	return st
}

func assertDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected directory %s to exist: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", path)
	}
}

func assertFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected %s to be a file, got directory", path)
	}
}

// run executes sdd with the given args and fatals on error.
func run(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := Run(args, &stdout, &stderr)
	if err != nil {
		t.Fatalf("sdd %s failed: %v\nstderr: %s", args[0], err, stderr.String())
	}
	return stdout.String()
}
