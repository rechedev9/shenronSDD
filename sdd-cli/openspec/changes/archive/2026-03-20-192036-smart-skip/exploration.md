# Exploration: smart-skip, slog, bounded concurrency

Change: smart-skip
Date: 2026-03-20

---

## Feature 1: Smart-skip verify

### Goal

If no source files changed since the last successful verify pass, skip
re-running build/lint/test and reuse the existing `verify-report.md`.

### What exists

**`internal/cli/commands.go` — `runVerify` (lines 387–455)**

- Loads config, builds `[]verify.CommandSpec`, calls `verify.Run(...)`, writes
  report, returns JSON.
- No skip logic anywhere in this path.
- `state.BaseRef` (SHA captured at `sdd new`) is already persisted to
  `state.json` — the machinery for comparing a git ref is in place.
- `gitDiffFiles(dir, ref)` (line 572) already shells out to
  `git diff --name-only <ref>` and returns a slice of changed files.
  That helper is private to `commands.go` and reusable as-is.

**`internal/verify/verify.go` — `WriteReport` (lines 170–221)**

- Writes `verify-report.md` atomically (tmp + rename).
- The report file's mtime is therefore the canonical timestamp of the last
  verify run.
- No skip/metadata concept exists here — `verify.Run` always executes.

**`internal/state/state.go` — `State.BaseRef` (via `runNew`, line 118–121)**

- SHA is recorded at change-creation time, not at verify time.
- There is no "last-verified-at-ref" field — that needs to be added, or we
  rely on mtime instead.

### What needs to change

**Option A — git-diff against HEAD (simpler, more correct)**

1. In `runVerify`, after loading config but before calling `verify.Run`:
   a. Read `verify-report.md` mtime — if it doesn't exist, proceed normally.
   b. Run `git diff --stat HEAD -- <source extensions>` (or reuse
      `gitDiffFiles` with `"HEAD"` as ref). Filter for source files
      (`.go`, configured extensions) vs SDD artifacts (`openspec/`).
   c. If zero source files changed since the report was written, emit a
      skip message and return the cached JSON with `"skipped": true`.
2. No changes needed to `verify.go` — skip fires before `verify.Run` is
   called.

**Option B — mtime comparison**

- Stat every `.go` file in the project, compare newest mtime to
  `verify-report.md` mtime.
- Simpler than git but misses deleted files and is unreliable in CI
  (checkouts reset mtimes).
- Prefer Option A.

**Implementation location:** `internal/cli/commands.go:runVerify`, lines
387–455. New private helper `shouldSkipVerify(cwd, changeDir string) bool`
mirrors discrawl's `shouldSkipChannelSync` pattern (reads a sentinel, shells
git, returns bool).

**New fields needed (optional, for correctness):**
- `state.VerifiedRef string` — SHA at which last verify passed. Allows
  comparing `git diff <verified-ref> HEAD` rather than relying on mtime.
  Add to `internal/state/types.go`. Not strictly required for MVP (mtime
  works for local dev).

### Exact lines to add/change

| File | Line(s) | Change |
|------|---------|--------|
| `internal/cli/commands.go` | after 410 (post-config-load) | insert `shouldSkipVerify` call |
| `internal/cli/commands.go` | ~589 | add `shouldSkipVerify(cwd, changeDir string) (bool, error)` func |
| `internal/state/types.go` | `State` struct | add `VerifiedRef string` (optional) |

### Risk

- Low. Skip logic is gated on a positive check; false negatives (running
  verify unnecessarily) cannot cause correctness issues.
- `git diff --stat HEAD` is fast (<50ms) on normal repos.
- Deleted source files: `git diff --stat HEAD` reports deletions, so a
  deleted `.go` file correctly triggers re-verify.
- CI: git is always available in CI; mtime-based fallback not needed.
- Edge case: if `verify-report.md` exists but was from a failed run, skip
  should only fire when the report shows `**Status:** PASSED`. Need to read
  first line of report or add a status field to avoid skipping on a prior
  failure. Check for `PASSED` string in report before skipping.

---

## Feature 2: Structured slog logging

### Goal

Replace ad-hoc `fmt.Fprintf(stderr/progress, "sdd: ...")` calls with
`log/slog` structured logging. Machine-parseable; consistent key=value
attributes.

### What exists

**All `fmt.Fprintf` calls that produce human log lines (not JSON output):**

| File | Line | Current |
|------|------|---------|
| `internal/verify/verify.go` | 87 | `"sdd: verify %s...\n"` |
| `internal/verify/verify.go` | 95 | `"sdd: verify %s: ok (%s)\n"` |
| `internal/verify/verify.go` | 97 | `"sdd: verify %s: TIMEOUT (%s)\n"` |
| `internal/verify/verify.go` | 99 | `"sdd: verify %s: FAILED (exit %d)\n"` |
| `internal/context/cache.go` | 220–226 | `writeMetrics` — phase/bytes/tokens/ms |
| `internal/context/cache.go` | 318–323 | `writePipelineSummary` — totals |
| `internal/cli/commands.go` | 136 | `"warning: explore context assembly failed: %v\n"` |

