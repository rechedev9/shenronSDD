# Spec: Concurrency and Performance

**Change ID**: concurrency-performance
**Date**: 2026-03-21
**Status**: draft

---

## Domain 1: csync (LazySlice)

Package `internal/csync` provides a generic bounded-concurrency primitive for fanning out I/O loaders (file reads, git exec) used by the 8 assembler functions in `internal/context/`.

### REQ-CSYNC-001: LazySlice Generic Type

`LazySlice[T]` MUST be a generic struct parameterized over result type `T`. It MUST accept a slice of loader functions with signature `func() (T, error)` at construction time. The constructor MUST be `NewLazySlice[T](loaders []func() (T, error)) *LazySlice[T]`.

#### Scenario: Construction with loaders · `code-based` · `critical`
- **WHEN** `NewLazySlice[[]byte]` is called with 5 loader functions
- **THEN** a non-nil `*LazySlice[[]byte]` is returned with `Len() == 5`

#### Scenario: Construction with empty slice · `code-based` · `critical`
- **WHEN** `NewLazySlice[string]` is called with an empty slice
- **THEN** a non-nil `*LazySlice[string]` is returned with `Len() == 0`

#### Scenario: Construction with nil slice · `code-based` · `critical`
- **WHEN** `NewLazySlice[int]` is called with `nil`
- **THEN** a non-nil `*LazySlice[int]` is returned with `Len() == 0`

### REQ-CSYNC-002: Bounded Goroutine Pool

`LoadAll()` MUST fan out loader execution across goroutines. The number of concurrent goroutines MUST NOT exceed `min(runtime.NumCPU(), 8)`. This bound prevents goroutine explosion on machines with many cores or when assemblers load many files.

#### Scenario: Goroutine bound respected · `code-based` · `critical`
- **GIVEN** `runtime.NumCPU()` returns 16
- **WHEN** `LoadAll()` is called on a `LazySlice` with 20 loaders
- **THEN** at no point during execution do more than 8 loader goroutines run concurrently

#### Scenario: Goroutine bound on low-core machine · `code-based` · `important`
- **GIVEN** `runtime.NumCPU()` returns 2
- **WHEN** `LoadAll()` is called on a `LazySlice` with 10 loaders
- **THEN** at no point during execution do more than 2 loader goroutines run concurrently

#### Scenario: Fewer loaders than bound · `code-based` · `important`
- **WHEN** `LoadAll()` is called on a `LazySlice` with 3 loaders and bound is 8
- **THEN** all 3 loaders execute concurrently (no unnecessary serialization)

### REQ-CSYNC-003: LoadAll Blocks Until Completion

`LoadAll()` MUST block the calling goroutine until every loader has either returned a value or an error. After `LoadAll()` returns, results MUST be accessible by index via `Get(i int) (T, error)`.

#### Scenario: All loaders complete successfully · `code-based` · `critical`
- **WHEN** `LoadAll()` is called on a `LazySlice` with 5 loaders that each return distinct values
- **THEN** `LoadAll()` returns nil, and `Get(0)` through `Get(4)` each return the corresponding loader's value with nil error

#### Scenario: Result ordering matches input ordering · `code-based` · `critical`
- **WHEN** loaders are provided in order [A, B, C] and loader B completes before A
- **THEN** `Get(0)` returns A's result, `Get(1)` returns B's result, `Get(2)` returns C's result (positional, not arrival-order)

#### Scenario: LoadAll on empty slice · `code-based` · `important`
- **WHEN** `LoadAll()` is called on a `LazySlice` with 0 loaders
- **THEN** `LoadAll()` returns nil immediately

### REQ-CSYNC-004: Per-Element Error Handling

Individual loader failures MUST NOT prevent other loaders from completing. `LoadAll()` MUST return a non-nil error if any loader failed. Each element's error MUST be retrievable independently via `Get(i)`.

#### Scenario: Partial failure · `code-based` · `critical`
- **WHEN** `LoadAll()` is called with loaders [ok, fail, ok] where loader 1 returns an error
- **THEN** `LoadAll()` returns a non-nil error, `Get(0)` returns (value, nil), `Get(1)` returns (zero, error), `Get(2)` returns (value, nil)

#### Scenario: All loaders fail · `code-based` · `important`
- **WHEN** `LoadAll()` is called with 3 loaders that all return errors
- **THEN** `LoadAll()` returns a non-nil error, and `Get(0)`, `Get(1)`, `Get(2)` each return their respective errors

