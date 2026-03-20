package errs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestUsageError(t *testing.T) {
	err := Usage("bad input")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() != "bad input" {
		t.Errorf("error = %q, want %q", err.Error(), "bad input")
	}
	if !IsUsage(err) {
		t.Error("expected IsUsage to return true")
	}
}

func TestIsUsageNonUsage(t *testing.T) {
	err := WriteJSON(new(bytes.Buffer), "test", "msg")
	if IsUsage(err) {
		t.Error("expected IsUsage to return false for non-usage error")
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	err := WriteJSON(&buf, "init", "not implemented")
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	var je JSONError
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &je); jsonErr != nil {
		t.Fatalf("invalid JSON: %v", jsonErr)
	}
	if je.Command != "init" {
		t.Errorf("command = %q, want %q", je.Command, "init")
	}
	if je.Code != "not_implemented" {
		t.Errorf("code = %q, want %q", je.Code, "not_implemented")
	}
}

func TestTransportError(t *testing.T) {
	err := Transport("connection refused")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Error() != "connection refused" {
		t.Errorf("error = %q, want %q", err.Error(), "connection refused")
	}
	if !IsTransport(err) {
		t.Error("expected IsTransport to return true")
	}
}

func TestIsTransportNonTransport(t *testing.T) {
	err := Usage("bad input")
	if IsTransport(err) {
		t.Error("expected IsTransport to return false for usage error")
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"usage error", Usage("bad"), "usage"},
		{"transport error", Transport("git timeout"), "transport"},
		{"internal error", WriteJSON(new(bytes.Buffer), "x", "fail"), "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			_ = WriteError(&buf, "test", tt.err)

			var je JSONError
			if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &je); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if je.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", je.Code, tt.wantCode)
			}
		})
	}
}
