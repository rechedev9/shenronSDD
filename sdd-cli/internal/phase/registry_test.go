package phase_test

import (
	"testing"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/phase"
)

func TestBuiltinPhaseCount(t *testing.T) {
	all := phase.DefaultRegistry.All()
	if len(all) != 10 {
		t.Fatalf("expected 10 built-in phases, got %d", len(all))
	}
}

func TestBuiltinPhaseNamesUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range phase.DefaultRegistry.All() {
		if seen[p.Name] {
			t.Fatalf("duplicate phase name: %s", p.Name)
		}
		seen[p.Name] = true
	}
}

func TestPrerequisiteGraphAcyclic(t *testing.T) {
	all := phase.DefaultRegistry.All()
	byName := map[string]phase.Phase{}
	for _, p := range all {
		byName[p.Name] = p
	}

	// DFS cycle detection.
	white, gray, black := 0, 1, 2
	color := map[string]int{}
	for _, p := range all {
		color[p.Name] = white
	}

	var visit func(string) bool
	visit = func(name string) bool {
		color[name] = gray
		p := byName[name]
		for _, req := range p.Prerequisites {
			switch color[req] {
			case gray:
				return true // cycle
			case white:
				if visit(req) {
					return true
				}
			}
		}
		color[name] = black
		return false
	}

	for _, p := range all {
		if color[p.Name] == white {
			if visit(p.Name) {
				t.Fatalf("cycle detected involving phase: %s", p.Name)
			}
		}
	}
}

func TestAllPhasesOrder(t *testing.T) {
	expected := []string{
		"explore", "propose", "spec", "design", "tasks",
		"apply", "review", "verify", "clean", "archive",
	}
	names := phase.DefaultRegistry.AllNames()
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Fatalf("position %d: expected %q, got %q", i, expected[i], name)
		}
	}
}

func TestVerifyAndArchiveHaveNilAssemble(t *testing.T) {
	// Before context.init() wires assemblers, all Assemble fields are nil.
	// verify and archive should remain nil even after wiring (they have
	// no assembler). This test runs in the phase package — context.init()
	// has NOT run, so we only check the default state.
	for _, name := range []string{"verify", "archive"} {
		p, ok := phase.DefaultRegistry.Get(name)
		if !ok {
			t.Fatalf("phase %q not found", name)
		}
		if p.Assemble != nil {
			t.Fatalf("phase %q should have nil Assemble", name)
		}
	}
}

func TestApplyHasRecoverSkip(t *testing.T) {
	p, ok := phase.DefaultRegistry.Get("apply")
	if !ok {
		t.Fatal("apply phase not found")
	}
	if !p.RecoverSkip {
		t.Fatal("apply should have RecoverSkip=true")
	}
}

func TestCustomPhaseRegistration(t *testing.T) {
	r := &phase.Registry{}
	r.Register(phase.Phase{
		Name:          "custom-phase",
		Prerequisites: []string{},
		ArtifactFile:  "custom.md",
		CacheTTL:      1 * time.Hour,
	})
	p, ok := r.Get("custom-phase")
	if !ok {
		t.Fatal("custom phase not found after registration")
	}
	if p.ArtifactFile != "custom.md" {
		t.Fatalf("expected ArtifactFile=custom.md, got %s", p.ArtifactFile)
	}
	names := r.AllNames()
	if len(names) != 1 || names[0] != "custom-phase" {
		t.Fatalf("unexpected AllNames: %v", names)
	}
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	r := &phase.Registry{}
	r.Register(phase.Phase{Name: "dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	r.Register(phase.Phase{Name: "dup"})
}

func TestEmptyNamePanics(t *testing.T) {
	r := &phase.Registry{}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty name")
		}
	}()
	r.Register(phase.Phase{Name: ""})
}

func TestSealedRegistryPanicsOnRegister(t *testing.T) {
	r := &phase.Registry{}
	r.Register(phase.Phase{Name: "seal-test"})
	r.Get("seal-test") // seals
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on Register after seal")
		}
	}()
	r.Register(phase.Phase{Name: "late"})
}