#### Scenario: Error message includes failed count · `code-based` · `important`
- **WHEN** 2 of 5 loaders fail
- **THEN** the error returned by `LoadAll()` indicates "2/5" or equivalent count information

### REQ-CSYNC-005: Thread Safety

`LazySlice` MUST be safe for concurrent use. Running tests with `-race` MUST produce zero data race reports. Internal state (result slots, error slots) MUST be written only by the owning goroutine and read only after `LoadAll()` returns.

#### Scenario: No data races under race detector · `code-based` · `critical`
- **WHEN** `go test -race ./internal/csync/...` is executed
- **THEN** the race detector reports zero warnings

#### Scenario: Concurrent LoadAll calls on different instances · `code-based` · `important`
- **WHEN** two `LazySlice` instances call `LoadAll()` concurrently from separate goroutines
- **THEN** both complete without data races and return correct results

### REQ-CSYNC-006: Zero Goroutine Leak

`LoadAll()` MUST NOT leak goroutines regardless of loader outcomes. All goroutines spawned by `LoadAll()` MUST have exited by the time `LoadAll()` returns. This applies to normal completion, partial errors, and loader panics.

#### Scenario: No leak on success · `code-based` · `critical`
- **WHEN** `LoadAll()` completes with all loaders succeeding
- **THEN** `runtime.NumGoroutine()` after `LoadAll()` returns is equal to or less than the count before `LoadAll()` was called (within a small delta for runtime goroutines)

#### Scenario: No leak on partial failure · `code-based` · `critical`
- **WHEN** `LoadAll()` completes with some loaders returning errors
- **THEN** goroutine count returns to baseline after `LoadAll()` returns

#### Scenario: Loader panic recovery · `code-based` · `critical`
- **WHEN** a loader function panics during `LoadAll()`
- **THEN** the panic is recovered, the corresponding `Get(i)` returns an error containing the panic value, other loaders complete normally, and no goroutine leaks

#### Scenario: No leak on empty slice · `code-based` · `important`
- **WHEN** `LoadAll()` is called on a `LazySlice` with 0 loaders
- **THEN** no goroutines are spawned

---

## Domain 2: events (Broker)

Package `internal/events` provides an in-process pub/sub broker that decouples side effects (metrics recording, cache saving, stderr output) from the assembly hot path. Fixes the metrics.json race in `AssembleConcurrent` by serializing all event-driven writes through a single dispatch path.

### REQ-EVENTS-001: Broker Type with Subscribe and Emit

`Broker` MUST provide `Subscribe(eventType string, handler func(Event))` for registering handlers and `Emit(event Event)` for dispatching. `Event` MUST be an interface or struct carrying at minimum `Type() string` for dispatch routing.

#### Scenario: Subscribe and emit single event · `code-based` · `critical`
- **WHEN** a handler is subscribed to "PhaseAssembled" and an event of type "PhaseAssembled" is emitted
- **THEN** the handler is called exactly once with the emitted event

#### Scenario: Multiple subscribers for same event type · `code-based` · `critical`
- **WHEN** two handlers are subscribed to "CacheHit" and a "CacheHit" event is emitted
- **THEN** both handlers are called exactly once each

#### Scenario: Handler not called for non-matching event type · `code-based` · `important`
- **WHEN** a handler is subscribed to "CacheHit" and a "CacheMiss" event is emitted
- **THEN** the handler is not called

#### Scenario: No subscribers for event type · `code-based` · `important`
- **WHEN** an event is emitted with no subscribers registered for its type
- **THEN** `Emit` completes without error and does not panic

### REQ-EVENTS-002: Event Types

The following typed event structs MUST be defined, each satisfying the `Event` interface:

- `PhaseAssembled` — emitted after `Assemble()` completes (cache hit or fresh assembly). MUST carry: phase name, byte count, token estimate, cached flag, duration.
- `CacheHit` — emitted when `tryCachedContext` returns cached content. MUST carry: phase name, byte count.
- `CacheMiss` — emitted when `tryCachedContext` returns no cached content. MUST carry: phase name.
- `ArtifactPromoted` — emitted when `runWrite()` promotes a pending artifact. MUST carry: change name, phase name, promoted file path.
- `StateAdvanced` — emitted when `runWrite()` advances the state machine. MUST carry: change name, from-phase, to-phase.

