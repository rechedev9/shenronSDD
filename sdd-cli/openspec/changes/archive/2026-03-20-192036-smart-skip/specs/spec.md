# Spec: smart-skip

Change: smart-skip
Date: 2026-03-20
Status: pending

---

## Feature 1: Smart-Skip Verify

### New function: `shouldSkipVerify`

**File:** `internal/cli/commands.go`, after `gitDiffFiles` (currently ends at line 588).

```go
// shouldSkipVerify returns true if verify can be safely skipped:
// verify-report.md exists, shows PASSED, and git diff HEAD reports no source files.
// Returns (false, nil) on any error — never skips when unsure.
func shouldSkipVerify(cwd, changeDir string) (bool, error)
```

**Logic (in order; short-circuit on any false condition):**

1. Read `filepath.Join(changeDir, "verify-report.md")`. If `os.IsNotExist(err)` → return `(false, nil)`. Any other read error → return `(false, err)`.
2. Scan the file content for the literal substring `**Status:** PASSED`. Use `strings.Contains`. If not found → return `(false, nil)`.
3. Call `gitDiffFiles(cwd, "HEAD")`. If error → return `(false, err)`.
4. Filter the returned slice: keep only entries that do NOT have the prefix `openspec/`. Extensions are not filtered — any non-openspec file counts as a source change.
5. If filtered slice is non-empty → return `(false, nil)`.
6. All checks passed → return `(true, nil)`.

**Error contract:** every error path returns `(false, <err>)`. The function never returns `(true, <non-nil err>)`.

**No new imports required.** `os`, `strings`, `path/filepath` are already in the file's import block.

---

### Output struct change: `runVerify`

**File:** `internal/cli/commands.go`, `runVerify` (~line 430).

Add `Skipped` field to the anonymous output struct:

```go
out := struct {
    Command    string `json:"command"`
    Status     string `json:"status"`
    Change     string `json:"change"`
    Passed     bool   `json:"passed"`
    Skipped    bool   `json:"skipped,omitempty"`
    ReportPath string `json:"report_path"`
}
```

---

### Call site: `runVerify`

**File:** `internal/cli/commands.go`, `runVerify`, after config load (~line 410), before `verify.Run` call (~line 419).

Insert:

```go
if skip, err := shouldSkipVerify(cwd, changeDir); err != nil {
    fmt.Fprintf(stderr, "sdd: skip check failed, running verify: %v\n", err)
} else if skip {
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

When `shouldSkipVerify` returns an error, emit the warning to stderr and fall through to run verify normally. Do not return the error.

---

### Source filter rule

The filter in step 4 above:

```go
// filterSourceFiles removes openspec/ paths — changes to SDD artifacts
// must not prevent verify from being skipped.
func filterSourceFiles(files []string) []string {
    out := make([]string, 0, len(files))
    for _, f := range files {
        if !strings.HasPrefix(f, "openspec/") {
            out = append(out, f)
        }
    }
    return out
}
```

Location: same file, just above `shouldSkipVerify`. Called from step 4 of `shouldSkipVerify`.

---

### Files changed — Feature 1

| File | Change |
|------|--------|
| `internal/cli/commands.go` | Add `filterSourceFiles`, `shouldSkipVerify`; insert skip call in `runVerify`; add `Skipped` field to output struct |

No changes to `internal/verify/verify.go`, `internal/state/types.go`, or any other file.

---

## Feature 2: Concurrent Assembly

### New function: `ReadyPhases`

**File:** `internal/state/state.go`, after `nextReady` (currently ends at line 107).

```go
// ReadyPhases returns all pending phases whose prerequisites are all completed.
// In the normal pipeline this is at most one phase, except during the
// spec+design parallel window where it returns [PhaseSpec, PhaseDesign].
// Returns nil if nothing is ready (pipeline complete or no phase is unblocked).
func ReadyPhases(s *State) []Phase
```

**Logic:**

```
var ready []Phase
for _, p := range AllPhases() {
    if s.Phases[p] != StatusPending {
        continue
    }
    prereqsMet := true
    for _, req := range prerequisites[p] {
        if s.Phases[req] != StatusCompleted {
            prereqsMet = false
            break
        }
    }
    if prereqsMet {
        ready = append(ready, p)
    }
}
return ready
```

Iteration is over `AllPhases()` which preserves pipeline order. Result order is therefore deterministic: `[PhaseSpec, PhaseDesign]` when both are unblocked (propose completed, neither yet completed), because `AllPhases()` returns spec before design.

**No new imports.** Uses only `AllPhases()`, `StatusPending`, `StatusCompleted`, `prerequisites` — all already in scope.

---

### New function: `AssembleConcurrent`

**File:** `internal/context/context.go`, after `Assemble` (currently ends at line 93).

```go
// AssembleConcurrent assembles multiple phases concurrently and writes results
// to w in the order of the phases slice (deterministic).
// len(phases)==0: returns nil immediately.
// len(phases)==1: delegates to Assemble (no goroutine overhead).
// len(phases)>1: one goroutine per phase, each assembles into its own
//               bytes.Buffer; after all goroutines complete, buffers are
//               written to w in slice order.
// First error encountered (any goroutine) is returned; remaining errors discarded.
func AssembleConcurrent(w io.Writer, phases []state.Phase, p *Params) error
```

**Internal result type (unexported, local to AssembleConcurrent):**

```go
type result struct {
    buf bytes.Buffer
    err error
}
```

**Implementation outline:**

```
if len(phases) == 0 {
    return nil
}
if len(phases) == 1 {
    return Assemble(w, phases[0], p)
}

