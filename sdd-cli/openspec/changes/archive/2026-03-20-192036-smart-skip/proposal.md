# Proposal: smart-skip verify + concurrent spec/design assembly

Change: smart-skip
Date: 2026-03-20
Status: proposed

---

## Summary

Two independent features shipped in one change. Each is independently rollback-safe.

**In scope:**
- Smart-skip verify: skip `build/lint/test` and reuse `verify-report.md` when no source files changed since the last PASSED run.
- Concurrent spec+design assembly: assemble spec and design context in parallel when both phases are ready.

**Out of scope (deferred):**
- Structured slog logging — current `fmt.Fprintf` pattern is readable, call-site count is high (~8 sites across 3 packages), risk is medium. Marginal benefit over current output. Deferred to a future dedicated change.

---

## Feature 1: Smart-skip verify

### Behavior

Before invoking `verify.Run`, `runVerify` checks:

1. Does `verify-report.md` exist in the change dir? If not — run verify normally.
2. Does `verify-report.md` contain `**Status:** PASSED`? If not (prior run failed) — run verify normally.
3. Does `git diff --name-only HEAD` return any source files (`.go` or config-extensions)? If yes — run verify normally.
4. All three checks pass → skip. Emit a skip message to stderr, return cached JSON with `"skipped": true`.

SDD artifacts under `openspec/` are excluded from the file filter; changes there never trigger re-verify.

### Design decisions

- **git diff against HEAD, not BaseRef.** `gitDiffFiles(dir, "HEAD")` already exists at `commands.go:572`. Reuse as-is. Comparing against HEAD covers all uncommitted work-in-progress changes, which is the correct trigger set.
- **PASSED check before skip.** Read the first ~200 bytes of `verify-report.md` and scan for `**Status:** PASSED`. Avoids skipping on a prior failure report. This is the critical correctness guard.
- **No `state.VerifiedRef` field for MVP.** Mtime is not used; git diff against HEAD is sufficient for local dev and CI. The optional `VerifiedRef` field in types.go is deferred — can be added later if we need `git diff <verified-ref>..HEAD` semantics.
- **Source file filter.** Filter `gitDiffFiles` output to files with extensions `.go`, plus any extensions from `cfg.Commands` (future). Exclude prefix `openspec/`. A changed `openspec/changes/` file must not trigger re-verify.

### New helper

```go
// shouldSkipVerify returns true if verify can be safely skipped:
// the last report shows PASSED and no source files have changed (git diff HEAD).
// Returns (false, nil) on any error — erring on the side of running verify.
func shouldSkipVerify(cwd, changeDir string) (bool, error)
```

Location: `internal/cli/commands.go`, after `gitDiffFiles` (~line 589).

Returns `(false, nil)` on any ambiguous or error condition — never skips when unsure.

### Call site

`runVerify` (`commands.go:387`), after config load (~line 410), before `verify.Run` call at line 419:

```go
if skip, err := shouldSkipVerify(cwd, changeDir); err != nil {
    fmt.Fprintf(stderr, "sdd: skip check failed, running verify: %v\n", err)
} else if skip {
    // return cached JSON with skipped:true
    ...
    return nil
}
```

The skip-path JSON output:

```json
{
  "command": "verify",
  "status": "success",
  "change": "<name>",
  "passed": true,
  "skipped": true,
  "report_path": "<changeDir>/verify-report.md"
}
```

### Files changed

| File | Location | Change |
|------|----------|--------|
| `internal/cli/commands.go` | after line 410 | insert `shouldSkipVerify` call + skip return |
| `internal/cli/commands.go` | after `gitDiffFiles` (~line 589) | add `shouldSkipVerify(cwd, changeDir string) (bool, error)` |
| `internal/cli/commands.go` | `runVerify` output struct (~line 430) | add `Skipped bool \`json:"skipped,omitempty"\`` field |

No changes to `internal/verify/verify.go` or `internal/state/types.go`.

### Risk: Low

- False negatives (running verify when unnecessary) cannot cause correctness problems.
- False positives (skipping when should not) are prevented by the PASSED check and the git diff filter.
- `git diff --name-only HEAD` is <50ms on normal repos.
- Deleted `.go` files: `git diff HEAD` reports deletions; correctly triggers re-verify.
- CI: git is always present in CI; no mtime fallback needed.

---

## Feature 2: Concurrent spec+design assembly

### Behavior

The state graph has one parallel window: after `propose` completes, both `spec` and `design` can be assembled independently (both require only `propose`; `tasks` requires both). Currently `nextReady` returns only the first ready phase. `runContext` calls `Assemble` for one phase.

This feature adds:
- `ReadyPhases(*State) []Phase` in `internal/state/state.go` — returns all pending phases whose prerequisites are fully completed.
- `AssembleConcurrent(w io.Writer, phases []state.Phase, p *Params) error` in `internal/context/context.go` — two goroutines, two `bytes.Buffer`s, writes results in deterministic (slice) order after `sync.WaitGroup.Wait()`.
- `runContext` updated to call `AssembleConcurrent` when auto-resolving the current phase detects >1 ready phase. Explicit `sdd context <name> spec` (one phase named) still uses the single-phase `Assemble` path.