#### Scenario: PhaseAssembled carries required fields · `code-based` · `critical`
- **WHEN** `Assemble()` completes for the "review" phase
- **THEN** the emitted `PhaseAssembled` event has `Phase == "review"`, `Bytes > 0`, `Tokens > 0`, `DurationMs >= 0`, and `Cached` is a valid boolean

#### Scenario: CacheHit carries byte count · `code-based` · `important`
- **GIVEN** a cached context exists for the "spec" phase
- **WHEN** `Assemble()` is called for "spec"
- **THEN** a `CacheHit` event is emitted with `Phase == "spec"` and `Bytes > 0`

#### Scenario: CacheMiss emitted on fresh assembly · `code-based` · `important`
- **GIVEN** no cached context exists for the "explore" phase
- **WHEN** `Assemble()` is called for "explore"
- **THEN** a `CacheMiss` event is emitted with `Phase == "explore"`

#### Scenario: ArtifactPromoted carries promoted path · `code-based` · `critical`
- **WHEN** `runWrite()` promotes a pending artifact for the "design" phase
- **THEN** an `ArtifactPromoted` event is emitted with the promoted file path

#### Scenario: StateAdvanced carries phase transition · `code-based` · `critical`
- **WHEN** `runWrite()` advances state from "design" to "tasks"
- **THEN** a `StateAdvanced` event is emitted with `FromPhase == "design"` and `ToPhase == "tasks"`

### REQ-EVENTS-003: Subscriber Panic Recovery

A panicking subscriber MUST NOT crash the CLI process. `Emit()` MUST wrap each subscriber call in `defer recover()`. The panic SHOULD be logged to stderr. Other subscribers for the same event MUST still execute.

#### Scenario: Panicking subscriber does not crash · `code-based` · `critical`
- **WHEN** a subscriber panics during `Emit()`
- **THEN** `Emit()` completes without propagating the panic to the caller

#### Scenario: Other subscribers still execute after panic · `code-based` · `critical`
- **GIVEN** three subscribers for "PhaseAssembled" where the second panics
- **WHEN** a "PhaseAssembled" event is emitted
- **THEN** the first and third subscribers are called; only the second's execution is interrupted

#### Scenario: Panic message logged to stderr · `code-based` · `important`
- **WHEN** a subscriber panics with message "oops"
- **THEN** stderr output contains "oops" or equivalent diagnostic information

### REQ-EVENTS-004: Serialized Dispatch

`Emit()` MUST serialize subscriber dispatch so that concurrent `Emit()` calls from different goroutines (as in `AssembleConcurrent`) do not cause data races in subscriber state. This fixes the current race where two goroutines in `AssembleConcurrent` concurrently call `recordMetrics()` which reads, modifies, and writes `metrics.json`.

#### Scenario: Concurrent emits do not race · `code-based` · `critical`
- **WHEN** 10 goroutines each call `Emit()` simultaneously, each targeting a subscriber that writes to a shared counter
- **THEN** `go test -race` reports zero data races, and the final counter value equals 10

#### Scenario: Serialized dispatch preserves ordering within single goroutine · `code-based` · `important`
- **WHEN** a single goroutine emits events A then B in sequence
- **THEN** the subscriber for A completes before the subscriber for B begins

#### Scenario: Metrics.json not corrupted under concurrent assembly · `code-based` · `critical`
- **GIVEN** `recordMetrics` is wired as an event subscriber
- **WHEN** `AssembleConcurrent` runs spec and design phases in parallel, each emitting `PhaseAssembled`
- **THEN** `metrics.json` contains valid JSON with entries for both phases and no data loss

### REQ-EVENTS-005: Nil-Safe Broker

A nil `*Broker` MUST be safe to call `Emit()` on. This allows callers (e.g., `Assemble()`, `runWrite()`) to emit events without nil-checking, and allows tests that don't set a broker to continue working.

#### Scenario: Nil broker emit is no-op · `code-based` · `critical`
- **WHEN** `Emit()` is called on a nil `*Broker`
- **THEN** the call returns immediately without panic

#### Scenario: Nil broker subscribe is no-op · `code-based` · `important`
- **WHEN** `Subscribe()` is called on a nil `*Broker`
- **THEN** the call returns immediately without panic

