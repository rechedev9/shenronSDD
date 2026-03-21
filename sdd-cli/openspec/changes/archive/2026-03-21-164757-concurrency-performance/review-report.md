# Review: Concurrency and Performance

**Change**: concurrency-performance
**Date**: 2026-03-21
**Reviewer**: Claude Opus 4.6
**Verdict**: PASS (with non-blocking issues)

---

## 1. Build and Test Verification

- `go build ./...` -- PASS (compiles cleanly, zero warnings)
- `go test ./... -count=1` -- PASS (all 10 packages, including `csync` and `events`)
- `go test -race` -- NOT RUNNABLE (environment lacks `gcc`/CGO; race detector unavailable in this CI-like environment). This is an environment limitation, not an implementation issue. The code's architecture (indexed result slots written by single goroutines, WaitGroup barrier, mutex-serialized broker dispatch) is structurally sound for race safety.

---

## 2. Spec Compliance (REQ-by-REQ)

### Domain 1: csync (LazySlice)

| REQ | Status | Notes |
|-----|--------|-------|
| REQ-CSYNC-001 | PASS | `NewLazySlice[T]` is generic, accepts `[]func() (T, error)`, nil-normalizes to empty. `Len()` correct. Tests: `TestNewLazySlice_NilLoaders`, `TestNewLazySlice_EmptyLoaders`, `TestNewLazySlice_Len`. |
| REQ-CSYNC-002 | PASS | `maxWorkers()` returns `min(runtime.NumCPU(), 8)` clamped to `[1, 8]`. Buffered channel semaphore enforces bound. Test: `TestLoadAll_GoroutineBound` tracks peak concurrency with atomic counter. Design adds floor clamp `n < 1 -> 1` not in spec (defensive; beneficial). |
| REQ-CSYNC-003 | PASS | `LoadAll()` blocks via `wg.Wait()`. Results positionally indexed. Idempotent (`loaded` flag). Tests: `TestLoadAll_Results`, `TestLoadAll_Idempotent`. Empty-slice returns nil immediately. |
| REQ-CSYNC-004 | PASS | Partial failures do not abort other loaders. Aggregate error includes `"N/M loaders failed"` format. Tests: `TestLoadAll_PartialFailure` checks `2/5` string and per-element `Get()`. |
| REQ-CSYNC-005 | PARTIAL | Code is structurally race-free (each `results[idx]` written by exactly one goroutine, reads gated behind `wg.Wait()`). Cannot verify with `-race` flag due to env limitation. Test exists (`TestLoadAll_GoroutineBound` uses atomics). |
| REQ-CSYNC-006 | PASS | `wg.Wait()` ensures all goroutines finish. Panic recovery via `defer recover()` prevents leak on panic. Test: `TestLoadAll_NoGoroutineLeak` checks goroutine delta <= 2. `TestLoadAll_PanicRecovery` verifies panicking loader becomes error without leak. |

### Domain 2: events (Broker)

| REQ | Status | Notes |
|-----|--------|-------|
| REQ-EVENTS-001 | PASS | `Subscribe(EventType, Handler)` and `Emit(Event)` implemented. `Event` struct with `Type EventType` and `Payload any`. Tests: `TestBroker_SubscribeAndEmit`, `TestBroker_MultipleSubscribers`, `TestBroker_NonMatchingType`, `TestBroker_EmitNoSubscribers`. |
| REQ-EVENTS-002 | PASS | All 5 event types defined as constants with typed payload structs: `PhaseAssembledPayload`, `CacheHitPayload`, `CacheMissPayload`, `ArtifactPromotedPayload`, `StateAdvancedPayload`. All required fields present per spec. Test: `TestBroker_Payload`, `TestBroker_AllEventTypes`. |
| REQ-EVENTS-003 | PASS | Each handler invoked inside anonymous `func()` with `defer recover()`. Panic logged to `b.stderr`. Other handlers still execute. Test: `TestBroker_SubscriberPanicRecovery` verifies handler 1 and 3 run while 2 panics, and stderr contains panic message. |
| REQ-EVENTS-004 | PASS | `sync.Mutex` serializes all `Emit()` calls. Test: `TestBroker_ConcurrentEmit` fires 10 goroutines, verifies counter==10. |
| REQ-EVENTS-005 | PASS | Both `Subscribe` and `Emit` start with `if b == nil { return }`. Test: `TestBroker_NilSafe`. Existing tests in `context_test.go` pass without setting broker (verified by test run). |
| REQ-EVENTS-006 | PASS | `Assemble()` emits `CacheHit`/`CacheMiss` and `PhaseAssembled` via `p.Broker.Emit()`. `Params` struct has `Broker *events.Broker` field. `runWrite()` emits `ArtifactPromoted` and `StateAdvanced`. Backward compatible -- nil broker means no events. |

