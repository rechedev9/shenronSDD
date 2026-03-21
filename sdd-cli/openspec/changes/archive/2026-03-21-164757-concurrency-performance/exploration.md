# Exploration: concurrency-performance

- **Date:** 2026-03-21
- **Detail Level:** deep
- **Change Name:** concurrency-performance
- **Intent:** Phase 4 concurrency and performance — lazy artifact loading via `csync.LazySlice`, pub/sub event broker to decouple state machine from side effects, pprof profiling gate behind env var.

## Current State

The sdd-cli is a Go CLI (go 1.24.1, sole dependency: `gopkg.in/yaml.v3`) that assembles structured context for each phase of a spec-driven development pipeline. The codebase is approximately 6,963 lines across all Go files (source + tests). It is single-threaded with one concurrency point (`AssembleConcurrent`), has no profiling infrastructure, and tightly couples state transitions with side effects (metrics recording, cache writes, stderr output).

### Architecture Overview

The pipeline has 10 phases: explore -> propose -> spec+design (parallel) -> tasks -> apply -> review -> verify -> clean -> archive. Each phase has a dedicated assembler function (`AssembleExplore`, `AssemblePropose`, etc.) that:

1. Reads a SKILL.md file from disk (`loadSkill`)
2. Reads 1-5 artifact files from disk (`loadArtifact`, `loadSpecs`)
3. Optionally runs shell commands (`git ls-files`, `git diff`)
4. Writes labeled sections to an `io.Writer`

The `Assemble()` dispatcher in `context.go` wraps each assembler with:
- Cache lookup (`tryCachedContext`) — reads 2 files: hash file + cached context
- Size guard check
- Cache save (`saveContextCache`) — writes 2 files atomically (hash + context)
- Metrics emission (`emitMetrics`) — writes to stderr + appends to metrics.json

The `runWrite` command in `commands.go` couples three operations sequentially: artifact promotion, state machine advance, and state persistence.

## Relevant Files

| File Path | Purpose | Lines | Complexity | Test Coverage |
|-----------|---------|-------|------------|---------------|
| `internal/context/context.go` | Core assembler dispatcher, `Assemble()`, `AssembleConcurrent()`, `loadSkill()`, `loadArtifact()` | 197 | Medium | Moderate (via context_test.go) |
| `internal/context/cache.go` | Content-hash cache, metrics recording, `tryCachedContext()`, `saveContextCache()`, `recordMetrics()` | 373 | High | Low (no direct unit tests for cache functions) |
| `internal/context/explore.go` | Explore assembler — `git ls-files` + manifest loading | 63 | Low | 1 test |
| `internal/context/propose.go` | Propose assembler — loads exploration.md, file tree | 39 | Low | 2 tests |
| `internal/context/spec.go` | Spec assembler — loads proposal.md, summary | 38 | Low | 2 tests |
| `internal/context/design.go` | Design assembler — loads proposal.md + specs/ directory | 75 | Low | 2 tests |
| `internal/context/tasks.go` | Tasks assembler — loads design.md + specs/ | 37 | Low | 2 tests |
| `internal/context/apply.go` | Apply assembler — loads tasks.md, design.md, specs/ + task extraction | 95 | Medium | 3 tests |
| `internal/context/review.go` | Review assembler — loads specs, design, tasks, git diff, project rules | 101 | Medium | 3 tests |
| `internal/context/clean.go` | Clean assembler — loads verify-report, tasks, design, specs | 52 | Low | 3 tests |
| `internal/context/summary.go` | Pipeline summary builder, `buildSummary()`, `extractDecisions()`, manifest loading | 218 | Medium | Tested via summary_test.go |
| `internal/context/context_test.go` | Test suite for all assemblers | 541 | — | — |
| `internal/state/state.go` | State machine: `Advance()`, `Save()`, `Load()`, `Recover()`, `ReadyPhases()` | 240 | Medium | High |
| `internal/state/types.go` | Phase/Status types, `NewState()`, `AllPhases()` | 87 | Low | High |
| `internal/cli/commands.go` | All CLI command handlers: `runNew`, `runContext`, `runWrite`, `runVerify`, etc. | 928 | High | Moderate |
| `internal/cli/cli.go` | Top-level dispatcher, help text | 276 | Low | Moderate |
| `internal/cli/doctor.go` | `sdd doctor` diagnostic checks | 227 | Medium | Low |
| `cmd/sdd/main.go` | Binary entry point, panic recovery | 36 | Low | None |
| `internal/artifacts/promote.go` | Artifact promotion (`.pending/` -> final location) | 55 | Low | High |
| `internal/artifacts/list.go` | Artifact listing for dump command | 90 | Low | Moderate |
| `internal/artifacts/reader.go` | Artifact file reading | 34 | Low | Via integration |
| `internal/verify/verify.go` | Build/lint/test quality gate execution | 221 | Medium | High |
| `internal/config/types.go` | Config struct definitions | 35 | Low | Via config tests |

