# Exploration: Resilience Patterns

Change: resilience-patterns
Date: 2026-03-20

---

## Feature 1: Skill-Hash Caching

### Current State

`loadSkill` [context.go:159–166] reads the SKILL.md unconditionally on every `sdd context` call:

```go
func loadSkill(skillsPath, phaseName string) ([]byte, error) {
    path := filepath.Join(skillsPath, phaseName, "SKILL.md")
    data, err := os.ReadFile(path)
    ...
}
```

Every assembler (AssembleExplore, AssemblePropose, etc.) calls `loadSkill` as its first step. For `sdd context <name>` — which users run once per phase, sometimes many times during a session — this is a fresh disk read every time. SKILL.md files are static during a project session; they only change when the workflow author updates the skill library.

The existing cache system [cache.go] already handles phase-context caching via `<phase>.hash` + `<phase>.ctx` files under `.cache/`. The hash key covers *input artifacts* (exploration.md, proposal.md, etc.) but SKILL.md is not included in `phaseInputs` — it is read raw every assembly, before the cache check in `Assemble()`.

Actually: looking more carefully at the flow, `Assemble()` [context.go:53–94] does call `tryCachedContext` first. If the cache hits, `fn(&buf, p)` is never called, so `loadSkill` is skipped. The cost is only paid on cache misses. The claim "every sdd context re-reads SKILL.md" holds only when cache misses occur (TTL expiry, first run, artifact change).

The real gap: if the user is at `explore` phase (which has no `phaseInputs` — `"explore": {}`), `tryCachedContext` returns `(nil, false)` immediately [cache.go:119–122] because `len(inputs) == 0`. So explore assembly always re-reads SKILL.md. Similarly, any cache miss re-reads the skill.

### What to Add

A separate `skills.hash` file at `.cache/skills.hash` mapping `"<skillName>:<sha256hex>"`. `loadSkill` would:
1. Stat the SKILL.md mtime or hash it once per process lifetime (in-memory map, keyed by path).
2. Return cached bytes if hash unchanged.

Simpler approach that fits the existing pattern: add skill names to `phaseInputs` as a virtual entry (like `"specs/"` today), so changing any SKILL.md invalidates the phase cache. This is actually already implicit — if SKILL.md changes and the phase cache is still valid by hash, the old SKILL.md content is in the cached `.ctx` file. This is a correctness bug independent of performance.

Cleanest fix: include the SKILL.md path in `inputHash` computation for each phase. Add `"skill:<phaseName>"` as a virtual key that hashes the skill file from `skillsPath`.

### Files Affected

- `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/cache.go` — `phaseInputs` map (add skill hash logic), `inputHash` function (handle `"skill:"` sentinel)
- `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go` — `loadSkill` (optionally add in-memory cache for the process lifetime)

### Risk

Low. The cache is best-effort; failures are silently ignored. The main risk is the `Params.SkillsPath` is not currently threaded into `inputHash` — it is on `Params` but `inputHash(changeDir, inputs)` only takes `changeDir`. Either `inputHash` needs `skillsPath` added, or `loadSkill` needs a process-level sync.Map cache. The process-level cache approach is simpler and avoids changing the `inputHash` signature.

---

## Feature 2: Zombie Change Detection

### Current State

`State` [state/types.go:32–40] has `UpdatedAt time.Time` which is set in `NewState` [types.go:43–57] and updated by `state.Save`. The field is present in JSON.

`runStatus` [commands.go:286–342] reads state and emits JSON but does not check staleness. Output struct has: `command`, `status`, `change`, `description`, `current_phase`, `completed`, `phases`, `is_complete`. No `warnings` or `updated_at` field.

`runList` [commands.go:344–399] scans all change dirs, loads state, emits a `changeInfo` struct with `name`, `current_phase`, `description`, `is_complete`. No staleness check, no `updated_at` in output.

### What to Add

A staleness predicate — analogous to RepoBar's `isStale(now, interval)`:

```go
func isZombie(st *state.State, threshold time.Duration) bool {
    return !st.IsComplete() && time.Since(st.UpdatedAt) > threshold
}
```

Threshold: 24h.

For `runStatus`: add `updated_at` and `warnings []string` to the output struct. Append `"change not updated in >24h — may be abandoned"` when `isZombie`.

For `runList`: add `updated_at` and `stale bool` to `changeInfo`. Writers of CI dashboards can act on `stale: true`.

### Files Affected

- `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go` — `runStatus` (output struct + zombie check), `runList` (changeInfo struct + zombie check)
- `/home/reche/projects/SDDworkflow/sdd-cli/internal/state/types.go` — optionally add `IsZombie(threshold) bool` method on `*State`

### Risk

Low. Read-only change. The only coupling is on `UpdatedAt` — verified present in `State` struct at `types.go:38`. Must confirm `state.Save` stamps `UpdatedAt` on every write (need to verify `state.Save` implementation). No behavior change, only added output fields and stderr warning.

---

## Feature 3: Partial-Failure Accumulator

### Current State

`AssembleConcurrent` [context.go:119–156] uses a `[]result` slice to collect goroutine outputs (data+err), then in the drain loop [lines 148–153] returns on first error:

```go
for i, r := range results {
    if r.err != nil {
        return fmt.Errorf("assemble %s: %w", phases[i], r.err)
    }
    w.Write(r.data)
}
```

All goroutines always run to completion (the `wg.Wait()` at line 145 ensures this). The issue is only in the drain: the first error discards results from all subsequent phases even if they succeeded.

In practice today, `AssembleConcurrent` is only called for the `spec+design` parallel window [commands.go:199]. If spec fails but design succeeds, the design context is silently dropped.

