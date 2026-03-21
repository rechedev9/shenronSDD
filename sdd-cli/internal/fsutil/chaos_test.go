package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestChaosAtomicWriteConcurrent writes to the same file from many goroutines.
// AtomicWrite (write-to-tmp + rename) must never produce corrupt reads.
func TestChaosAtomicWriteConcurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "target.json")

	const goroutines = 20
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				data := []byte(fmt.Sprintf(`{"writer":%d,"iter":%d}`, id, j))
				if err := AtomicWrite(path, data); err != nil {
					t.Errorf("AtomicWrite(%d,%d): %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Final file must exist and contain valid JSON (from some writer).
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if len(data) == 0 {
		t.Error("final file is empty")
	}
	// Must start with { — not truncated or corrupt.
	if data[0] != '{' {
		t.Errorf("corrupt content: starts with %q", string(data[:1]))
	}
}

// TestChaosAtomicWriteReadConcurrent reads while writing atomically.
// Reads must never see partial content.
func TestChaosAtomicWriteReadConcurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "rw.json")

	// Seed initial content.
	AtomicWrite(path, []byte(`{"init":true}`))

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			data := []byte(fmt.Sprintf(`{"writer":true,"iter":%d}`, i))
			_ = AtomicWrite(path, data)
		}
	}()

	// Reader — must never see partial/corrupt content.
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			data, err := os.ReadFile(path)
			if err != nil {
				// File might briefly not exist during rename — acceptable.
				continue
			}
			if len(data) == 0 {
				continue // empty read during rename
			}
			if data[0] != '{' {
				t.Errorf("corrupt read at iteration %d: %q", i, string(data[:min(20, len(data))]))
			}
		}
	}()

	wg.Wait()
}

// TestChaosAtomicWriteDifferentFiles writes to many different files concurrently.
func TestChaosAtomicWriteDifferentFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	const files = 50
	var wg sync.WaitGroup
	wg.Add(files)

	for i := 0; i < files; i++ {
		go func(n int) {
			defer wg.Done()
			path := filepath.Join(dir, fmt.Sprintf("file-%d.json", n))
			for j := 0; j < 20; j++ {
				data := []byte(fmt.Sprintf(`{"file":%d,"iter":%d}`, n, j))
				if err := AtomicWrite(path, data); err != nil {
					t.Errorf("AtomicWrite(file-%d, %d): %v", n, j, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// All files should exist.
	entries, _ := os.ReadDir(dir)
	// Filter out .tmp files that might linger.
	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			count++
		}
	}
	if count != files {
		t.Errorf("file count = %d, want %d", count, files)
	}
}
