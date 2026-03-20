package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInitGoProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0o644)

	result, err := Init(dir, false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Verify config.yaml exists.
	if _, err := os.Stat(result.ConfigPath); err != nil {
		t.Errorf("config.yaml should exist: %v", err)
	}

	// Verify directory structure.
	for _, d := range []string{"openspec", "openspec/changes", "openspec/changes/archive"} {
		path := filepath.Join(dir, d)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("directory %s should exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s should be a directory", d)
		}
	}

	// Verify config content.
	cfg, err := Load(result.ConfigPath)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}
	if cfg.Stack.Language != "go" {
		t.Errorf("language = %q, want go", cfg.Stack.Language)
	}
}

func TestInitNodeProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0o644)

	result, err := Init(dir, false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	cfg, err := Load(result.ConfigPath)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}
	if cfg.Stack.Language != "typescript" {
		t.Errorf("language = %q, want typescript", cfg.Stack.Language)
	}
}

func TestInitAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "openspec"), 0o755)

	_, err := Init(dir, false)
	if err == nil {
		t.Fatal("expected error for existing openspec/")
	}
	if !errors.Is(err, ErrAlreadyInitialized) {
		t.Errorf("error = %v, want ErrAlreadyInitialized", err)
	}
}

func TestInitForce(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "openspec"), 0o755)

	result, err := Init(dir, true)
	if err != nil {
		t.Fatalf("Init --force: %v", err)
	}
	if result.Config.Stack.Language != "go" {
		t.Errorf("language = %q, want go", result.Config.Stack.Language)
	}
}

func TestInitNoManifest(t *testing.T) {
	dir := t.TempDir()

	_, err := Init(dir, false)
	if err == nil {
		t.Fatal("expected error for no manifest")
	}
}
