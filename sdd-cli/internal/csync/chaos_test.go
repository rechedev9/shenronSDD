package csync

import (
	"fmt"
	"sync"
	"testing"
)

// TestChaosLazySliceConcurrentLoadAll creates and loads many LazySlices concurrently.
func TestChaosLazySliceConcurrentLoadAll(t *testing.T) {
	t.Parallel()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			loaders := make([]func() (string, error), 10)
			for j := range loaders {
				j := j
				loaders[j] = func() (string, error) {
					return fmt.Sprintf("result-%d-%d", id, j), nil
				}
			}
			ls := NewLazySlice(loaders)
			if err := ls.LoadAll(); err != nil {
				t.Errorf("goroutine %d: LoadAll: %v", id, err)
				return
			}
			for j := 0; j < ls.Len(); j++ {
				val, err := ls.Get(j)
				if err != nil {
					t.Errorf("goroutine %d: Get(%d): %v", id, j, err)
				}
				want := fmt.Sprintf("result-%d-%d", id, j)
				if val != want {
					t.Errorf("goroutine %d: Get(%d) = %q, want %q", id, j, val, want)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestChaosLazySliceWithErrors verifies concurrent loads where some loaders fail.
func TestChaosLazySliceWithErrors(t *testing.T) {
	t.Parallel()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			loaders := []func() (string, error){
				func() (string, error) { return "ok", nil },
				func() (string, error) { return "", fmt.Errorf("fail-%d", id) },
				func() (string, error) { return "ok2", nil },
			}
			ls := NewLazySlice(loaders)
			err := ls.LoadAll()
			if err == nil {
				t.Errorf("goroutine %d: expected error from failing loader", id)
			}
		}(i)
	}

	wg.Wait()
}
