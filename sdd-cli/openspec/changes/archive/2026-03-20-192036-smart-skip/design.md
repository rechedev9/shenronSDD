# Design: smart-skip verify + concurrent spec/design assembly

Change: smart-skip
Date: 2026-03-20
Status: pending

---

## Overview

Two independent features. Each has its own section below. No new packages. No breaking API changes.

---

## Feature 1: shouldSkipVerify

### Location

`internal/cli/commands.go` — new private function after `gitDiffFiles` (line 588).

### Signature

```go
func shouldSkipVerify(cwd, changeDir string) (bool, error)
```

Returns `(false, nil)` on any error or ambiguity — never skips when unsure.

### Logic (in order)

1. `os.ReadFile(filepath.Join(changeDir, "verify-report.md"))` — if error (missing), return `(false, nil)`.
2. Read first 256 bytes max. Check `bytes.Contains(head, []byte("**Status:** PASSED"))`. If not found, return `(false, nil)`.
3. `gitDiffFiles(cwd, "HEAD")` — if error, return `(false, nil)`.
4. Filter the returned file list: keep only entries that have extension `.go` and do NOT have prefix `openspec/`. If any survive filtering, return `(false, nil)`.
5. All guards passed: return `(true, nil)`.

### Source file filter

```go
func isSourceFile(path string) bool {
    return !strings.HasPrefix(path, "openspec/") &&
        filepath.Ext(path) == ".go"
}
```

Inline helper, not exported. Future extension point: add `cfg.Commands` extensions when needed (Rule of 3 not met yet).

### Call site in runVerify

Insert after config load (line 409) and before `verify.Run` call (line 419):

```go
if skip, err := shouldSkipVerify(cwd, changeDir); err != nil {
    fmt.Fprintf(stderr, "sdd: skip check failed, running verify: %v\n", err)
} else if skip {
    fmt.Fprintf(stderr, "sdd: verify skipped (no source changes since last PASSED run)\n")
    out := struct {
        Command    string `json:"command"`
        Status     string `json:"status"`
        Change     string `json:"change"`
        Passed     bool   `json:"passed"`
        Skipped    bool   `json:"skipped,omitempty"`
        ReportPath string `json:"report_path"`
    }{
        Command:    "verify",
        Status:     "success",
        Change:     name,
        Passed:     true,
        Skipped:    true,
        ReportPath: filepath.Join(changeDir, "verify-report.md"),
    }
    data, _ := json.MarshalIndent(out, "", "  ")
    fmt.Fprintln(stdout, string(data))
    return nil
}
```

The existing `out` struct at line 430 does not need a `Skipped` field; the skip path returns early with its own literal struct. No shared struct required.

### What does NOT change

- `verify.Run` — untouched.
- `internal/verify/verify.go` — untouched.
- `internal/state/types.go` — no `VerifiedRef` field added (deferred per proposal).
- The `gitDiffFiles` function — called as-is with `"HEAD"` as ref.

### Edge cases

| Condition | Outcome |
|-----------|---------|
| `verify-report.md` missing | run verify (no prior record) |
| Report exists but contains FAILED | run verify (prior failure) |
| Report PASSED, git diff errors | run verify (safe default) |
| Report PASSED, `.go` file changed | run verify |
| Report PASSED, only `openspec/` files changed | skip |
| Report PASSED, no changes at all | skip |
| Deleted `.go` file | `git diff HEAD` reports deletion — triggers re-verify |

---

## Feature 2: ReadyPhases

### Location

`internal/state/state.go` — new exported function after `nextReady` (line 107).

### Signature

```go
// ReadyPhases returns all pending phases whose prerequisites are completed.
// In the normal pipeline this returns at most one phase, except during the
// spec+design parallel window where it may return [PhaseSpec, PhaseDesign].
func ReadyPhases(s *State) []Phase
```

### Logic

Iterate `AllPhases()` in pipeline order. For each phase `p`:
- Skip if `s.Phases[p] != StatusPending`.
- Check all entries in `prerequisites[p]`; skip if any is not `StatusCompleted`.
- Append to result slice.

