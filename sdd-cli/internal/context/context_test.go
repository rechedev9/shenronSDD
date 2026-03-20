package context

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// setupFixture creates a temp directory with skills and artifacts for testing.
func setupFixture(t *testing.T) (string, string, *Params) {
	t.Helper()

	projectDir := t.TempDir()
	skillsDir := filepath.Join(projectDir, "skills")
	changeDir := filepath.Join(projectDir, "openspec", "changes", "test-feat")

	// Create skill directories with minimal SKILL.md files.
	for _, skill := range []string{"sdd-explore", "sdd-propose", "sdd-spec", "sdd-design", "sdd-tasks", "sdd-apply", "sdd-review", "sdd-clean"} {
		dir := filepath.Join(skillsDir, skill)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+skill+"\n\nInstructions for "+skill+".\n"), 0o644)
	}

	// Create change directory.
	os.MkdirAll(changeDir, 0o755)

	cfg := &config.Config{
		ProjectName: "test-project",
		Stack: config.Stack{
			Language:  "go",
			BuildTool: "go",
			Manifests: []string{"go.mod"},
		},
	}

	p := &Params{
		ChangeDir:   changeDir,
		ChangeName:  "test-feat",
		Description: "Add test feature",
		ProjectDir:  projectDir,
		Config:      cfg,
		SkillsPath:  skillsDir,
	}

	return changeDir, skillsDir, p
}

