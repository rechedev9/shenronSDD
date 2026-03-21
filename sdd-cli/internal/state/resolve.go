package state

import (
	"fmt"
	"strconv"
	"strings"
)

func ResolvePhase(input string) (Phase, error) {
	phases := AllPhases()

	for _, p := range phases {
		if string(p) == input {
			return p, nil
		}
	}

	if idx, err := strconv.Atoi(input); err == nil {
		if idx < 0 || idx >= len(phases) {
			return "", fmt.Errorf("phase index out of range: %s (valid: 0-%d)", input, len(phases)-1)
		}
		return phases[idx], nil
	}

	lower := strings.ToLower(input)
	var matches []string
	for _, p := range phases {
		if strings.HasPrefix(strings.ToLower(string(p)), lower) {
			matches = append(matches, string(p))
		}
	}
	switch len(matches) {
	case 1:
		return Phase(matches[0]), nil
	case 0:
		return "", fmt.Errorf("unknown phase: %q", input)
	default:
		return "", fmt.Errorf("ambiguous phase prefix %q: matches %s", input, strings.Join(matches, ", "))
	}
}
