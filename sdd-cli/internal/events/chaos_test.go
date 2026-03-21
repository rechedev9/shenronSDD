package events

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestChaosBrokerConcurrentEmit fires events from many goroutines simultaneously.
func TestChaosBrokerConcurrentEmit(t *testing.T) {
	t.Parallel()
	b := NewBroker()

	var count atomic.Int64
	b.Subscribe(PhaseAssembled, func(_ Event) {
		count.Add(1)
	})
	b.Subscribe(CacheHit, func(_ Event) {
		count.Add(1)
	})

	const goroutines = 50
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				b.Emit(Event{Type: PhaseAssembled, Payload: PhaseAssembledPayload{Phase: "test"}})
				b.Emit(Event{Type: CacheHit, Payload: CacheHitPayload{Phase: "test"}})
			}
		}()
	}

	wg.Wait()

	got := count.Load()
	want := int64(goroutines * iterations * 2)
	if got != want {
		t.Errorf("event count = %d, want %d", got, want)
	}
}

// TestChaosBrokerSubscribeDuringEmit adds subscribers while events are firing.
func TestChaosBrokerSubscribeDuringEmit(t *testing.T) {
	t.Parallel()
	b := NewBroker()

	var wg sync.WaitGroup
	wg.Add(2)

	// Emitter goroutine.
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			b.Emit(Event{Type: PhaseAssembled, Payload: PhaseAssembledPayload{Phase: "test"}})
		}
	}()

	// Subscriber goroutine — adds handlers while emitter is running.
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			b.Subscribe(PhaseAssembled, func(_ Event) {})
		}
	}()

	wg.Wait()
}

// TestChaosBrokerNilSafe verifies nil broker doesn't panic under concurrent access.
func TestChaosBrokerNilSafe(t *testing.T) {
	t.Parallel()
	var b *Broker // nil

	var wg sync.WaitGroup
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			b.Emit(Event{Type: PhaseAssembled})
			b.Subscribe(PhaseAssembled, func(_ Event) {})
		}()
	}
	wg.Wait()
}
