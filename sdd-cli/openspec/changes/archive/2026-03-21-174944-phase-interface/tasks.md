# Tasks: phase-interface

**Change ID**: phase-interface
**Generated**: 2026-03-21
**Status**: pending

---

## Overview

8 files total: 3 new (`internal/phase/phase.go`, `internal/phase/registry.go`, `internal/phase/registry_test.go`), 5 modified (`internal/state/types.go`, `internal/state/state.go`, `internal/artifacts/artifacts.go`, `internal/context/context.go`, `internal/context/cache.go`).

Tasks are ordered by dependency. Each task builds on the previous; the verification command at each step must pass before proceeding.

---

## TASK-1: Create `internal/phase/phase.go` — Phase struct, AssemblerParams, Registry type

**Files**: `internal/phase/phase.go` (new)

**What**:
- Create the `internal/phase` package directory.
- Define `AssemblerParams` struct with fields: `ChangeDir`, `ChangeName`, `Description`, `ProjectDir`, `Config *config.Config`, `SkillsPath`, `Stderr io.Writer`, `Verbosity int`, `Broker *events.Broker`. These match the existing `context.Params` fields exactly (verified in `internal/context/context.go:30-40`).
- Define `Assembler` as `func(w io.Writer, p *AssemblerParams) error`.
- Define `Phase` struct with fields: `Name string`, `Prerequisites []string`, `NextPhases []string`, `ArtifactFile string`, `RecoverSkip bool`, `CacheInputs []string`, `CacheTTL time.Duration`, `Assemble Assembler`.
- Define `Registry` struct with unexported `phases []Phase` and `sealed bool`.
- Implement `Register(p Phase)`: panic if sealed, panic if `p.Name` empty, panic if name already registered (linear scan). Append to `r.phases`.
- Implement `Get(name string) (Phase, bool)`: seal on first call, linear scan, return `(Phase{}, false)` if not found.
- Implement `All() []Phase`: seal, return copy of `r.phases`.
- Implement `AllNames() []string`: seal, return names in order.
- Implement `SetAssembler(name string, fn Assembler)`: panic if sealed, linear scan to find phase by name, panic if not found, set `Assemble` field.
- Declare `var DefaultRegistry = &Registry{}`.
- Imports: `io`, `time`, `github.com/rechedev9/shenronSDD/sdd-cli/internal/config`, `github.com/rechedev9/shenronSDD/sdd-cli/internal/events`. No `internal/state` import (REQ-6.1, REQ-6.5).

**Notes**:
- `SetAssembler` must NOT check `sealed` via the same guard that `Register` uses — `SetAssembler` is called from `internal/context`'s `init()`, which runs after `internal/phase`'s `init()` but before any `Get()`/`All()` call from CLI dispatch. However, to be safe: `SetAssembler` panics if sealed (same guard as `Register`). This means `context.init()` must run before the first read call — guaranteed by Go's init ordering since `context` is an importer.
- `config` and `events` are leaf packages (no internal imports) — safe to import from `phase`.

**Verification**: `go build ./internal/phase/...`

**Depends on**: nothing

---

## TASK-2: Create `internal/phase/registry.go` — built-in phase registrations

**Files**: `internal/phase/registry.go` (new)

