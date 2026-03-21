package phase

import (
	"sync"
	"testing"
)

// TestChaosRegistryConcurrentReads hammers Get/All/AllNames from many goroutines.
// The race detector will catch any unsafe concurrent access.
func TestChaosRegistryConcurrentReads(t *testing.T) {
	t.Parallel()
	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				DefaultRegistry.Get("explore")
				DefaultRegistry.Get("nonexistent")
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				DefaultRegistry.All()
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				DefaultRegistry.AllNames()
			}
		}()
	}

	wg.Wait()
}