#### Scenario: Existing tests pass without broker · `code-based` · `critical`
- **GIVEN** `Params.Broker` is not set (zero value nil)
- **WHEN** `Assemble()` is called
- **THEN** assembly completes successfully, no panic, no events emitted

### REQ-EVENTS-006: Integration with Assemble and runWrite

`Assemble()` in `context.go` MUST emit `CacheHit` or `CacheMiss` and `PhaseAssembled` events via the broker on `Params`. `runWrite()` in `commands.go` MUST emit `ArtifactPromoted` and `StateAdvanced` events. The broker MUST be injected via a new `Broker *events.Broker` field on `Params`.

#### Scenario: Assemble emits CacheHit on cached context · `code-based` · `critical`
- **GIVEN** a valid cached context exists for the "propose" phase
- **WHEN** `Assemble()` is called with a broker that records events
- **THEN** the recorder contains exactly one `CacheHit` and one `PhaseAssembled` event for "propose"

#### Scenario: Assemble emits CacheMiss on fresh assembly · `code-based` · `critical`
- **GIVEN** no cached context for the "explore" phase
- **WHEN** `Assemble()` is called with a broker that records events
- **THEN** the recorder contains exactly one `CacheMiss` and one `PhaseAssembled` event for "explore"

#### Scenario: runWrite emits both events · `code-based` · `critical`
- **WHEN** `runWrite()` successfully promotes and advances for the "spec" phase
- **THEN** the broker has received one `ArtifactPromoted` and one `StateAdvanced` event

#### Scenario: Broker field on Params is backward compatible · `code-based` · `critical`
- **WHEN** `Params{}` is constructed without setting the `Broker` field
- **THEN** the zero-value `Broker` field is nil, and `Assemble()` skips event emission

---

## Domain 3: pprof (Profiling Gate)

An environment-variable-gated profiling hook in `cmd/sdd/main.go` that enables CPU and heap profiling to file. Zero overhead when unset.

### REQ-PPROF-001: SDD_PPROF Environment Variable

The `main()` function MUST check `os.Getenv("SDD_PPROF")` before calling `cli.Run()`. Recognized values: `"cpu"`, `"mem"`, `"all"`. Any other non-empty value SHOULD be treated as unrecognized and logged to stderr as a warning. Empty or unset MUST result in no profiling and zero overhead.

#### Scenario: SDD_PPROF=cpu enables CPU profiling · `code-based` · `critical`
- **GIVEN** `SDD_PPROF` is set to `"cpu"`
- **WHEN** `sdd` runs and exits
- **THEN** `sdd-cpu.prof` is created in the current working directory

#### Scenario: SDD_PPROF=mem enables memory profiling · `code-based` · `critical`
- **GIVEN** `SDD_PPROF` is set to `"mem"`
- **WHEN** `sdd` runs and exits
- **THEN** `sdd-mem.prof` is created in the current working directory

#### Scenario: SDD_PPROF=all enables both profiles · `code-based` · `critical`
- **GIVEN** `SDD_PPROF` is set to `"all"`
- **WHEN** `sdd` runs and exits
- **THEN** both `sdd-cpu.prof` and `sdd-mem.prof` are created in the current working directory

#### Scenario: SDD_PPROF unset means no profiling · `code-based` · `critical`
- **GIVEN** `SDD_PPROF` is not set
- **WHEN** `sdd` runs and exits
- **THEN** no `sdd-cpu.prof` or `sdd-mem.prof` files are created

#### Scenario: Unrecognized value warns · `code-based` · `important`
- **GIVEN** `SDD_PPROF` is set to `"trace"`
- **WHEN** `sdd` runs
- **THEN** stderr contains a warning about the unrecognized value, and no profiling is enabled

### REQ-PPROF-002: Profile File Output

CPU profile MUST be written to `sdd-cpu.prof` and heap profile to `sdd-mem.prof`, both in the current working directory. These follow the CWD convention established by the crash log in `main.go` (line 17: `.sdd-crash-{ts}.log`).

#### Scenario: CPU profile is valid pprof format · `code-based` · `critical`
- **GIVEN** `SDD_PPROF=cpu` and `sdd` has completed execution
- **WHEN** `go tool pprof sdd-cpu.prof` is invoked
- **THEN** the tool loads the profile without errors

#### Scenario: Heap profile is valid pprof format · `code-based` · `critical`
- **GIVEN** `SDD_PPROF=mem` and `sdd` has completed execution
- **WHEN** `go tool pprof sdd-mem.prof` is invoked
- **THEN** the tool loads the profile without errors

