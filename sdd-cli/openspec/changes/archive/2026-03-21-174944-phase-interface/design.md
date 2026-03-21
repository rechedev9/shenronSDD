# Design: Phase Interface

**Change ID**: phase-interface
**Date**: 2026-03-21
**Status**: draft

---

## Overview

Replace four parallel package-level maps with a single `Phase` struct and a `Registry`. All phase metadata (prerequisites, next-phases, artifact filename, cache inputs, cache TTL, assembler function) lives in one place. Existing call sites read from the registry; the `state.Phase` string type and all public function signatures are unchanged.

---

## Package Structure

```
internal/
  phase/
    phase.go       — Phase struct, AssemblerParams, Registry type, DefaultRegistry var
    registry.go    — init() block registering all 10 built-in phases
    registry_test.go — structural invariants: count, uniqueness, acyclicity, nil fields
  state/
    types.go       — AllPhases() delegates to registry
    state.go       — prerequisites / validNextPhases / Recover() driven by registry
  artifacts/
    artifacts.go   — ArtifactFileName map deleted; ArtifactFileName() func added
  context/
    context.go     — dispatchers map deleted; Assemble() reads registry
    cache.go       — phaseInputs / phaseTTL maps deleted; reads registry
```

No new directories beyond `internal/phase/`. No changes outside `internal/`.

---

## Import Graph

```
internal/config   (no internal imports)
internal/events   (no internal imports)
internal/state    (imports: config)
internal/phase    (imports: config, events, state)  ← new package
   ↑
internal/artifacts (imports: phase, state)
internal/context   (imports: phase, state, config, events, csync)
internal/cli       (imports: state, artifacts, context, config, events — unchanged)
```

Cycle check:

- `internal/phase` imports `internal/state` (for the `state.Phase` key type) — safe; `state` does not import `phase` before this change. After: `state` imports `phase`. This creates `state → phase → state` — a cycle.

