package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectGo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if cfg.Stack.Language != "go" {
		t.Errorf("language = %q, want go", cfg.Stack.Language)
	}
	if cfg.Commands.Test != "go test ./..." {
		t.Errorf("test cmd = %q, want %q", cfg.Commands.Test, "go test ./...")
	}
	if cfg.Commands.Lint != "golangci-lint run ./..." {
		t.Errorf("lint cmd = %q, want %q", cfg.Commands.Lint, "golangci-lint run ./...")
	}
	if len(cfg.Stack.Manifests) != 1 || cfg.Stack.Manifests[0] != "go.mod" {
		t.Errorf("manifests = %v, want [go.mod]", cfg.Stack.Manifests)
	}
}

func TestDetectNode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if cfg.Stack.Language != "typescript" {
		t.Errorf("language = %q, want typescript", cfg.Stack.Language)
	}
	if cfg.Commands.Build != "npm run build" {
		t.Errorf("build cmd = %q, want %q", cfg.Commands.Build, "npm run build")
	}
}

func TestDetectPython(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"test\"\n"), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if cfg.Stack.Language != "python" {
		t.Errorf("language = %q, want python", cfg.Stack.Language)
	}
	if cfg.Commands.Test != "pytest" {
		t.Errorf("test cmd = %q, want %q", cfg.Commands.Test, "pytest")
	}
	if cfg.Commands.Lint != "ruff check ." {
		t.Errorf("lint cmd = %q, want %q", cfg.Commands.Lint, "ruff check .")
	}
}

func TestDetectRust(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if cfg.Stack.Language != "rust" {
		t.Errorf("language = %q, want rust", cfg.Stack.Language)
	}
	if cfg.Commands.Build != "cargo build" {
		t.Errorf("build cmd = %q, want %q", cfg.Commands.Build, "cargo build")
	}
}

func TestDetectJavaGradle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(""), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if cfg.Stack.Language != "java" {
		t.Errorf("language = %q, want java", cfg.Stack.Language)
	}
	if cfg.Stack.BuildTool != "gradle" {
		t.Errorf("build tool = %q, want gradle", cfg.Stack.BuildTool)
	}
}

func TestDetectJavaMaven(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if cfg.Stack.Language != "java" {
		t.Errorf("language = %q, want java", cfg.Stack.Language)
	}
	if cfg.Stack.BuildTool != "maven" {
		t.Errorf("build tool = %q, want maven", cfg.Stack.BuildTool)
	}
}

func TestDetectNoManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := Detect(dir)
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
	if !errors.Is(err, ErrNoManifest) {
		t.Errorf("error = %v, want ErrNoManifest", err)
	}
}

func TestDetectMonorepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Go + Node in same directory — Go wins (first in scan order).
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if cfg.Stack.Language != "go" {
		t.Errorf("language = %q, want go (first match)", cfg.Stack.Language)
	}
	if len(cfg.Stack.Manifests) != 2 {
		t.Errorf("manifests count = %d, want 2", len(cfg.Stack.Manifests))
	}
}

func TestDetectProjectName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	expected := filepath.Base(dir)
	if cfg.ProjectName != expected {
		t.Errorf("project name = %q, want %q", cfg.ProjectName, expected)
	}
}

func TestDetectSkillsPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	cfg, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if cfg.SkillsPath == "" {
		t.Error("skills_path should not be empty")
	}
}

func TestSaveLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := &Config{
		ProjectName: "myproject",
		Stack: Stack{
			Language:  "go",
			BuildTool: "go",
			Manifests: []string{"go.mod"},
		},
		Commands: Commands{
			Build: "go build ./...",
			Test:  "go test ./...",
		},
		SkillsPath: "/home/test/.claude/skills/sdd",
	}

	if err := Save(original, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.ProjectName != original.ProjectName {
		t.Errorf("project name = %q, want %q", loaded.ProjectName, original.ProjectName)
	}
	if loaded.Stack.Language != original.Stack.Language {
		t.Errorf("language = %q, want %q", loaded.Stack.Language, original.Stack.Language)
	}
	if loaded.Commands.Test != original.Commands.Test {
		t.Errorf("test cmd = %q, want %q", loaded.Commands.Test, original.Commands.Test)
	}
}

func TestSaveAtomicNoTmpLeftover(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{ProjectName: "test"}
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tmp := path + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temp file should not remain after save")
	}
}

func TestLoadMissing(t *testing.T) {
	t.Parallel()
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error loading missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(":\n  :\n    - :\n  invalid: [unclosed"), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}
