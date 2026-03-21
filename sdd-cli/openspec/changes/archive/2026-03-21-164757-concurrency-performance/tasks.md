# Implementation Tasks: Concurrency and Performance

**Change**: concurrency-performance
**Date**: 2026-03-21
**Status**: pending
**Depends On**: design.md, specs/spec.md

---

## Summary

- **Total Tasks**: 21
- **Phases**: 5
- **Estimated Files Changed**: 19 (5 created, 14 modified)
- **Test Cases Planned**: 20 (from design.md Testing Strategy table)

## Verification Commands

After each phase:

```
go build ./...
go test ./... -race
```

Per-package targeted runs are listed in each phase checkpoint.

---

## Phase 1: Foundation — New Packages (2 tasks)

Creates the two leaf packages (`csync`, `events`) that have zero internal dependencies. Every subsequent phase depends on these.

> Tasks 1.1 and 1.2 can run in parallel.

- [x] 1.1 Create — `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice.go`, implement `LazySlice[T]` generic type: `result[T]` internal struct, `NewLazySlice[T]`, `Len`, `LoadAll` with buffered-channel semaphore (`min(runtime.NumCPU(), 8)`), `sync.WaitGroup`, per-slot panic recovery, aggregate error formatting (`N/M loaders failed`), `Get(i)`, `MustGet(i)`

- [x] 1.2 Create — `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker.go`, implement `EventType` string type, event type constants (`PhaseAssembled`, `CacheHit`, `CacheMiss`, `ArtifactPromoted`, `StateAdvanced`), `Event` struct with `Type EventType` and `Payload any`, all payload structs (`PhaseAssembledPayload`, `CacheHitPayload`, `CacheMissPayload`, `ArtifactPromotedPayload`, `StateAdvancedPayload`), `Handler` type alias, `Broker` struct with `sync.Mutex` + `subs map[EventType][]Handler` + `stderr io.Writer`, `NewBroker(stderr io.Writer) *Broker`, nil-safe `Subscribe(t EventType, h Handler)`, nil-safe `Emit(e Event)` with per-handler `defer recover()` and panic logging to stderr

**Phase 1 Checkpoint**: `go build ./internal/csync/... ./internal/events/...` exits 0. Both packages compile with zero imports from internal modules.

---

## Phase 2: pprof Gate (1 task)

Simplest feature — modifies only `cmd/sdd/main.go` with no dependencies on Phase 1 packages. Can be done concurrently with Phase 1 but listed here for sequencing clarity.

- [x] 2.1 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/cmd/sdd/main.go`, add `startProfile(mode string, stderr *os.File) func()` function: check `mode == ""` early return (zero overhead), parse `cpu`/`mem`/`all` values, warn on unrecognized non-empty value via `fmt.Fprintf(stderr, ...)`, open `sdd-cpu.prof` and call `pprof.StartCPUProfile` for cpu/all, open `sdd-mem.prof` with `runtime.GC()` + `pprof.WriteHeapProfile` closer for mem/all, return aggregate closer; wire `stopProfile := startProfile(os.Getenv("SDD_PPROF"), os.Stderr)` + `defer stopProfile()` in `main()` before `cli.Run()`. Add `runtime`, `runtime/pprof` to imports.

**Phase 2 Checkpoint**: `go build ./cmd/sdd/...` exits 0. Existing panic-recovery defer still present.

---

## Phase 3: Assembler Refactors (9 tasks)

Mechanical refactor of each assembler to use `LazySlice[[]byte]`. Each assembler file is independent. Requires Phase 1 (csync package). Output must be byte-identical to the current sequential implementation.

> Tasks 3.1 through 3.8 can run in parallel.

- [x] 3.1 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/explore.go`, refactor `AssembleExplore` to build `[]func() ([]byte, error)` closures for skill, gitFileTree, manifests; create `csync.NewLazySlice(loaders)`, call `ls.LoadAll()`, check critical loader errors via `ls.Get(i)`, retrieve results; add `"github.com/rechedev9/shenronSDD/sdd-cli/internal/csync"` import

- [x] 3.2 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/propose.go`, refactor `AssemblePropose` to use `LazySlice[[]byte]` for skill, exploration artifact, gitFileTree loaders; handle critical/optional error distinction

- [x] 3.3 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/spec.go`, refactor `AssembleSpec` to use `LazySlice[[]byte]` for skill, proposal artifact, buildSummary loaders; wrap string-returning functions via `[]byte(...)` conversion

- [x] 3.4 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/design.go`, refactor `AssembleDesign` to use `LazySlice[[]byte]` for skill, proposal, specs, buildSummary loaders; `loadSpecs` returns `(string, error)` — wrap as `func() ([]byte, error)` closure

- [x] 3.5 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/tasks.go`, refactor `AssembleTasks` to use `LazySlice[[]byte]` for skill, design artifact, specs loaders

- [x] 3.6 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/apply.go`, refactor `AssembleApply` to use `LazySlice[[]byte]` for skill, tasks artifact, design artifact, specs, buildSummary loaders

- [x] 3.7 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/review.go`, refactor `AssembleReview` to use `LazySlice[[]byte]` for skill, specs, design, tasks, gitDiff (non-fatal), project rules (non-fatal) loaders; gitDiff error results in fallback string (loader never fails)

