# Technical Design: Concurrency and Performance

**Change**: concurrency-performance
**Date**: 2026-03-21
**Status**: draft
**Depends On**: proposal.md, spec.md

---

## 1. Architecture Overview

### Module Graph

```
cmd/sdd/main.go
  |
  |  SDD_PPROF gate (new)
  |  runtime/pprof start/stop
  |
  v
internal/cli/commands.go
  |
  |  creates events.Broker (new)
  |  wires Broker into Params
  |  emits ArtifactPromoted, StateAdvanced from runWrite()
  |
  v
internal/context/context.go
  |
  |  Params.Broker field (new)
  |  Assemble() emits CacheHit/CacheMiss/PhaseAssembled
  |
  +---> internal/events/broker.go  (NEW PACKAGE)
  |       Broker, Event, EventType
  |       Subscribe(), Emit()
  |       sync.Mutex serialized dispatch
  |
  +---> internal/csync/lazyslice.go  (NEW PACKAGE)
  |       LazySlice[T]
  |       bounded worker pool via buffered channel
  |       sync.WaitGroup + panic recovery
  |
  +---> internal/context/{explore,propose,spec,design,tasks,apply,review,clean}.go
          each assembler uses csync.LazySlice for concurrent I/O
```

### Dependency Direction

```
cmd/sdd/main.go
    |
    v
internal/cli  --->  internal/events
    |                     ^
    v                     |
internal/context ---------+
    |
    v
internal/csync   (no upward deps; leaf package)
```

`internal/csync` and `internal/events` are leaf packages with zero internal dependencies. `internal/context` imports both. `internal/cli` imports `internal/events` (to create the Broker) and `internal/context` (existing).

### Data Flow: Lazy Loading Within an Assembler

```
AssembleReview(w, p)
  |
  |  1. Build []func() ([]byte, error) closures
  |     [loadSkill, loadSpecs, loadDesign, loadTasks, gitDiff, loadRules]
  |
  |  2. ls = csync.NewLazySlice(loaders)
  |
  |  3. ls.LoadAll()
  |     |
  |     +---> sem <- struct{}{} (acquire from buffered chan, cap=min(NumCPU,8))
  |     |     go func(i) {
  |     |       defer wg.Done()
  |     |       defer func() { <-sem }()  // release
  |     |       defer recover()           // panic -> error
  |     |       results[i] = loader()
  |     |     }
  |     |
  |     +---> wg.Wait()  // blocks until all done
  |
  |  4. Access results by index:
  |     skill, err := ls.Get(0)   // loadSkill result
  |     specs, err := ls.Get(1)   // loadSpecs result
  |     design, err := ls.Get(2)  // loadDesign result
  |     ...
  |
  |  5. writeSection(w, ...) calls — same order as before
  v
  return
```

### Event Flow: Emitter to Subscriber

```
Assemble(w, phase, p)
  |
  |  cache miss path:
  |    p.Broker.Emit(Event{Type: CacheMiss, ...})
  |    fn(&buf, p)   // run assembler
  |    p.Broker.Emit(Event{Type: PhaseAssembled, ...})
  |
  v
Broker.Emit(event)
  |
  |  mu.Lock()            // serialize all dispatches
  |  handlers := subs[event.Type]
  |  for _, h := range handlers {
  |    func() {
  |      defer recover()  // per-handler panic recovery
  |      h(event)
  |    }()
  |  }
  |  mu.Unlock()
  |
  +---> metricsSubscriber(event)     // was inline recordMetrics()
  |       recordMetrics(changeDir, m)
  |
  +---> stderrSubscriber(event)      // was inline writeMetrics()
  |       writeMetrics(stderr, m, verbosity)
  |
  +---> cacheSubscriber(event)       // was inline saveContextCache()
          saveContextCache(changeDir, phase, skills, content)
```

## 2. Package: internal/csync

File: `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice.go`

### Type Definitions

```go
package csync

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
)

// result holds the outcome of a single loader invocation.
type result[T any] struct {
	value T
	err   error
}

// LazySlice fans out loader functions across a bounded goroutine pool.
// Results are indexed positionally — Get(i) returns the result of loaders[i]
// regardless of completion order.
type LazySlice[T any] struct {
	results []result[T]
	loaded  bool
	err     error // aggregate error from LoadAll
}

// maxWorkers returns the bounded concurrency limit.
func maxWorkers() int {
	n := runtime.NumCPU()
	if n > 8 {
		n = 8
	}
	return n
}

// NewLazySlice creates a LazySlice from a slice of loader functions.
// Does not start any goroutines — call LoadAll() to execute.
func NewLazySlice[T any](loaders []func() (T, error)) *LazySlice[T] {
	ls := &LazySlice[T]{
		results: make([]result[T], len(loaders)),
	}
	// Store loaders internally for LoadAll to execute.
	// We attach them via closure in LoadAll to avoid storing
	// the slice as a field (it's only needed once).
	ls.loadAll(loaders)
	return ls
}
```

Wait — per REQ-CSYNC-003, `NewLazySlice` should not call `LoadAll` automatically. The constructor and `LoadAll` are separate. Revised:

```go
// LazySlice fans out loader functions across a bounded goroutine pool.
type LazySlice[T any] struct {
	loaders []func() (T, error)
	results []result[T]
	loaded  bool
}

// NewLazySlice creates a LazySlice. Call LoadAll() to execute loaders.
func NewLazySlice[T any](loaders []func() (T, error)) *LazySlice[T] {
	if loaders == nil {
		loaders = []func() (T, error){}
	}
	return &LazySlice[T]{
		loaders: loaders,
		results: make([]result[T], len(loaders)),
	}
}

// Len returns the number of loader slots.
func (ls *LazySlice[T]) Len() int {
	return len(ls.loaders)
}
```