results := make([]result, len(phases))
var wg sync.WaitGroup

sem := make(chan struct{}, 4) // semaphore; cap 4 for forward-compat

for i, phase := range phases {
    wg.Add(1)
    go func(idx int, ph state.Phase) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        results[idx].err = Assemble(&results[idx].buf, ph, p)
    }(i, phase)
}

wg.Wait()

// Write in slice order; return first error.
for i := range results {
    if results[i].err != nil {
        return results[i].err
    }
    w.Write(results[i].buf.Bytes())
}
return nil
```

Each goroutine writes to `results[i].buf` exclusively. No shared mutable state between goroutines. The shared `io.Writer w` is only written after `wg.Wait()`, sequentially.

**New import:** add `"sync"` to the import block in `context.go`.

**Cache safety:** `tryCachedContext` and `saveContextCache` operate on per-phase files (`<changeDir>/<phase>.ctx`). Since spec and design are different phases, no two goroutines write the same file. Safe for concurrent execution.

**Metrics:** each phase calls `emitMetrics` via `Assemble`; `emitMetrics` writes to `p.Stderr`. Concurrent writes to `p.Stderr` may interleave lines but this is acceptable (stderr is diagnostic, not machine-parsed output).

---

### Call site change: `runContext`

**File:** `internal/cli/commands.go`, `runContext`, phase dispatch (~line 191).

Current code:

```go
var phase state.Phase
if len(args) >= 2 {
    phase = state.Phase(args[1])
} else {
    phase = st.CurrentPhase
}

// ...build p...

if err := sddctx.Assemble(stdout, phase, p); err != nil {
    return errs.WriteError(stderr, "context", err)
}
```

New code:

```go
phaseExplicit := len(args) >= 2
var phase state.Phase
if phaseExplicit {
    phase = state.Phase(args[1])
}

// ...build p (unchanged)...

