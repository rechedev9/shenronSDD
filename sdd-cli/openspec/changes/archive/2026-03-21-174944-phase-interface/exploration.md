# Exploration: phase-interface

**Change:** phase-interface
**Goal:** Define a Phase interface that makes phases pluggable, self-documenting, and testable. Custom phases can be added without modifying the core state machine.

---

## Current State

The SDD pipeline consists of 10 phases: `explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `review`, `verify`, `clean`, `archive`. Each phase has an assembler function — a free function that takes `(io.Writer, *Params) error` and writes assembled context for an AI sub-agent to consume.

The assembler type alias is already defined:

```go
// internal/context/context.go
type Assembler func(w io.Writer, p *Params) error
```

Dispatch is via a package-level `map[state.Phase]Assembler`:

```go
var dispatchers = map[state.Phase]Assembler{
    state.PhaseExplore: AssembleExplore,
    state.PhasePropose: AssemblePropose,
    // ... 8 total
}
```

Phase metadata (prerequisites, next-phases, artifact filenames) lives in three separate, parallel data structures across two packages:

1. `internal/state/state.go` — `validNextPhases`, `prerequisites` (both `map[Phase][]Phase`)
2. `internal/artifacts/artifacts.go` — `ArtifactFileName` (`map[state.Phase]string`)
3. `internal/context/cache.go` — `phaseInputs` (`map[string][]string`), `phaseTTL` (`map[string]time.Duration`)

There is no single authoritative description of a phase. All phase knowledge is scattered across 4 package-level maps.

---

## Relevant Files

| File | Role | Impact |
|------|------|--------|
| `internal/context/context.go` | Defines `Assembler` type alias, `Params` struct, `dispatchers` map, `Assemble()`, `AssembleConcurrent()` | Core of refactor |
| `internal/context/explore.go` | `AssembleExplore` — loads skill + git file tree + manifests | Per-phase assembler |
| `internal/context/propose.go` | `AssemblePropose` — requires `exploration.md` | Per-phase assembler |
| `internal/context/spec.go` | `AssembleSpec` — requires `proposal.md` | Per-phase assembler |
| `internal/context/design.go` | `AssembleDesign` — requires `proposal.md` + `specs/` | Per-phase assembler |
| `internal/context/tasks.go` | `AssembleTasks` — requires `design.md` + `specs/` | Per-phase assembler |
| `internal/context/apply.go` | `AssembleApply` — requires `tasks.md`, `design.md`, `specs/`; extracts current task | Per-phase assembler |
| `internal/context/review.go` | `AssembleReview` — loads git diff, optional project rules | Per-phase assembler |
| `internal/context/clean.go` | `AssembleClean` — requires `verify-report.md` + `tasks.md` | Per-phase assembler |
| `internal/context/cache.go` | `phaseInputs`, `phaseTTL`, cache logic | Must be co-located with phase def |
| `internal/context/subscribers.go` | Wires event broker for metrics, cache persistence | Cross-cutting |
| `internal/state/types.go` | Defines `Phase` string type, `AllPhases()` ordered slice | State machine core |
| `internal/state/state.go` | `prerequisites`, `validNextPhases`, `CanTransition`, `Advance`, `Recover` | State machine transitions |
| `internal/state/resolve.go` | `ResolvePhase` — name/index/prefix resolution | CLI input parsing |
| `internal/artifacts/artifacts.go` | `ArtifactFileName` map, `PendingFileName()` | Artifact name registry |
| `internal/cli/commands.go` | `runContext`, `runWrite`, `runNew` — CLI dispatch to assemblers | Consumer of Phase |
| `internal/context/context_test.go` | Per-assembler unit tests, fixture helper `setupFixture` | Test coverage |

---

## Dependency Map

```
state.Phase (string type)
  ├── state.AllPhases() — ordered slice
  ├── state.prerequisites — map[Phase][]Phase (in state.go)
  ├── state.validNextPhases — map[Phase][]Phase (in state.go)
  ├── artifacts.ArtifactFileName — map[Phase]string
  ├── context.dispatchers — map[Phase]Assembler
  ├── context.phaseInputs — map[string][]string  (uses string, not Phase!)
  └── context.phaseTTL — map[string]time.Duration (uses string, not Phase!)