## Dependency Map

```
cmd/sdd/main.go
  └─ internal/cli
       ├─ internal/cli/errs
       ├─ internal/config
       ├─ internal/context
       │    ├─ internal/config (Config struct)
       │    └─ internal/state (Phase type)
       ├─ internal/state
       ├─ internal/artifacts
       │    └─ internal/state (Phase type)
       └─ internal/verify

External: gopkg.in/yaml.v3 (config loading only)
stdlib: sync (WaitGroup in AssembleConcurrent), crypto/sha256, os/exec, encoding/json
```

**Key observation:** The project has **zero external runtime dependencies** beyond yaml.v3. Any new concurrency primitive (`csync.LazySlice`) must be implemented in-tree as an `internal/csync` package.

## Data Flow

### 1. Artifact Loading Flow (current)

```
Assembler called
  ├─ loadSkill(skillsPath, phaseName)        → os.ReadFile (synchronous)
  ├─ loadArtifact(changeDir, "proposal.md")  → os.ReadFile (synchronous)
  ├─ loadArtifact(changeDir, "design.md")    → os.ReadFile (synchronous)
  ├─ loadSpecs(changeDir)                    → os.ReadDir + N×os.ReadFile (synchronous, sequential)
  ├─ gitFileTree(projectDir)                 → exec.Command("git", "ls-files") (synchronous)
  ├─ gitDiff(projectDir)                     → 2× exec.Command("git", "diff") (synchronous, sequential)
  ├─ buildSummary(changeDir, p)              → up to 4× os.ReadFile (synchronous, sequential)
  └─ loadManifestContents(projectDir, ...)   → N× os.ReadFile (synchronous, sequential)
```

**Per-assembler I/O call counts:**

| Assembler | loadSkill | loadArtifact | loadSpecs | git exec | buildSummary | Total I/O ops |
|-----------|-----------|-------------|-----------|----------|-------------|---------------|
| explore   | 1 | 0 | 0 | 1 (ls-files) | 0 | 2 + manifests |
| propose   | 1 | 1 | 0 | 1 (ls-files) | 0 | 3 |
| spec      | 1 | 1 | 0 | 0 | 1 (up to 4 reads) | 2-6 |
| design    | 1 | 1 | 1 (N reads) | 0 | 1 (up to 4 reads) | 3-9+N |
| tasks     | 1 | 1 | 1 (N reads) | 0 | 0 | 2+N |
| apply     | 1 | 2 | 1 (N reads) | 0 | 1 (up to 4 reads) | 4-8+N |
| review    | 1 | 2 | 1 (N reads) | 2 (diff) | 0 | 5+N+rules |
| clean     | 1 | 2 | 1 (N reads) | 0 | 1 (up to 4 reads) | 4-10+N |

**Lazy loading opportunity:** Most assemblers load 3-10+ files sequentially. The I/O calls are independent — skill, artifacts, and git commands could load concurrently.

### 2. State Machine Side Effects Flow (current)

```
cli.runWrite()
  ├─ artifacts.Promote(changeDir, phase)   → os.ReadFile + os.WriteFile + os.Remove
  ├─ state.Advance(completed)              → mutates in-memory state
  └─ state.Save(st, statePath)             → json.MarshalIndent + atomic write

cli.Assemble()  (via runContext / runNew)
  ├─ tryCachedContext()                    → 2× os.ReadFile (hash + cached context)
  ├─ fn(&buf, p)                           → assembler (I/O described above)
  ├─ saveContextCache()                    → 2× atomic write (hash + context)
  └─ emitMetrics()
       ├─ recordMetrics()                  → read metrics.json + recompute + atomic write
       └─ writeMetrics()                   → fmt.Fprintf to stderr
```

**Coupled side effects in `Assemble()`:**
1. Cache read (pre-check)
2. Assembler execution (I/O-bound)
3. Cache write (post-assembly)
4. Metrics recording (read-modify-write metrics.json)
5. Metrics display (stderr write)