### Domain 3: pprof (Profiling Gate)

| REQ | Status | Notes |
|-----|--------|-------|
| REQ-PPROF-001 | PASS | `startProfile()` checks `os.Getenv("SDD_PPROF")` with recognized values `"cpu"`, `"mem"`, `"all"`. Unrecognized non-empty values logged to stderr. Empty/unset returns no-op closure. |
| REQ-PPROF-002 | PASS | CPU profile -> `sdd-cpu.prof`, heap profile -> `sdd-mem.prof` in CWD. CPU profile uses `pprof.StartCPUProfile`/`StopCPUProfile`. Heap profile calls `runtime.GC()` then `pprof.WriteHeapProfile`. Both via `defer` (written even on CLI error). |
| REQ-PPROF-003 | PASS | When mode is empty, `startProfile` returns immediately with no-op closure. No `pprof` calls, no `os.Create`, no allocations. Single `os.Getenv` in `main()`. |
| REQ-PPROF-004 | PASS | `checkPprof()` in `doctor.go` reports `SDD_PPROF` value or "not set (no profiling)". Always returns status "pass". Wired into `runDoctor` check list. |

---

## 3. Code Quality Observations

### What was done well

- **Clean, mechanical refactor pattern** -- All 8 assemblers follow the identical pattern: build loader slice, `NewLazySlice`, `LoadAll`, check critical errors, `Get` by index, `writeSection` in same order. Very consistent.
- **Encapsulation preserved** -- `recordMetrics`, `writeMetrics`, `saveContextCache` remain unexported in `cache.go`. The `RegisterSubscribers` function in `subscribers.go` exposes the wiring point without leaking internals. This follows the design document's explicit decision.
- **Nil-safety throughout** -- The nil broker pattern eliminates nil-check boilerplate at every call site. Well-documented.
- **Idempotent LoadAll** -- The `loaded` flag prevents double-execution, which is a nice defensive touch not explicitly required by spec but clearly beneficial.
- **MustGet convenience** -- Added `MustGet(i)` that panics on error. Not in spec but useful for optional/non-critical loaders.
- **Test quality** -- Tests are well-structured, cover the important scenarios (construction, results, partial failure, panic recovery, goroutine bound, goroutine leak, idempotency, all event types, concurrent emit, nil safety, payload type assertion).
- **pprof implementation is clean** -- The `startProfile` function is a well-factored helper that returns a closure, keeping `main()` minimal. The heap profile correctly calls `runtime.GC()` before writing.

### Design deviations (all acceptable)

1. **Helper wrappers not created** (design section 4 suggested `mustLoadSpecs`, `gitDiffOrFallback`, `loadProjectRulesOptional`). Instead, error handling and type conversion are inlined in loader closures. This is cleaner and avoids unnecessary indirection. Acceptable deviation.

2. **`maxWorkers()` has floor clamp** (`n < 1 -> 1`) not in spec. Defensive improvement for edge cases.

3. **`LazySlice` stores `loaders` as a field** (design initially considered not storing them and using closure attachment, then revised). Implementation matches the revised design correctly.