- [x] 3.8 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/clean.go`, refactor `AssembleClean` to use `LazySlice[[]byte]` for skill, verifyReport, tasks, buildSummary, design, specs loaders

> Task 3.9 depends on 3.1 through 3.8 compiling.

- [x] 3.9 Verify — run `go build ./internal/context/...` to confirm all assemblers compile after LazySlice refactor; fix any type errors (string-to-[]byte conversions, import paths)

**Phase 3 Checkpoint**: `go build ./...` exits 0. All 8 assembler functions compile with csync import. No behavior change — same sections written in same order.

---

## Phase 4: Event Broker Integration (6 tasks)

Wires `events.Broker` into the context assembly and CLI dispatch path. Requires Phase 1 (events package) and Phase 3 (assemblers compile). Tasks 4.1 through 4.3 modify different files and can run in parallel; 4.4 through 4.6 depend on 4.1–4.3.

> Tasks 4.1, 4.2+4.3 share context.go — do 4.1 first, then 4.2 and 4.3 together.

- [x] 4.1 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go`, add `Broker *events.Broker` field to `Params` struct (after `Verbosity int`); add `"github.com/rechedev9/shenronSDD/sdd-cli/internal/events"` import; this is the only change to the struct — all existing tests continue to work (nil zero value is nil-safe)

- [x] 4.2 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go`, replace inline `emitMetrics(p.Stderr, ...)` and `saveContextCache(...)` calls in `Assemble()` with `p.Broker.Emit(...)` calls: emit `events.CacheHit` on cache hit path (before return), emit `events.CacheMiss` on cache miss path (before `fn(&buf, p)`), emit `events.PhaseAssembled` on both paths (cache hit: `Cached: true`, cache miss: `Cached: false, Content: content`); delete the `emitMetrics` function (lines 98-115)

- [x] 4.3 Create — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/subscribers.go`, implement `RegisterSubscribers(broker *events.Broker, stderr io.Writer, verbosity int)`: nil-guard on broker, subscribe three handlers on `events.PhaseAssembled`: (1) metrics recording — type-assert payload, build `contextMetrics`, call `recordMetrics(p.ChangeDir, m)`; (2) stderr output — call `writeMetrics(stderr, m, verbosity)` guarded by `stderr != nil && verbosity >= 0`; (3) cache persistence — skip if `p.Cached || p.Content == nil`, call `saveContextCache(p.ChangeDir, p.Phase, p.SkillsPath, p.Content)`

> Tasks 4.4, 4.5, 4.6 depend on 4.1–4.3 completing.

- [x] 4.4 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/cli.go`, in `Run()` create `broker := events.NewBroker(stderr)`, call `sddctx.RegisterSubscribers(broker, stderr, verbosity)` (extract verbosity from args or default 0), pass broker to `runContext`, `runNew`, and `runWrite` as additional parameter; add `events` and `sddctx` imports; update function signatures for `runContext(rest, stdout, stderr, broker)`, `runNew(rest, stdout, stderr, broker)`, `runWrite(rest, stdout, stderr, broker)`

- [x] 4.5 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go`, update `runContext` and `runNew` to accept `broker *events.Broker` parameter and set `p.Broker = broker` when constructing `sddctx.Params`; update `runWrite` to accept `broker *events.Broker` parameter, capture `prevPhase := st.CurrentPhase` before `st.Advance(phase)`, emit `events.ArtifactPromoted` after `artifacts.Promote` returns, emit `events.StateAdvanced` after `state.Save` succeeds; add `events` import