if phaseExplicit {
    if err := sddctx.Assemble(stdout, phase, p); err != nil {
        return errs.WriteError(stderr, "context", err)
    }
} else {
    ready := state.ReadyPhases(st)
    switch len(ready) {
    case 0:
        return errs.WriteError(stderr, "context", fmt.Errorf("no ready phase; pipeline may be complete"))
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

The `case 0` branch replaces the silent behavior of the old code when `st.CurrentPhase` is `""` (pipeline complete). It now returns a clear error instead of passing `""` to `Assemble`.

**No new imports in commands.go.** `state` and `sddctx` already imported.

---

### Files changed — Feature 2

| File | Change |
|------|--------|
| `internal/state/state.go` | Add `ReadyPhases(*State) []Phase` after `nextReady` |
| `internal/context/context.go` | Add `AssembleConcurrent`, add `"sync"` to imports |
| `internal/cli/commands.go` | Replace phase dispatch in `runContext` |

---

## Tests

### `TestShouldSkipVerify_Passed`

**File:** `internal/cli/commands_test.go` (new test, or append to existing cli test file).

**Setup:**
- `t.TempDir()` as project root
- `git init` in project root
- Write `openspec/changes/test-change/verify-report.md` containing `**Status:** PASSED`
- No staged or unstaged changes (empty working tree after init + commit of report file)

**Assert:** `shouldSkipVerify(projectDir, changeDir)` returns `(true, nil)`.

---

### `TestShouldSkipVerify_Failed`

**Setup:** same git repo, but `verify-report.md` contains `**Status:** FAILED`.

**Assert:** returns `(false, nil)`.

---

### `TestShouldSkipVerify_ChangedFiles`

**Setup:** git repo with PASSED report; also write a new `.go` file that is not staged (untracked or modified — `git diff HEAD` sees it).

**Assert:** returns `(false, nil)`.

---

### `TestShouldSkipVerify_MissingReport`

**Setup:** git repo; no `verify-report.md` file at all.

**Assert:** returns `(false, nil)`.

---

### `TestShouldSkipVerify_OpenspecChangesIgnored`

**Setup:** git repo with PASSED report; write/modify a file under `openspec/changes/` (e.g., `openspec/changes/test-change/proposal.md`) — appears in `git diff HEAD`.

**Assert:** returns `(true, nil)` — openspec changes do not block skip.

---

### `TestReadyPhases`

**File:** `internal/state/state_test.go` (append).

Table-driven:

| scenario | state setup | want |
|----------|-------------|------|
| propose complete | `Phases[explore]=completed, Phases[propose]=completed`, all others pending | `[PhaseSpec, PhaseDesign]` |
| spec also complete | above + `Phases[spec]=completed` | `[PhaseDesign]` |
| spec and design complete | above + `Phases[design]=completed` | `[PhaseTasks]` |
| all complete | all `StatusCompleted` | `[]` (nil or empty slice) |
| fresh state | all pending except explore ready | `[PhaseExplore]` |

Assert: `reflect.DeepEqual(got, want)` on the returned slice.

---

### `TestAssembleConcurrent`

**File:** `internal/context/context_test.go` (append).

**Setup:** use `setupFixture(t)` to get a valid `*Params`. Write minimal artifacts so both `AssembleSpec` and `AssembleDesign` can succeed:
- `changeDir/proposal.md`
- `changeDir/specs/spec.md`

Table cases:

| case | phases | expected |
|------|--------|----------|
| empty | `[]Phase{}` | returns nil, w unchanged |
| single | `[]Phase{PhaseSpec}` | delegates to Assemble; output contains "sdd-spec" |
| spec+design | `[]Phase{PhaseSpec, PhaseDesign}` | output contains "sdd-spec" followed by "sdd-design" (spec section appears before design section) |

For the `spec+design` case, assert both sections present and spec section offset < design section offset in the output string (deterministic order).

---

## Invariants

- `shouldSkipVerify` never returns `(true, non-nil error)`.
- `filterSourceFiles` is pure (no I/O).
- `AssembleConcurrent` never writes to `w` until `wg.Wait()` returns.
- `ReadyPhases` result order matches `AllPhases()` order.
- Explicit phase arg (`sdd context <name> spec`) always uses single-phase `Assemble` path, never concurrent.
- Skip path in `runVerify` emits `"skipped": true` in JSON and exits 0. It does not call `verify.Run` or `verify.WriteReport`.