### LoadAll Implementation

```go
// LoadAll executes all loaders concurrently with a bounded worker pool.
// Blocks until every loader has completed. Returns a non-nil error if
// any loader failed (individual errors retrievable via Get).
func (ls *LazySlice[T]) LoadAll() error {
	if ls.loaded || len(ls.loaders) == 0 {
		return nil
	}
	ls.loaded = true

	workers := maxWorkers()
	sem := make(chan struct{}, workers) // buffered channel as semaphore
	var wg sync.WaitGroup

	for i, loader := range ls.loaders {
		wg.Add(1)
		sem <- struct{}{} // acquire slot (blocks if pool is full)
		go func(idx int, fn func() (T, error)) {
			defer wg.Done()
			defer func() { <-sem }() // release slot

			// Panic recovery: convert panic to error.
			defer func() {
				if r := recover(); r != nil {
					ls.results[idx] = result[T]{
						err: fmt.Errorf("loader %d panicked: %v", idx, r),
					}
				}
			}()

			val, err := fn()
			ls.results[idx] = result[T]{value: val, err: err}
		}(i, loader)
	}

	wg.Wait()

	// Build aggregate error if any loaders failed.
	var failed int
	var msgs []string
	for i, r := range ls.results {
		if r.err != nil {
			failed++
			msgs = append(msgs, fmt.Sprintf("loader %d: %v", i, r.err))
		}
	}
	if failed > 0 {
		ls.err = fmt.Errorf("%d/%d loaders failed: %s",
			failed, len(ls.loaders), strings.Join(msgs, "; "))
	}

	return ls.err
}
```

**Thread safety note**: Each `results[idx]` slot is written by exactly one goroutine (the one with `idx`). The caller reads results only after `wg.Wait()` returns, establishing a happens-before relationship. No mutex needed on the results slice.

**Semaphore pattern**: The buffered channel `sem` with capacity `min(NumCPU, 8)` limits active goroutines. The `sem <- struct{}{}` in the launching loop (not inside the goroutine) means the launching goroutine blocks when the pool is full, preventing goroutine explosion. The release `<-sem` is in the goroutine's defer, ensuring the slot is freed even on panic.

### Get and MustGet

```go
// Get returns the result at index i. Panics if i is out of range.
// Must be called after LoadAll().
func (ls *LazySlice[T]) Get(i int) (T, error) {
	return ls.results[i].value, ls.results[i].err
}

// MustGet returns the value at index i, panicking if the loader returned an error.
func (ls *LazySlice[T]) MustGet(i int) T {
	if ls.results[i].err != nil {
		panic(fmt.Sprintf("csync.LazySlice.MustGet(%d): %v", i, ls.results[i].err))
	}
	return ls.results[i].value
}
```

## 3. Package: internal/events

File: `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker.go`

### Type Definitions

```go
package events

import (
	"fmt"
	"io"
	"sync"
)

// EventType identifies the kind of event.
type EventType string

// Event type constants.
const (
	PhaseAssembled   EventType = "PhaseAssembled"
	CacheHit         EventType = "CacheHit"
	CacheMiss        EventType = "CacheMiss"
	ArtifactPromoted EventType = "ArtifactPromoted"
	StateAdvanced    EventType = "StateAdvanced"
)

// Event carries a typed payload through the broker.
type Event struct {
	Type    EventType
	Payload any
}

// PhaseAssembledPayload is the payload for PhaseAssembled events.
type PhaseAssembledPayload struct {
	Phase      string
	Bytes      int
	Tokens     int
	Cached     bool
	DurationMs int64
	ChangeDir  string
	SkillsPath string
	Content    []byte // non-nil only on cache miss (for cache subscriber)
}

// CacheHitPayload is the payload for CacheHit events.
type CacheHitPayload struct {
	Phase string
	Bytes int
}

// CacheMissPayload is the payload for CacheMiss events.
type CacheMissPayload struct {
	Phase string
}

// ArtifactPromotedPayload is the payload for ArtifactPromoted events.
type ArtifactPromotedPayload struct {
	Change      string
	Phase       string
	PromotedTo  string
}

// StateAdvancedPayload is the payload for StateAdvanced events.
type StateAdvancedPayload struct {
	Change    string
	FromPhase string
	ToPhase   string
}

// Handler processes an event.
type Handler func(Event)
```

### Broker Implementation

```go
// Broker dispatches events to registered subscribers.
// Safe for concurrent Emit() calls from multiple goroutines.
// A nil *Broker is safe to call Emit() and Subscribe() on (no-op).
type Broker struct {
	mu     sync.Mutex
	subs   map[EventType][]Handler
	stderr io.Writer // for panic logging
}

// NewBroker creates a Broker. stderr is used for panic diagnostics.
func NewBroker(stderr io.Writer) *Broker {
	return &Broker{
		subs:   make(map[EventType][]Handler),
		stderr: stderr,
	}
}

// Subscribe registers a handler for the given event type.
// Nil-safe: calling on a nil *Broker is a no-op.
func (b *Broker) Subscribe(t EventType, h Handler) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[t] = append(b.subs[t], h)
}

// Emit dispatches an event to all subscribers for its type.
// Nil-safe: calling on a nil *Broker is a no-op.
// Serialized via mutex — concurrent Emit() calls are safe.
// Each subscriber is called with panic recovery.
func (b *Broker) Emit(e Event) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.subs[e.Type]
	for _, h := range handlers {
		func(handler Handler) {
			defer func() {
				if r := recover(); r != nil {
					if b.stderr != nil {
						fmt.Fprintf(b.stderr, "sdd: event subscriber panic [%s]: %v\n", e.Type, r)
					}
				}
			}()
			handler(e)
		}(h)
	}
}
```