context.Params (inputs to every assembler)
  ├── ChangeDir string
  ├── ChangeName string
  ├── Description string
  ├── ProjectDir string
  ├── Config *config.Config
  ├── SkillsPath string
  ├── Stderr io.Writer
  ├── Verbosity int
  └── Broker *events.Broker

context.Assembler = func(io.Writer, *Params) error
  ← implemented by 8 AssembleXxx free functions

cli.runContext
  → state.ReadyPhases()    — from State
  → context.Assemble()     — dispatches to assembler
  → context.AssembleConcurrent() — parallel spec+design

cli.runWrite
  → artifacts.Promote()    — moves .pending file to canonical name
  → state.Advance()        — marks phase completed
```

Phases `spec` and `design` are the only ones that can run concurrently (both require `propose` completed, neither requires the other).

---

## Data Flow

### Context assembly (sdd context <name> [phase])

```
CLI args
  → state.ResolvePhase(input)
  → state.ReadyPhases() or explicit phase
  → context.Assemble(stdout, phase, params)
      → tryCachedContext(changeDir, phase, skillsPath) → cache hit? write & return
      → dispatchers[phase](buf, params)
          → csync.NewLazySlice([loaders...]).LoadAll()
              → loadSkill(skillsPath, "sdd-<phase>") — SKILL.md
              → loadArtifact(changeDir, "<artifact>") — required artifacts
              → [optional loaders: git diff, file tree, project rules]
          → writeSection(w, "SKILL", ...) + per-phase sections
      → size guard (> 100KB = error)
      → write buf to stdout
      → emit PhaseAssembled event
          → saveContextCache (subscriber)
          → recordMetrics (subscriber)
          → writeMetrics to stderr (subscriber)
```

### Phase advancement (sdd write <name> <phase>)

```
CLI args
  → artifacts.Promote(changeDir, phase) — .pending/<phase>.md → <canonical>
  → state.Advance(phase)
      → state.CanTransition(phase) — check prerequisites
      → s.Phases[phase] = StatusCompleted
      → s.CurrentPhase = s.nextReady()
  → state.Save(st, statePath)