**Resolution**: `internal/phase` must NOT import `internal/state`. The `Phase` struct uses `string` for all name fields rather than `state.Phase`. The registry is keyed on plain `string` (matching `state.Phase`'s underlying type). `state.Phase` constants remain in `internal/state`; the registry accepts them as `string` at registration time via a thin conversion.

Revised import graph:

```
internal/config   (leaf)
internal/events   (leaf)
internal/phase    (imports: config, events — no internal/state import)
   ↑
internal/state    (imports: phase, config)
internal/artifacts (imports: phase, state)
internal/context   (imports: phase, state, config, events, csync)
internal/cli       (imports: state, artifacts, context, config, events — unchanged)
```

No cycles. `internal/phase` is a leaf among internal packages.

---

## Type Definitions

### `internal/phase/phase.go`

```go
package phase

import (
    "io"
    "time"

    "github.com/rechedev9/shenronSDD/sdd-cli/internal/config"
    "github.com/rechedev9/shenronSDD/sdd-cli/internal/events"
)

// AssemblerParams holds everything an assembler function needs.
// Moved here from context.Params (same fields; context.Params becomes
// a type alias or thin wrapper — see context.go changes below).
type AssemblerParams struct {
    ChangeDir   string
    ChangeName  string
    Description string
    ProjectDir  string
    Config      *config.Config
    SkillsPath  string
    Stderr      io.Writer
    Verbosity   int
    Broker      *events.Broker
}

// Assembler is the function type for per-phase context assembly.
type Assembler func(w io.Writer, p *AssemblerParams) error

// Phase is the canonical descriptor for one SDD pipeline phase.
// Name uses plain string (not state.Phase) to avoid import cycles.
type Phase struct {
    Name          string        // matches state.Phase constant values
    Prerequisites []string      // names of phases that must be completed first
    NextPhases    []string      // names of phases this phase can transition to
    ArtifactFile  string        // final artifact filename or dir; "" for verify, archive
    CacheInputs   []string      // artifact paths that invalidate the cache
    CacheTTL      time.Duration // 0 = no TTL (explore, verify, archive)
    Assemble      Assembler     // nil for verify, archive
}

// Registry holds the ordered slice of Phase descriptors.
// Order = pipeline position; used by AllPhases() and nextReady().
// Read-only after init(); safe for concurrent reads.
type Registry struct {
    phases []Phase
    sealed bool // set true on first Get(); prevents late registration
}

// Register appends a Phase to the registry.
// Panics if called after the registry is sealed (after first Get()).
// Panics if p.Name is empty or already registered.
func (r *Registry) Register(p Phase)

// Get returns the Phase descriptor for the given name.
// Seals the registry on first call.
// Returns (Phase{}, false) if not found.
func (r *Registry) Get(name string) (Phase, bool)

// All returns a snapshot of the ordered phase slice.
// Seals the registry.
func (r *Registry) All() []Phase

// DefaultRegistry is the package-level singleton used by all internal packages.
var DefaultRegistry = &Registry{}
```

---

## Registration Pattern

`internal/phase/registry.go` contains a single `init()` block that registers all 10 built-in phases on `DefaultRegistry`. Because `internal/phase` is imported by `internal/state`, `internal/artifacts`, and `internal/context`, Go guarantees `phase`'s `init()` runs before any of those packages' `init()` blocks and before any package-level variable initializers that call into the registry.

```go
// internal/phase/registry.go
func init() {
    for _, p := range builtinPhases {
        DefaultRegistry.Register(p)
    }
}

// builtinPhases is a var (not const) so the Assemble fields (function values)
// can be assigned. Defined in the same file as the registrations.
var builtinPhases = []Phase{
    {
        Name:          "explore",
        Prerequisites: []string{},
        NextPhases:    []string{"propose"},
        ArtifactFile:  "exploration.md",
        CacheInputs:   []string{},
        CacheTTL:      0,
        Assemble:      nil, // set by context package init — see Wiring section
    },
    // ... one entry per phase
}
```

**Assembler wiring problem**: `internal/phase` must not import `internal/context` (context imports phase — that would cycle). The `Assemble` fields cannot be set at `phase/registry.go` `init()` time.

**Resolution**: `internal/context` registers assemblers in its own `init()` via a `SetAssembler(name string, fn Assembler)` method on `Registry`:

```go
// internal/phase/phase.go — additional method
func (r *Registry) SetAssembler(name string, fn Assembler)
```

`internal/context/context.go` `init()` calls `phase.DefaultRegistry.SetAssembler("explore", AssembleExplore)` etc. for all 8 assembling phases. `SetAssembler` panics on unknown name. `verify` and `archive` are never passed to `SetAssembler`.

This means `builtinPhases` in `registry.go` can be declared with `Assemble: nil` for all phases; `context`'s `init()` fills them in before any `Assemble()` call can occur (CLI dispatch happens after all `init()` complete).

`SetAssembler` is blocked after `sealed = true` — same guard as `Register`.

---

## Before → After: Each File

### `internal/state/types.go`

**Before**: `AllPhases()` returns a hardcoded `[]Phase` literal.

**After**: `AllPhases()` delegates to the registry.

```go
// After
func AllPhases() []Phase {
    all := phase.DefaultRegistry.All()
    result := make([]Phase, len(all))
    for i, p := range all {
        result[i] = Phase(p.Name)
    }
    return result
}
```

The `state.Phase` string type and all 10 constants (`PhaseExplore` etc.) remain in `types.go` unchanged. `NewState` and `validate` are unchanged.

**New import**: `internal/phase` added to `state/types.go`.

---

### `internal/state/state.go`

**Before**: Two package-level maps:

```go
var validNextPhases = map[Phase][]Phase{ ... }
var prerequisites   = map[Phase][]Phase{ ... }
```

Both maps are referenced in `CanTransition`, `nextReady`, `ReadyPhases`.

**After**: Both maps deleted. Callers read from registry:

```go
// CanTransition — after
func (s *State) CanTransition(target Phase) error {
    desc, ok := phase.DefaultRegistry.Get(string(target))
    if !ok {
        return fmt.Errorf("unknown phase: %s", target)
    }
    if s.Phases[target] == StatusCompleted {
        return fmt.Errorf("%w: %s already completed", ErrAlreadyCompleted, target)
    }
    for _, req := range desc.Prerequisites {
        if s.Phases[Phase(req)] != StatusCompleted {
            return fmt.Errorf("%w: %s requires %s completed (currently %s)",
                ErrPrerequisitesNotMet, target, req, s.Phases[Phase(req)])
        }
    }
    return nil
}

// nextReady — after
func (s *State) nextReady() Phase {
    for _, p := range AllPhases() {
        if s.Phases[p] != StatusPending {
            continue
        }
        desc, _ := phase.DefaultRegistry.Get(string(p))
        ready := true
        for _, req := range desc.Prerequisites {
            if s.Phases[Phase(req)] != StatusCompleted {
                ready = false
                break
            }
        }
        if ready {
            return p
        }
    }
    return ""
}
```

`ReadyPhases` follows the same pattern as `nextReady`.

**`Recover()` — before**: Contains an inline `map[Phase]string` with 7 entries (excludes apply, clean, archive — phases with no unique recovery artifact).

**`Recover()` — after**: Iterates registry `All()`, reads `ArtifactFile` field, skips phases with empty `ArtifactFile`. The existing special-case for directories (specs/) is preserved: `state.Recover` checks `os.Stat` and if `IsDir()` verifies non-empty entries. No behavior change.

```go
// Recover — after
func Recover(name, description, changeDir string) *State {
    s := NewState(name, description)
    for _, desc := range phase.DefaultRegistry.All() {
        if desc.ArtifactFile == "" {
            continue // verify, archive — no canonical recovery artifact
        }
        ph := Phase(desc.Name)
        // apply maps to tasks.md (same file as tasks) — skip to avoid
        // incorrectly marking apply completed on tasks artifact alone.
        // apply.RecoverSkip field (bool) gates this — see Phase struct note.
        if desc.RecoverSkip {
            continue
        }
        path := filepath.Join(changeDir, desc.ArtifactFile)
        info, err := os.Stat(path)
        if err != nil {
            continue
        }
        if info.IsDir() {
            entries, _ := os.ReadDir(path)
            if len(entries) == 0 {
                continue
            }
        }
        s.Phases[ph] = StatusCompleted
    }
    s.CurrentPhase = s.nextReady()
    s.UpdatedAt = time.Now().UTC()
    return s
}
```

**`apply` special case**: `artifacts.go` maps `apply` → `"tasks.md"` (same file as `tasks`). The existing `Recover()` deliberately excludes `apply` from recovery because its artifact is indistinguishable from `tasks`. The `Phase` struct gains a `RecoverSkip bool` field to encode this. `apply`'s registration sets `RecoverSkip: true`.

The `Phase` struct updated to include this field:

```go
type Phase struct {
    Name          string
    Prerequisites []string
    NextPhases    []string
    ArtifactFile  string
    RecoverSkip   bool          // true = Recover() skips this phase
    CacheInputs   []string
    CacheTTL      time.Duration
    Assemble      Assembler
}
```

---

### `internal/artifacts/artifacts.go`

**Before**: Package-level exported map:

```go
var ArtifactFileName = map[state.Phase]string{ ... } // 10 entries
```

**After**: Map deleted. Exported function replaces it (same lookup semantics, backward-compatible call sites):

```go
// ArtifactFileName returns the canonical artifact filename for a phase.
// Returns ("", false) for phases with no artifact (verify, archive are excluded
// but actually have entries — see proposal; all 10 phases are in the registry).
func ArtifactFileName(ph state.Phase) (string, bool) {
    desc, ok := phase.DefaultRegistry.Get(string(ph))
    if !ok {
        return "", false
    }
    return desc.ArtifactFile, ok
}
```

**Call site update in `artifacts/promote.go`**: The one direct call to `ArtifactFileName[phase]` (line 23) becomes:

```go
finalName, ok := ArtifactFileName(phase)
if !ok || finalName == "" {
    return "", fmt.Errorf("no artifact mapping for phase: %s", phase)
}
```

`PendingFileName` is unchanged.

**Note on naming collision**: The exported `ArtifactFileName` var becomes an exported `ArtifactFileName` func. Go permits this (identifier can be either). All existing call sites use `ArtifactFileName[phase]` (map index) which must change to `ArtifactFileName(phase)` (function call). The compiler catches all missed sites.

---

### `internal/context/context.go`

**Before**: `dispatchers` map (8 entries) and `Assembler`/`Params` type definitions.

```go
type Assembler func(w io.Writer, p *Params) error
type Params struct { ... }
var dispatchers = map[state.Phase]Assembler{ ... }
```

**After**:

1. `Assembler` type alias removed from `context` — it now lives in `internal/phase` as `phase.Assembler`.
2. `context.Params` kept as-is (same fields). `phase.AssemblerParams` is defined with identical fields. `context.Params` becomes a type alias: `type Params = phase.AssemblerParams`. This makes the rename zero-impact at all call sites that use `*context.Params` or `context.Params{...}`.
3. `dispatchers` map deleted. `Assemble()` reads from registry.

```go
// context/context.go — after
// Params is a type alias for phase.AssemblerParams.
// All existing usage of *context.Params and context.Params{} continues to compile.
type Params = phase.AssemblerParams

// Assemble — after (key change: dispatcher lookup)
func Assemble(w io.Writer, ph state.Phase, p *Params) error {
    desc, ok := phase.DefaultRegistry.Get(string(ph))
    if !ok || desc.Assemble == nil {
        return fmt.Errorf("no assembler for phase: %s", ph)
    }
    // ... rest of function unchanged (cache check, size guard, events)
    if err := desc.Assemble(&buf, p); err != nil {
        return err
    }
    // ...
}
```

4. `init()` in `context.go` registers assemblers:

```go
func init() {
    for _, pair := range []struct {
        name string
        fn   phase.Assembler
    }{
        {"explore", AssembleExplore},
        {"propose", AssemblePropose},
        {"spec",    AssembleSpec},
        {"design",  AssembleDesign},
        {"tasks",   AssembleTasks},
        {"apply",   AssembleApply},
        {"review",  AssembleReview},
        {"clean",   AssembleClean},
    } {
        phase.DefaultRegistry.SetAssembler(pair.name, pair.fn)
    }
}
```

Individual assembler files (`explore.go`, `propose.go`, etc.) change only their function signatures from `func(w io.Writer, p *Params) error` — which now resolves to `phase.AssemblerParams` via the alias. No body changes required.

---

### `internal/context/cache.go`

**Before**: Two package-level maps, both keyed on plain `string`:

```go
var phaseTTL    = map[string]time.Duration{ ... } // 7 entries
var phaseInputs = map[string][]string{ ... }      // 8 entries
```

Referenced in `tryCachedContext`, `saveContextCache`, `CheckCacheIntegrity`.

**After**: Both maps deleted. Registry lookup replaces each access:

```go
// tryCachedContext — after (key changes only)
func tryCachedContext(changeDir, phase, skillsPath string) ([]byte, bool) {
    desc, ok := phaseDefaultRegistry().Get(phase)
    inputs := []string{}
    if ok {
        inputs = desc.CacheInputs
    }
    // ... hash computation unchanged ...
    if desc.CacheTTL > 0 {
        // TTL check — same logic, reads desc.CacheTTL
    }
    // ...
}
```

`phaseDefaultRegistry()` is a local helper returning `phase.DefaultRegistry` — avoids repeating the package-qualified name and makes test injection possible.

`saveContextCache` likewise reads `desc.CacheInputs` from registry.

`CheckCacheIntegrity` reads `desc.CacheInputs` from registry instead of `phaseInputs[phase]`.

**`explore` has no cache inputs** (empty `CacheInputs: []string{}`). The existing behavior — `tryCachedContext` passing `nil`/empty inputs, which still hashes SKILL.md — is preserved unchanged. Registry returns an empty slice; `inputHash` handles nil/empty identically.

---

## Custom Phase Registration API

Third-party or test code registers a custom phase before the first registry `Get()` call:

```go
phase.DefaultRegistry.Register(phase.Phase{
    Name:          "my-phase",
    Prerequisites: []string{"design"},
    NextPhases:    []string{"tasks"},
    ArtifactFile:  "my-phase.md",
    CacheInputs:   []string{"design.md"},
    CacheTTL:      1 * time.Hour,
    Assemble:      myAssemblerFunc,
})
```

After registration, `state.AllPhases()` includes `"my-phase"`, `context.Assemble()` can dispatch to it, and `artifacts.ArtifactFileName()` returns its artifact name.

**Ordering constraint**: Custom phases appended after built-ins appear last in `AllPhases()` and `nextReady()`. This is correct for phases that succeed built-in phases. For insertion at a specific position, the caller must register before the `DefaultRegistry` is sealed. This constraint is documented in `Registry.Register` godoc.

**`validNextPhases` / `NextPhases`**: The `NextPhases` field on the current phase and `Prerequisites` on the successor must be consistent. `Register` validates this: for each name in `p.NextPhases`, if that phase is already registered, its `Prerequisites` must include `p.Name`. Inconsistencies panic at startup. This is the same invariant the existing hardcoded maps relied on implicitly.

---

## Registry Unit Tests: `internal/phase/registry_test.go`

The test file validates structural invariants without testing assembler behavior:

```go
func TestBuiltinPhaseCount(t *testing.T)          // == 10
func TestBuiltinPhaseNamesUnique(t *testing.T)     // no duplicates
func TestPrerequisiteGraphAcyclic(t *testing.T)    // DFS cycle detection
func TestVerifyAndArchiveHaveNilAssemble(t *testing.T)
func TestVerifyAndArchiveHaveEmptyArtifactFile(t *testing.T) // for verify; archive has one
func TestAllPhasesOrderMatchesPipeline(t *testing.T) // explore first, archive last
func TestCustomPhaseRegistration(t *testing.T)     // Register + Get round-trips
func TestSealedRegistryPanicsOnRegister(t *testing.T)
```

Note on `archive`: `archive` has `ArtifactFile: "archive-manifest.md"` (matches current `ArtifactFileName` map). `archive` has nil `Assemble`. The test that checks nil assembler phases covers both `verify` and `archive`. The test checking empty `ArtifactFile` covers only `verify` (the one phase with no recovery artifact and no final artifact in the current `Recover()` map).

---

## Resolved Open Questions

**`context.Params` vs `phase.AssemblerParams`**: All fields of `context.Params` (`ChangeDir`, `ChangeName`, `Description`, `ProjectDir`, `Config`, `SkillsPath`, `Stderr`, `Verbosity`, `Broker`) are used inside assembler functions. Using a type alias (`type Params = phase.AssemblerParams`) gives zero blast radius at call sites.

**Registry mutability after startup**: `Register()` and `SetAssembler()` panic after `sealed = true`. First `Get()` or `All()` seals the registry. This prevents any late registration that could race `AssembleConcurrent`.

**`nextReady()` ordering with custom phases**: Custom phases appended after built-ins are considered last. This is correct default behavior. No position hint needed for this change; documented as a constraint.

**`Recover()` rebuild**: Yes, `Recover()` is driven from the registry in this change. The `RecoverSkip` field handles the `apply`/`tasks` artifact collision explicitly. This is cleaner than maintaining a separate inline map.

---

## Risk Summary

| Risk | Where | Mitigation |
|------|-------|------------|
| `state → phase → state` import cycle | `phase` must not import `state` | Registry uses `string`, not `state.Phase`; conversion at boundary |
| `context → phase → context` import cycle | `phase` must not import `context` | `SetAssembler` wires functions from `context.init()` after registry built |
| `ArtifactFileName` var→func rename | `artifacts/promote.go` | Single call site; compiler catches all misses |
| `apply` recovery collision with `tasks` | `state/state.go` `Recover()` | `RecoverSkip: true` on `apply` Phase; same as current exclusion |
| Registry sealed too early in tests | `registry_test.go` | Tests use a fresh `&Registry{}` per test, not `DefaultRegistry` |
| `phaseInputs["explore"]` = empty, `phaseTTL` has no "explore" key | `cache.go` | Empty slice + zero TTL both handled identically before and after |