#### Scenario: Profile file is written even on CLI error · `code-based` · `important`
- **GIVEN** `SDD_PPROF=cpu`
- **WHEN** `sdd` exits with a non-zero exit code due to a CLI error
- **THEN** `sdd-cpu.prof` is still written (profiling stop is in a `defer`)

#### Scenario: Profile file overwrite on re-run · `code-based` · `important`
- **GIVEN** `sdd-cpu.prof` already exists from a previous run
- **WHEN** `sdd` runs again with `SDD_PPROF=cpu`
- **THEN** `sdd-cpu.prof` is overwritten with the new profile data

### REQ-PPROF-003: Zero Overhead When Unset

When `SDD_PPROF` is not set or empty, the profiling gate MUST NOT call any `runtime/pprof` functions, MUST NOT open any files, and MUST NOT allocate profiling-related buffers. The check MUST be a single `os.Getenv` call with an early return.

#### Scenario: No pprof imports executed when unset · `code-based` · `critical`
- **GIVEN** `SDD_PPROF` is not set
- **WHEN** `main()` executes
- **THEN** no `pprof.StartCPUProfile`, `pprof.WriteHeapProfile`, or `os.Create` calls for profile files occur

#### Scenario: Startup latency unaffected · `code-based` · `important`
- **GIVEN** `SDD_PPROF` is not set
- **WHEN** `sdd version` is timed over 10 runs
- **THEN** mean execution time does not increase by more than 1ms compared to baseline (within measurement noise)

### REQ-PPROF-004: Doctor Check for Pprof

`sdd doctor` MUST include a check that reports the current `SDD_PPROF` value if set, or "not set (no profiling)" if unset. This is informational only and SHOULD always return status "pass" (never "fail").

#### Scenario: Doctor reports SDD_PPROF when set · `code-based` · `important`
- **GIVEN** `SDD_PPROF=cpu`
- **WHEN** `sdd doctor` runs
- **THEN** output includes a check named "pprof" with status "pass" and message indicating `SDD_PPROF=cpu`

#### Scenario: Doctor reports pprof not set · `code-based` · `important`
- **GIVEN** `SDD_PPROF` is not set
- **WHEN** `sdd doctor` runs
- **THEN** output includes a check named "pprof" with status "pass" and message "not set (no profiling)" or equivalent

---

## Eval Definitions

| Scenario | Eval Type | Criticality | Threshold |
|----------|-----------|-------------|-----------|
| Construction with loaders | unit test | critical | PASS |
| Construction with empty slice | unit test | critical | PASS |
| Construction with nil slice | unit test | critical | PASS |
| Goroutine bound respected | unit test | critical | PASS |
| Goroutine bound on low-core machine | unit test | important | PASS |
| Fewer loaders than bound | unit test | important | PASS |
| All loaders complete successfully | unit test | critical | PASS |
| Result ordering matches input ordering | unit test | critical | PASS |
| LoadAll on empty slice | unit test | important | PASS |
| Partial failure | unit test | critical | PASS |
| All loaders fail | unit test | important | PASS |
| Error message includes failed count | unit test | important | PASS |
| No data races under race detector | unit test + race | critical | PASS |
| Concurrent LoadAll calls on different instances | unit test + race | important | PASS |
| No leak on success | unit test | critical | goroutine delta <= 2 |
| No leak on partial failure | unit test | critical | goroutine delta <= 2 |
| Loader panic recovery | unit test | critical | PASS |
| No leak on empty slice | unit test | important | PASS |
| Subscribe and emit single event | unit test | critical | PASS |
| Multiple subscribers for same event type | unit test | critical | PASS |
| Handler not called for non-matching event type | unit test | important | PASS |
| No subscribers for event type | unit test | important | PASS |
| PhaseAssembled carries required fields | integration test | critical | PASS |
| CacheHit carries byte count | integration test | important | PASS |
| CacheMiss emitted on fresh assembly | integration test | important | PASS |
| ArtifactPromoted carries promoted path | integration test | critical | PASS |
| StateAdvanced carries phase transition | integration test | critical | PASS |
| Panicking subscriber does not crash | unit test | critical | PASS |
| Other subscribers still execute after panic | unit test | critical | PASS |
| Panic message logged to stderr | unit test | important | PASS |
| Concurrent emits do not race | unit test + race | critical | PASS |
| Serialized dispatch preserves ordering | unit test | important | PASS |
| Metrics.json not corrupted under concurrent assembly | integration test + race | critical | valid JSON |
| Nil broker emit is no-op | unit test | critical | PASS |
| Nil broker subscribe is no-op | unit test | important | PASS |
| Existing tests pass without broker | integration test | critical | PASS |
| Assemble emits CacheHit on cached context | integration test | critical | PASS |
| Assemble emits CacheMiss on fresh assembly | integration test | critical | PASS |
| runWrite emits both events | integration test | critical | PASS |
| Broker field on Params is backward compatible | unit test | critical | PASS |
| SDD_PPROF=cpu enables CPU profiling | integration test | critical | file exists |
| SDD_PPROF=mem enables memory profiling | integration test | critical | file exists |
| SDD_PPROF=all enables both profiles | integration test | critical | files exist |
| SDD_PPROF unset means no profiling | integration test | critical | files absent |
| Unrecognized value warns | integration test | important | stderr contains warning |
| CPU profile is valid pprof format | integration test | critical | pprof loads |
| Heap profile is valid pprof format | integration test | critical | pprof loads |
| Profile file is written even on CLI error | integration test | important | file exists |
| Profile file overwrite on re-run | integration test | important | file modified time changes |
| No pprof imports executed when unset | code review | critical | PASS |
| Startup latency unaffected | benchmark | important | delta < 1ms |
| Doctor reports SDD_PPROF when set | integration test | important | PASS |
| Doctor reports pprof not set | integration test | important | PASS |