**`verify.Run` signature (verify.go:71):**
```go
func Run(workDir string, commands []CommandSpec, timeout time.Duration, progress io.Writer) (*Report, error)
```
`progress io.Writer` is the log sink. All callers pass `stderr` directly.

**`context.Assemble` / `emitMetrics` / `writeMetrics` / `writePipelineSummary`:**
All output via bare `io.Writer` — no structured attributes.

### What needs to change

**Approach:** introduce a project-internal `slog.Logger` instance.

1. Create `internal/log/log.go` — thin wrapper or just exported `New()` that
   returns `*slog.Logger` with a text or JSON handler aimed at an
   `io.Writer`.
   - Text handler for human dev use (`LOG_FORMAT=text`).
   - JSON handler for CI (`LOG_FORMAT=json` or auto-detect TTY).
   - Keep it small: `log/slog` is stdlib in Go 1.21+; no external dep.

2. **`internal/verify/verify.go`:**
   - Change `progress io.Writer` to `logger *slog.Logger` (or add it
     alongside and keep `io.Writer` for backward compat in tests).
   - Replace 4× `fmt.Fprintf(progress, ...)` with:
     ```go
     logger.Info("verify", "cmd", spec.Name, "status", "running")
     logger.Info("verify", "cmd", spec.Name, "status", "ok", "duration", result.Duration)
     logger.Warn("verify", "cmd", spec.Name, "status", "timeout", "limit", timeout)
     logger.Error("verify", "cmd", spec.Name, "status", "failed", "exit_code", result.ExitCode)
     ```

3. **`internal/context/cache.go`:**
   - `writeMetrics(w io.Writer, m *contextMetrics)` → accept `*slog.Logger`.
   - `writePipelineSummary(w io.Writer, ...)` → same.
   - `emitMetrics` in `context.go:96` passes `p.Stderr`; change `Params.Stderr`
     from `io.Writer` to `*slog.Logger` — OR keep `io.Writer` and wrap it at
     call site. Prefer changing `Params.Stderr` to `*slog.Logger` for
     consistency; all callers already pass `stderr`.

4. **`internal/cli/commands.go:136`:**
   - Replace with `logger.Warn("explore context assembly failed", "error", err)`.

### Exact lines to add/change

| File | Lines | Change |
|------|-------|--------|
| `internal/log/log.go` | new file | `New(w io.Writer) *slog.Logger` |
| `internal/verify/verify.go` | 4 (L87,95,97,99) | `fmt.Fprintf` → `logger.Info/Warn/Error` |
| `internal/verify/verify.go` | func sig L71 | `progress io.Writer` → `logger *slog.Logger` |
| `internal/context/cache.go` | `writeMetrics` L214 | `w io.Writer` → `logger *slog.Logger` |
| `internal/context/cache.go` | `writePipelineSummary` L310 | same |
| `internal/context/context.go` | `Params.Stderr` L34 | type `io.Writer` → `*slog.Logger` |
| `internal/context/context.go` | `emitMetrics` L96 | pass logger, not writer |
| `internal/cli/commands.go` | L136 | warning → `logger.Warn` |
| `internal/cli/commands.go` | all `runVerify`/`runNew` callers | thread logger through |

### Risk

- Medium. `Params.Stderr io.Writer` is used in ~8 call sites (all in
  `commands.go`). Each needs a logger constructed from the existing `stderr`
  writer. Mechanical change but touching many files.
- Tests: `verify_test.go` passes `nil` for `progress`; needs nil-safe logger
  (use `slog.New(slog.NewTextHandler(io.Discard, nil))` when nil).
- `writePipelineSummary` is defined but never called from `Assemble` — it
  is dead code. Verify before converting (line 310 in cache.go, no callers
  found via grep). Can be converted anyway for completeness or noted as
  cleanup.
- Go version: `log/slog` requires Go 1.21+. Check `go.mod`.

---

## Feature 3: Bounded concurrent assembly

### Goal

When the dispatcher detects that both `spec` and `design` phases could run
(propose is complete, neither is complete), assemble their contexts
concurrently rather than sequentially.

### What exists

**`internal/state/state.go` — `nextReady` (line 90)**

- Returns only one phase at a time (first ready phase in `AllPhases()`
  order).
- `spec` comes before `design` in that order — design is never returned
  until spec is complete, even though they are truly parallel per the graph.

**`internal/context/context.go` — `Assemble` (line 52)**

- Single-phase entry point: takes one `state.Phase`, runs one assembler,
  writes to one `io.Writer`.
- No concurrency; entirely sequential.
- Buffer pattern already exists: assembles into `bytes.Buffer`, then writes
  to `w`. This makes concurrent assembly easy — two goroutines, two buffers,
  merge after `WaitGroup.Wait()`.

**`internal/cli/commands.go` — callers of `Assemble`**