Return the accumulated slice (may be nil/empty if pipeline is complete or blocked).

### Relationship to nextReady

`nextReady` returns only the first match (used by `Advance` to set `CurrentPhase`). `ReadyPhases` returns all matches. They share the same inner predicate; `nextReady` is NOT refactored to call `ReadyPhases` — that would change existing behavior (early return vs full scan). The duplication is intentional (Rule of 3 not met; they serve different callers).

### Behavior table

| State | ReadyPhases result |
|-------|--------------------|
| Fresh (all pending, explore prereqs met) | `[PhaseExplore]` |
| explore+propose completed | `[PhaseSpec, PhaseDesign]` |
| explore+propose+spec completed | `[PhaseDesign]` |
| explore+propose+spec+design completed | `[PhaseTasks]` |
| All completed | `[]` |

---

## Feature 3: AssembleConcurrent

### Location

`internal/context/context.go` — new exported function after `Assemble` (line 93). Add `"sync"` to imports.

### Signature

```go
func AssembleConcurrent(w io.Writer, phases []state.Phase, p *Params) error
```

### Logic

```
len(phases) == 0  → return nil
len(phases) == 1  → return Assemble(w, phases[0], p)
len(phases) > 1   →
    type result struct {
        buf bytes.Buffer
        err error
    }
    results := make([]result, len(phases))
    sem := make(chan struct{}, 4)  // cap 4, actual concurrency ≤ 2
    var wg sync.WaitGroup
    for i, ph := range phases {
        wg.Add(1)
        go func(i int, ph state.Phase) {
            defer wg.Done()
            sem <- struct{}{}
            defer func() { <-sem }()
            results[i].err = Assemble(&results[i].buf, ph, p)
        }(i, ph)
    }
    wg.Wait()
    // Collect first error; write all buffers in slice order.
    var firstErr error
    for i := range results {
        if results[i].err != nil && firstErr == nil {
            firstErr = results[i].err
        }
        w.Write(results[i].buf.Bytes())
    }
    return firstErr
```

### Concurrency safety

- Each goroutine has its own `results[i].buf` — no shared writer during assembly.
- `tryCachedContext` and `saveContextCache` write to per-phase files (`spec.ctx`, `design.ctx`) — no shared file.
- `emitMetrics` → `recordMetrics` writes `metrics.json` (one file, two goroutines). Race condition: last writer wins. Acceptable: metrics are best-effort, explicitly documented as such in existing code (`// best-effort`). No lock added; metrics correctness is not a correctness invariant.
- `p.Stderr` writes from both goroutines may interleave. Acceptable: `fmt.Fprintf` to an `io.Writer` is not atomic but each `writeMetrics` call emits a single `fmt.Fprintf` line; interleaving at the line level is acceptable for human-readable diagnostic output.

### Error semantics

First error encountered (in results slice order after `wg.Wait`) is returned. All buffers are still written to `w` — partial output is acceptable since phase outputs are self-contained sections.

### What does NOT change

- `Assemble` — untouched. All existing callers unaffected.
- `runNew` — still calls `Assemble(stdout, state.PhaseExplore, p)` directly.

---

## Feature 4: runContext wiring

### Location

`internal/cli/commands.go`, `runContext` function, around line 174–195.

### Current state

```go
var phase state.Phase
if len(args) >= 2 {
    phase = state.Phase(args[1])
} else {
    phase = st.CurrentPhase
}
// ... build p ...
if err := sddctx.Assemble(stdout, phase, p); err != nil {
    return errs.WriteError(stderr, "context", err)
}
```

### New state

