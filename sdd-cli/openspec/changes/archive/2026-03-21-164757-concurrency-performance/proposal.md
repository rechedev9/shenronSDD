# Proposal: Concurrency and Performance

**Change ID**: concurrency-performance
**Date**: 2026-03-21
**Status**: draft

---

## Intent

The sdd-cli assembles context per-phase by sequentially reading 3-10+ files and running git commands, tightly couples side effects (metrics, caching, stderr output) into the assembly hot path, and has no profiling infrastructure. This change introduces lazy concurrent artifact loading, an event broker to decouple side effects, and an env-var-gated pprof profiling hook — improving assembly latency, fixing a metrics.json race in `AssembleConcurrent`, and enabling data-driven performance work.

## Scope

### In Scope
- New `internal/csync` package with `LazySlice[T]` generic type (bounded goroutine pool, per-element `sync.Once`)
- Refactor all 8 assembler functions to use `LazySlice` for concurrent file/exec loading
- New `internal/events` package with `Broker` type and typed event structs
- Migrate `recordMetrics()`, `writeMetrics()`, and `saveContextCache()` from inline calls to event subscribers
- Emit events from `Assemble()` in `context.go` and `runWrite()` in `commands.go`
- `SDD_PPROF` env var check in `cmd/sdd/main.go` for CPU and heap profiling to file
- Add pprof availability check to `sdd doctor`
- Unit tests for `LazySlice` (with `-race`), `Broker`, and pprof gating

### Out of Scope
- HTTP-based pprof server — no HTTP server exists in the CLI; `runtime/pprof` write-to-file is sufficient
- Replacing `AssembleConcurrent` with a generic pipeline scheduler — current WaitGroup pattern is adequate for the spec+design parallel window
- Making `LazySlice` concurrency bound configurable via env var — hardcoded `min(NumCPU, 8)` is sufficient; can revisit if CI tuning is needed
- External dependencies — all implementations use stdlib only
- Tracing or OpenTelemetry integration — event broker provides the hook point; actual telemetry is future work

## Approach

Implementation proceeds in three stages ordered by ascending risk and descending immediacy of value. First, the pprof gate: a ~15-line addition to `cmd/sdd/main.go` that checks `SDD_PPROF` and wraps `cli.Run()` with `runtime/pprof` start/stop. This gives immediate profiling capability for validating the subsequent work.

Second, `csync.LazySlice[T]`: a new generic type that wraps `[]func() (T, error)` with a bounded goroutine pool. Each assembler (explore, propose, spec, design, tasks, apply, review, clean) is refactored to construct loader functions for its I/O calls (file reads via `loadSkill`/`loadArtifact`/`loadSpecs`, git exec via `gitFileTree`/`gitDiff`, summary building via `buildSummary`), then call `LoadAll()` to fan out concurrently. The assembler body then accesses results by index. This is a mechanical refactor — each assembler's sequential calls become loader closures.

Third, the event broker: a simple observer pattern where `Assemble()` (context.go:55-96) emits `PhaseAssembled`/`CacheHit`/`CacheMiss` events and `runWrite()` (commands.go:312-371) emits `ArtifactPromoted`/`StateAdvanced` events. The current inline `recordMetrics()` (cache.go:274-313) and `writeMetrics()` (cache.go:230-246) calls become subscribers registered at startup. This fixes the metrics.json race in `AssembleConcurrent` by routing all writes through a single serialized subscriber, and decouples the assembler from knowledge of caching and metrics concerns.

### Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Lazy loading strategy | LazySlice per-assembler (not pre-fetch in `Assemble()` or `fs.FS`) | Handles git exec and `buildSummary` calls, not just file reads. Composable generic primitive reusable elsewhere. Pre-fetch misses git commands; `fs.FS` is over-engineered. |
| Event dispatch model | In-process pub/sub broker (not callbacks on State, not channels) | Clean separation of concerns, fixes metrics.json race via serialized subscriber, testable with mock listeners. Callbacks don't fix concurrency; channels add lifecycle complexity. |
| Profiling gate | `SDD_PPROF` env var in `main.go` (not CLI flag, not build tag) | Dev-only diagnostic — env var is zero-cost when off, follows Go idioms (`GODEBUG`, `GORACE`). CLI flag pollutes user-facing help. Build tag requires recompilation. |
| Implementation order | pprof -> LazySlice -> event broker | Ascending risk. pprof provides profiling for validating the other two. LazySlice is independent of the broker. Broker depends on understanding the side-effect call sites that LazySlice may shift. |
| Goroutine bound | `min(runtime.NumCPU(), 8)` hardcoded | Typical assembler loads 3-10 files; 8 goroutines saturates local I/O. Avoids config complexity. |
| Broker injection | Via `Params` struct field (not global singleton) | Testable, no global state. The field is set once in `runContext`/`runNew` and passed through existing `Params` plumbing. Nil-safe — nil broker means no event dispatch. |

## Affected Areas