**Serialization**: The `sync.Mutex` in `Emit()` ensures that when `AssembleConcurrent` runs two phases in parallel (context.go:121-164) and both emit `PhaseAssembled`, the metrics subscriber sees them sequentially. This fixes the current race where two goroutines call `recordMetrics()` concurrently (cache.go:274), both reading, modifying, and writing `metrics.json` — a last-writer-wins data loss bug.

**Nil-safe pattern**: Both `Subscribe` and `Emit` check `b == nil` as the first line. This means `Params.Broker` can be left as its zero value (nil) in tests, and all `p.Broker.Emit(...)` calls in `Assemble()` and `runWrite()` are safe without nil guards at the call site.

**Panic recovery**: Each handler is invoked inside its own `func()` with `defer recover()`. If handler 2 of 3 panics, handler 1 has already run and handler 3 will still execute.

## 4. Assembler Refactor Pattern

### Before: AssembleReview (review.go, lines 13-59)

```go
func AssembleReview(w io.Writer, p *Params) error {
	skill, err := loadSkill(p.SkillsPath, "sdd-review")
	if err != nil {
		return err
	}

	specs, err := loadSpecs(p.ChangeDir)
	if err != nil {
		return fmt.Errorf("review requires spec artifacts: %w", err)
	}

	design, err := loadArtifact(p.ChangeDir, "design.md")
	if err != nil {
		return fmt.Errorf("review requires design artifact: %w", err)
	}

	tasks, err := loadArtifact(p.ChangeDir, "tasks.md")
	if err != nil {
		return fmt.Errorf("review requires tasks artifact: %w", err)
	}

	diff, err := gitDiff(p.ProjectDir)
	if err != nil {
		diff = fmt.Sprintf("(git diff unavailable: %v)", err)
	}

	// ... writeSection calls ...
}
```

### After: AssembleReview with LazySlice

```go
func AssembleReview(w io.Writer, p *Params) error {
	// Define loader closures — each captures its required params.
	loaders := []func() ([]byte, error){
		func() ([]byte, error) { return loadSkill(p.SkillsPath, "sdd-review") },           // 0: skill
		func() ([]byte, error) { return []byte(mustLoadSpecs(p.ChangeDir)), nil },          // 1: specs
		func() ([]byte, error) { return loadArtifact(p.ChangeDir, "design.md") },           // 2: design
		func() ([]byte, error) { return loadArtifact(p.ChangeDir, "tasks.md") },            // 3: tasks
		func() ([]byte, error) { return []byte(gitDiffOrFallback(p.ProjectDir)), nil },     // 4: diff
		func() ([]byte, error) { return loadProjectRulesOptional(p.ProjectDir), nil },      // 5: rules
	}

	ls := csync.NewLazySlice(loaders)
	if err := ls.LoadAll(); err != nil {
		// Check critical loaders (skill, specs, design, tasks).
		if _, e := ls.Get(0); e != nil {
			return e
		}
		if _, e := ls.Get(1); e != nil {
			return fmt.Errorf("review requires spec artifacts: %w", e)
		}
		if _, e := ls.Get(2); e != nil {
			return fmt.Errorf("review requires design artifact: %w", e)
		}
		if _, e := ls.Get(3); e != nil {
			return fmt.Errorf("review requires tasks artifact: %w", e)
		}
		// Loaders 4 and 5 are non-fatal — fall through.
	}

	skill, _ := ls.Get(0)
	specsData, _ := ls.Get(1)
	design, _ := ls.Get(2)
	tasks, _ := ls.Get(3)
	diff, _ := ls.Get(4)
	rules, _ := ls.Get(5)

	// Write sections in the same order as before — output is deterministic.
	writeSection(w, "SKILL", skill)

	writeSectionStr(w, "CHANGE", fmt.Sprintf(
		"Name: %s\nDescription: %s",
		p.ChangeName, p.Description,
	))

	writeSection(w, "SPECIFICATIONS", specsData)
	writeSection(w, "DESIGN", design)
	writeSectionStr(w, "COMPLETED TASKS", extractCompletedTasks(string(tasks)))
	writeSection(w, "TASKS", tasks)
	if len(diff) > 0 {
		writeSectionStr(w, "GIT DIFF", string(diff))
	}

	if len(rules) > 0 {
		writeSection(w, "PROJECT RULES", rules)
	}

	return nil
}
```

**Helper wrappers** needed for the refactor (added to review.go or a shared helpers file):

```go
// mustLoadSpecs wraps loadSpecs to match the func() ([]byte, error) signature.
// Returns error via the error return; caller checks via Get().
func mustLoadSpecs(changeDir string) (string, error) {
	return loadSpecs(changeDir)  // loadSpecs already returns (string, error)
}

// gitDiffOrFallback wraps gitDiff to never fail — returns fallback string on error.
func gitDiffOrFallback(projectDir string) string {
	diff, err := gitDiff(projectDir)
	if err != nil {
		return fmt.Sprintf("(git diff unavailable: %v)", err)
	}
	return diff
}

// loadProjectRulesOptional wraps loadProjectRules, returning nil on miss.
func loadProjectRulesOptional(projectDir string) []byte {
	rules, err := loadProjectRules(projectDir)
	if err != nil {
		return nil
	}
	return rules
}
```