### What to Add

Collect all errors instead of returning on first:

```go
var errs []string
for i, r := range results {
    if r.err != nil {
        errs = append(errs, fmt.Sprintf("%s: %v", phases[i], r.err))
        continue
    }
    w.Write(r.data)
}
if len(errs) > 0 {
    return fmt.Errorf("concurrent assembly: %d phase(s) failed:\n%s",
        len(errs), strings.Join(errs, "\n"))
}
```

The successful phases' content is still written to `w`. The caller gets a multi-error that names which phases failed.

Optionally wrap the errors with a structured type for programmatic inspection:

```go
type AssembleErrors struct {
    Failed    []string // phase names
    Succeeded []string // phase names
    Errs      []error
}
func (e *AssembleErrors) Error() string { ... }
```

This matches the RepoBar `RepoErrorAccumulator` pattern.

### Files Affected

- `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go` — `AssembleConcurrent` drain loop (lines 148–153), possibly add `AssembleErrors` type
- `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/errs/` — may need to expose `IsUsage` check or similar for the new error type (check this package)

### Risk

Low-medium. Behavior change: currently a single phase failure returns no output; after this change, partial output is written to `w` before the error is returned. Callers (only `runContext` [commands.go:199]) pipe `w` to stdout. Claude receives partial context on failure — which is better than nothing but is a semantic change. The `errs.WriteError` call in `runContext` would still print the error summary. Need to verify callers handle partial stdout correctly.

---

## Feature 4: Pipeline Health Summary (`sdd health`)

### Current State

`pipelineMetrics` [cache.go:230–244] is already accumulated per-change in `.cache/metrics.json`:

```go
type pipelineMetrics struct {
    Version     int                     `json:"version"`
    Phases      map[string]phaseMetrics `json:"phases"`
    TotalBytes  int                     `json:"total_bytes"`
    TotalTokens int                     `json:"total_tokens"`
    CacheHits   int                     `json:"cache_hits"`
    CacheMisses int                     `json:"cache_misses"`
}
```

`loadPipelineMetrics` [cache.go:289–307] reads it. `writePipelineSummary` [cache.go:310–324] prints a one-liner to stderr but is never called (dead code — no callers in the codebase).

`state.json` has `UpdatedAt`, `CurrentPhase`, `Phases` (map of phase → status).

`verify-report.md` exists in the change dir if verify has run; contains `**Status:** PASSED` or `FAILED`.

The `sdd health` command would be zero-token (pure Go, no Claude calls) — consistent with `verify` and `archive`.

### What to Add

New `runHealth(args []string, stdout, stderr io.Writer) error` in `commands.go`:

1. Load `state.json` → completed phases count, current phase, zombie check.
2. Load `.cache/metrics.json` → total tokens, cache hit rate.
3. Check `verify-report.md` → pass/fail status.
4. Compute time since `UpdatedAt`.
5. Emit JSON:

```json
{
  "command": "health",
  "status": "ok",
  "change": "my-feature",
  "phases_completed": 4,
  "phases_total": 10,
  "current_phase": "apply",
  "cache_hit_rate": 0.75,
  "total_tokens_estimated": 48200,
  "last_activity": "2026-03-20T19:21:44Z",
  "hours_since_activity": 2.3,
  "verify_status": "passed",
  "warnings": []
}
```

Warnings populated from:
- `isZombie`: `"no activity in >24h"`
- verify failed: `"last verify failed"`
- current phase stuck in `in_progress` > N hours: could add later

Wire in `cli.go`: add `case "health":` to the switch, add to `printHelp`, add `commandHelp["health"]`.

### Files Affected

- `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go` — new `runHealth` function
- `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/cli.go` — routing switch + help text
- `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/cache.go` — `loadPipelineMetrics` is already exported-ready (lowercase, but accessible within package); `runHealth` is in `cli` package so needs either: (a) export `loadPipelineMetrics` → `LoadPipelineMetrics`, or (b) add a thin `context.HealthSummary(changeDir) (*PipelineMetrics, error)` accessor

The cleanest approach: export `LoadPipelineMetrics` and `PipelineMetrics`/`PhaseMetrics` from `internal/context`. They are already JSON-serializable structs.

### Risk

Low. Entirely additive. The only non-trivial decision is whether to export `loadPipelineMetrics` from `internal/context` (breaks nothing, adds surface) or create a dedicated accessor. The `writePipelineSummary` function is dead code and can be removed or called from `runHealth`.

---

## Cross-Cutting Notes

1. **`state.Save` stamps `UpdatedAt`** — must verify in `internal/state/` before relying on it for zombie detection. If `Save` does not update `UpdatedAt`, that field is useless.

2. **`errs` package** — `internal/cli/errs/` needs review before adding new error types for Feature 3. The structured `AssembleErrors` type may want to implement the `errs.IsUsage` interface contract.

3. **Test coverage** — all four features have straightforward table-driven test paths. Feature 3 (partial accumulator) needs a test that confirms partial output IS written when one phase fails. Feature 2 needs time manipulation (use a `now func() time.Time` parameter or inject threshold). Feature 1 needs a mock fs or temp dir with a SKILL.md.

4. **`writePipelineSummary` dead code** — should be called somewhere (end of `runContext`?) or removed. Feature 4 effectively supersedes it as the canonical health surface.

5. **Ordering recommendation** — implement in dependency order: Feature 2 (state types, self-contained) → Feature 3 (context package, self-contained) → Feature 1 (context/cache, depends on understanding existing cache) → Feature 4 (CLI, depends on exported cache API from Feature 1).