## Acceptance Criteria Summary

| Requirement ID | Type | Priority | Scenarios |
|----------------|------|----------|-----------|
| REQ-CSYNC-001 | Functional | P0 | Construction with loaders, Construction with empty slice, Construction with nil slice |
| REQ-CSYNC-002 | Performance | P0 | Goroutine bound respected, Goroutine bound on low-core machine, Fewer loaders than bound |
| REQ-CSYNC-003 | Functional | P0 | All loaders complete successfully, Result ordering matches input ordering, LoadAll on empty slice |
| REQ-CSYNC-004 | Functional | P0 | Partial failure, All loaders fail, Error message includes failed count |
| REQ-CSYNC-005 | Non-functional | P0 | No data races under race detector, Concurrent LoadAll calls on different instances |
| REQ-CSYNC-006 | Reliability | P0 | No leak on success, No leak on partial failure, Loader panic recovery, No leak on empty slice |
| REQ-EVENTS-001 | Functional | P0 | Subscribe and emit single event, Multiple subscribers for same event type, Handler not called for non-matching event type, No subscribers for event type |
| REQ-EVENTS-002 | Functional | P0 | PhaseAssembled carries required fields, CacheHit carries byte count, CacheMiss emitted on fresh assembly, ArtifactPromoted carries promoted path, StateAdvanced carries phase transition |
| REQ-EVENTS-003 | Reliability | P0 | Panicking subscriber does not crash, Other subscribers still execute after panic, Panic message logged to stderr |
| REQ-EVENTS-004 | Non-functional | P0 | Concurrent emits do not race, Serialized dispatch preserves ordering, Metrics.json not corrupted under concurrent assembly |
| REQ-EVENTS-005 | Reliability | P0 | Nil broker emit is no-op, Nil broker subscribe is no-op, Existing tests pass without broker |
| REQ-EVENTS-006 | Integration | P0 | Assemble emits CacheHit on cached context, Assemble emits CacheMiss on fresh assembly, runWrite emits both events, Broker field on Params is backward compatible |
| REQ-PPROF-001 | Functional | P0 | SDD_PPROF=cpu enables CPU profiling, SDD_PPROF=mem enables memory profiling, SDD_PPROF=all enables both profiles, SDD_PPROF unset means no profiling, Unrecognized value warns |
| REQ-PPROF-002 | Functional | P0 | CPU profile is valid pprof format, Heap profile is valid pprof format, Profile file is written even on CLI error, Profile file overwrite on re-run |
| REQ-PPROF-003 | Performance | P1 | No pprof imports executed when unset, Startup latency unaffected |
| REQ-PPROF-004 | Informational | P2 | Doctor reports SDD_PPROF when set, Doctor reports pprof not set |