**Note on type parameter**: Most assemblers load `[]byte` artifacts. For `loadSpecs` which returns `string`, and `gitDiff`/`buildSummary` which return `string`, we wrap them into `func() ([]byte, error)` closures via `[]byte(...)` conversion. This keeps all assemblers using `LazySlice[[]byte]` — a single type parameter across the codebase.

### Assembler I/O Load Summary

Each assembler's loader closures:

| Assembler | Loaders | Critical | Optional |
|-----------|---------|----------|----------|
| explore | skill, gitFileTree, manifests | skill | fileTree, manifests |
| propose | skill, exploration, gitFileTree | skill, exploration | fileTree |
| spec | skill, proposal, buildSummary | skill, proposal | summary |
| design | skill, proposal, loadSpecs, buildSummary | skill, proposal, specs | summary |
| tasks | skill, design, loadSpecs | skill, design, specs | - |
| apply | skill, tasks, design, loadSpecs, buildSummary | skill, tasks, design, specs | summary |
| review | skill, loadSpecs, design, tasks, gitDiff, rules | skill, specs, design, tasks | diff, rules |
| clean | skill, verifyReport, tasks, buildSummary, design, loadSpecs | skill, verifyReport, tasks | summary, design, specs |

## 5. Integration: Assemble() Changes

Current `Assemble()` at context.go:55-96. Modified version:

```go
// Assemble resolves the phase and runs the appropriate assembler.
// Emits CacheHit/CacheMiss and PhaseAssembled events via p.Broker.
func Assemble(w io.Writer, phase state.Phase, p *Params) error {
	fn, ok := dispatchers[phase]
	if !ok {
		return fmt.Errorf("no assembler for phase: %s", phase)
	}

	phaseStr := string(phase)
	start := time.Now()

	// Try cache first.
	if cached, ok := tryCachedContext(p.ChangeDir, phaseStr, p.SkillsPath); ok {
		size := len(cached)
		w.Write(cached)

		// Emit cache hit + assembled events (replaces inline emitMetrics).
		p.Broker.Emit(events.Event{
			Type: events.CacheHit,
			Payload: events.CacheHitPayload{
				Phase: phaseStr,
				Bytes: size,
			},
		})
		p.Broker.Emit(events.Event{
			Type: events.PhaseAssembled,
			Payload: events.PhaseAssembledPayload{
				Phase:      phaseStr,
				Bytes:      size,
				Tokens:     estimateTokens(size),
				Cached:     true,
				DurationMs: time.Since(start).Milliseconds(),
				ChangeDir:  p.ChangeDir,
				SkillsPath: p.SkillsPath,
			},
		})
		return nil
	}

	// Cache miss.
	p.Broker.Emit(events.Event{
		Type:    events.CacheMiss,
		Payload: events.CacheMissPayload{Phase: phaseStr},
	})

	// Assemble into buffer for caching + size check.
	var buf bytes.Buffer
	if err := fn(&buf, p); err != nil {
		return err
	}

	size := buf.Len()

	// Size guard.
	if size > maxContextBytes {
		return fmt.Errorf("context too large: %s (%d bytes, ~%dK tokens) exceeds limit of %s (~%dK tokens)",
			formatBytes(size), size, estimateTokens(size)/1000,
			formatBytes(maxContextBytes), estimateTokens(maxContextBytes)/1000)
	}

	// Write to output.
	content := buf.Bytes()
	w.Write(content)

	// Emit assembled event (subscribers handle caching + metrics).
	p.Broker.Emit(events.Event{
		Type: events.PhaseAssembled,
		Payload: events.PhaseAssembledPayload{
			Phase:      phaseStr,
			Bytes:      size,
			Tokens:     estimateTokens(size),
			Cached:     false,
			DurationMs: time.Since(start).Milliseconds(),
			ChangeDir:  p.ChangeDir,
			SkillsPath: p.SkillsPath,
			Content:    content, // for cache subscriber to persist
		},
	})

	return nil
}
```

**What changed**:
- Removed inline `emitMetrics(p.Stderr, ...)` call (context.go:68, context.go:94)
- Removed inline `saveContextCache(...)` call (context.go:92)
- Added `p.Broker.Emit(...)` for `CacheHit`/`CacheMiss`/`PhaseAssembled`
- `PhaseAssembledPayload.Content` carries the assembled bytes so the cache subscriber can persist them without the assembler knowing about caching

## 6. Integration: runWrite() Changes

Current `runWrite()` at commands.go:312-371. Events emitted after the existing promote/advance logic:

```go
func runWrite(args []string, stdout io.Writer, stderr io.Writer) error {
	// ... existing arg parsing, resolve changeDir, load state (lines 312-335) ...

	// Promote pending artifact.
	promoted, err := artifacts.Promote(changeDir, phase)
	if err != nil {
		return errs.WriteError(stderr, "write", err)
	}

	// Emit ArtifactPromoted event.
	// broker is obtained from the function's scope — see wiring below.
	broker.Emit(events.Event{
		Type: events.ArtifactPromoted,
		Payload: events.ArtifactPromotedPayload{
			Change:     name,
			Phase:      phaseStr,
			PromotedTo: promoted,
		},
	})

	// Advance state.
	prevPhase := st.CurrentPhase
	if err := st.Advance(phase); err != nil {
		return errs.WriteError(stderr, "write", fmt.Errorf("advance state: %w", err))
	}

	// Save state.
	if err := state.Save(st, statePath); err != nil {
		return errs.WriteError(stderr, "write", err)
	}

	// Emit StateAdvanced event.
	broker.Emit(events.Event{
		Type: events.StateAdvanced,
		Payload: events.StateAdvancedPayload{
			Change:    name,
			FromPhase: string(prevPhase),
			ToPhase:   string(st.CurrentPhase),
		},
	})

	// ... existing JSON output (lines 353-371) ...
}
```