func TestAssembleExplore(t *testing.T) {
	_, _, p := setupFixture(t)

	var buf bytes.Buffer
	err := AssembleExplore(&buf, p)
	if err != nil {
		t.Fatalf("AssembleExplore: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "--- SKILL ---") {
		t.Error("missing SKILL section")
	}
	if !strings.Contains(out, "sdd-explore") {
		t.Error("missing sdd-explore skill content")
	}
	if !strings.Contains(out, "--- PROJECT ---") {
		t.Error("missing PROJECT section")
	}
	if !strings.Contains(out, "test-project") {
		t.Error("missing project name")
	}
	if !strings.Contains(out, "--- CHANGE ---") {
		t.Error("missing CHANGE section")
	}
	if !strings.Contains(out, "--- FILE TREE ---") {
		t.Error("missing FILE TREE section")
	}
}

func TestAssemblePropose(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	// Create exploration artifact.
	os.WriteFile(filepath.Join(changeDir, "exploration.md"), []byte("# Exploration\n\nKey finding: X is Y.\n"), 0o644)

	var buf bytes.Buffer
	err := AssemblePropose(&buf, p)
	if err != nil {
		t.Fatalf("AssemblePropose: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sdd-propose") {
		t.Error("missing sdd-propose skill content")
	}
	if !strings.Contains(out, "--- EXPLORATION ---") {
		t.Error("missing EXPLORATION section")
	}
	if !strings.Contains(out, "Key finding: X is Y") {
		t.Error("missing exploration content")
	}
}

func TestAssembleProposeNoExploration(t *testing.T) {
	_, _, p := setupFixture(t)

	var buf bytes.Buffer
	err := AssemblePropose(&buf, p)
	if err == nil {
		t.Fatal("expected error when exploration.md is missing")
	}
}

func TestAssembleSpec(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Proposal\n\nWe should add auth.\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleSpec(&buf, p)
	if err != nil {
		t.Fatalf("AssembleSpec: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sdd-spec") {
		t.Error("missing sdd-spec skill content")
	}
	if !strings.Contains(out, "--- PROPOSAL ---") {
		t.Error("missing PROPOSAL section")
	}
	if !strings.Contains(out, "We should add auth") {
		t.Error("missing proposal content")
	}
}

func TestAssembleSpecNoProposal(t *testing.T) {
	_, _, p := setupFixture(t)

	var buf bytes.Buffer
	err := AssembleSpec(&buf, p)
	if err == nil {
		t.Fatal("expected error when proposal.md is missing")
	}
}

func TestAssembleDesign(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Proposal\n"), 0o644)
	specsDir := filepath.Join(changeDir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "auth-spec.md"), []byte("# Auth Spec\n\nMUST validate tokens.\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleDesign(&buf, p)
	if err != nil {
		t.Fatalf("AssembleDesign: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sdd-design") {
		t.Error("missing sdd-design skill content")
	}
	if !strings.Contains(out, "--- SPECIFICATIONS ---") {
		t.Error("missing SPECIFICATIONS section")
	}
	if !strings.Contains(out, "MUST validate tokens") {
		t.Error("missing spec content")
	}
	if !strings.Contains(out, "--- PROPOSAL ---") {
		t.Error("missing PROPOSAL section")
	}
}

func TestAssembleDesignNoSpecs(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "proposal.md"), []byte("# Proposal\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleDesign(&buf, p)
	if err == nil {
		t.Fatal("expected error when specs/ is missing")
	}
}

func TestAssembleTasks(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\n\nUse middleware pattern.\n"), 0o644)
	specsDir := filepath.Join(changeDir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "auth-spec.md"), []byte("# Auth Spec\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleTasks(&buf, p)
	if err != nil {
		t.Fatalf("AssembleTasks: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sdd-tasks") {
		t.Error("missing sdd-tasks skill content")
	}
	if !strings.Contains(out, "--- DESIGN ---") {
		t.Error("missing DESIGN section")
	}
	if !strings.Contains(out, "middleware pattern") {
		t.Error("missing design content")
	}
	if !strings.Contains(out, "--- SPECIFICATIONS ---") {
		t.Error("missing SPECIFICATIONS section")
	}
}

func TestAssembleTasksNoDesign(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	specsDir := filepath.Join(changeDir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "auth-spec.md"), []byte("# Auth Spec\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleTasks(&buf, p)
	if err == nil {
		t.Fatal("expected error when design.md is missing")
	}
}

func TestAssembleDispatcher(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	// Set up enough artifacts for a propose assembler.
	os.WriteFile(filepath.Join(changeDir, "exploration.md"), []byte("# Explore\n"), 0o644)

	var buf bytes.Buffer
	err := Assemble(&buf, state.PhasePropose, p)
	if err != nil {
		t.Fatalf("Assemble propose: %v", err)
	}
	if !strings.Contains(buf.String(), "sdd-propose") {
		t.Error("dispatcher didn't route to propose assembler")
	}
}

func TestAssembleUnknownPhase(t *testing.T) {
	_, _, p := setupFixture(t)

	var buf bytes.Buffer
	err := Assemble(&buf, "nonexistent", p)
	if err == nil {
		t.Fatal("expected error for unknown phase")
	}
}

func TestAssembleMissingSkill(t *testing.T) {
	changeDir, _, p := setupFixture(t)
	p.SkillsPath = "/nonexistent/skills"

	os.WriteFile(filepath.Join(changeDir, "exploration.md"), []byte("# Explore\n"), 0o644)

	var buf bytes.Buffer
	err := AssemblePropose(&buf, p)
	if err == nil {
		t.Fatal("expected error when skill file is missing")
	}
}

// --- Phase 6: Apply ---

func TestAssembleApply(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte(`# Tasks

## Phase 1: Setup

- [x] Create project structure
- [x] Add config loading

## Phase 2: Core

- [ ] Implement state machine
- [ ] Add transition validation
`), 0o644)
	os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\n\nUse middleware.\n"), 0o644)
	specsDir := filepath.Join(changeDir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "core-spec.md"), []byte("# Core Spec\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleApply(&buf, p)
	if err != nil {
		t.Fatalf("AssembleApply: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sdd-apply") {
		t.Error("missing sdd-apply skill content")
	}
	if !strings.Contains(out, "--- CURRENT TASK ---") {
		t.Error("missing CURRENT TASK section")
	}
	if !strings.Contains(out, "Implement state machine") {
		t.Error("missing current incomplete task")
	}
	if !strings.Contains(out, "--- DESIGN ---") {
		t.Error("missing DESIGN section")
	}
	if !strings.Contains(out, "--- SPECIFICATIONS ---") {
		t.Error("missing SPECIFICATIONS section")
	}
}

func TestAssembleApplyNoTasks(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\n"), 0o644)
	specsDir := filepath.Join(changeDir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "spec.md"), []byte("# Spec\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleApply(&buf, p)
	if err == nil {
		t.Fatal("expected error when tasks.md is missing")
	}
}

func TestExtractCurrentTask(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		notWant string
	}{
		{
			name: "finds first incomplete task",
			input: `# Tasks

## Phase 1: Setup

- [x] Create project
- [x] Add config

## Phase 2: Core

- [ ] Implement state machine
- [ ] Add validation

## Phase 3: Polish

- [ ] Add tests`,
			want:    "Implement state machine",
			notWant: "Add tests",
		},
		{
			name: "all complete returns full",
			input: `# Tasks

- [x] Done 1
- [x] Done 2`,
			want: "Done 1",
		},
		{
			name: "includes section header",
			input: `## Phase 2: Core

- [ ] Build the thing`,
			want: "## Phase 2: Core",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCurrentTask(tt.input)
			if !strings.Contains(got, tt.want) {
				t.Errorf("output missing %q\ngot: %s", tt.want, got)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Errorf("output should not contain %q\ngot: %s", tt.notWant, got)
			}
		})
	}
}

// --- Phase 6: Review ---

func TestAssembleReview(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	specsDir := filepath.Join(changeDir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "auth-spec.md"), []byte("# Auth Spec\nMUST validate.\n"), 0o644)
	os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\n"), 0o644)
	os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte("# Tasks\n- [x] All done\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleReview(&buf, p)
	if err != nil {
		t.Fatalf("AssembleReview: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sdd-review") {
		t.Error("missing sdd-review skill content")
	}
	if !strings.Contains(out, "--- SPECIFICATIONS ---") {
		t.Error("missing SPECIFICATIONS section")
	}
	if !strings.Contains(out, "--- DESIGN ---") {
		t.Error("missing DESIGN section")
	}
	if !strings.Contains(out, "--- TASKS ---") {
		t.Error("missing TASKS section")
	}
	if !strings.Contains(out, "--- GIT DIFF ---") {
		t.Error("missing GIT DIFF section")
	}
}

func TestAssembleReviewNoDesign(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	specsDir := filepath.Join(changeDir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "spec.md"), []byte("# Spec\n"), 0o644)
	os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte("# Tasks\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleReview(&buf, p)
	if err == nil {
		t.Fatal("expected error when design.md is missing")
	}
}

func TestAssembleReviewWithProjectRules(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	specsDir := filepath.Join(changeDir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "spec.md"), []byte("# Spec\n"), 0o644)
	os.WriteFile(filepath.Join(changeDir, "design.md"), []byte("# Design\n"), 0o644)
	os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte("# Tasks\n"), 0o644)

	// Create CLAUDE.md in project root.
	os.WriteFile(filepath.Join(p.ProjectDir, "CLAUDE.md"), []byte("# Rules\nNo any types.\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleReview(&buf, p)
	if err != nil {
		t.Fatalf("AssembleReview: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "--- PROJECT RULES ---") {
		t.Error("missing PROJECT RULES section")
	}
	if !strings.Contains(out, "No any types") {
		t.Error("missing CLAUDE.md content")
	}
}

// --- Phase 6: Clean ---

func TestAssembleClean(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "verify-report.md"), []byte("# Verify Report\nVerdict: PASS\n"), 0o644)
	os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte("# Tasks\n- [x] All done\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleClean(&buf, p)
	if err != nil {
		t.Fatalf("AssembleClean: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "sdd-clean") {
		t.Error("missing sdd-clean skill content")
	}
	if !strings.Contains(out, "--- VERIFY REPORT ---") {
		t.Error("missing VERIFY REPORT section")
	}
	if !strings.Contains(out, "Verdict: PASS") {
		t.Error("missing verify report content")
	}
	if !strings.Contains(out, "--- TASKS ---") {
		t.Error("missing TASKS section")
	}
}

func TestAssembleCleanNoVerifyReport(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte("# Tasks\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleClean(&buf, p)
	if err == nil {
		t.Fatal("expected error when verify-report.md is missing")
	}
}

func TestAssembleCleanNoTasks(t *testing.T) {
	changeDir, _, p := setupFixture(t)

	os.WriteFile(filepath.Join(changeDir, "verify-report.md"), []byte("# Report\n"), 0o644)

	var buf bytes.Buffer
	err := AssembleClean(&buf, p)
	if err == nil {
		t.Fatal("expected error when tasks.md is missing")
	}
}

func TestLoadSpecsMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	os.MkdirAll(specsDir, 0o755)
	os.WriteFile(filepath.Join(specsDir, "auth-spec.md"), []byte("auth content"), 0o644)
	os.WriteFile(filepath.Join(specsDir, "api-spec.md"), []byte("api content"), 0o644)
	os.WriteFile(filepath.Join(specsDir, "readme.txt"), []byte("not a spec"), 0o644) // non-.md ignored

	specs, err := loadSpecs(dir)
	if err != nil {
		t.Fatalf("loadSpecs: %v", err)
	}
	if !strings.Contains(specs, "auth content") {
		t.Error("missing auth spec content")
	}
	if !strings.Contains(specs, "api content") {
		t.Error("missing api spec content")
	}
	if strings.Contains(specs, "not a spec") {
		t.Error("non-.md file should be ignored")
	}
}