```

### What each assembler reads

| Phase | Required Artifacts | Optional |
|-------|-------------------|----------|
| explore | — | git file tree, manifests |
| propose | `exploration.md` | git file tree |
| spec | `proposal.md` | buildSummary |
| design | `proposal.md`, `specs/` | buildSummary |
| tasks | `design.md`, `specs/` | — |
| apply | `tasks.md`, `design.md`, `specs/` | buildSummary |
| review | `specs/`, `design.md`, `tasks.md` | git diff, project rules |
| clean | `verify-report.md`, `tasks.md` | buildSummary, `design.md`, `specs/` |

Phases `verify` and `archive` have **no assembler** — they are handled outside the context assembly path (`runVerify` runs build/test commands directly; `runArchive` calls `verify.Archive()`).

---

## Risk Assessment

### Low risk
- The `Assembler` type alias already exists (`func(io.Writer, *Params) error`) — wrapping it in an interface is mechanical.
- All assembler functions already have identical signatures.
- Tests call assemblers directly (`AssembleExplore(&buf, p)`) — they would still work after refactoring.

### Medium risk
- **Four separate maps must stay in sync.** `prerequisites`, `ArtifactFileName`, `phaseInputs`, and `phaseTTL` are currently maintained independently. A Phase interface that consolidates them eliminates drift — but moving `phaseInputs` out of `cache.go` touches the internal caching logic that references it directly.
- **`phaseInputs` and `phaseTTL` use `string` keys, not `state.Phase`.** Any interface defined in `internal/state` or `internal/context` must handle this type mismatch.
- **`Recover()` in `state.go` encodes artifact→phase mapping independently** of `ArtifactFileName`. A Phase interface with `ArtifactFile() string` would let `Recover()` be driven from the registry.
- **`AllPhases()` returns a hardcoded ordered slice.** If custom phases are inserted, ordering must be deterministic (prerequisite satisfaction in `nextReady()`).

### High risk
- **Cross-package interface placement.** A `Phase` interface needs to be visible to `state`, `context`, `artifacts`, and `cli`. If defined in `internal/state`, `context` would import `state` (already does) — safe. If defined in a new `internal/phase` package, all four packages import it.
- **No assembler for `verify` and `archive`.** Any interface must accommodate phases that don't participate in context assembly. Options: nil assembler field, or a separate optional interface (`Assembler`).
- **`AssembleConcurrent` assumes `spec`+`design` can run in parallel** because both have `propose` as their sole prerequisite. If custom phases are added with the same pattern, `ReadyPhases()` already handles the general case — `AssembleConcurrent` would need no changes.

---

## Approach Comparison

### Approach A: Phase as Interface (consolidates all metadata)

Define a `Phase` interface (or struct) that carries: name, prerequisites, artifact filename, cache inputs, cache TTL, and assembler function. Register instances in a single registry slice.

```go
// internal/phase/phase.go
type Phase interface {
    Name() string
    Prerequisites() []string
    ArtifactFile() string
    CacheInputs() []string
    CacheTTL() time.Duration
    Assemble(w io.Writer, p *context.Params) error
}
```

**Pros:** Single source of truth per phase. Custom phases: implement interface and register. No map synchronization. `Recover()` and `validate()` become data-driven.
**Cons:** Circular import problem — `context.Params` references `config.Config`, `events.Broker`, etc. A pure `phase` package cannot import `context` without a cycle. Would need `Params` to be defined in a neutral package (e.g., `internal/phase`), or assembler extracted as a separate optional interface.

### Approach B: Phase as Struct with Assembler Field (simpler, no interface needed)

```go
// internal/phase/phase.go
type Phase struct {
    Name          string
    Prerequisites []string
    ArtifactFile  string
    CacheInputs   []string
    CacheTTL      time.Duration
    Assemble      func(w io.Writer, p *AssemblerParams) error // nullable
}
```

Registry is `[]Phase`, accessed by name. `state.Phase` (the string type) remains unchanged; the struct is a richer descriptor.

**Pros:** No interface, no circular import. Nil `Assemble` field = non-assembling phase (verify, archive). Custom phases: append to registry. Map synchronization eliminated.
**Cons:** Not extensible via interface polymorphism. Custom phases share the same struct layout — sufficient for the stated goal.

### Approach C: Minimal — keep current maps, add registry type for documentation/testing

Add a `PhaseRegistry` struct that wraps the existing four maps with validation methods and a `RegisterCustom()` function. Core maps stay; registry validates consistency at startup.

**Pros:** Least invasive. No cross-package movement. Existing tests unchanged.
**Cons:** Does not eliminate map synchronization. Custom phases still require editing multiple maps. Doesn't achieve the "self-documenting" goal. Addresses testability partially (validator) but not pluggability.

---

## Recommendation

**Approach B** (Phase struct with assembler field) is the right fit given the actual codebase.

Rationale:
1. The codebase is Go and already uses `type Assembler func(...)` — a struct with a function field is idiomatic Go.
2. The four parallel maps (`prerequisites`, `ArtifactFileName`, `phaseInputs`, `phaseTTL`) are the primary pain point. A struct eliminates all four in one move.
3. The interface approach (A) requires moving `context.Params` to a neutral package, which is a larger refactor with more blast radius than the value justifies.
4. Custom phases need only construct a `Phase` struct and call `registry.Register(p)` — no core files modified.
5. `verify` and `archive` (no assembler) are handled naturally by a nil `Assemble` field.

The proposed refactor touches these files directly:
- **New:** `internal/phase/phase.go` — `Phase` struct + registry + `AssemblerParams` (moved from `context.Params`)
- **Modified:** `internal/state/types.go` — `AllPhases()` becomes registry-driven
- **Modified:** `internal/state/state.go` — `prerequisites`, `validNextPhases`, `Recover()` driven from registry
- **Modified:** `internal/artifacts/artifacts.go` — `ArtifactFileName` driven from registry
- **Modified:** `internal/context/context.go` — `dispatchers` + `phaseInputs` + `phaseTTL` driven from registry; `Params` → `phase.AssemblerParams`
- **Modified:** `internal/context/cache.go` — `phaseInputs`, `phaseTTL` read from registry

`internal/cli/commands.go` sees no changes — it uses `state.Phase` (the string), `context.Assemble()`, and `state.ReadyPhases()`, all of which remain.

**Scope note:** This is a moderate refactor touching 6 files. The interface boundary (`phase.Phase` struct + `phase.Registry`) is the value-add. The assembler function signatures (`func(io.Writer, *AssemblerParams) error`) do not change structurally — just their location.