**Broker wiring in runWrite**: `runWrite` does not currently receive a `Params` struct. The broker must be created at the CLI dispatch level and passed down. Two options:

| # | Approach | Pros | Cons |
|---|----------|------|------|
| 1 | Create broker in `Run()` dispatcher, pass to all `run*` functions | Single creation point, consistent | Signature change for all `run*` functions |
| 2 | Create broker locally in `runWrite` | Minimal change surface | No subscriber from `runContext` can observe write events |

**Choice**: Option 1 — create broker in `Run()` and pass to functions that need it. `runWrite` and `runContext`/`runNew` are the only functions that emit events. The broker is created once per CLI invocation in `Run()` and passed as an argument.

Specifically, the `Run()` function in `internal/cli/run.go` (or wherever the command dispatch lives) will:

```go
func Run(args []string, stdout, stderr io.Writer) error {
	broker := events.NewBroker(stderr)
	registerDefaultSubscribers(broker, stderr)
	// ... dispatch to runContext, runWrite, etc., passing broker ...
}
```

For `runContext` and `runNew`, the broker is wired into `Params.Broker`. For `runWrite`, it is passed as an additional parameter.

## 7. Integration: Params Struct Changes

Current `Params` at context.go:29-38:

```go
type Params struct {
	ChangeDir   string
	ChangeName  string
	Description string
	ProjectDir  string
	Config      *config.Config
	SkillsPath  string
	Stderr      io.Writer
	Verbosity   int
}
```

Modified:

```go
import "github.com/rechedev9/shenronSDD/sdd-cli/internal/events"

type Params struct {
	ChangeDir   string
	ChangeName  string
	Description string
	ProjectDir  string
	Config      *config.Config
	SkillsPath  string
	Stderr      io.Writer
	Verbosity   int
	Broker      *events.Broker // nil-safe; nil means no event emission
}
```

**Nil-safe usage pattern** — no nil checks needed at call sites:

```go
// In Assemble() — works whether Broker is nil or not:
p.Broker.Emit(events.Event{Type: events.CacheMiss, Payload: ...})

// In tests that don't care about events:
p := &Params{ChangeDir: dir, Config: cfg}  // Broker is nil — all Emit calls are no-ops
```

This is possible because `Broker.Emit()` and `Broker.Subscribe()` both start with `if b == nil { return }`.

### Default Subscriber Registration

```go
// registerDefaultSubscribers wires the standard side-effect subscribers.
// Called once per CLI invocation in Run().
func registerDefaultSubscribers(broker *events.Broker, stderr io.Writer, verbosity int) {
	// Metrics recording subscriber (was inline recordMetrics in cache.go:274).
	broker.Subscribe(events.PhaseAssembled, func(e events.Event) {
		p, ok := e.Payload.(events.PhaseAssembledPayload)
		if !ok {
			return
		}
		m := &sddctx.ContextMetrics{
			Phase:      p.Phase,
			Bytes:      p.Bytes,
			Tokens:     p.Tokens,
			Cached:     p.Cached,
			DurationMs: p.DurationMs,
		}
		sddctx.RecordMetrics(p.ChangeDir, m)
	})

	// Stderr metrics output subscriber (was inline writeMetrics in cache.go:230).
	broker.Subscribe(events.PhaseAssembled, func(e events.Event) {
		p, ok := e.Payload.(events.PhaseAssembledPayload)
		if !ok {
			return
		}
		m := &sddctx.ContextMetrics{
			Phase:      p.Phase,
			Bytes:      p.Bytes,
			Tokens:     p.Tokens,
			Cached:     p.Cached,
			DurationMs: p.DurationMs,
		}
		sddctx.WriteMetricsOutput(stderr, m, verbosity)
	})

	// Cache persistence subscriber (was inline saveContextCache in context.go:92).
	broker.Subscribe(events.PhaseAssembled, func(e events.Event) {
		p, ok := e.Payload.(events.PhaseAssembledPayload)
		if !ok || p.Cached || p.Content == nil {
			return // only cache on fresh assembly
		}
		_ = sddctx.SaveContextCache(p.ChangeDir, p.Phase, p.SkillsPath, p.Content)
	})
}
```

**Note**: This requires exporting `recordMetrics` -> `RecordMetrics`, `writeMetrics` -> `WriteMetricsOutput`, `saveContextCache` -> `SaveContextCache`, and `contextMetrics` -> `ContextMetrics` from the `context` package. The alternative is to keep them unexported and register the subscribers from within the `context` package via a `RegisterSubscribers(broker)` function. The latter preserves encapsulation better.

**Decision**: Use an exported `RegisterSubscribers` function on the `context` package:

