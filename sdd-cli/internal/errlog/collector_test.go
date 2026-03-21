package errlog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprint_Deterministic(t *testing.T) {
	t.Parallel()
	a := Fingerprint("go test ./...", []string{"FAIL main_test.go:12"})
	b := Fingerprint("go test ./...", []string{"FAIL main_test.go:12"})
	if a != b {
		t.Errorf("same input produced different fingerprints: %s vs %s", a, b)
	}
	if len(a) != 16 {
		t.Errorf("fingerprint length = %d, want 16", len(a))
	}
}

func TestFingerprint_DifferentCommands(t *testing.T) {
	t.Parallel()
	a := Fingerprint("go test ./...", []string{"FAIL"})
	b := Fingerprint("golangci-lint run", []string{"FAIL"})
	if a == b {
		t.Error("different commands produced the same fingerprint")
	}
}

func TestFingerprint_EmptyErrorLines(t *testing.T) {
	t.Parallel()
	fp := Fingerprint("go build ./...", nil)
	if fp == "" {
		t.Error("fingerprint is empty for nil error lines")
	}
}

func TestRecord_Roundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Create openspec/.cache directory structure.
	os.MkdirAll(filepath.Join(dir, "openspec", ".cache"), 0o755)

	entry := ErrorEntry{
		Timestamp:   "2026-03-21T12:00:00Z",
		Change:      "test-change",
		CommandName: "build",
		Command:     "go build ./...",
		ExitCode:    1,
		ErrorLines:  []string{"error: undefined"},
		Fingerprint: Fingerprint("go build ./...", []string{"error: undefined"}),
	}
	Record(dir, entry)

	log := Load(dir)
	if len(log.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(log.Entries))
	}
	if log.Entries[0].Change != "test-change" {
		t.Errorf("change = %q, want %q", log.Entries[0].Change, "test-change")
	}
}

func TestRecord_EvictsOldest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "openspec", ".cache"), 0o755)

	for i := 0; i < 105; i++ {
		Record(dir, ErrorEntry{
			Timestamp:   "2026-03-21T12:00:00Z",
			Change:      "evict-test",
			CommandName: "test",
			Command:     "go test",
			ExitCode:    1,
			Fingerprint: "aaaa",
		})
	}

	log := Load(dir)
	if len(log.Entries) != maxEntries {
		t.Errorf("entries = %d, want %d", len(log.Entries), maxEntries)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()
	log := Load(t.TempDir())
	if log == nil {
		t.Fatal("Load returned nil for missing file")
	}
	if len(log.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(log.Entries))
	}
	if log.Version != logVersion {
		t.Errorf("version = %d, want %d", log.Version, logVersion)
	}
}

func TestLoad_CorruptJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := LogPath(dir)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("{bad json"), 0o644)

	log := Load(dir)
	if len(log.Entries) != 0 {
		t.Errorf("entries = %d, want 0 for corrupt JSON", len(log.Entries))
	}
}

func TestRecurringFingerprints(t *testing.T) {
	t.Parallel()
	log := &ErrorLog{
		Version: logVersion,
		Entries: []ErrorEntry{
			{Fingerprint: "aaa"},
			{Fingerprint: "aaa"},
			{Fingerprint: "aaa"},
			{Fingerprint: "bbb"},
			{Fingerprint: "bbb"},
			{Fingerprint: "ccc"},
		},
	}

	got := log.RecurringFingerprints(3)
	if len(got) != 1 {
		t.Fatalf("recurring = %d, want 1", len(got))
	}
	if got["aaa"] != 3 {
		t.Errorf("aaa count = %d, want 3", got["aaa"])
	}

	got2 := log.RecurringFingerprints(2)
	if len(got2) != 2 {
		t.Errorf("recurring(2) = %d, want 2", len(got2))
	}
}