4. **Design said `runWrite` broker should be passed from `Run()` dispatcher** (Option 1 in design section 6). Implementation uses Option 2 -- creates broker locally in `runWrite`. This is a notable deviation; see issue #1 below.

---

## 4. Issues Found

### Issue #1 (Important -- should fix): `runWrite` creates a bare broker with no subscribers

**Location**: `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go`, line 349

`runWrite` creates `broker := events.NewBroker(stderr)` directly instead of using `newBroker(stderr, verbosity)`. The `newBroker` helper calls `sddctx.RegisterSubscribers(broker, stderr, verbosity)` to wire metrics recording, stderr output, and cache persistence. The bare broker in `runWrite` has zero subscribers, so `ArtifactPromoted` and `StateAdvanced` events are emitted into a void.

This means:
- Events are structurally correct (the emit calls and payloads are there)
- But no subscriber will ever observe them
- This is functionally harmless today (no subscriber currently handles `ArtifactPromoted`/`StateAdvanced`), but violates the architectural intent of the broker pattern

The design document (section 6) explicitly chose Option 1 (pass broker from `Run()` dispatcher) over Option 2 (create locally). The implementation chose Option 2 without subscriber wiring.

**Recommendation**: Either use `newBroker(stderr, 0)` (matching `runContext`/`runNew`), or accept that `runWrite` events are currently fire-and-forget with no observers. If the latter is intentional, add a comment documenting that.

### Issue #2 (Suggestion): `runWrite` does not parse verbosity flags

**Location**: `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go`, line 324

`runContext` and `runNew` both call `ParseVerbosityFlags(args)` but `runWrite` does not. If `newBroker` is used (per Issue #1), the hardcoded verbosity of `0` would be used. Minor inconsistency.

### Issue #3 (Suggestion): `TestBroker_ConcurrentEmit` handler uses non-atomic increment in `TestBroker_SubscriberPanicRecovery`

**Location**: `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go`, line 55

In `TestBroker_SubscriberPanicRecovery`, the `order` slice is appended to without synchronization. This is safe because the broker's mutex serializes handler calls (so no concurrent access occurs), but it relies on an implementation detail. If the broker were ever changed to async dispatch, this test would have a data race.

`TestBroker_SubscribeAndEmit` (line 13) similarly uses a plain `int` counter. Again safe under current synchronous dispatch but fragile against future changes. Contrast with `TestBroker_MultipleSubscribers` which correctly uses `atomic.Int32`.

**Recommendation**: Low priority. The serialized dispatch is a core invariant (REQ-EVENTS-004), so these tests are correct. No action needed unless dispatch model changes.

### Issue #4 (Suggestion): No integration tests for event emission from `Assemble()`

The spec defines scenarios for `Assemble` emitting `CacheHit`/`CacheMiss`/`PhaseAssembled` events (REQ-EVENTS-006), but no integration tests verify this end-to-end. The existing `context_test.go` tests pass (backward compatible) but don't subscribe to the broker and assert events were emitted. Unit tests for the broker itself are thorough, but there's a gap in verifying the wiring.

**Recommendation**: Consider adding a test that creates a broker with a recording subscriber, calls `Assemble()`, and asserts the expected events. Not blocking.

---

## 5. Summary

| Category | Count |
|----------|-------|
| Requirements total | 15 |
| Requirements PASS | 14 |
| Requirements PARTIAL | 1 (REQ-CSYNC-005: env lacks race detector) |
| Requirements FAIL | 0 |
| Blocking issues | 0 |
| Important issues | 1 (#1: bare broker in runWrite) |
| Suggestions | 3 |

---

## 6. Recommendation

**PASS**. The implementation is faithful to the spec and design across all three domains. All tests pass. The code is well-structured, consistent, and maintains backward compatibility. The one important issue (bare broker in `runWrite`) is functionally harmless today but should be addressed to complete the architectural intent. The race detector verification should be run in a CI environment with CGO support.