```go
phaseExplicit := len(args) >= 2
var phase state.Phase
if phaseExplicit {
    phase = state.Phase(args[1])
} else {
    phase = st.CurrentPhase
}
// ... build p (unchanged) ...
if phaseExplicit {
    if err := sddctx.Assemble(stdout, phase, p); err != nil {
        return errs.WriteError(stderr, "context", err)
    }
} else {
    ready := state.ReadyPhases(st)
    switch len(ready) {
    case 0:
        // Pipeline complete or blocked — fall back to CurrentPhase.
        if err := sddctx.Assemble(stdout, st.CurrentPhase, p); err != nil {
            return errs.WriteError(stderr, "context", err)
        }
    case 1:
        if err := sddctx.Assemble(stdout, ready[0], p); err != nil {
            return errs.WriteError(stderr, "context", err)
        }
    default:
        if err := sddctx.AssembleConcurrent(stdout, ready, p); err != nil {
            return errs.WriteError(stderr, "context", err)
        }
    }
}
```

The `case 0` fallback is defensive: `ReadyPhases` returns `[]` only when the pipeline is fully complete or in an unexpected blocked state. Falling back to `st.CurrentPhase` preserves existing behavior (`CurrentPhase` is always set by `nextReady` after each `Advance`).

---

## Files changed

| File | Change |
|------|--------|
| `internal/cli/commands.go` | Add `shouldSkipVerify` + `isSourceFile` after `gitDiffFiles`. Insert skip call in `runVerify` after config load. Update `runContext` phase dispatch. |
| `internal/state/state.go` | Add `ReadyPhases(*State) []Phase` after `nextReady`. |
| `internal/context/context.go` | Add `AssembleConcurrent`. Add `"sync"` to imports. |

No new packages. No changes to `internal/verify/`, `internal/artifacts/`, `internal/config/`, `internal/state/types.go`.

---

## Tests

### shouldSkipVerify (commands_test.go, table-driven)

| name | reportContent | diffOutput | diffErr | want |
|------|---------------|------------|---------|------|
| missing_report | (no file) | — | — | false |
| failed_report | `**Status:** FAILED` | `[]` | nil | false |
| passed_no_changes | `**Status:** PASSED` | `[]` | nil | true |
| passed_go_changed | `**Status:** PASSED` | `["main.go"]` | nil | false |
| passed_openspec_only | `**Status:** PASSED` | `["openspec/changes/x/proposal.md"]` | nil | true |
| passed_diff_error | `**Status:** PASSED` | — | `errors.New("git fail")` | false |

Use `t.TempDir()` for report file. Inject `gitDiffFiles` via a func parameter or test the full `shouldSkipVerify` by writing real files and using a fake git binary on `$PATH`. The simpler approach: factor git diff into an injectable `func(dir, ref string) ([]string, error)` parameter on `shouldSkipVerify` — but that's a new parameter not shown in the proposal signature. Instead, use the real `gitDiffFiles` and test `shouldSkipVerify` at a higher level via a test git repo (`git init` + `git commit` in `t.TempDir()`). This matches existing `gitDiffFiles` test strategy.

### ReadyPhases (state_test.go, table-driven)

Each case: construct a `*State` with specific `Phases` map values, call `ReadyPhases`, assert result slice.

Key cases: fresh state returns `[PhaseExplore]`; propose-completed returns `[PhaseSpec, PhaseDesign]`; spec+propose completed returns `[PhaseDesign]`; all completed returns `[]`.

### AssembleConcurrent (context_test.go, table-driven)

Use a fake assembler that writes a known string to `w`. Inject via a test-only variant or by overriding `dispatchers` in the test package (package `context`, so `dispatchers` is accessible).

| case | phases | expect |
|------|--------|--------|
| empty | `[]` | nil error, empty output |
| single | `[PhaseSpec]` | delegates to Assemble, output = spec sentinel |
| two | `[PhaseSpec, PhaseDesign]` | output = spec-sentinel + design-sentinel in order, nil error |
| two_first_errors | `[PhaseSpec, PhaseDesign]` (spec assembler returns error) | error returned, design output still written |

---

## Rollback

Features are independent:
- Smart-skip: revert `shouldSkipVerify`, `isSourceFile`, and the call site in `runVerify`. No state or schema changes.
- Concurrent assembly: revert `ReadyPhases`, `AssembleConcurrent`, and the `runContext` dispatch update. `Assemble` is untouched.