### Design decisions

- **Concurrent function is additive.** `Assemble` is unchanged. `AssembleConcurrent` is a new exported function. No existing callers are broken.
- **Deterministic output.** Results are written in `phases` slice order after all goroutines complete. Never interleaved on stdout.
- **Bounded semaphore: cap 2 in practice.** The only parallel window in the current pipeline is spec+design (exactly 2 goroutines). Semaphore of capacity 4 is used for forward-compatibility but the actual concurrency is always ≤2.
- **No shared writer.** Each goroutine gets its own `bytes.Buffer`. The shared `io.Writer` (stdout) is only touched after `wg.Wait()`, sequentially.
- **Cache safety.** `tryCachedContext` and `saveContextCache` write to per-phase files (`spec.ctx`, `design.ctx`). No shared file; concurrent access is safe.
- **`runNew` unchanged.** It calls `Assemble` for `PhaseExplore` only — always single-phase, no change.
- **Explicit-phase arg bypasses concurrent path.** `sdd context <name> spec` — user asked for one phase; use `Assemble`. `sdd context <name>` with no phase arg — check `ReadyPhases`; if >1, use `AssembleConcurrent`.

### New function: `ReadyPhases`

```go
// ReadyPhases returns all pending phases whose prerequisites are completed.
// In the normal pipeline this returns at most one phase, except during the
// spec+design parallel window where it returns [spec, design].
func ReadyPhases(s *State) []Phase
```

Location: `internal/state/state.go`, after `nextReady` (~line 107).

### New function: `AssembleConcurrent`

```go
func AssembleConcurrent(w io.Writer, phases []state.Phase, p *Params) error
```

- `len(phases) == 0`: return nil.
- `len(phases) == 1`: delegate to `Assemble` (no goroutine overhead).
- `len(phases) > 1`: goroutine per phase, semaphore cap 4, collect into `[]result`, write in order.

Location: `internal/context/context.go`, after `Assemble` (~line 93).

Requires adding `"sync"` to the import block.

### Call site change: `runContext`

`internal/cli/commands.go` line 191, current:

```go
if err := sddctx.Assemble(stdout, phase, p); err != nil {
```

New logic (pseudo):

```go
if phaseExplicit {
    // user named a phase — single path
    if err := sddctx.Assemble(stdout, phase, p); err != nil { ... }
} else {
    ready := state.ReadyPhases(st)
    if len(ready) <= 1 {
        if err := sddctx.Assemble(stdout, ready[0], p); err != nil { ... }
    } else {
        if err := sddctx.AssembleConcurrent(stdout, ready, p); err != nil { ... }
    }
}
```

### Files changed

| File | Location | Change |
|------|----------|--------|
| `internal/state/state.go` | after `nextReady` (~line 107) | add `ReadyPhases(*State) []Phase` |
| `internal/context/context.go` | after `Assemble` (~line 93) | add `AssembleConcurrent`, import `"sync"` |
| `internal/cli/commands.go` | `runContext` phase dispatch (~line 191) | branch on `ReadyPhases` length |

### Risk: Low-medium

- Parallelism is bounded: at most 2 goroutines (spec+design window).
- Read-only FS: each assembler reads different artifacts (`proposal.md` for both, `specs/` for spec only, `design.md` for design only). No write contention during assembly.
- Cache: per-phase files (`spec.ctx`, `design.ctx`) — no shared file.
- Output determinism: enforced by collecting all results before writing, not streaming from goroutines.
- Existing tests for `Assemble` are unaffected. `AssembleConcurrent` needs a new table test with a parallel case (use two stub assemblers, verify ordered output).

---

## Rollback

Features are independent. Either can be reverted without affecting the other.

- Smart-skip: revert the `shouldSkipVerify` call and helper in `commands.go`. No state schema changes.
- Concurrent assembly: revert `ReadyPhases`, `AssembleConcurrent`, and the dispatch change in `runContext`. `Assemble` is untouched.

---

## Deferred: structured slog logging

Scoped out. The current `fmt.Fprintf(stderr, "sdd: ...")` pattern is simple and readable. Migration would touch ~8 call sites across `internal/verify/verify.go`, `internal/context/cache.go`, and `internal/cli/commands.go`, change `Params.Stderr` from `io.Writer` to `*slog.Logger` (a breaking API change), and require nil-safe logger plumbing in tests. Medium risk, marginal benefit over current output format. Revisit in a future dedicated change if machine-parseable log output becomes a real requirement.

---

## Test plan

- `shouldSkipVerify`: unit test — PASSED report + empty diff → skip; FAILED report → no skip; non-empty diff → no skip; missing report → no skip; git error → no skip (safe default).
- `ReadyPhases`: unit test — state with propose completed → returns `[spec, design]`; state with spec completed too → returns `[design]`; all complete → returns `[]`.
- `AssembleConcurrent`: table test with stub assemblers — single phase delegates to `Assemble`; two phases run concurrently, output in slice order, first-error wins.
- Integration: `sdd verify <name>` twice in a row with no file changes between calls → second call returns `"skipped": true`.
