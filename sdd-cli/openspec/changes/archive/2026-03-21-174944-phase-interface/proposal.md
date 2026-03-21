# Proposal: Phase Interface

**Change ID**: phase-interface
**Date**: 2026-03-21
**Status**: draft

---

## Intent

Replace four parallel, unsynchronized package-level maps (`prerequisites`, `validNextPhases`, `ArtifactFileName`, `phaseInputs`/`phaseTTL`) with a single authoritative `Phase` struct and a `Registry`. Each phase becomes self-describing: it carries its own prerequisites, artifact filename, cache inputs, cache TTL, and assembler function. Custom phases can be registered at startup without touching core state machine files.

## Scope

### In Scope

- New `internal/phase` package with `Phase` struct, `Registry` type, and `AssemblerParams` (moved from `context.Params`)
- Registration of all 10 built-in phases in `internal/phase/registry.go` (8 with assemblers, 2 with nil assembler — `verify`, `archive`)
- Refactor `internal/state/state.go` — drive `prerequisites`, `validNextPhases`, `Recover()` from registry
- Refactor `internal/state/types.go` — drive `AllPhases()` ordered slice from registry
- Refactor `internal/artifacts/artifacts.go` — drive `ArtifactFileName` from registry
- Refactor `internal/context/context.go` — drive `dispatchers` map and `Params` from registry; rename `Params` → `phase.AssemblerParams`
- Refactor `internal/context/cache.go` — read `phaseInputs` and `phaseTTL` from registry instead of local maps
- Unit test: `internal/phase/registry_test.go` — validate all 10 phases are registered, prerequisite graph is acyclic, no missing artifact files

### Out of Scope

- Go interface polymorphism (`Phase` as an interface type) — struct with function field is sufficient; interface approach requires moving `AssemblerParams` to a neutral package due to circular import, not worth the blast radius
- Moving `context.Params` in its entirety — only the assembler-relevant fields become `phase.AssemblerParams`; the full `Params` struct stays in `context` if fields unrelated to assembly remain
- CLI layer changes — `internal/cli/commands.go` is unchanged; it uses `state.Phase` (string), `context.Assemble()`, `state.ReadyPhases()`, none of which change signature
- Custom phase loading from config file (YAML/TOML) — registry exposes `Register()` for programmatic use; file-based discovery is future work
- Changing `AssembleConcurrent` spec+design parallelism logic — `ReadyPhases()` already handles the general case
- Changing any on-disk artifact names or state.json schema

## Approach

All four parallel maps are eliminated in a single commit sequence. The new `phase.Phase` struct is the canonical descriptor:

```go
// internal/phase/phase.go
type Phase struct {
    Name          string
    Prerequisites []string
    NextPhases    []string
    ArtifactFile  string        // "" for verify, archive
    CacheInputs   []string
    CacheTTL      time.Duration
    Assemble      func(w io.Writer, p *AssemblerParams) error  // nil for verify, archive
}
```

A `Registry` holds `[]Phase` ordered by pipeline position. Lookup is by name (O(N), N=10 — acceptable). Packages that previously owned a map call `phase.Registry.Get(name)` and read the relevant field.

`AssemblerParams` is the sub-struct of current `context.Params` fields that assembler functions actually need. If all fields are needed, `AssemblerParams` is simply an alias. The key constraint is that `internal/phase` must not import `internal/context` (would cycle), so `AssemblerParams` is defined in `internal/phase` and `context` imports `phase`.

Import graph after refactor:

```
internal/phase   (new, no internal imports)
   ↑
internal/state   (imports phase, drops local maps)
internal/artifacts (imports phase, drops ArtifactFileName map)
internal/context   (imports phase, assemblers registered here)
internal/cli       (unchanged — imports state, context, artifacts as before)
```

### Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Struct vs interface | `Phase` struct with `func` field | No circular import problem; nil field handles verify/archive naturally; idiomatic Go for this pattern |
| Package placement | New `internal/phase` package | Must be imported by `state`, `artifacts`, and `context` without cycles; cannot live in any of them |
| `AssemblerParams` placement | Defined in `internal/phase` | Assembler func type references it; `context` assemblers import `phase` — no cycle |
| `context.Params` fate | Keep as-is; assemblers receive `*phase.AssemblerParams` | Params has fields beyond assembler use (`Verbosity`, `Broker`, `Stderr`); avoid mass rename |
| Registry ordering | `[]Phase` slice, position = pipeline order | `AllPhases()` and `nextReady()` depend on ordering; map lookup would lose this |
| nil Assemble field | Permitted for verify, archive | Cleaner than a sentinel no-op function; callers guard with `p.Assemble != nil` |
| Custom phase API | `phase.DefaultRegistry.Register(p Phase)` | One call, no core file edits; ordering constraint documented |

## Affected Areas

