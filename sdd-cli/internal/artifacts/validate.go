package artifacts

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/state"
)

// ErrValidation indicates that an artifact failed content validation.
var ErrValidation = errors.New("artifact validation failed")

// rule checks one requirement against artifact content.
type rule struct {
	name  string
	check func([]byte) bool
}

var fileLineRef = regexp.MustCompile(`\w+\.\w+:\d+`)

var phaseRules = map[state.Phase][]rule{
	state.PhaseExplore: {
		{name: "## Current State heading", check: containsStr("## Current State")},
		{name: "## Relevant Files heading", check: containsStr("## Relevant Files")},
	},
	state.PhasePropose: {
		{name: "## Intent heading", check: containsStr("## Intent")},
		{name: "## Scope heading", check: containsStr("## Scope")},
	},
	state.PhaseSpec: {
		{name: "at least one ## heading", check: containsStr("## ")},
	},
	state.PhaseDesign: {
		{name: "at least one ## heading", check: containsStr("## ")},
	},
	state.PhaseTasks: {
		{name: "at least one task checkbox", check: containsStr("- [")},
	},
	state.PhaseApply: {
		{name: "at least one task checkbox", check: containsStr("- [")},
	},
	state.PhaseReview: {
		{name: "at least one ## heading", check: containsStr("## ")},
		{name: "file:line reference (e.g. main.go:42)", check: matchesRegex(fileLineRef)},
		{name: "verdict (PASS, FAIL, APPROVED, or REJECTED)", check: containsAny("PASS", "FAIL", "APPROVED", "REJECTED")},
	},
	// verify, clean, archive — no content rules (or minimal)
	state.PhaseClean: {
		{name: "at least one ## heading", check: containsStr("## ")},
	},
}

// Validate checks that content satisfies all rules for the given phase.
// Returns nil if valid or if the phase has no rules.
func Validate(phase state.Phase, content []byte) error {
	rules, ok := phaseRules[phase]
	if !ok || len(rules) == 0 {
		return nil
	}

	var missing []string
	for _, r := range rules {
		if !r.check(content) {
			missing = append(missing, r.name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s: missing %s", ErrValidation, phase, strings.Join(missing, ", "))
}

func containsStr(s string) func([]byte) bool {
	return func(content []byte) bool {
		return strings.Contains(string(content), s)
	}
}

func containsAny(options ...string) func([]byte) bool {
	return func(content []byte) bool {
		text := string(content)
		for _, opt := range options {
			if strings.Contains(text, opt) {
				return true
			}
		}
		return false
	}
}

func matchesRegex(re *regexp.Regexp) func([]byte) bool {
	return func(content []byte) bool {
		return re.Match(content)
	}
}