```go
// In internal/context/subscribers.go (new file):
package context

import "github.com/rechedev9/shenronSDD/sdd-cli/internal/events"

// RegisterSubscribers wires the default event subscribers for context assembly.
// Call once per CLI invocation after creating the Broker.
func RegisterSubscribers(broker *events.Broker, stderr io.Writer, verbosity int) {
	if broker == nil {
		return
	}

	broker.Subscribe(events.PhaseAssembled, func(e events.Event) {
		p, ok := e.Payload.(events.PhaseAssembledPayload)
		if !ok {
			return
		}
		m := &contextMetrics{
			Phase:      p.Phase,
			Bytes:      p.Bytes,
			Tokens:     p.Tokens,
			Cached:     p.Cached,
			DurationMs: p.DurationMs,
		}
		recordMetrics(p.ChangeDir, m)
	})

	broker.Subscribe(events.PhaseAssembled, func(e events.Event) {
		p, ok := e.Payload.(events.PhaseAssembledPayload)
		if !ok {
			return
		}
		if stderr == nil || verbosity < 0 {
			return
		}
		m := &contextMetrics{
			Phase:      p.Phase,
			Bytes:      p.Bytes,
			Tokens:     p.Tokens,
			Cached:     p.Cached,
			DurationMs: p.DurationMs,
		}
		writeMetrics(stderr, m, verbosity)
	})

	broker.Subscribe(events.PhaseAssembled, func(e events.Event) {
		p, ok := e.Payload.(events.PhaseAssembledPayload)
		if !ok || p.Cached || p.Content == nil {
			return
		}
		_ = saveContextCache(p.ChangeDir, p.Phase, p.SkillsPath, p.Content)
	})
}
```

This avoids exporting internal types and keeps the subscriber logic co-located with the functions it wraps.

## 8. pprof Gate

### main.go Changes

Current `main.go` at cmd/sdd/main.go:1-36. Modified:

```go
package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli"
)

func main() {
	// Panic recovery (existing, lines 14-31).
	defer func() {
		if r := recover(); r != nil {
			ts := time.Now()
			name := fmt.Sprintf(".sdd-crash-%d.log", ts.Unix())
			content := fmt.Sprintf("sdd crash report\ntimestamp: %s\nargs: %s\npanic: %v\n\nstack trace:\n%s",
				ts.Format(time.RFC3339),
				strings.Join(os.Args, " "),
				r,
				debug.Stack(),
			)
			if err := os.WriteFile(name, []byte(content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "sdd: panic recovered; failed to write crash log: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "sdd: panic recovered; crash log written to %s\n", name)
			}
			os.Exit(3)
		}
	}()

	// pprof gate — zero overhead when SDD_PPROF is unset.
	stopProfile := startProfile(os.Getenv("SDD_PPROF"), os.Stderr)
	defer stopProfile()

	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}

// startProfile enables CPU and/or heap profiling based on the mode string.
// Returns a cleanup function that stops profiling and closes files.
// Unrecognized non-empty values are warned on stderr.
func startProfile(mode string, stderr *os.File) func() {
	if mode == "" {
		return func() {} // zero overhead: no allocations, no pprof calls
	}

	var closers []func()

	wantCPU := mode == "cpu" || mode == "all"
	wantMem := mode == "mem" || mode == "all"

	if !wantCPU && !wantMem {
		fmt.Fprintf(stderr, "sdd: warning: unrecognized SDD_PPROF value %q (expected cpu, mem, or all)\n", mode)
		return func() {}
	}

	if wantCPU {
		f, err := os.Create("sdd-cpu.prof")
		if err != nil {
			fmt.Fprintf(stderr, "sdd: pprof: cannot create sdd-cpu.prof: %v\n", err)
		} else {
			if err := pprof.StartCPUProfile(f); err != nil {
				fmt.Fprintf(stderr, "sdd: pprof: cannot start CPU profile: %v\n", err)
				f.Close()
			} else {
				fmt.Fprintf(stderr, "sdd: pprof: CPU profiling to sdd-cpu.prof\n")
				closers = append(closers, func() {
					pprof.StopCPUProfile()
					f.Close()
				})
			}
		}
	}

	if wantMem {
		f, err := os.Create("sdd-mem.prof")
		if err != nil {
			fmt.Fprintf(stderr, "sdd: pprof: cannot create sdd-mem.prof: %v\n", err)
		} else {
			closers = append(closers, func() {
				runtime.GC() // get up-to-date statistics
				if err := pprof.WriteHeapProfile(f); err != nil {
					fmt.Fprintf(stderr, "sdd: pprof: cannot write heap profile: %v\n", err)
				}
				f.Close()
				fmt.Fprintf(stderr, "sdd: pprof: heap profile written to sdd-mem.prof\n")
			})
		}
	}

	return func() {
		for _, c := range closers {
			c()
		}
	}
}
```

**Profile file naming**: `sdd-cpu.prof` and `sdd-mem.prof` in CWD, consistent with the crash log pattern at main.go:17 (`.sdd-crash-{ts}.log`). The profile files do not include a timestamp because they are overwritten on each run — unlike crash logs which accumulate.

**Zero overhead when unset**: The `if mode == ""` early return means no `os.Create`, no `pprof.*` calls, no allocations. The returned `func(){}` is a no-op closure.

### Doctor Check

Addition to `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/doctor.go`:

```go
func checkPprof() CheckResult {
	val := os.Getenv("SDD_PPROF")
	if val == "" {
		return CheckResult{
			Name:    "pprof",
			Status:  "pass",
			Message: "not set (no profiling)",
		}
	}
	return CheckResult{
		Name:    "pprof",
		Status:  "pass",
		Message: fmt.Sprintf("SDD_PPROF=%s", val),
	}
}
```

Added to the `checks` slice in `runDoctor()` (doctor.go:163-169):

```go
checks := []CheckResult{
	configResult,
	checkCache(changesDir, cfg),
	checkOrphanedPending(changesDir),
	checkSkillsPath(cfg),
	checkBuildTools(cfg),
	checkPprof(),  // NEW
}
```

## 9. Migration Strategy

### Existing Tests Continue to Work (nil broker)