- [x] 4.6 Modify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/doctor.go`, add `checkPprof() CheckResult` function: `val := os.Getenv("SDD_PPROF")`, return `CheckResult{Name: "pprof", Status: "pass", Message: "not set (no profiling)"}` when empty, else `CheckResult{Name: "pprof", Status: "pass", Message: fmt.Sprintf("SDD_PPROF=%s", val)}`; add `checkPprof()` to the `checks` slice in `runDoctor()`

**Phase 4 Checkpoint**: `go build ./...` exits 0. All packages compile. `sdd context` and `sdd write` commands still work end-to-end (events emitted, subscribers handle metrics/cache/stderr output).

---

## Phase 5: Tests (3 tasks)

Tests are the final phase but are not optional. Each test file is independent.

> Tasks 5.1 and 5.2 can run in parallel.

- [x] 5.1 Create — `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go`, write unit tests covering: construction with N/0/nil loaders + `Len()` (REQ-CSYNC-001); goroutine bound — use `sync/atomic` counter with `runtime.Gosched()` to verify concurrent count never exceeds `min(NumCPU, 8)` (REQ-CSYNC-002); `LoadAll` blocks and returns positional results (REQ-CSYNC-003); partial failure — loader 1 returns error, `LoadAll` returns non-nil, `Get(0)` and `Get(2)` return values, `Get(1)` returns error (REQ-CSYNC-004); error message includes `"2/5"` format (REQ-CSYNC-004); goroutine leak — compare `runtime.NumGoroutine()` before and after with delta <= 2 (REQ-CSYNC-006); panic recovery — panicking loader produces error result, other loaders complete, no goroutine leak (REQ-CSYNC-006); all tests run with `-race` (REQ-CSYNC-005)
  Verify: `go test -race ./internal/csync/...`

- [x] 5.2 Create — `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go`, write unit tests covering: single subscribe + emit calls handler once (REQ-EVENTS-001); two subscribers for same type both called (REQ-EVENTS-001); handler not called for non-matching type (REQ-EVENTS-001); emit with no subscribers is no-op (REQ-EVENTS-001); panicking subscriber does not propagate panic, other subscribers still execute, panic message appears in stderr capture (REQ-EVENTS-003); 10 concurrent `Emit()` calls targeting a shared `atomic.Int64` counter — final value == 10, no races (REQ-EVENTS-004); nil broker `Emit()` and `Subscribe()` are no-ops without panic (REQ-EVENTS-005)
  Verify: `go test -race ./internal/events/...`

> Task 5.3 depends on Phase 4 completing.

- [x] 5.3 Verify — `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context_test.go`, confirm existing tests pass unchanged (nil `Params.Broker` triggers nil-safe no-ops — no behavior change to assembled output); add test: `Assemble()` with a recording broker emits exactly `CacheMiss` then `PhaseAssembled` on fresh assembly; add test: `AssembleConcurrent` under `-race` with broker and `PhaseAssembled` subscriber that writes to a file — no data races, file contains valid JSON after both phases complete (REQ-EVENTS-004, REQ-EVENTS-005, REQ-EVENTS-006)
  Verify: `go test -race ./internal/context/...`

**Phase 5 Checkpoint**: `go test -race ./...` exits 0 with zero race warnings. All 20 test scenarios from the Testing Strategy table pass.

---

## Requirement Traceability Matrix

| Requirement ID  | Implementation Task(s) | Test Task(s) | Status  |
|-----------------|------------------------|--------------|---------|
| REQ-CSYNC-001   | 1.1                    | 5.1          | pending |
| REQ-CSYNC-002   | 1.1                    | 5.1          | pending |
| REQ-CSYNC-003   | 1.1, 3.1–3.8           | 5.1, 5.3     | pending |
| REQ-CSYNC-004   | 1.1                    | 5.1          | pending |
| REQ-CSYNC-005   | 1.1                    | 5.1          | pending |
| REQ-CSYNC-006   | 1.1                    | 5.1          | pending |
| REQ-EVENTS-001  | 1.2                    | 5.2          | pending |
| REQ-EVENTS-002  | 1.2                    | 5.2, 5.3     | pending |
| REQ-EVENTS-003  | 1.2                    | 5.2          | pending |
| REQ-EVENTS-004  | 1.2, 4.2, 4.3          | 5.2, 5.3     | pending |
| REQ-EVENTS-005  | 1.2, 4.1               | 5.2, 5.3     | pending |
| REQ-EVENTS-006  | 4.1, 4.2, 4.3, 4.4, 4.5| 5.3          | pending |
| REQ-PPROF-001   | 2.1                    | (manual)     | pending |
| REQ-PPROF-002   | 2.1                    | (manual)     | pending |
| REQ-PPROF-003   | 2.1                    | (code review)| pending |
| REQ-PPROF-004   | 4.6                    | (manual)     | pending |

---

## Success Criteria Checklist

From the proposal and spec acceptance criteria — all must be true when tasks are complete:

- [ ] `go build ./...` exits 0 — all packages compile
- [ ] `go test -race ./...` exits 0 — zero test failures, zero race detector warnings
- [ ] `internal/csync/lazyslice.go` exists and `LazySlice[T]` bounds goroutines to `min(NumCPU, 8)` (REQ-CSYNC-002)
- [ ] `internal/events/broker.go` exists and `Emit()` serializes concurrent calls via `sync.Mutex` (REQ-EVENTS-004)
- [ ] `Params.Broker` field is nil-compatible — existing tests with no broker pass without modification (REQ-EVENTS-005)
- [ ] `AssembleConcurrent` with real phases emits `PhaseAssembled` events without data races in the metrics subscriber (REQ-EVENTS-004 scenario: Metrics.json not corrupted)
- [ ] `SDD_PPROF=cpu` creates `sdd-cpu.prof` in CWD (REQ-PPROF-001)
- [ ] `SDD_PPROF=mem` creates `sdd-mem.prof` in CWD (REQ-PPROF-001)
- [ ] `SDD_PPROF=all` creates both files (REQ-PPROF-001)
- [ ] `SDD_PPROF` unset — no profile files created, no pprof calls made (REQ-PPROF-003)
- [ ] `sdd doctor` output includes `pprof` check with `status: pass` in all cases (REQ-PPROF-004)
- [ ] Assembler output is byte-identical to pre-refactor for all 8 phases (REQ-CSYNC-003 result ordering)
- [ ] All delta scenarios from spec.md verified by automated tests (20 test cases from Testing Strategy)