**Coupled side effects in `runWrite()`:**
1. Artifact promotion (file move)
2. State advance (in-memory mutation)
3. State save (file write)

### 3. Current Concurrency Model

`AssembleConcurrent` (context.go:121-164):
- Spawns one goroutine per phase using `sync.WaitGroup`
- Each goroutine calls full `Assemble()` (including cache + metrics)
- Results collected in indexed slice for deterministic output ordering
- No semaphore bounding — unbounded goroutine creation
- Partial output: errors are collected but successful phases still emit output
- **Currently only used for spec+design parallel window** (max 2 goroutines)

**Limitations:**
- No bounded concurrency (fine for 2, problematic if expanded)
- Each goroutine shares `*Params` (read-only, safe) but `recordMetrics()` has a read-modify-write race on `metrics.json` (both goroutines read/recompute/write simultaneously). Currently benign because file writes are atomic renames, but the final state may lose one phase's metrics.
- No cancellation support — if one assembler fails, others continue to completion
- No within-assembler concurrency (all file loads are sequential within a single assembler)

### 4. Entry Points for pprof

The sole binary entry point is `cmd/sdd/main.go:main()`. It calls `cli.Run()` which dispatches to subcommands. There is no HTTP server — pprof would need to be CPU/memory profile-to-file (not the HTTP handler approach). The `runtime/pprof` package is the appropriate tool.

The panic recovery `defer` in main.go is the natural insertion point for pprof start/stop.

## Risk Assessment

| Dimension | Level | Notes |
|-----------|-------|-------|
| Scope | **Medium** | 3 independent features, touching context/, state/, cli/, and new csync/ package |
| Complexity | **Medium** | LazySlice is a well-understood pattern (sync.Once per element). Pub/sub adds an abstraction layer. pprof is stdlib. |
| Backwards Compatibility | **Low risk** | All features are additive. LazySlice replaces sequential reads with concurrent ones (same output). Pub/sub decouples existing side effects (same behavior). pprof is opt-in via env var. |
| Concurrency Safety | **Medium** | LazySlice must be goroutine-safe. Pub/sub must handle subscriber panics. Current metrics.json race should be fixed. |
| Testing | **Medium** | LazySlice needs unit tests with race detector. Pub/sub needs event ordering tests. pprof needs env var gating test. |
| Performance | **Low risk** | All changes aim to improve performance. LazySlice adds goroutine overhead but I/O parallelism should dominate. Risk of regression is minimal for typical workloads (2-10 files per assembler). |
| Dependency | **None** | All implementations use stdlib only. No new external deps. |

## Approach Comparison

### Feature 1: Lazy Artifact Loading (`csync.LazySlice`)

| Dimension | A: LazySlice per-assembler | B: Pre-fetch all artifacts in Assemble() | C: io/fs.FS with caching layer |
|-----------|---------------------------|----------------------------------------|-------------------------------|
| **Description** | New `internal/csync.LazySlice[T]` type wraps `[]func() (T, error)` with `sync.Once` per element. Each assembler constructs a LazySlice of its I/O loads, calls `LoadAll()` to fan out, then accesses results. | Before calling `fn(&buf, p)`, the `Assemble()` dispatcher pre-reads all `phaseInputs[phase]` files concurrently into a `map[string][]byte` passed via `Params`. | Replace `os.ReadFile` calls with an `fs.FS` implementation that caches file contents and supports concurrent reads. |
| **Invasiveness** | Medium — each assembler refactored to use LazySlice. ~8 assembler files touched. New `internal/csync/` package. | Low — only `context.go` and `Params` struct change. Assemblers get data from `Params.PreloadedArtifacts`. | High — requires replacing all `os.ReadFile` with `fs.Open`, new FS implementation, changes to all call sites. |
| **Concurrency model** | Per-element `sync.Once` + bounded goroutine pool. Fine-grained: each file loaded in its own goroutine. | Single fan-out before assembler runs. Coarser-grained but simpler. | Lazy per-file caching with `sync.Map` or similar. |
| **Reusability** | `csync.LazySlice` is a general-purpose primitive usable elsewhere. | Single-purpose. | Overly general for this use case. |
| **Handles git exec?** | Yes — loader functions can wrap exec.Command calls. | No — only handles file reads from `phaseInputs`. Git commands (`ls-files`, `diff`) not in that map. | No — git exec is not a filesystem operation. |
| **Handles buildSummary?** | Yes — summary builder can use a LazySlice internally. | Partially — would need separate pre-fetch for summary artifacts. | Partially — summary reads files but also does text processing. |
| **Error handling** | Per-element errors collected; assembler decides which are fatal. | Errors in pre-fetch are opaque to assembler — harder to distinguish required vs optional artifacts. | Per-file errors via `fs.Open` return. |
| **Testing** | Unit-testable in isolation. Race detector validates safety. | Requires integration test with real file I/O. | Complex FS mock setup. |