All existing tests construct `Params` without a `Broker` field. Because `Broker` is a pointer type, its zero value is `nil`. Because `Broker.Emit()` and `Broker.Subscribe()` are nil-safe (no-op on nil receiver), all existing test code continues to work without modification.

The only behavioral change: the inline `emitMetrics()` and `saveContextCache()` calls are removed from `Assemble()`. Tests that previously verified metrics side effects will need to either:
1. Pass a non-nil broker with subscribers (for integration tests), or
2. Accept that metrics/cache are no longer written without a broker (for unit tests).

Since `emitMetrics` and `saveContextCache` are best-effort and not asserted in existing `context_test.go` tests (which verify assembled output content, not side effects), no existing test assertions break.

### Metrics Recording Migration

**Before**: `recordMetrics()` called inline from `emitMetrics()` (cache.go:109), which is called from `Assemble()` (context.go:68, 94).

**After**: `recordMetrics()` called from a `PhaseAssembled` subscriber registered via `RegisterSubscribers()`.

The `recordMetrics` and `writeMetrics` functions remain in `cache.go` with their existing signatures — they become internal helpers called by the subscriber closures in `subscribers.go`.

The `emitMetrics` function (context.go:98-115) is deleted entirely. Its responsibilities are split:
- `recordMetrics` call -> metrics subscriber
- `writeMetrics` call -> stderr subscriber

### Cache Persistence Migration

**Before**: `saveContextCache()` called inline from `Assemble()` (context.go:92).

**After**: `saveContextCache()` called from a `PhaseAssembled` subscriber that checks `!p.Cached && p.Content != nil`.

The `PhaseAssembledPayload.Content` field carries the assembled bytes on fresh assembly (nil on cache hit). The subscriber conditionally calls `saveContextCache` only when there is content to persist.

### Cache Version Bump

**Not needed.** The cache format (hash files, context files) is unchanged. The `cacheVersion` constant (cache.go:21, currently `5`) does not need incrementing because:
- The cached context bytes are identical (assembler output is unchanged)
- The hash computation is unchanged
- Only the orchestration of when caching happens changes (inline -> subscriber)

### Import Cycle Prevention

The dependency graph is acyclic:
```
internal/csync       -> (stdlib only)
internal/events      -> (stdlib only)
internal/context     -> internal/csync, internal/events
internal/cli         -> internal/context, internal/events
```

No package imports a package that imports it back.

## Architecture Decisions

| # | Decision | Choice | Alternatives Considered | Rationale |
|---|----------|--------|-------------------------|-----------|
| 1 | LazySlice type parameter | `LazySlice[[]byte]` for all assemblers | `LazySlice[any]` with type assertions; separate types per assembler | `[]byte` is the common denominator — skill, artifact, specs, diff all produce bytes. String results wrapped via `[]byte(s)`. Avoids type assertions and keeps generic usage simple. |
| 2 | Semaphore implementation | Buffered channel `make(chan struct{}, N)` | `sync.Semaphore` (x/sync); `atomic` counter with spin | Buffered channel is idiomatic Go, zero dependencies, well-understood. `x/sync` adds external dep. Atomic spin wastes CPU. |
| 3 | Subscriber registration location | `context.RegisterSubscribers()` function | Export `RecordMetrics`/`WriteMetrics`/`SaveContextCache` and register in `cli` | Keeps unexported functions unexported. Subscriber logic co-located with the functions it delegates to. Cleaner encapsulation. |
| 4 | Event payload design | Concrete struct per event type, carried via `any` in `Event.Payload` | Interface with methods; sum type via sealed interface | Concrete structs are simplest — subscribers do a single type assertion. No method boilerplate. `any` payload with documented types matches Go patterns (context.Value, slog.Attr). |
| 5 | Broker creation point | Single broker in `Run()`, passed down | Global singleton; per-command broker | Single creation enables cross-command subscriber visibility (e.g., metrics subscriber sees both context and write events). Testable via injection. Global singleton is anti-pattern. Per-command misses cross-cutting concerns. |
| 6 | pprof output location | CWD (sdd-cpu.prof, sdd-mem.prof) | openspec/.cache/ directory | CWD matches crash log convention (main.go:17). openspec/.cache/ would require config loading before profiling starts, adding latency to the profiling gate. |

## File Changes