- `runNew` (line 134): calls `Assemble` for `PhaseExplore` only.
- `runContext` (line 191): calls `Assemble` for a single resolved phase.
- Neither caller currently handles multi-phase dispatch.

**`internal/state/state.go` — `prerequisites` (line 38)**

- `PhaseSpec` requires `{PhasePropose}`.
- `PhaseDesign` requires `{PhasePropose}`.
- Both require only propose; tasks requires both spec AND design.
- The parallel window is: propose completed, tasks not yet started.

### What needs to change

The natural place for concurrent assembly is a new function in
`internal/context/context.go` (not in the CLI layer):

```go
// AssembleConcurrent assembles all ready phases concurrently.
// phases must be independent (no shared writer).
// Returns a map[Phase][]byte of assembled content.
func AssembleConcurrent(phases []state.Phase, p *Params) (map[state.Phase][]byte, error)
```

Or, since the CLI's `runContext` uses a single stdout writer, a simpler
approach: the CLI detects the parallel window, calls `Assemble` twice with
two goroutines, collects two `bytes.Buffer` results, writes them sequentially
to stdout.

**Concrete plan:**

1. Add `ReadyPhases(s *State) []Phase` to `internal/state/state.go` —
   returns all phases whose prerequisites are met and which are still
   pending. Currently `nextReady` returns only the first; this returns all.

2. In `internal/context/context.go`, add:
   ```go
   func AssembleAll(w io.Writer, phases []state.Phase, p *Params) error {
       if len(phases) <= 1 {
           // fast path: no concurrency needed
           return Assemble(w, phases[0], p)
       }
       type result struct {
           phase state.Phase
           data  []byte
           err   error
       }
       sem := make(chan struct{}, 4) // bounded: sag pattern
       results := make([]result, len(phases))
       var wg sync.WaitGroup
       for i, ph := range phases {
           wg.Add(1)
           go func(i int, ph state.Phase) {
               defer wg.Done()
               sem <- struct{}{}
               defer func() { <-sem }()
               var buf bytes.Buffer
               err := Assemble(&buf, ph, p)
               results[i] = result{phase: ph, data: buf.Bytes(), err: err}
           }(i, ph)
       }
       wg.Wait()
       for _, r := range results {
           if r.err != nil { return r.err }
           w.Write(r.data)
       }
       return nil
   }
   ```

3. In `internal/cli/commands.go:runContext` (line 191), replace the
   single `sddctx.Assemble(stdout, phase, p)` call with logic that:
   - Calls `state.ReadyPhases(st)` to check if >1 phase is ready.
   - If the requested phase is explicit (arg given), use single-phase path.
   - If using current phase and >1 are ready, use `AssembleAll`.

### Exact lines to add/change

| File | Lines | Change |
|------|-------|--------|
| `internal/state/state.go` | after `nextReady` (~L107) | add `ReadyPhases(*State) []Phase` |
| `internal/context/context.go` | after `Assemble` (~L93) | add `AssembleAll` with goroutine+WaitGroup+sem |
| `internal/cli/commands.go` | `runContext` L191 | check ready phases, call `AssembleAll` when >1 |
| `internal/cli/commands.go` | `runNew` L134 | no change — explore is always single-phase |

### Risk

- Low-medium. The parallelism is bounded (sem cap 4, but in practice
  spec+design is exactly 2 goroutines). Each assembler reads different
  artifacts (spec reads `proposal.md`; design reads `proposal.md` +
  `specs/`). Both are read-only FS operations — no write contention.
- Cache: `tryCachedContext` / `saveContextCache` both do `os.WriteFile` to
  separate per-phase files (`spec.ctx`, `design.ctx`). No shared file.
  Safe.
- `bytes.Buffer` per goroutine — no shared writer. Safe.
- Output order: `AssembleAll` must write results in deterministic order
  (slice order) after `wg.Wait()`, not as they complete, to avoid
  interleaved stdout. The plan above does this.
- The `runContext` phase-resolution path (lines 175–179) needs to handle the
  case where `state.ReadyPhases` returns `[spec, design]` but the user
  explicitly asked for just one. Explicit arg → single phase; auto → all ready.
- Test: existing `context_test.go` tests `Assemble`; `AssembleAll` needs its
  own table test with a parallel case.

---

## Cross-cutting notes

- All three features touch `internal/cli/commands.go`. Plan changes in that
  file together to avoid merge conflicts within the same change.
- Feature 2 (slog) changes `Params.Stderr` type — this is a breaking change
  to the package API. Any external test that constructs `Params` directly
  needs updating. Check `internal/context/context_test.go` before landing.
- Feature 3 relies on Feature 1's `shouldSkipVerify` only indirectly; they
  are independent at the code level.
- Go version gate: confirm `go.mod` minimum is ≥ 1.21 for `log/slog`
  (stdlib since 1.21). `sync.WaitGroup` + buffered channels are stdlib since
  1.0 — no gate needed for Feature 3.
