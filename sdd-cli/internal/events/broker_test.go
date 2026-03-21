package events

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestBroker_SubscribeAndEmit(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	var called int
	b.Subscribe(PhaseAssembled, func(e Event) {
		called++
	})
	b.Emit(Event{Type: PhaseAssembled})
	if called != 1 {
		t.Errorf("handler called %d times, want 1", called)
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	var count atomic.Int32
	b.Subscribe(CacheHit, func(e Event) { count.Add(1) })
	b.Subscribe(CacheHit, func(e Event) { count.Add(1) })
	b.Emit(Event{Type: CacheHit})
	if c := count.Load(); c != 2 {
		t.Errorf("count = %d, want 2", c)
	}
}

func TestBroker_NonMatchingType(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	var called bool
	b.Subscribe(CacheHit, func(e Event) { called = true })
	b.Emit(Event{Type: CacheMiss})
	if called {
		t.Error("handler called for non-matching event type")
	}
}

func TestBroker_EmitNoSubscribers(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	// Should not panic.
	b.Emit(Event{Type: PhaseAssembled})
}

func TestBroker_SubscriberPanicRecovery(t *testing.T) {
	t.Parallel()
	b := NewBroker()

	var order []int
	b.Subscribe(PhaseAssembled, func(e Event) { order = append(order, 1) })
	b.Subscribe(PhaseAssembled, func(e Event) { panic("test panic") })
	b.Subscribe(PhaseAssembled, func(e Event) { order = append(order, 3) })

	b.Emit(Event{Type: PhaseAssembled})

	if len(order) != 2 || order[0] != 1 || order[1] != 3 {
		t.Errorf("order = %v, want [1, 3] (panicking subscriber skipped)", order)
	}
}

func TestBroker_ConcurrentEmit(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	var count atomic.Int64
	b.Subscribe(PhaseAssembled, func(e Event) {
		count.Add(1)
	})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Emit(Event{Type: PhaseAssembled})
		}()
	}
	wg.Wait()

	if c := count.Load(); c != 10 {
		t.Errorf("count = %d, want 10", c)
	}
}

func TestBroker_NilSafe(t *testing.T) {
	t.Parallel()
	var b *Broker
	// Should not panic.
	b.Subscribe(PhaseAssembled, func(e Event) {})
	b.Emit(Event{Type: PhaseAssembled})
}

func TestBroker_Payload(t *testing.T) {
	t.Parallel()
	b := NewBroker()

	var got PhaseAssembledPayload
	b.Subscribe(PhaseAssembled, func(e Event) {
		got = e.Payload.(PhaseAssembledPayload)
	})

	b.Emit(Event{
		Type: PhaseAssembled,
		Payload: PhaseAssembledPayload{
			Phase:      "explore",
			Bytes:      1024,
			Tokens:     256,
			Cached:     false,
			DurationMs: 42,
			ChangeDir:  "/tmp/test",
			SkillsPath: "/skills",
		},
	})

	if got.Phase != "explore" || got.Bytes != 1024 || got.Tokens != 256 {
		t.Errorf("payload = %+v, unexpected values", got)
	}
}

func TestBroker_AllEventTypes(t *testing.T) {
	t.Parallel()
	b := NewBroker()
	types := []EventType{PhaseAssembled, CacheHit, CacheMiss, ArtifactPromoted, StateAdvanced, VerifyFailed}

	received := make(map[EventType]bool)
	for _, et := range types {
		et := et
		b.Subscribe(et, func(e Event) {
			received[et] = true
		})
	}

	for _, et := range types {
		b.Emit(Event{Type: et})
	}

	for _, et := range types {
		if !received[et] {
			t.Errorf("event type %s not received", et)
		}
	}
}