| # | File Path | Action | Description |
|---|-----------|--------|-------------|
| 1 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice.go` | create | LazySlice[T] generic type with bounded worker pool |
| 2 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go` | create | Unit tests: construction, LoadAll, Get, panic recovery, race detector |
| 3 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker.go` | create | Broker type, Event/EventType, Subscribe, Emit with serialized dispatch |
| 4 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go` | create | Unit tests: subscribe/emit, panic recovery, concurrent emit, nil-safe |
| 5 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go` | modify | Add Broker field to Params (line 29); replace inline emitMetrics/saveContextCache in Assemble() (lines 55-96) with Broker.Emit calls; delete emitMetrics function (lines 98-115) |
| 6 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/subscribers.go` | create | RegisterSubscribers() — wires metrics, stderr, and cache subscribers |
| 7 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/cache.go` | modify | No functional changes; recordMetrics (line 274) and writeMetrics (line 230) remain as-is, now called by subscribers instead of inline |
| 8 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/explore.go` | modify | Refactor AssembleExplore to use LazySlice for skill, gitFileTree, manifests |
| 9 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/propose.go` | modify | Refactor AssemblePropose to use LazySlice for skill, exploration, gitFileTree |
| 10 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/spec.go` | modify | Refactor AssembleSpec to use LazySlice for skill, proposal, buildSummary |
| 11 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/design.go` | modify | Refactor AssembleDesign to use LazySlice for skill, proposal, specs, buildSummary |
| 12 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/tasks.go` | modify | Refactor AssembleTasks to use LazySlice for skill, design, specs |
| 13 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/apply.go` | modify | Refactor AssembleApply to use LazySlice for skill, tasks, design, specs, buildSummary |
| 14 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/review.go` | modify | Refactor AssembleReview to use LazySlice for skill, specs, design, tasks, diff, rules |
| 15 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/clean.go` | modify | Refactor AssembleClean to use LazySlice for skill, verifyReport, tasks, summary, design, specs |
| 16 | `/home/reche/projects/SDDworkflow/sdd-cli/cmd/sdd/main.go` | modify | Add startProfile() function and SDD_PPROF gate before cli.Run() |
| 17 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go` | modify | Wire broker into Params in runContext/runNew; emit events from runWrite; store prevPhase before Advance |
| 18 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/doctor.go` | modify | Add checkPprof() function and include in runDoctor checks slice |
| 19 | `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/run.go` | modify | Create broker in Run(), call RegisterSubscribers(), pass broker to run* functions |

**Summary**: 5 files created, 14 files modified, 0 files deleted

## Testing Strategy

| # | What to Test | Type | File Path | Maps to Requirement |
|---|-------------|------|-----------|---------------------|
| 1 | LazySlice construction (loaders, empty, nil) | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go` | REQ-CSYNC-001 |
| 2 | LazySlice bounded goroutine pool | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go` | REQ-CSYNC-002 |
| 3 | LazySlice LoadAll blocking + result ordering | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go` | REQ-CSYNC-003 |
| 4 | LazySlice partial failure + error messages | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go` | REQ-CSYNC-004 |
| 5 | LazySlice race detector (go test -race) | unit+race | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go` | REQ-CSYNC-005 |
| 6 | LazySlice no goroutine leak + panic recovery | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go` | REQ-CSYNC-006 |
| 7 | Broker subscribe + emit single/multiple | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go` | REQ-EVENTS-001 |
| 8 | Event type payloads carry required fields | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go` | REQ-EVENTS-002 |
| 9 | Broker subscriber panic recovery | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go` | REQ-EVENTS-003 |
| 10 | Broker serialized dispatch under concurrency | unit+race | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go` | REQ-EVENTS-004 |
| 11 | Nil broker emit/subscribe no-op | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go` | REQ-EVENTS-005 |
| 12 | Assemble emits CacheHit/CacheMiss/PhaseAssembled | integration | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context_test.go` | REQ-EVENTS-006 |
| 13 | runWrite emits ArtifactPromoted + StateAdvanced | integration | `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands_test.go` | REQ-EVENTS-006 |
| 14 | SDD_PPROF=cpu/mem/all creates profile files | integration | `/home/reche/projects/SDDworkflow/sdd-cli/cmd/sdd/main_test.go` | REQ-PPROF-001, REQ-PPROF-002 |
| 15 | SDD_PPROF unset creates no files | integration | `/home/reche/projects/SDDworkflow/sdd-cli/cmd/sdd/main_test.go` | REQ-PPROF-001 |
| 16 | Unrecognized SDD_PPROF warns on stderr | integration | `/home/reche/projects/SDDworkflow/sdd-cli/cmd/sdd/main_test.go` | REQ-PPROF-001 |
| 17 | Doctor checkPprof output | unit | `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/doctor_test.go` | REQ-PPROF-004 |
| 18 | Existing context_test.go passes with nil broker | regression | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context_test.go` | REQ-EVENTS-005 |
| 19 | AssembleConcurrent + metrics.json race fix | integration+race | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context_test.go` | REQ-EVENTS-004 |
| 20 | Assembler output byte-identical with LazySlice | regression | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context_test.go` | REQ-CSYNC-003 |

### Test Dependencies

- **Mocks needed**: None. LazySlice and Broker are tested with real closures. Assembler tests use the existing temp-dir fixture pattern in context_test.go.
- **Fixtures needed**: Existing context_test.go fixtures (temp changeDir with skills, artifacts, specs). No new fixtures.
- **Infrastructure**: `go test -race` for all packages containing concurrency (csync, events, context).

## Migration and Rollout

No migration or rollout steps required. All changes are additive:
- New packages (`csync`, `events`) are self-contained
- `Params.Broker` is backward-compatible (nil zero value)
- pprof gate is opt-in via environment variable
- Assembler output is byte-identical (only I/O scheduling changes)
- No state format changes, no config format changes, no cache format changes

### Rollback Steps

1. Revert the merge commit — all features are additive
2. Remove `internal/csync/` and `internal/events/` directories
3. Assemblers revert to sequential `loadSkill`/`loadArtifact` calls
4. `Assemble()` reverts to inline `emitMetrics`/`saveContextCache`
5. `main.go` pprof block is removed
6. `Params.Broker` field is removed
7. `sdd doctor` checkPprof is removed

Verification: `go test ./... -race` passes, `go build ./cmd/sdd` succeeds.

## Open Questions

- **buildSummary inside LazySlice**: `buildSummary()` (summary.go:13) reads multiple files itself (exploration.md, proposal.md, design.md, review-report.md). Should it also use LazySlice internally, or is the sequential file-read pattern acceptable given that each file is small (<2KB)? Decision: leave `buildSummary` sequential for now — its I/O is negligible compared to the assembler-level fan-out.

---

**Next Step**: After both design and specs are complete, run `sdd-tasks` to generate the implementation checklist.
