# Spec: phase-interface

**Domain**: phase-registry
**Change ID**: phase-interface
**Status**: pending
**Date**: 2026-03-21

---

## Overview

This spec defines the requirements for replacing four parallel package-level maps with a single `Phase` struct and `Registry`. The refactor introduces a new `internal/phase` package that has no internal imports, making it importable by `state`, `artifacts`, and `context` without cycles. All phase metadata — prerequisites, next phases, artifact filename, cache inputs, cache TTL, and assembler function — is consolidated into one `Phase` struct value per phase.

---

## RFC 2119 Usage

The key words MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT, RECOMMENDED, MAY, and OPTIONAL in this document are to be interpreted as described in RFC 2119.

---

## Requirements

### REQ-1: Phase Struct (ADDED)

**REQ-1.1** The `internal/phase` package MUST define a `Phase` struct with the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `Name` | `string` | Canonical phase name (e.g., `"explore"`) |
| `Prerequisites` | `[]string` | Phase names that must be completed before this phase |
| `NextPhases` | `[]string` | Phase names that may start after this phase completes |
| `ArtifactFile` | `string` | Final artifact filename or directory name; empty string for `verify` and `archive` |
| `CacheInputs` | `[]string` | Artifact filenames (relative to change dir) whose content affects cached context; `"specs/"` is a directory sentinel |
| `CacheTTL` | `time.Duration` | Maximum age of a valid cache entry; zero means no TTL check |
| `Assemble` | `func(w io.Writer, p *AssemblerParams) error` | Context assembler function; MUST be nil for `verify` and `archive` |

**REQ-1.2** The `Phase` struct MUST be defined in `internal/phase/phase.go` (or equivalent file in the `internal/phase` package).

**REQ-1.3** The `Assemble` field MUST be permitted to be nil. Callers MUST guard with `p.Assemble != nil` before invoking it.

**REQ-1.4** The `ArtifactFile` field MUST be empty (`""`) for `verify` and `archive`. All other eight phases MUST have a non-empty `ArtifactFile`.

**REQ-1.5** The `Phase` struct MUST NOT embed or reference any type from `internal/state`, `internal/context`, `internal/artifacts`, `internal/config`, or `internal/events` in its definition. `AssemblerParams` fields that reference `config.Config` and `events.Broker` are defined on `AssemblerParams`, not on `Phase` itself.

---

### REQ-2: AssemblerParams (ADDED)

**REQ-2.1** The `internal/phase` package MUST define an `AssemblerParams` struct that carries the inputs needed by every assembler function.

**REQ-2.2** `AssemblerParams` MUST include at minimum: `ChangeDir string`, `ChangeName string`, `Description string`, `ProjectDir string`, `SkillsPath string`. It MAY include `Config`, `Stderr`, `Verbosity`, and `Broker` fields as needed by the assembler functions, subject to the import constraint in REQ-6.

**REQ-2.3** The `Assemble` function field on `Phase` MUST have the type `func(w io.Writer, p *AssemblerParams) error`.

**REQ-2.4** `internal/context` MUST update its assembler function signatures to accept `*phase.AssemblerParams` instead of `*Params` where the assembler is registered on a `Phase` struct. The existing `context.Params` struct MAY continue to exist if it serves purposes beyond assembly dispatch (e.g., holding `Broker`, `Stderr`, `Verbosity` for use after assembly completes).

---

### REQ-3: Registry Type (ADDED)

**REQ-3.1** The `internal/phase` package MUST define a `Registry` type backed by an ordered `[]Phase` slice. Pipeline position is defined by slice index.

**REQ-3.2** `Registry` MUST expose the following methods:

| Method | Signature | Semantics |
|--------|-----------|-----------|
| `Register` | `func (r *Registry) Register(p Phase)` | Appends `p` to the registry; MUST panic if a phase with the same `Name` is already registered |
| `Get` | `func (r *Registry) Get(name string) (Phase, bool)` | Returns the `Phase` for `name` and true, or zero value and false if not found |
| `AllNames` | `func (r *Registry) AllNames() []string` | Returns phase names in registration order |
| `All` | `func (r *Registry) All() []Phase` | Returns all phases in registration order |

