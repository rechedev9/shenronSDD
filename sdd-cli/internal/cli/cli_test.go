package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

func TestRunNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if ExitCode(err) != 2 {
		t.Errorf("exit code = %d, want 2", ExitCode(err))
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run([]string{"bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
	if ExitCode(err) != 2 {
		t.Errorf("exit code = %d, want 2", ExitCode(err))
	}
}

func TestRunVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run([]string{"--version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != "dev" {
		t.Errorf("version = %q, want %q", got, "dev")
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := Run([]string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "sdd") {
		t.Error("help output should contain 'sdd'")
	}
}

func TestRunSubcommands(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
	}{
		{"init no manifest", []string{"init"}, 1},
		{"new missing args", []string{"new"}, 2},
		{"new no config", []string{"new", "feat", "desc"}, 1},
		{"context missing args", []string{"context"}, 2},
		{"context no change", []string{"context", "feat"}, 1},
		{"write missing args", []string{"write"}, 2},
		{"write no change", []string{"write", "feat", "explore"}, 1},
		{"status missing args", []string{"status"}, 2},
		{"status no change", []string{"status", "nonexistent"}, 1},
		{"verify missing args", []string{"verify"}, 2},
		{"verify no change", []string{"verify", "nonexistent"}, 1},
		{"archive missing args", []string{"archive"}, 2},
		{"archive no change", []string{"archive", "nonexistent"}, 1},
		{"diff missing args", []string{"diff"}, 2},
		{"diff no change", []string{"diff", "nonexistent"}, 1},
		{"health missing args", []string{"health"}, 2},
		{"health no change", []string{"health", "nonexistent"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(tt.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected error from stub")
			}
			got := ExitCode(err)
			if got != tt.wantExit {
				t.Errorf("exit code = %d, want %d", got, tt.wantExit)
			}
		})
	}
}

func TestRunErrorsWriteJSON(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"init", []string{"init"}, `"command":"init"`},
		{"new", []string{"new", "x", "y"}, `"command":"new"`},
		{"status no change", []string{"status", "nope"}, `"command":"status"`},
		{"verify no change", []string{"verify", "nope"}, `"command":"verify"`},
		{"archive no change", []string{"archive", "nope"}, `"command":"archive"`},
		{"diff no change", []string{"diff", "nope"}, `"command":"diff"`},
		{"health no change", []string{"health", "nope"}, `"command":"health"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			_ = Run(tt.args, &stdout, &stderr)
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestExitCodeNil(t *testing.T) {
	if ExitCode(nil) != 0 {
		t.Error("nil error should return exit code 0")
	}
}

// setupChange creates a temp project with openspec/changes/{name}/state.json.
func setupChange(t *testing.T, name, desc string) string {
	t.Helper()
	root := t.TempDir()
	changeDir := filepath.Join(root, "openspec", "changes", name)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	st := state.NewState(name, desc)
	if err := state.Save(st, filepath.Join(changeDir, "state.json")); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunStatus(t *testing.T) {
	root := setupChange(t, "my-feature", "add auth")

	// chdir so resolveChangeDir works.
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(root)

	var stdout, stderr bytes.Buffer
	err := Run([]string{"status", "my-feature"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout.String())
	}

	if out["command"] != "status" {
		t.Errorf("command = %v, want status", out["command"])
	}
	if out["change"] != "my-feature" {
		t.Errorf("change = %v, want my-feature", out["change"])
	}
	if out["current_phase"] != "explore" {
		t.Errorf("current_phase = %v, want explore", out["current_phase"])
	}
	if out["description"] != "add auth" {
		t.Errorf("description = %v, want 'add auth'", out["description"])
	}
}

func TestRunListEmpty(t *testing.T) {
	root := t.TempDir()

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(root)

	// No openspec/ at all.
	var stdout, stderr bytes.Buffer
	err := Run([]string{"list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out["count"] != float64(0) {
		t.Errorf("count = %v, want 0", out["count"])
	}
}

func TestRunListMultiple(t *testing.T) {
	root := t.TempDir()
	changesDir := filepath.Join(root, "openspec", "changes")

	// Create two changes.
	for _, name := range []string{"feat-a", "feat-b"} {
		dir := filepath.Join(changesDir, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		st := state.NewState(name, "desc-"+name)
		if err := state.Save(st, filepath.Join(dir, "state.json")); err != nil {
			t.Fatal(err)
		}
	}

	// Create archive dir — should be excluded.
	if err := os.MkdirAll(filepath.Join(changesDir, "archive"), 0o755); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	os.Chdir(root)

	var stdout, stderr bytes.Buffer
	err := Run([]string{"list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out struct {
		Count   int `json:"count"`
		Changes []struct {
			Name string `json:"name"`
		} `json:"changes"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if out.Count != 2 {
		t.Errorf("count = %d, want 2", out.Count)
	}
	if len(out.Changes) != 2 {
		t.Errorf("changes len = %d, want 2", len(out.Changes))
	}

	// Verify archive is excluded.
	for _, c := range out.Changes {
		if c.Name == "archive" {
			t.Error("archive directory should be excluded from list")
		}
	}
}

func TestRunDiff(t *testing.T) {
	// Helper: init a git repo with one commit.
	// Uses /usr/bin/git directly to bypass any git shims.
	gitBin := "/usr/bin/git"
	if _, err := os.Stat(gitBin); err != nil {
		gitBin = "git" // fallback
	}

	initGitRepo := func(t *testing.T, root string) string {
		t.Helper()
		mustRun := func(args ...string) {
			t.Helper()
			cmd := exec.Command(gitBin, args...)
			cmd.Dir = root
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		mustRun("init")
		mustRun("config", "user.email", "test@test.com")
		mustRun("config", "user.name", "Test")
		if err := os.WriteFile(filepath.Join(root, "init.go"), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustRun("add", "init.go")
		mustRun("commit", "-m", "initial")
		cmd := exec.Command(gitBin, "rev-parse", "HEAD")
		cmd.Dir = root
		sha, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(sha))
	}

	t.Run("happy path", func(t *testing.T) {
		root := t.TempDir()
		sha := initGitRepo(t, root)

		// Create change with BaseRef.
		changeDir := filepath.Join(root, "openspec", "changes", "feat")
		if err := os.MkdirAll(changeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		st := state.NewState("feat", "desc")
		st.BaseRef = sha
		if err := state.Save(st, filepath.Join(changeDir, "state.json")); err != nil {
			t.Fatal(err)
		}

		// Modify the tracked file after baseline.
		if err := os.WriteFile(filepath.Join(root, "init.go"), []byte("package main\n// modified\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		orig, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(orig) })
		os.Chdir(root)

		var stdout, stderr bytes.Buffer
		err := Run([]string{"diff", "feat"}, &stdout, &stderr)
		if err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
		}

		var out struct {
			Files []string `json:"files"`
			Count int      `json:"count"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout.String())
		}
		if out.Count != 1 {
			t.Errorf("count = %d, want 1", out.Count)
		}
	})

	t.Run("no base_ref", func(t *testing.T) {
		root := t.TempDir()
		changeDir := filepath.Join(root, "openspec", "changes", "old")
		if err := os.MkdirAll(changeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		st := state.NewState("old", "desc")
		// BaseRef deliberately empty.
		if err := state.Save(st, filepath.Join(changeDir, "state.json")); err != nil {
			t.Fatal(err)
		}

		orig, _ := os.Getwd()
		t.Cleanup(func() { os.Chdir(orig) })
		os.Chdir(root)

		var stdout, stderr bytes.Buffer
		err := Run([]string{"diff", "old"}, &stdout, &stderr)
		if err == nil {
			t.Fatal("expected error for missing base_ref")
		}
		if !strings.Contains(stderr.String(), "base_ref not recorded") {
			t.Errorf("stderr = %q, want base_ref error", stderr.String())
		}
	})
}
