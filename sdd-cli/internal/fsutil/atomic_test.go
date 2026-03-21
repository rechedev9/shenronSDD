package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWrite(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.txt")
	if err := AtomicWrite(path, []byte("hello")); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestAtomicWrite_NoTmpLeftOnSuccess(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.txt")
	AtomicWrite(path, []byte("data"))
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("tmp file still exists after successful write")
	}
}
