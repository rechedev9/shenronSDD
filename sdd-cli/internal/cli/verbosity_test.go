package cli

import (
	"reflect"
	"testing"
)

func TestParseVerbosityFlags(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		wantArgs  []string
		wantLevel Verbosity
	}{
		{"quiet short", []string{"-q", "foo"}, []string{"foo"}, VerbosityQuiet},
		{"quiet long", []string{"--quiet"}, []string{}, VerbosityQuiet},
		{"verbose short", []string{"-v"}, []string{}, VerbosityVerbose},
		{"verbose long", []string{"--verbose"}, []string{}, VerbosityVerbose},
		{"debug short", []string{"-d"}, []string{}, VerbosityDebug},
		{"debug long", []string{"--debug"}, []string{}, VerbosityDebug},
		{"no flags", []string{"foo", "bar"}, []string{"foo", "bar"}, VerbosityDefault},
		{"last wins qv", []string{"-q", "-v"}, []string{}, VerbosityVerbose},
		{"last wins dq", []string{"--debug", "--quiet"}, []string{}, VerbosityQuiet},
		{"unknown passes through", []string{"--unknown", "-v"}, []string{"--unknown"}, VerbosityVerbose},
		{"nil input", nil, []string{}, VerbosityDefault},
		{"mixed positional", []string{"name", "-q", "phase"}, []string{"name", "phase"}, VerbosityQuiet},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotArgs, gotLevel := ParseVerbosityFlags(tt.input)
			if gotLevel != tt.wantLevel {
				t.Errorf("verbosity = %d, want %d", gotLevel, tt.wantLevel)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("args = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}
