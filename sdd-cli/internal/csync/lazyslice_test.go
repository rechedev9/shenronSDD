package csync

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLazySlice_NilLoaders(t *testing.T) {
	t.Parallel()
	ls := NewLazySlice[int](nil)
	if ls.Len() != 0 {
		t.Errorf("Len() = %d, want 0", ls.Len())
	}
	if err := ls.LoadAll(); err != nil {
		t.Errorf("LoadAll() = %v, want nil", err)
	}
}

func TestNewLazySlice_EmptyLoaders(t *testing.T) {
	t.Parallel()
	ls := NewLazySlice[int]([]func() (int, error){})
	if ls.Len() != 0 {
		t.Errorf("Len() = %d, want 0", ls.Len())
	}
}

func TestNewLazySlice_Len(t *testing.T) {
	t.Parallel()
	loaders := []func() (string, error){
		func() (string, error) { return "a", nil },
		func() (string, error) { return "b", nil },
		func() (string, error) { return "c", nil },
	}
	ls := NewLazySlice(loaders)
	if ls.Len() != 3 {
		t.Errorf("Len() = %d, want 3", ls.Len())
	}
}

func TestLoadAll_Results(t *testing.T) {
	t.Parallel()
	loaders := []func() (string, error){
		func() (string, error) { return "alpha", nil },
		func() (string, error) { return "beta", nil },
		func() (string, error) { return "gamma", nil },
	}

	ls := NewLazySlice(loaders)
	if err := ls.LoadAll(); err != nil {
		t.Fatalf("LoadAll() = %v", err)
	}

	for i, want := range []string{"alpha", "beta", "gamma"} {
		got, err := ls.Get(i)
		if err != nil {
			t.Errorf("Get(%d) error = %v", i, err)
		}
		if got != want {
			t.Errorf("Get(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestLoadAll_Idempotent(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	loaders := []func() (int, error){
		func() (int, error) { count.Add(1); return 1, nil },
	}
	ls := NewLazySlice(loaders)
	ls.LoadAll()
	ls.LoadAll() // second call should be no-op
	if c := count.Load(); c != 1 {
		t.Errorf("loader called %d times, want 1", c)
	}
}

func TestLoadAll_PartialFailure(t *testing.T) {
	t.Parallel()
	loaders := []func() (string, error){
		func() (string, error) { return "ok-0", nil },
		func() (string, error) { return "", fmt.Errorf("fail-1") },
		func() (string, error) { return "ok-2", nil },
		func() (string, error) { return "", fmt.Errorf("fail-3") },
		func() (string, error) { return "ok-4", nil },
	}

	ls := NewLazySlice(loaders)
	err := ls.LoadAll()
	if err == nil {
		t.Fatal("LoadAll() = nil, want error")
	}

	// Error message should contain "2/5".
	if !strings.Contains(err.Error(), "2/5") {
		t.Errorf("error %q missing '2/5' count", err.Error())
	}

	// Successful loaders still have their values.
	v0, e0 := ls.Get(0)
	if e0 != nil || v0 != "ok-0" {
		t.Errorf("Get(0) = (%q, %v), want ('ok-0', nil)", v0, e0)
	}

	_, e1 := ls.Get(1)
	if e1 == nil {
		t.Error("Get(1) error = nil, want error")
	}

	v2, e2 := ls.Get(2)
	if e2 != nil || v2 != "ok-2" {
		t.Errorf("Get(2) = (%q, %v), want ('ok-2', nil)", v2, e2)
	}
}

func TestLoadAll_PanicRecovery(t *testing.T) {
	t.Parallel()
	loaders := []func() (string, error){
		func() (string, error) { return "ok", nil },
		func() (string, error) { panic("boom") },
		func() (string, error) { return "also-ok", nil },
	}

	ls := NewLazySlice(loaders)
	err := ls.LoadAll()
	if err == nil {
		t.Fatal("LoadAll() = nil, want error")
	}

	v0, e0 := ls.Get(0)
	if e0 != nil || v0 != "ok" {
		t.Errorf("Get(0) = (%q, %v), want ('ok', nil)", v0, e0)
	}

	_, e1 := ls.Get(1)
	if e1 == nil || !strings.Contains(e1.Error(), "panicked") {
		t.Errorf("Get(1) error = %v, want panic error", e1)
	}

	v2, e2 := ls.Get(2)
	if e2 != nil || v2 != "also-ok" {
		t.Errorf("Get(2) = (%q, %v), want ('also-ok', nil)", v2, e2)
	}
}

func TestLoadAll_GoroutineBound(t *testing.T) {
	t.Parallel()
	var peak atomic.Int32
	var active atomic.Int32

	n := 20
	loaders := make([]func() (int, error), n)
	for i := range loaders {
		loaders[i] = func() (int, error) {
			cur := active.Add(1)
			// Track peak concurrency.
			for {
				p := peak.Load()
				if cur <= p || peak.CompareAndSwap(p, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			active.Add(-1)
			return 0, nil
		}
	}

	ls := NewLazySlice(loaders)
	ls.LoadAll()

	maxW := maxWorkers()
	if p := int(peak.Load()); p > maxW {
		t.Errorf("peak concurrency = %d, exceeds bound %d", p, maxW)
	}
}

func TestLoadAll_NoGoroutineLeak(t *testing.T) {
	t.Parallel()
	before := runtime.NumGoroutine()

	loaders := []func() (int, error){
		func() (int, error) { return 1, nil },
		func() (int, error) { return 2, fmt.Errorf("err") },
		func() (int, error) { panic("boom") },
	}

	ls := NewLazySlice(loaders)
	ls.LoadAll()

	// Give goroutines time to fully exit.
	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()
	delta := after - before
	if delta > 2 {
		t.Errorf("goroutine leak: before=%d after=%d delta=%d", before, after, delta)
	}
}

func TestMustGet_Success(t *testing.T) {
	t.Parallel()
	ls := NewLazySlice([]func() (string, error){
		func() (string, error) { return "hello", nil },
	})
	ls.LoadAll()

	got := ls.MustGet(0)
	if got != "hello" {
		t.Errorf("MustGet(0) = %q, want 'hello'", got)
	}
}

func TestMustGet_Panics(t *testing.T) {
	t.Parallel()
	ls := NewLazySlice([]func() (string, error){
		func() (string, error) { return "", fmt.Errorf("bad") },
	})
	ls.LoadAll()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustGet did not panic")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, "bad") {
			t.Errorf("panic message %q missing 'bad'", msg)
		}
	}()
	ls.MustGet(0)
}

func TestMaxWorkers(t *testing.T) {
	t.Parallel()
	w := maxWorkers()
	if w < 1 || w > 8 {
		t.Errorf("maxWorkers() = %d, want [1, 8]", w)
	}
}