**REQ-3.3** A package-level `DefaultRegistry` of type `*Registry` MUST be initialized in `internal/phase`. All 10 built-in phases MUST be registered into `DefaultRegistry` via a package `init()` function (or equivalent top-level `var` initialization) in `internal/phase/registry.go`.

**REQ-3.4** `Registry.Get` lookup MUST be O(N) linear scan on the `[]Phase` slice. N=10 for built-in phases; this is acceptable. No map-based index is required.

**REQ-3.5** After the `internal/phase` package `init()` completes, `DefaultRegistry` MUST contain exactly 10 phases in pipeline order: `explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `review`, `verify`, `clean`, `archive`.

**REQ-3.6** `Registry` MUST be safe for concurrent reads after initialization. It MUST NOT be modified after the first `Get` or `AllNames` call during normal operation. Implementations SHOULD panic if `Register` is called after any read method has been invoked.

---

### REQ-4: Built-in Phase Registrations (ADDED)

**REQ-4.1** The built-in phase definitions MUST preserve all existing pipeline semantics. The following table is normative:

| Phase | Prerequisites | NextPhases | ArtifactFile | CacheTTL | Assemble nil? |
|-------|--------------|------------|--------------|----------|----------------|
| `explore` | `[]` | `[propose]` | `exploration.md` | 0 | No |
| `propose` | `[explore]` | `[spec, design]` | `proposal.md` | 4h | No |
| `spec` | `[propose]` | `[tasks]` | `specs` | 2h | No |
| `design` | `[propose]` | `[tasks]` | `design.md` | 2h | No |
| `tasks` | `[spec, design]` | `[apply]` | `tasks.md` | 1h | No |
| `apply` | `[tasks]` | `[review]` | `tasks.md` | 30m | No |
| `review` | `[apply]` | `[verify]` | `review-report.md` | 1h | No |
| `verify` | `[review]` | `[clean]` | `""` | 0 | Yes |
| `clean` | `[verify]` | `[archive]` | `clean-report.md` | 1h | No |
| `archive` | `[clean]` | `[]` | `""` | 0 | Yes |

**REQ-4.2** The `CacheInputs` values MUST match the current `phaseInputs` map in `internal/context/cache.go`. Specifically: `explore=[]`, `propose=[exploration.md]`, `spec=[proposal.md, exploration.md]`, `design=[proposal.md, specs/]`, `tasks=[design.md, specs/]`, `apply=[tasks.md, design.md, specs/]`, `review=[tasks.md, design.md, specs/]`, `clean=[verify-report.md, tasks.md, design.md, specs/]`. `verify` and `archive` MUST have empty `CacheInputs`.

**REQ-4.3** `apply`'s `ArtifactFile` MUST remain `"tasks.md"` — apply updates tasks.md in place, not a separate file.

---

### REQ-5: Map Elimination (REMOVED)

**REQ-5.1** The `prerequisites` map in `internal/state/state.go` MUST be deleted. `CanTransition`, `nextReady`, and `ReadyPhases` MUST be rewritten to obtain prerequisites from `phase.DefaultRegistry.Get(name).Prerequisites`.

**REQ-5.2** The `validNextPhases` map in `internal/state/state.go` MUST be deleted.

**REQ-5.3** The `ArtifactFileName` map in `internal/artifacts/artifacts.go` MUST be deleted. Callers MUST obtain artifact filenames via `phase.DefaultRegistry.Get(name).ArtifactFile`.

**REQ-5.4** The `dispatchers` map in `internal/context/context.go` MUST be deleted. `Assemble()` MUST obtain the assembler function via `phase.DefaultRegistry.Get(string(phase)).Assemble`.

**REQ-5.5** The `phaseInputs` map in `internal/context/cache.go` MUST be deleted. `tryCachedContext`, `saveContextCache`, and `CheckCacheIntegrity` MUST obtain cache inputs via `phase.DefaultRegistry.Get(phaseName).CacheInputs`.

**REQ-5.6** The `phaseTTL` map in `internal/context/cache.go` MUST be deleted. Cache TTL checks MUST use `phase.DefaultRegistry.Get(phaseName).CacheTTL`. A zero `CacheTTL` MUST mean no TTL check (current behavior for `explore`).

**REQ-5.7** `AllPhases()` in `internal/state/types.go` MUST be rewritten to return phases in registry order via `phase.DefaultRegistry.AllNames()`, converted to `[]state.Phase`.

**REQ-5.8** `Recover()` in `internal/state/state.go` MUST derive its artifact→phase reverse lookup from the registry. It MUST iterate over all registered phases and skip any phase whose `ArtifactFile` is empty. The independent inline map currently at `state.go:213–221` MUST be deleted.

---

### REQ-6: Import Graph (ADDED)

**REQ-6.1** `internal/phase` MUST NOT import any package under `internal/` (no internal imports). Its only imports MUST be from the Go standard library.

**REQ-6.2** The import graph after refactor MUST be:

```
internal/phase          (no internal imports)
    ↑