| Module / Area | File Path | Change Type | Risk Level |
|---------------|-----------|-------------|------------|
| Entry point | `/home/reche/projects/SDDworkflow/sdd-cli/cmd/sdd/main.go` | Modified — pprof gate | Low |
| Concurrency primitives | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice.go` | New — `LazySlice[T]` type | Medium |
| Concurrency tests | `/home/reche/projects/SDDworkflow/sdd-cli/internal/csync/lazyslice_test.go` | New — race-detector tests | Low |
| Event broker | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker.go` | New — `Broker`, event types | Medium |
| Event broker tests | `/home/reche/projects/SDDworkflow/sdd-cli/internal/events/broker_test.go` | New — dispatch/subscribe tests | Low |
| Assembler dispatcher | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go` | Modified — emit events from `Assemble()`, add `Broker` to `Params` | Medium |
| Cache/metrics | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/cache.go` | Modified — extract `recordMetrics`/`writeMetrics`/`saveContextCache` into event subscribers | Medium |
| Explore assembler | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/explore.go` | Modified — LazySlice refactor | Low |
| Propose assembler | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/propose.go` | Modified — LazySlice refactor | Low |
| Spec assembler | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/spec.go` | Modified — LazySlice refactor | Low |
| Design assembler | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/design.go` | Modified — LazySlice refactor | Low |
| Tasks assembler | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/tasks.go` | Modified — LazySlice refactor | Low |
| Apply assembler | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/apply.go` | Modified — LazySlice refactor | Low |
| Review assembler | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/review.go` | Modified — LazySlice refactor | Low |
| Clean assembler | `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/clean.go` | Modified — LazySlice refactor | Low |
| CLI commands | `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go` | Modified — emit events from `runWrite()`, wire broker into `Params` | Medium |
| Doctor checks | `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/doctor.go` | Modified — add pprof env var check | Low |

**Total files affected**: 18
**New files**: 4
**Modified files**: 14
**Deleted files**: 0

## Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| LazySlice goroutine leak on assembler error | Low | Medium | `LoadAll()` waits on all goroutines via `sync.WaitGroup` before returning, regardless of errors. Unit test with intentional errors + race detector. |
| Event broker subscriber panic crashes CLI | Low | High | Broker wraps each subscriber dispatch in `defer recover()`. Subscriber panics logged to stderr but do not propagate to caller. |
| Assembler refactor changes output byte order | Medium | High | Each assembler's `writeSection` calls remain identical — only the I/O scheduling changes (results accessed by index, not arrival order). Existing `context_test.go` tests (541 lines) validate output content. Full test suite with `-race` before merge. |
| `Params.Broker` nil dereference in tests | Medium | Low | `Assemble()` nil-checks broker before emitting. Tests that don't set a broker continue to work without events. |
| Metrics.json data loss during migration | Low | Low | Metrics are best-effort and regenerated on next assembly. No user-visible data depends on metrics.json continuity. |
| pprof profile file left in CWD | Low | Low | Profile files (`sdd-cpu.prof`, `sdd-mem.prof`) only created when `SDD_PPROF` is explicitly set. Documented in `sdd doctor` output. |

**Overall Risk Level**: medium

## Rollback Plan

### Steps to Rollback
1. Revert the merge commit — all three features are additive with no schema or state format changes
2. The `internal/csync` and `internal/events` packages are self-contained; removing them has no cascade effects
3. Assemblers revert to sequential `loadSkill`/`loadArtifact` calls (the pre-change pattern)
4. `Assemble()` and `runWrite()` revert to inline `recordMetrics`/`saveContextCache` calls
5. `main.go` pprof block is removed (single `if` block)

### Rollback Verification
- `go test ./... -race` passes (confirms no residual concurrency references)
- `go build ./cmd/sdd` succeeds (confirms no import of removed packages)
- `sdd context <change> explore` produces identical output to pre-change version
- `sdd doctor` runs without errors

## Dependencies

### Internal Dependencies
- `internal/state.Phase` type used as event payload field — no changes to state package required
- `internal/context.Params` struct gains a `Broker` field — backward compatible (zero-value nil means no events)
- `internal/config.Config` unchanged

### External Dependencies
- None. All implementations use Go stdlib only (`sync`, `runtime/pprof`, `runtime`). No new external dependencies added to `go.mod`.

### Infrastructure Dependencies
- Go 1.24.1 generics support (already in use per `go.mod`)
- Race detector (`go test -race`) for CI validation of `csync` and `events` packages

## Success Criteria

- [ ] `go test ./... -race` passes with zero data races
- [ ] `SDD_PPROF=cpu sdd context <change> explore` produces `sdd-cpu.prof` loadable by `go tool pprof`
- [ ] `SDD_PPROF=mem sdd context <change> explore` produces `sdd-mem.prof`
- [ ] `sdd doctor` reports pprof availability status
- [ ] All 8 assemblers produce byte-identical output with LazySlice vs sequential loading (verified by existing context_test.go)
- [ ] `AssembleConcurrent` spec+design parallel window no longer races on metrics.json (verified via `-race` flag)
- [ ] Event broker emits `PhaseAssembled`, `CacheHit`, `CacheMiss` events observable by test subscriber
- [ ] `runWrite()` emits `ArtifactPromoted` and `StateAdvanced` events observable by test subscriber
- [ ] No new external dependencies in `go.mod`
- [ ] Benchmark: `sdd context` for review phase (highest I/O count at 5+N+rules ops) shows measurable wall-clock improvement with LazySlice vs sequential baseline

## Open Questions

- **LazySlice concurrency bound**: Should the goroutine pool be configurable via `SDD_CONCURRENCY` env var, or is `min(runtime.NumCPU(), 8)` sufficient? Current assemblers load 3-10 files; 8 goroutines likely saturates local disk I/O. Deferring configurability unless CI profiling shows contention.
- **Event broker sync vs async dispatch**: Should subscribers run synchronously in the emitter's goroutine or asynchronously in a dedicated goroutine? Current side effects (metrics write, cache save) are best-effort and fast (<1ms each), suggesting synchronous is adequate. Will decide during spec phase after profiling overhead.
- **Metrics.json race fix scope**: The current race in `AssembleConcurrent` where two goroutines both call `recordMetrics()` (cache.go:274) is a last-writer-wins data loss bug affecting the spec+design parallel window. The event broker fixes this by serializing writes. Should we also add `sync.Mutex` as belt-and-suspenders, or is the broker sufficient?
- **pprof output directory**: Profile files written to CWD matches the crash log pattern in `main.go` (line 24). Should they go to `openspec/.cache/` instead to avoid cluttering the project root?
