package artifacts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

func TestWritePending(t *testing.T) {
	dir := t.TempDir()
	content := []byte("# Exploration\n\nFindings here.\n")

	err := WritePending(dir, state.PhaseExplore, content)
	if err != nil {
		t.Fatalf("WritePending: %v", err)
	}

	path := PendingPath(dir, state.PhaseExplore)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pending: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestPendingExists(t *testing.T) {
	dir := t.TempDir()

	if PendingExists(dir, state.PhaseExplore) {
		t.Error("should not exist before write")
	}

	WritePending(dir, state.PhaseExplore, []byte("test"))

	if !PendingExists(dir, state.PhaseExplore) {
		t.Error("should exist after write")
	}
}

func TestPromote(t *testing.T) {
	dir := t.TempDir()
	content := []byte("# Exploration results\n")

	WritePending(dir, state.PhaseExplore, content)

	promoted, err := Promote(dir, state.PhaseExplore)
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}

	expected := filepath.Join(dir, "exploration.md")
	if promoted != expected {
		t.Errorf("promoted path = %q, want %q", promoted, expected)
	}

	// Verify promoted file exists with correct content.
	got, err := os.ReadFile(promoted)
	if err != nil {
		t.Fatalf("read promoted: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	// Verify pending file is gone.
	if PendingExists(dir, state.PhaseExplore) {
		t.Error("pending file should be removed after promotion")
	}
}

func TestPromoteSpec(t *testing.T) {
	dir := t.TempDir()
	content := []byte("# Auth Spec\n")

	WritePending(dir, state.PhaseSpec, content)

	promoted, err := Promote(dir, state.PhaseSpec)
	if err != nil {
		t.Fatalf("Promote spec: %v", err)
	}

	// Spec should go into specs/ directory.
	if filepath.Dir(promoted) != filepath.Join(dir, "specs") {
		t.Errorf("promoted dir = %q, want specs/", filepath.Dir(promoted))
	}

	got, err := os.ReadFile(promoted)
	if err != nil {
		t.Fatalf("read promoted spec: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestPromoteNoPending(t *testing.T) {
	dir := t.TempDir()

	_, err := Promote(dir, state.PhaseExplore)
	if err == nil {
		t.Fatal("expected error for missing pending")
	}
	if !errors.Is(err, ErrNoPending) {
		t.Errorf("error = %v, want ErrNoPending", err)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	content := []byte("# Exploration\n")
	os.WriteFile(filepath.Join(dir, "exploration.md"), content, 0o644)

	got, err := Read(dir, state.PhaseExplore)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestReadMissing(t *testing.T) {
	dir := t.TempDir()

	_, err := Read(dir, state.PhaseExplore)
	if err == nil {
		t.Fatal("expected error for missing artifact")
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("custom content")
	os.WriteFile(filepath.Join(dir, "custom.md"), content, 0o644)

	got, err := ReadFile(dir, "custom.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()

	// Create some artifacts.
	os.WriteFile(filepath.Join(dir, "exploration.md"), []byte("explore"), 0o644)
	os.WriteFile(filepath.Join(dir, "proposal.md"), []byte("propose"), 0o644)
	os.WriteFile(filepath.Join(dir, "design.md"), []byte("design content"), 0o644)

	items, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("item count = %d, want 3", len(items))
	}

	// Verify phases are present.
	phases := map[state.Phase]bool{}
	for _, item := range items {
		phases[item.Phase] = true
		if item.Size == 0 {
			t.Errorf("artifact %s has zero size", item.Filename)
		}
	}
	for _, p := range []state.Phase{state.PhaseExplore, state.PhasePropose, state.PhaseDesign} {
		if !phases[p] {
			t.Errorf("missing phase %s in list", p)
		}
	}
}

func TestListWithSpecs(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "auth-spec.md"), []byte("auth"), 0o644)
	os.WriteFile(filepath.Join(specsDir, "api-spec.md"), []byte("api"), 0o644)

	items, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("item count = %d, want 2 (spec files)", len(items))
	}
	for _, item := range items {
		if item.Phase != state.PhaseSpec {
			t.Errorf("phase = %s, want spec", item.Phase)
		}
	}
}

func TestListEmpty(t *testing.T) {
	dir := t.TempDir()

	items, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("item count = %d, want 0", len(items))
	}
}

func TestListPending(t *testing.T) {
	dir := t.TempDir()
	WritePending(dir, state.PhaseExplore, []byte("explore"))
	WritePending(dir, state.PhasePropose, []byte("propose"))

	items, err := ListPending(dir)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("pending count = %d, want 2", len(items))
	}
}

func TestListPendingEmpty(t *testing.T) {
	dir := t.TempDir()

	items, err := ListPending(dir)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil for missing .pending dir, got %v", items)
	}
}

func TestPendingFileName(t *testing.T) {
	tests := []struct {
		phase state.Phase
		want  string
	}{
		{state.PhaseExplore, "explore.md"},
		{state.PhasePropose, "propose.md"},
		{state.PhaseSpec, "spec.md"},
		{state.PhaseDesign, "design.md"},
		{state.PhaseTasks, "tasks.md"},
	}
	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			got := PendingFileName(tt.phase)
			if got != tt.want {
				t.Errorf("PendingFileName(%s) = %q, want %q", tt.phase, got, tt.want)
			}
		})
	}
}

func TestPromoteAllPhases(t *testing.T) {
	// Verify every phase with an artifact mapping can be promoted.
	phases := []state.Phase{
		state.PhaseExplore, state.PhasePropose, state.PhaseDesign,
		state.PhaseTasks, state.PhaseReview, state.PhaseVerify,
		state.PhaseClean, state.PhaseArchive,
	}
	for _, phase := range phases {
		t.Run(string(phase), func(t *testing.T) {
			dir := t.TempDir()
			WritePending(dir, phase, []byte("content for "+string(phase)))

			promoted, err := Promote(dir, phase)
			if err != nil {
				t.Fatalf("Promote(%s): %v", phase, err)
			}
			if _, err := os.Stat(promoted); err != nil {
				t.Errorf("promoted file missing: %v", err)
			}
		})
	}
}