internal/state          (imports internal/phase)
internal/artifacts      (imports internal/phase)
internal/context        (imports internal/phase)
    ↑
internal/cli            (unchanged — imports state, context, artifacts)
```

**REQ-6.3** No import cycle MUST be introduced. `go build ./...` MUST succeed without cycle errors.

**REQ-6.4** `internal/context` MUST import `internal/phase` and register assembler functions onto `Phase` structs. `internal/phase` MUST NOT import `internal/context`.

**REQ-6.5** `internal/state` MUST import `internal/phase`. `internal/phase` MUST NOT import `internal/state`.

---

### REQ-7: Custom Phase Registration (ADDED)

**REQ-7.1** External callers (e.g., test code, plugin packages) MUST be able to register a custom phase by constructing a `phase.Phase` struct and calling `phase.DefaultRegistry.Register(p)`.

**REQ-7.2** A registered custom phase MUST be returned by `phase.DefaultRegistry.AllNames()`, and therefore by `state.AllPhases()`, and MUST be considered by `state.ReadyPhases()` and `state.nextReady()`.

**REQ-7.3** A registered custom phase with a non-nil `Assemble` field MUST be dispatchable by `context.Assemble()` without modifying any core file.

**REQ-7.4** The `Register` method MUST panic if called with a `Phase` whose `Name` is empty.

**REQ-7.5** Custom phases appended after the 10 built-ins are considered last in pipeline order. No position-hint API is required in this change.

---

### REQ-8: Unit Tests (ADDED)

**REQ-8.1** `internal/phase/registry_test.go` MUST exist and MUST assert:

1. Exactly 10 phases are registered in `DefaultRegistry`.
2. No duplicate phase names exist.
3. The prerequisite graph is acyclic (DFS or topological sort).
4. `verify` and `archive` have nil `Assemble` and empty `ArtifactFile`.
5. All other 8 phases have non-nil `Assemble` and non-empty `ArtifactFile`.
6. A custom phase registered via `Register()` appears in `AllNames()`.
7. `Register()` panics on duplicate name.

**REQ-8.2** Existing tests in `internal/state/state_test.go` and `internal/context/context_test.go` MUST pass without modification (behavioral parity).

---

## Scenarios

### SCN-1: Registry initialization order
- **Eval**: static / compile-time
- **Criticality**: critical

**Given** `internal/phase/registry.go` registers all 10 phases in a package `init()` block
**When** any importing package (`state`, `artifacts`, `context`) initializes
**Then** the Go runtime guarantees `internal/phase.init()` completes before the importer's `init()`, so `DefaultRegistry` is populated before any consumer reads it.

---

### SCN-2: Assemble dispatch — hit
- **Eval**: unit test
- **Criticality**: high

**Given** `DefaultRegistry` contains all 10 built-in phases
**When** `context.Assemble(w, state.PhaseExplore, p)` is called
**Then** `DefaultRegistry.Get("explore")` returns a `Phase` with non-nil `Assemble`; the assembler is called and writes to `w`; no error is returned.

---

### SCN-3: Assemble dispatch — phase with nil assembler
- **Eval**: unit test
- **Criticality**: high

**Given** `verify` is registered with `Assemble == nil`
**When** `context.Assemble(w, state.PhaseVerify, p)` is called
**Then** the function returns an error (e.g., `"no assembler for phase: verify"`) without panicking. Callers that guard with `p.Assemble != nil` before calling will avoid this path entirely.

---

### SCN-4: Cache TTL — zero TTL phase
- **Eval**: unit test
- **Criticality**: medium

**Given** `explore` has `CacheTTL == 0`
**When** `tryCachedContext(changeDir, "explore", skillsPath)` is called with a valid content hash
**Then** no TTL check is performed and the cached content is returned (matching current behavior where `explore` has no entry in `phaseTTL`).

---

### SCN-5: Cache TTL — expired entry
- **Eval**: unit test
- **Criticality**: high

**Given** `propose` has `CacheTTL == 4h` and a cache entry was written 5 hours ago
**When** `tryCachedContext(changeDir, "propose", skillsPath)` is called
**Then** the function returns `nil, false` (cache miss due to TTL expiry).

---

### SCN-6: Recover() — registry-driven artifact scan
- **Eval**: unit test
- **Criticality**: high

**Given** a change directory contains `exploration.md`, `proposal.md`, and `design.md` but no other phase artifacts
**When** `state.Recover(name, desc, changeDir)` is called
**Then** phases `explore`, `propose`, and `design` are marked `StatusCompleted`; all other phases remain `StatusPending`; `CurrentPhase` is computed by `nextReady()` from the registry.

---

### SCN-7: Custom phase registration
- **Eval**: unit test
- **Criticality**: medium

**Given** a test constructs `phase.Phase{Name: "custom-lint", Prerequisites: []string{"review"}, ArtifactFile: "lint-report.md", Assemble: myLintAssembler}` and calls `phase.DefaultRegistry.Register(p)`
**When** `state.AllPhases()` is called
**Then** `"custom-lint"` appears in the returned slice after `"archive"`.

---

### SCN-8: Duplicate registration panics
- **Eval**: unit test
- **Criticality**: medium

**Given** `"explore"` is already registered in `DefaultRegistry`
**When** `phase.DefaultRegistry.Register(phase.Phase{Name: "explore", ...})` is called
**Then** the call panics with a message identifying the duplicate name.

---

### SCN-9: Import cycle absence
- **Eval**: static / compile-time
- **Criticality**: critical

**Given** the refactored codebase
**When** `go build ./...` is run
**Then** the build succeeds with exit code 0 and no `import cycle` error in the output.

---

### SCN-10: No-change CLI layer
- **Eval**: diff / code review
- **Criticality**: high

**Given** `internal/cli/commands.go` uses `state.Phase` (string type), `context.Assemble()`, and `state.ReadyPhases()`
**When** the refactor is applied
**Then** `internal/cli/commands.go` has zero line diffs. The file MUST NOT be modified.

---

### SCN-11: Prerequisite graph acyclicity
- **Eval**: unit test
- **Criticality**: critical

**Given** all 10 built-in phases registered in `DefaultRegistry`
**When** the registry test performs a depth-first traversal of the prerequisite graph
**Then** no cycle is detected; the traversal completes without revisiting a node.

---

### SCN-12: AllPhases ordering preserved
- **Eval**: unit test
- **Criticality**: high

**Given** `DefaultRegistry` registers phases in pipeline order
**When** `state.AllPhases()` is called
**Then** the returned slice is `[explore, propose, spec, design, tasks, apply, review, verify, clean, archive]`, matching the current hardcoded slice order.

---

## Constraints

- `go.mod` MUST NOT gain new entries. This refactor uses only standard library and existing module dependencies.
- On-disk formats (artifact filenames, `state.json` schema) MUST NOT change.
- `state.Phase` string type constants (`PhaseExplore`, `PhasePropose`, …) MUST remain in `internal/state/types.go`. They are unchanged.
- The `cacheVersion` constant in `internal/context/cache.go` MUST be bumped when this refactor ships, because the cache hash inputs path changes from a local map lookup to a registry lookup. This ensures stale pre-refactor cache entries are invalidated.

---

## Out of Scope

- Go interface polymorphism — `Phase` is a struct, not an interface.
- `context.Params` full replacement — only assembler function types are updated.
- File-based custom phase discovery (YAML/TOML config).
- Changes to `AssembleConcurrent` parallelism logic.
- Registry mutability lock with a read-after-write sentinel (deferred to implementation decision).
- `Register()` position-hint parameter for ordering custom phases mid-pipeline.