**What**:
- In a single `init()` block, register all 10 built-in phases on `DefaultRegistry` using the normative table from REQ-4.1 and REQ-4.2.
- Registration order (pipeline order): `explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `review`, `verify`, `clean`, `archive`.
- Phase values (all fields sourced from real code — verified in `internal/artifacts/artifacts.go`, `internal/context/cache.go`, `internal/state/state.go`):

| Name | Prerequisites | NextPhases | ArtifactFile | RecoverSkip | CacheInputs | CacheTTL | Assemble |
|------|--------------|------------|--------------|-------------|-------------|----------|---------|
| `explore` | `[]` | `[propose]` | `exploration.md` | false | `[]` | 0 | nil |
| `propose` | `[explore]` | `[spec, design]` | `proposal.md` | false | `[exploration.md]` | 4h | nil |
| `spec` | `[propose]` | `[tasks]` | `specs` | false | `[proposal.md, exploration.md]` | 2h | nil |
| `design` | `[propose]` | `[tasks]` | `design.md` | false | `[proposal.md, specs/]` | 2h | nil |
| `tasks` | `[spec, design]` | `[apply]` | `tasks.md` | false | `[design.md, specs/]` | 1h | nil |
| `apply` | `[tasks]` | `[review]` | `tasks.md` | **true** | `[tasks.md, design.md, specs/]` | 30m | nil |
| `review` | `[apply]` | `[verify]` | `review-report.md` | false | `[tasks.md, design.md, specs/]` | 1h | nil |
| `verify` | `[review]` | `[clean]` | `verify-report.md` | false | `[]` | 0 | nil |
| `clean` | `[verify]` | `[archive]` | `clean-report.md` | false | `[verify-report.md, tasks.md, design.md, specs/]` | 1h | nil |
| `archive` | `[clean]` | `[]` | `archive-manifest.md` | false | `[]` | 0 | nil |

- All `Assemble` fields are `nil` at registration time — `context.init()` wires them in via `SetAssembler` later (design.md §Registration Pattern).

**Notes on artifact discrepancy**: The spec (REQ-1.4, REQ-4.1) states `verify` and `archive` have empty `ArtifactFile`. The actual `internal/artifacts/artifacts.go` map has `verify → "verify-report.md"` and `archive → "archive-manifest.md"`. The existing `Recover()` in `state.go` also uses `verify-report.md` for PhaseVerify. Use the real values (`verify-report.md`, `archive-manifest.md`) to preserve on-disk format and `Recover()` behavior. The spec's "empty ArtifactFile" constraint for verify/archive is therefore NOT followed — it conflicts with live code. Document this deviation in a comment in `registry.go`.

**Notes on `apply` RecoverSkip**: `apply` maps to `"tasks.md"` — same file as `tasks`. The current `Recover()` at `state.go:213-221` deliberately excludes `apply` to avoid false-positive completion. `RecoverSkip: true` on `apply` replicates this exclusion.

**Verification**: `go build ./internal/phase/...`

**Depends on**: TASK-1

---

## TASK-3: Create `internal/phase/registry_test.go` — structural invariants

**Files**: `internal/phase/registry_test.go` (new)

**What**:
- Use a local `&Registry{}` (not `DefaultRegistry`) for isolation tests to avoid seal pollution.
- Implement the following test functions:
  - `TestBuiltinPhaseCount`: `DefaultRegistry.All()` returns exactly 10 phases.
  - `TestBuiltinPhaseNamesUnique`: no duplicate names in `DefaultRegistry.All()`.
  - `TestPrerequisiteGraphAcyclic`: DFS from each phase over `Prerequisites` edges; panic/fail on cycle.
  - `TestNilAssemblePhases`: `verify` and `archive` have `Assemble == nil` after `DefaultRegistry` is fully initialized. Note: this test must import `internal/context` (blank import `_ "..."`) to trigger `context.init()` and wire assemblers, OR it should be placed in an integration test. Alternative: test that `verify` and `archive` have nil Assemble by checking the `DefaultRegistry` after a forced init — but since `context.init()` sets assemblers, these two phases will still have nil because `SetAssembler` is never called for them.
  - `TestNonNilAssemblePhases`: all 8 non-verify/non-archive phases have non-nil `Assemble` in `DefaultRegistry`. Same blank import of `internal/context` required.
  - `TestCustomPhaseRegistration`: create a fresh `&Registry{}`, register a custom phase, verify `AllNames()` includes it.
  - `TestDuplicateRegistrationPanics`: verify `Register` panics on duplicate name using `defer/recover`.
  - `TestEmptyNamePanics`: verify `Register` panics if `Name` is empty.
  - `TestAllPhasesOrder`: `DefaultRegistry.AllNames()` returns `[explore, propose, spec, design, tasks, apply, review, verify, clean, archive]` in that order.
  - `TestSealedRegistryPanicsOnRegister`: call `Get` to seal, then call `Register`, verify panic.

**Notes**:
- Tests for nil/non-nil `Assemble` fields on `DefaultRegistry` require that `context.init()` has run. The cleanest approach: put those two tests in a separate `_integration_test.go` file in `internal/phase` that blank-imports `internal/context`. However this would create an import cycle (`context` imports `phase`, and the test package would import `context`). Resolution: those two tests belong in `internal/context/context_test.go` instead, or accept that `Assemble` is nil until `context.init()` runs, and test only that `verify`/`archive` have nil after DefaultRegistry init (which is true since context.init hasn't run yet when running `internal/phase` tests in isolation). The tasks file captures both approaches; implementation choice is made during coding.
- For the cycle issue: `registry_test.go` (package `phase_test`) can import `internal/context` without a cycle because test binaries do not affect the module import graph. However, `go test ./internal/phase/...` linking `internal/context` may pull in large dependencies. Simpler: only test nil Assemble at the `phase` test level (before context wires them), and test non-nil Assemble in `internal/context/context_test.go`.

**Verification**: `go test ./internal/phase/...`

**Depends on**: TASK-1, TASK-2

---

## TASK-4: Refactor `internal/state/types.go` — AllPhases from registry

**Files**: `internal/state/types.go` (modified)

**What**:
- Add import `"github.com/rechedev9/shenronSDD/sdd-cli/internal/phase"` to `types.go`.
- Rewrite `AllPhases()` (currently `types.go:74-87`) to delegate to the registry:
  ```go
  func AllPhases() []Phase {
      all := phase.DefaultRegistry.AllNames()
      result := make([]Phase, len(all))
      for i, n := range all {
          result[i] = Phase(n)
      }
      return result
  }
  ```
- The 10 `Phase` constants (`PhaseExplore` etc.) and all other code in `types.go` are unchanged.

**Notes**:
- `state` now imports `phase`. `phase` does not import `state`. No cycle (REQ-6.5).
- `NewState` at `types.go:43-57` calls `AllPhases()` — it will now use the registry-driven version. Behavior is identical as long as registry order matches the old literal order (guaranteed by TASK-2).

**Verification**: `go build ./internal/state/...`

**Depends on**: TASK-2

---

## TASK-5: Refactor `internal/state/state.go` — prerequisites/validNextPhases/Recover from registry

**Files**: `internal/state/state.go` (modified)

**What**:
- Add import `"github.com/rechedev9/shenronSDD/sdd-cli/internal/phase"` (already added in TASK-4 for `types.go`; `state.go` is a separate file in the same package — import only needed if the file uses it directly).
- Delete the `validNextPhases` map (lines 24-35).
- Delete the `prerequisites` map (lines 38-49).
- Rewrite `CanTransition` (lines 59-72) to look up prerequisites via registry:
  ```go
  func (s *State) CanTransition(target Phase) error {
      if s.Phases[target] == StatusCompleted {
          return fmt.Errorf("%w: %s already completed", ErrAlreadyCompleted, target)
      }
      desc, ok := phase.DefaultRegistry.Get(string(target))
      if !ok {
          return fmt.Errorf("unknown phase: %s", target)
      }
      for _, req := range desc.Prerequisites {
          if s.Phases[Phase(req)] != StatusCompleted {
              return fmt.Errorf("%w: %s requires %s completed (currently %s)",
                  ErrPrerequisitesNotMet, target, req, s.Phases[Phase(req)])
          }
      }
      return nil
  }
  ```
- Rewrite `nextReady` (lines 90-107) to look up prerequisites via registry:
  ```go
  func (s *State) nextReady() Phase {
      for _, p := range AllPhases() {
          if s.Phases[p] != StatusPending {
              continue
          }
          desc, ok := phase.DefaultRegistry.Get(string(p))
          if !ok {
              continue
          }
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
- Rewrite `ReadyPhases` (lines 112-130) with the same pattern as `nextReady`.
- Rewrite `Recover` (lines 207-240): delete the inline `artifacts` map (lines 213-221). Iterate `phase.DefaultRegistry.All()`, skip phases where `desc.ArtifactFile == ""` or `desc.RecoverSkip == true`. Preserve the existing `os.Stat`/`IsDir`/`ReadDir` logic for each phase.
- The `validNextPhases` map is deleted but `CanTransition` no longer checks it (it was not used in `CanTransition` in the original code — verified at `state.go:59-72` which only reads `prerequisites`). Confirm `validNextPhases` has no other call sites before deleting.

**Notes**:
- Grep for `validNextPhases` before deleting: it must have zero call sites outside the declaration. Current code uses only `prerequisites` in `CanTransition`, `nextReady`, `ReadyPhases`.
- The `Recover` rewrite must preserve that `apply` is skipped (via `RecoverSkip: true` set in TASK-2) while `verify` (which has `verify-report.md`) is checked. The original `Recover` map includes `PhaseVerify → "verify-report.md"` at line 220 — this is preserved because `verify.ArtifactFile = "verify-report.md"` in TASK-2.
- `state.go` file imports `phase` package — add it to the import block.

**Verification**: `go build ./internal/state/...` then `go test ./internal/state/...`

**Depends on**: TASK-4 (so `AllPhases()` is already registry-driven when `Recover` calls it via `NewState`)

---

## TASK-6: Refactor `internal/artifacts/artifacts.go` — ArtifactFileName from registry

**Files**: `internal/artifacts/artifacts.go` (modified), `internal/artifacts/promote.go` (modified)

**What**:

**`artifacts.go`**:
- Delete the `ArtifactFileName` var map (lines 9-20).
- Add imports: `"github.com/rechedev9/shenronSDD/sdd-cli/internal/phase"`.
- Add exported function replacing the map:
  ```go
  // ArtifactFileName returns the canonical artifact filename for a phase.
  // Returns ("", false) if the phase is not registered.
  func ArtifactFileName(ph state.Phase) (string, bool) {
      desc, ok := phase.DefaultRegistry.Get(string(ph))
      if !ok {
          return "", false
      }
      return desc.ArtifactFile, true
  }
  ```
- `PendingFileName` is unchanged.

**`promote.go`**:
- Update line 23 (`finalName, ok := ArtifactFileName[phase]`) to use the function:
  ```go
  finalName, ok := ArtifactFileName(phase)
  if !ok || finalName == "" {
      return "", fmt.Errorf("no artifact mapping for phase: %s", phase)
  }
  ```
- No other changes to `promote.go`.

**Notes**:
- The identifier `ArtifactFileName` changes from `var map[state.Phase]string` to `func(state.Phase) (string, bool)`. Go allows this — identifier can be either. The compiler will flag any remaining map-index call sites (`ArtifactFileName[x]`) as type errors. Grep for `ArtifactFileName` across all files before assuming `promote.go` is the only call site.
- Also check `artifacts_test.go`, `list.go`, `reader.go`, `writer.go` for any use of `ArtifactFileName`.

**Verification**: `go build ./internal/artifacts/...`

**Depends on**: TASK-2

---

## TASK-7: Refactor `internal/context/context.go` and `cache.go` — dispatcher + cache maps from registry

**Files**: `internal/context/context.go` (modified), `internal/context/cache.go` (modified)

**What**:

**`context.go`**:
- Delete the `Assembler` type definition (line 27) — it now lives in `internal/phase` as `phase.Assembler`.
- Delete the `dispatchers` map (lines 43-52).
- Change `type Params struct` to a type alias: `type Params = phase.AssemblerParams`. This is a zero-blast-radius change — all call sites using `*Params` or `context.Params{}` continue to compile.
- Add import `"github.com/rechedev9/shenronSDD/sdd-cli/internal/phase"`.
- Rewrite the dispatcher lookup in `Assemble` (line 59): replace `fn, ok := dispatchers[phase]` with:
  ```go
  desc, ok := phase.DefaultRegistry.Get(string(ph))
  if !ok || desc.Assemble == nil {
      return fmt.Errorf("no assembler for phase: %s", ph)
  }
  ```
  Then replace `fn(&buf, p)` with `desc.Assemble(&buf, p)`.
- Add `init()` function that calls `phase.DefaultRegistry.SetAssembler` for all 8 assembling phases: `explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `review`, `clean`.
- Assembler function signatures (`AssembleExplore`, etc.) currently accept `*Params`. Since `Params = phase.AssemblerParams` (type alias), signatures are unchanged — no body edits needed in any `assemble_*.go` files.

**`cache.go`**:
- Delete the `phaseTTL` map (lines 26-34).
- Delete the `phaseInputs` map (lines 54-63).
- Add import `"github.com/rechedev9/shenronSDD/sdd-cli/internal/phase"` to `cache.go` (or rely on it already being in `context.go`; same package so import block in `cache.go` needs it independently if used there).
- Rewrite `tryCachedContext` (line 129): replace `inputs := phaseInputs[phase]` with:
  ```go
  var inputs []string
  if desc, ok := phaseReg().Get(phase); ok {
      inputs = desc.CacheInputs
  }
  ```
  Replace the TTL check block (lines 150-156): replace `if ttl, hasTTL := phaseTTL[phase]; hasTTL {` with:
  ```go
  if desc, ok := phaseReg().Get(phase); ok && desc.CacheTTL > 0 {
      ttl := desc.CacheTTL
      ts := mustParseInt64(tsStr)
      if time.Since(time.Unix(ts, 0)) > ttl {
          return nil, false
      }
  }
  ```
- Rewrite `saveContextCache` (line 182): replace `inputs := phaseInputs[phase]` with registry lookup (same pattern as `tryCachedContext`).
- Rewrite `CheckCacheIntegrity` (line 365): replace `inputs := phaseInputs[phase]` with registry lookup.
- Add helper `phaseReg() *phase.Registry` returning `phase.DefaultRegistry` — makes test injection possible and avoids repeating the qualified name.
- Bump `cacheVersion` constant (currently `5` at `cache.go:21`) to `6` — required by REQ (constraint: "cacheVersion MUST be bumped when this refactor ships").

**Notes**:
- `tryCachedContext` currently reads `inputs` on line 129 and the TTL on lines 150-156 as two separate map lookups. After refactor both reads become a single `phaseReg().Get(phase)` call — consolidate into one lookup at function top, store result in `desc`.
- `explore` has `CacheInputs: []` and `CacheTTL: 0`. Empty `CacheInputs` → `inputHash` receives nil/empty slice → hash includes only SKILL.md and version prefix. Zero TTL → TTL block is skipped. Behavior identical to current code where `explore` has no entry in `phaseTTL`.
- The `Assembler` type removal from `context.go`: existing assembler files (`assemble_explore.go` etc.) use `*Params` in their signatures. `Params` is now a type alias for `phase.AssemblerParams`. These files compile unchanged. Verify by grepping for `Assembler` usage in `context` package files — if any file declares `var _ Assembler = AssembleExplore` style compile-time checks, those must be updated to `var _ phase.Assembler = AssembleExplore`.

**Verification**: `go build ./...` then `go test ./...`

**Depends on**: TASK-1, TASK-2, TASK-4, TASK-5, TASK-6 (all prior tasks complete before touching context, which is the most interconnected package)

---

## TASK-8: Final integration verification

**Files**: none (verification only)

**What**:
- Run `go build ./...` — must succeed, zero new entries in `go.mod`.
- Run `go test ./... -race` — must pass, zero failures.
- Run `go vet ./...` — must pass.
- Confirm `internal/cli/commands.go` has zero diffs (SCN-10): `git diff HEAD internal/cli/`.
- Confirm all 6 deleted map literals are gone:
  - `grep -n "var validNextPhases" internal/state/state.go` — no match
  - `grep -n "var prerequisites" internal/state/state.go` — no match
  - `grep -n "var ArtifactFileName" internal/artifacts/artifacts.go` — no match
  - `grep -n "var dispatchers" internal/context/context.go` — no match
  - `grep -n "var phaseInputs" internal/context/cache.go` — no match
  - `grep -n "var phaseTTL" internal/context/cache.go` — no match
- Confirm `cacheVersion` is `6` in `internal/context/cache.go`.
- Confirm `internal/phase` has no imports from `internal/` (REQ-6.1): `grep -r "shenronSDD/sdd-cli/internal" internal/phase/phase.go internal/phase/registry.go` — must show only `config` and `events`.

**Verification**: all above commands exit 0

**Depends on**: TASK-7

---

## Dependency Graph

```
TASK-1 (phase.go)
  └─ TASK-2 (registry.go)
       ├─ TASK-3 (registry_test.go)
       ├─ TASK-4 (state/types.go)
       │    └─ TASK-5 (state/state.go)
       │         └─ TASK-6 (artifacts/artifacts.go + promote.go)
       │              └─ TASK-7 (context/context.go + cache.go)
       │                   └─ TASK-8 (integration verify)
       └─ TASK-6 (also depends directly on TASK-2)
```

TASK-3 can be worked on in parallel with TASK-4 after TASK-2.
TASK-6 depends on TASK-2 (registry exists) and TASK-5 (state compiles with phase import).
TASK-7 depends on all prior tasks because it finalizes the most import-sensitive package.