| Module / Area | File Path | Change Type | Risk Level |
|---------------|-----------|-------------|------------|
| Phase descriptor + registry | `internal/phase/phase.go` | New | Low |
| Built-in phase registrations | `internal/phase/registry.go` | New | Low |
| Registry unit tests | `internal/phase/registry_test.go` | New | Low |
| State machine transitions | `internal/state/state.go` | Modified — drop `prerequisites`, `validNextPhases` maps; read from registry | Medium |
| Phase type + ordering | `internal/state/types.go` | Modified — `AllPhases()` reads registry | Low |
| Artifact name registry | `internal/artifacts/artifacts.go` | Modified — drop `ArtifactFileName` map; read from registry | Low |
| Assembler dispatcher | `internal/context/context.go` | Modified — drop `dispatchers` map; read from registry; update `Params` assembler field type | Medium |
| Cache inputs + TTL | `internal/context/cache.go` | Modified — drop `phaseInputs`, `phaseTTL` maps; read from registry | Low |

**Total files affected**: 8
**New files**: 3
**Modified files**: 5
**Deleted files**: 0

## Risks

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Import cycle: `context` → `phase` → `context` | Low | High | `internal/phase` has no internal imports by design; `AssemblerParams` defined there breaks potential cycle at root |
| Registry not initialized before `state`/`context` init | Low | High | Go `init()` ordering: `phase` package `init()` runs before importers; built-in registrations in `registry.go` `init()` block |
| `Recover()` drives artifact→phase lookup from registry — mismatch if `ArtifactFile` field is empty for non-canonical phases | Low | Medium | `Recover()` skips phases with empty `ArtifactFile`; existing behavior for verify/archive unchanged |
| Parallel map access during `AssembleConcurrent` | Low | Medium | Registry is read-only after init; no synchronization needed for reads |
| `AllPhases()` ordering change breaks `nextReady()` prerequisite walk | Low | High | Registry slice position == pipeline order; existing test suite (`state_test.go`) validates transition sequences |
| Assembler function type mismatch at registration time | Medium | Low | Compile-time error; caught immediately by `go build` |

**Overall Risk Level**: low-medium

## Rollback Plan

### Steps

1. `git revert <merge-commit>` — all changes are in `internal/`; CLI and on-disk formats are unchanged
2. The `internal/phase` package is self-contained; removing it causes compile errors only in the modified files, not in CLI
3. Restore deleted map literals in `state.go`, `artifacts.go`, `context.go`, `cache.go` from revert
4. `go build ./...` — confirms no dangling imports
5. `go test ./...` — confirms behavioral parity

### Rollback Verification

- `go test ./... -race` passes
- `go build ./cmd/sdd` succeeds
- `sdd context <change> explore` produces output identical to pre-change baseline
- `sdd write <change> explore` advances state without error

## Dependencies

### Internal

- `internal/state.Phase` (string type) unchanged — registry keyed on it
- `internal/context.Params` struct unchanged at call sites — only the assembler function field type annotation changes
- `internal/events.Broker` unchanged — passed through `AssemblerParams`

### External

- None. No new entries in `go.mod`.

### Infrastructure

- Go 1.24.1 (already in use) — no generics required for this change
- `go test ./internal/phase/...` added to CI matrix

## Success Criteria

- [ ] `go build ./...` succeeds with zero new imports in `go.mod`
- [ ] `go test ./... -race` passes with no data races
- [ ] `internal/phase/registry_test.go` asserts: all 10 phases registered, no duplicate names, prerequisite graph is acyclic, `verify` and `archive` have nil `Assemble` and empty `ArtifactFile`
- [ ] `ArtifactFileName` map in `artifacts.go` deleted — replaced by registry lookup
- [ ] `prerequisites` and `validNextPhases` maps in `state.go` deleted — replaced by registry lookup
- [ ] `phaseInputs` and `phaseTTL` maps in `cache.go` deleted — replaced by registry lookup
- [ ] `dispatchers` map in `context.go` deleted — replaced by registry lookup
- [ ] Custom phase smoke test: a test-only `Phase` struct registered via `phase.DefaultRegistry.Register()` is picked up by `state.AllPhases()` and `context.Assemble()`
- [ ] `internal/cli/commands.go` has zero diffs — CLI layer untouched

## Open Questions

- **`context.Params` vs `phase.AssemblerParams`**: Do all current fields of `context.Params` belong in `AssemblerParams`, or is a sub-struct cleaner? If `Broker`, `Stderr`, `Verbosity` are only used post-assembly (for metrics/logging), they may not need to move. Decision deferred to spec phase after reading call sites.
- **Registry mutability after startup**: Should `Register()` panic if called after the first `Get()`? Prevents late registrations that race with reads during `AssembleConcurrent`. Conservative approach; revisit if tests need dynamic registration.
- **`nextReady()` ordering with custom phases**: A custom phase appended to the registry after the 10 built-ins will be considered last in `nextReady()`. Is that the right default, or should `Register()` accept a position hint?
- **`Recover()` rebuild**: Currently `Recover()` in `state.go` encodes an artifact→phase reverse map independently of `ArtifactFileName`. Should the registry-driven refactor also fix `Recover()` to derive this map from the registry, or is that a separate cleanup task?
