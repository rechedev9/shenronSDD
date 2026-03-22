package cli

import (
	"strings"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
)

func TestCheckSkillsPathEmpty(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SkillsPath: ""}
	r := checkSkillsPath(cfg)
	if r.Status != "warn" {
		t.Errorf("expected warn, got %q", r.Status)
	}
	if !strings.Contains(r.Message, "embedded") {
		t.Errorf("expected message about embedded prompts, got %q", r.Message)
	}
}

func TestCheckSkillsPathMissingDir(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SkillsPath: "/nonexistent/skills/dir"}
	r := checkSkillsPath(cfg)
	if r.Status != "fail" {
		t.Errorf("expected fail, got %q", r.Status)
	}
	if !strings.Contains(r.Message, "/nonexistent/skills/dir") {
		t.Errorf("expected message to contain path, got %q", r.Message)
	}
}

func TestCheckSkillsPathNilConfig(t *testing.T) {
	t.Parallel()
	r := checkSkillsPath(nil)
	if r.Status != "warn" {
		t.Errorf("expected warn, got %q", r.Status)
	}
	if !strings.Contains(r.Message, "config unavailable") {
		t.Errorf("expected 'config unavailable', got %q", r.Message)
	}
}