**Recommendation: Approach A (LazySlice per-assembler)**

Rationale: Most general-purpose, handles git exec and buildSummary, composable primitive. The per-assembler refactor is mechanical. Approach B misses git commands and buildSummary, and Approach C is over-engineered for the problem.

### Feature 2: Pub/Sub Event Broker

| Dimension | A: In-process event bus | B: Callback hooks on State | C: Channel-based pipeline |
|-----------|------------------------|---------------------------|--------------------------|
| **Description** | New `internal/events` package with `Broker` type. Events: `PhaseCompleted`, `CacheHit`, `CacheMiss`, `MetricsRecorded`, `ArtifactPromoted`. Current side effects become subscribers. | Add `OnAdvance`, `OnSave` callback slices to `State` struct. Side effects register as callbacks. | Use Go channels to pipe events from producers (Assemble, Advance) to consumers (metrics, cache, logging). |
| **Decoupled concerns** | Full — emitters don't know about subscribers. | Partial — State knows about callbacks but not their implementations. | Full — producers and consumers connected only by channel type. |
| **Current side effects to decouple** | 1. `recordMetrics()` in cache.go (read-modify-write metrics.json)<br>2. `writeMetrics()` (stderr output)<br>3. `saveContextCache()` (cache write)<br>4. State persistence in `runWrite` | Same set but attached to State type | Same set |
| **Invasiveness** | Medium — new package, ~5 event types, ~3 subscriber registrations. `Assemble()` emits events. `runWrite()` emits events. | Low — add fields to State, call hooks in Advance/Save. | Medium-High — channels require lifecycle management (close, drain). |
| **Goroutine safety** | Broker handles dispatch (sync or async). Subscriber panics recoverable. | Callbacks run synchronously in caller's goroutine. | Channels are inherently goroutine-safe. |
| **Testability** | High — subscribe test-only listeners, assert events emitted. | Medium — test callbacks invoked in correct order. | Medium — need to drain channels in tests. |
| **Complexity** | Low-Medium — simple observer pattern. | Low — just function slices. | Medium — channel lifecycle, select loops, shutdown. |
| **Fixes metrics.json race?** | Yes — single subscriber serializes writes. | No — callbacks still run concurrently in AssembleConcurrent. | Yes — single consumer goroutine. |

**Recommendation: Approach A (In-process event bus)**

Rationale: Clean separation, testable, fixes the metrics.json race, and future-extensible (logging, telemetry, plugin hooks). Approach B is simpler but doesn't fix the concurrency issues and couples callbacks to the State type. Approach C adds unnecessary channel lifecycle complexity for what are simple fire-and-forget events.

### Feature 3: pprof Profiling Gate

| Dimension | A: Env var in main.go | B: CLI flag `--profile` | C: Build tag `pprof` |
|-----------|----------------------|------------------------|---------------------|
| **Description** | Check `SDD_PPROF=1` env var in `main()`. If set, start CPU profiling to `sdd-cpu.prof`, defer `pprof.StopCPUProfile()`. Optional: `SDD_PPROF=mem` for heap profile on exit. | Add `--profile` global flag parsed before subcommand dispatch. | Use `//go:build pprof` to conditionally include profiling init code. Requires separate binary build. |
| **Discoverability** | Low (env var) — but appropriate for dev-only tooling. Document in `sdd doctor`. | Medium — visible in `--help`. But pollutes user-facing CLI. | Very low — requires knowing to build with tag. |
| **Zero-cost when off** | Yes — single `os.Getenv` check (nanoseconds). | Yes — flag parsing adds trivial overhead. | Yes — code not compiled in. |
| **Binary size impact** | None — `runtime/pprof` is already linked (used by `testing` package in tests). | None. | Slightly smaller without tag, but negligible. |
| **User experience** | `SDD_PPROF=1 sdd context my-change` → profile saved. Clean. | `sdd context my-change --profile` — flag position matters, subcommand parsing more complex. | `go build -tags pprof` — requires recompilation. |
| **Implementation effort** | ~15 lines in main.go. | ~30 lines — need to extract flag before subcommand dispatch. | ~25 lines + build tag plumbing. |
| **Memory profiling** | Easy: `SDD_PPROF=mem` or `SDD_PPROF=all`. | Easy: `--profile=cpu,mem`. | Easy but requires rebuild to switch. |

