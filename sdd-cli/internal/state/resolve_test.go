package state

import "testing"

func TestResolvePhase(t *testing.T) {
	tests := []struct {
		input   string
		want    Phase
		wantErr string
	}{
		{"explore", PhaseExplore, ""},
		{"propose", PhasePropose, ""},
		{"spec", PhaseSpec, ""},
		{"design", PhaseDesign, ""},
		{"tasks", PhaseTasks, ""},
		{"apply", PhaseApply, ""},
		{"review", PhaseReview, ""},
		{"verify", PhaseVerify, ""},
		{"clean", PhaseClean, ""},
		{"archive", PhaseArchive, ""},
		// Index
		{"0", PhaseExplore, ""},
		{"1", PhasePropose, ""},
		{"5", PhaseApply, ""},
		{"9", PhaseArchive, ""},
		{"10", "", "out of range"},
		{"-1", "", "out of range"},
		// Prefix
		{"exp", PhaseExplore, ""},
		{"pro", PhasePropose, ""},
		{"sp", PhaseSpec, ""},
		{"d", PhaseDesign, ""},
		{"t", PhaseTasks, ""},
		{"ap", PhaseApply, ""},
		{"rev", PhaseReview, ""},
		{"v", PhaseVerify, ""},
		{"cl", PhaseClean, ""},
		{"ar", PhaseArchive, ""},
		// Case insensitive
		{"EXP", PhaseExplore, ""},
		{"Propose", PhasePropose, ""},
		// Unknown
		{"xyz", "", "unknown phase"},
		{"", "", "ambiguous"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ResolvePhase(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