**Recommendation: Approach A (Env var in main.go)**

Rationale: Lowest friction, zero-cost when off, appropriate for developer-facing profiling. pprof is not a user feature — it's a developer diagnostic. Env vars are the standard Go idiom for this (see `GODEBUG`, `GORACE`, `GOGC`). Implementation is trivial.

## Recommendation

### Implementation Order

1. **pprof gate** (lowest risk, immediate value, ~15 LOC)
   - Add `SDD_PPROF` env var check in `cmd/sdd/main.go`
   - Support values: `cpu` (CPU profile), `mem` (heap profile), `all` (both)
   - Output files: `sdd-cpu.prof`, `sdd-mem.prof`
   - Add pprof check to `sdd doctor`

2. **csync.LazySlice** (medium risk, highest performance impact)
   - New `internal/csync/` package with `LazySlice[T]` type
   - Requires Go 1.24 generics (available)
   - Refactor each assembler to construct loader functions, then `LoadAll()`
   - Bounded goroutine pool (default: `runtime.NumCPU()`, capped at 8)
   - Unit tests with `-race` flag

3. **Event broker** (medium risk, architectural improvement)
   - New `internal/events/` package with `Broker`, `Event` types
   - Event types: `PhaseAssembled`, `CacheHit`, `CacheMiss`, `ArtifactPromoted`, `StateAdvanced`
   - Migrate `recordMetrics()` to async subscriber (fixes metrics.json race)
   - Migrate `writeMetrics()` to subscriber
   - Migrate `saveContextCache()` to subscriber (post-assembly cache write)
   - Wire broker into `Params` struct, emit in `Assemble()` and `runWrite()`

### Estimated Scope

| Feature | New Files | Modified Files | New LOC (est.) | Modified LOC (est.) |
|---------|-----------|---------------|----------------|---------------------|
| pprof gate | 0 | 1 (main.go) + 1 (doctor.go) | ~30 | ~15 |
| csync.LazySlice | 2 (lazyslice.go, lazyslice_test.go) | 8 assembler files + context.go | ~120 | ~160 |
| Event broker | 2 (broker.go, broker_test.go) | context.go, cache.go, commands.go | ~150 | ~80 |
| **Total** | **4-6** | **~12** | **~300** | **~255** |

## Clarification Required (BLOCKING)

None. All three features are well-scoped with clear implementation paths. No external dependencies needed. No API changes visible to CLI users.

## Open Questions (DEFERRED)

1. **LazySlice concurrency bound:** Should the goroutine pool be configurable via `SDD_CONCURRENCY` env var, or hardcoded to `min(runtime.NumCPU(), 8)`? The default is sufficient but power users might want to tune for CI environments with constrained I/O.

2. **Event broker synchronous vs asynchronous dispatch:** Should the broker dispatch events synchronously (simpler, deterministic ordering) or asynchronously (non-blocking for performance-critical paths like `Assemble`)? Current side effects are all best-effort, suggesting async is safe. But synchronous is simpler to reason about and test.

3. **Metrics.json race severity:** The current race in `AssembleConcurrent` where two goroutines both call `recordMetrics()` simultaneously is a data loss bug (last writer wins). It only affects the spec+design parallel window and only loses one phase's metrics. The event broker fixes this, but should we also add a file lock as a belt-and-suspenders measure?

4. **pprof output location:** Should profile files be written to CWD (current behavior of crash logs), to a temp directory, or to `openspec/.cache/`? CWD matches the crash log pattern but may clutter user's project root.

5. **LazySlice for cache operations:** Should `tryCachedContext` and `saveContextCache` also use LazySlice for their 2-file I/O patterns, or is the overhead not worth it for just 2 files?

6. **Event broker lifecycle:** Should the broker be a global singleton (simple) or injected via `Params` (testable, no global state)? The latter is cleaner but adds a field to `Params` that every assembler receives but most don't use directly.
