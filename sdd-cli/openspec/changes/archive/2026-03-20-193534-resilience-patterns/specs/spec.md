# Spec: Resilience Patterns

Change: resilience-patterns
Phase: spec
Date: 2026-03-20

---

## Overview

Four independent improvements to correctness, observability, and fault tolerance. No new dependencies. No breaking changes to existing command output. Implementation order: Feature 2 → Feature 3 → Feature 1 → Feature 4.

---

## Feature 1: Skill-Hash in Cache

### Problem

`inputHash` [cache.go:67] hashes only `phaseInputs` artifact files. SKILL.md is excluded. If a workflow author edits a SKILL.md the cached context for that phase continues serving stale content until TTL expiry or an artifact change. Additionally, `phaseInputs["explore"] == []string{}` so `tryCachedContext` hits the `len(inputs) == 0` guard at line 120 and returns `(nil, false)` immediately — explore is never cached.

### Decisions

- Hash SKILL.md content into `inputHash` alongside artifacts.
- Thread `skillsPath` and `phaseName` into `inputHash`, `tryCachedContext`, and `saveContextCache`.
- Relax the `len(inputs) == 0` short-circuit to also check `skillsPath == ""` so explore gets cached.
- Bump `cacheVersion` from 3 to 4 — invalidates all existing entries, correct because hash format changed.
- No in-memory skill cache; disk I/O on SKILL.md only runs on cache-miss path.

### Spec

#### `internal/context/cache.go`

**`cacheVersion`** — change from `3` to `4`.

**`inputHash` signature** — new parameter order matches proposal exactly:

```go
func inputHash(changeDir string, inputs []string, skillsPath, phaseName string) string
```

After writing the `v%d:` version prefix and before iterating `inputs`, hash the skill file:

```go
if skillsPath != "" && phaseName != "" {
    skillPath := filepath.Join(skillsPath, phaseName, "SKILL.md")
    if data, err := os.ReadFile(skillPath); err == nil {
        fmt.Fprintf(h, "skill:%d:", len(data))
        h.Write(data)
    }
    // Missing SKILL.md: contributes nothing — effectively a cache miss on next skill add.
}
```

**`tryCachedContext` signature** — add `skillsPath`:

```go
func tryCachedContext(changeDir, skillsPath, phase string) ([]byte, bool)
```

Replace the `len(inputs) == 0` guard:

```go
inputs, ok := phaseInputs[phase]
if !ok {
    return nil, false
}
// Skip only when there is truly nothing to hash.
if len(inputs) == 0 && skillsPath == "" {
    return nil, false
}
```

Update the `inputHash` call inside `tryCachedContext`:

```go
currentHash := inputHash(changeDir, inputs, skillsPath, phase)
```

**`saveContextCache` signature** — add `skillsPath`:

```go
func saveContextCache(changeDir, skillsPath, phase string, content []byte) error
```

Replace the `len(inputs) == 0` guard identically to `tryCachedContext`. Update the `inputHash` call:

```go
hash := inputHash(changeDir, inputs, skillsPath, phase)
```

#### `internal/context/context.go`

**`Assemble`** [context.go:63, 90] — pass `p.SkillsPath` and `phaseStr` to both call sites:

```go
if cached, ok := tryCachedContext(p.ChangeDir, p.SkillsPath, phaseStr); ok {
```

```go
_ = saveContextCache(p.ChangeDir, p.SkillsPath, phaseStr, content)
```

`p.SkillsPath` already exists on `Params` [context.go:33]; no struct change needed.

### Files

- `internal/context/cache.go` — `cacheVersion`, `inputHash`, `tryCachedContext`, `saveContextCache`
- `internal/context/context.go` — `Assemble` call sites (lines 63, 90)

### Tests

**`TestInputHashIncludesSkill`** in `cache_test.go` (new file or existing):

- Create a temp dir with a fake SKILL.md and two artifact files.
- Assert: same artifacts + identical SKILL.md → same hash.
- Assert: same artifacts + mutated SKILL.md content → different hash.
- Assert: explore (empty inputs) with skillsPath set → non-empty, deterministic hash.

**`TestExplorePhaseCached`**:

- Call `tryCachedContext` for explore phase with skillsPath set; expect `(nil, false)` (first call, no stored hash).
- Call `saveContextCache` for explore, then `tryCachedContext` again; expect cache hit.

---

## Feature 2: Zombie Detection

### Problem

`runStatus` and `runList` omit `updated_at`. An abandoned change mid-pipeline is indistinguishable from one actively in progress. `State.UpdatedAt` is set reliably by `Advance` [state.go:81] and `NewState` [types.go:54].

### Decisions

- Add `IsStale(threshold time.Duration) bool` to `*State` in `types.go`. Complete changes are never stale.
- Default threshold 24h, hardcoded — no flag.
- Add `updated_at`, `stale`, `stale_hours` to `runStatus` output struct.
- Add `updated_at`, `stale`, `stale_hours` to `runList` `changeInfo` struct.
- `stale_hours` uses `omitempty` — omitted when `stale == false`.

### Spec

#### `internal/state/types.go`

New method on `*State`:

```go
// IsStale reports whether the change has had no activity for longer than threshold.
// Complete changes are never considered stale.
func (s *State) IsStale(threshold time.Duration) bool {
    if s.IsComplete() {
        return false
    }
    return time.Since(s.UpdatedAt) > threshold
}
```

No new imports required (`time` already imported).

#### `internal/cli/commands.go` — `runStatus`

Add three fields to the anonymous output struct after `IsComplete`:

```go
UpdatedAt  time.Time `json:"updated_at"`
Stale      bool      `json:"stale"`
StaleHours float64   `json:"stale_hours,omitempty"`
```

Compute before constructing the struct (after `state.Load`):

```go
const staleThreshold = 24 * time.Hour
stale := st.IsStale(staleThreshold)
staleHours := 0.0
if stale {
    staleHours = time.Since(st.UpdatedAt).Hours()
}
```

Populate in the struct literal:

```go
UpdatedAt:  st.UpdatedAt,
Stale:      stale,
StaleHours: staleHours,
```

#### `internal/cli/commands.go` — `runList`

Add three fields to `changeInfo`:

```go
type changeInfo struct {
    Name         string    `json:"name"`
    CurrentPhase string    `json:"current_phase"`
    Description  string    `json:"description"`
    IsComplete   bool      `json:"is_complete"`
    UpdatedAt    time.Time `json:"updated_at"`
    Stale        bool      `json:"stale"`
    StaleHours   float64   `json:"stale_hours,omitempty"`
}
```

Populate in the loop body:

```go
const staleThreshold = 24 * time.Hour
stale := st.IsStale(staleThreshold)
staleHours := 0.0
if stale {
    staleHours = time.Since(st.UpdatedAt).Hours()
}
changes = append(changes, changeInfo{
    Name:         st.Name,
    CurrentPhase: string(st.CurrentPhase),
    Description:  st.Description,
    IsComplete:   st.IsComplete(),
    UpdatedAt:    st.UpdatedAt,
    Stale:        stale,
    StaleHours:   staleHours,
})
```

`time` is already imported in `commands.go` via `os/exec` transitive use — verify before finalizing. If not present, add `"time"` to imports.

### Files

- `internal/state/types.go` — `IsStale` method
- `internal/cli/commands.go` — `runStatus` output struct + population; `runList` `changeInfo` struct + loop body

### Tests

**`TestIsStale`** in `internal/state/types_test.go`:

Table-driven, three cases:

| case | UpdatedAt | IsComplete() precondition | threshold | expected |
|------|-----------|--------------------------|-----------|----------|
| fresh | `time.Now().Add(-30 * time.Minute)` | false | 1h | false |
| stale | `time.Now().Add(-25 * time.Hour)` | false | 24h | true |
| complete | `time.Now().Add(-48 * time.Hour)` | true (all phases StatusCompleted) | 24h | false |

Set `UpdatedAt` directly via struct literal; call `IsStale`.

**`TestRunStatus_StaleFields`** and **`TestRunList_StaleFields`** in `internal/cli/cli_test.go` or new `commands_test.go`:

- Write a `state.json` with `updated_at` set to 48h ago and `current_phase` not archive.
- Invoke `runStatus`/`runList` with a temp dir; decode JSON; assert `"stale": true` and `stale_hours >= 48`.

---

## Feature 3: Partial-Failure Accumulator

### Problem

`AssembleConcurrent` drain loop [context.go:148–153] returns on the first error, discarding successful results from subsequent phases. In `runContext`, during the spec+design parallel window, a spec failure silently drops already-assembled design context.

### Decisions

- Write all successful buffers to `w` before returning.
- Collect all phase errors; return a combined error string after all writes.
- No structured error type (no caller currently branches on error kind).
- Import `"strings"` in `context.go` — confirmed not currently in the import block [context.go:11–22].

### Spec

#### `internal/context/context.go`

Replace the drain loop [context.go:148–153]:

```go
// Write successful results in input order; accumulate errors.
var errs []string
for i, r := range results {
    if r.err != nil {
        errs = append(errs, fmt.Sprintf("%s: %v", phases[i], r.err))
        continue
    }
    w.Write(r.data)
}
if len(errs) > 0 {
    return fmt.Errorf("%d/%d phases failed: %s",
        len(errs), len(phases), strings.Join(errs, "; "))
}
return nil
```

Add `"strings"` to the import block.

**Caller behavior note**: `runContext` [commands.go:199] calls `errs.WriteError` on the returned error, which writes a JSON envelope to stderr. Partial stdout content has already been written at that point. This is intentional — partial context is better than none.

### Files

- `internal/context/context.go` — `AssembleConcurrent` drain loop, `"strings"` import

### Tests

**`TestAssembleConcurrent_PartialFailure`** in `internal/context/context_test.go`:

Table-driven, three cases using stub assemblers:

| case | phase 0 | phase 1 | expected writer content | expected error |
|------|---------|---------|------------------------|----------------|
| first fails, second succeeds | error | "phase1-output" | "phase1-output" | contains "phase1:" and "1/2 phases failed" |
| both fail | error | error | "" (empty) | contains "2/2 phases failed" |
| both succeed | "phase0-output" | "phase1-output" | "phase0-outputphase1-output" | nil |

Use hand-written stub `Assembler` funcs that write known strings or return errors.

---

## Feature 4: `sdd health` Command

### Problem

`loadPipelineMetrics` and `writePipelineSummary` are unexported. `writePipelineSummary` has no callers (confirmed by grep). There is no command to get a structured health summary — completed phases, cache efficiency, staleness, token budget — without invoking Claude.

### Decisions

- Export `LoadPipelineMetrics`, `PipelineMetrics`, `PhaseMetrics` from `internal/context/cache.go` using type aliases (zero-cost).
- Remove `writePipelineSummary` (dead code; superseded by `sdd health`).
- New `runHealth` in `internal/cli/commands.go` — zero-token, pure Go.
- Wire in `cli.go`: routing switch, `printHelp`, `commandHelp`.
- Verify-report parsing: look for `**Status:** PASSED` (same sentinel as `shouldSkipVerify` [commands.go:638]).

### Spec

#### `internal/context/cache.go`

Add type aliases after the `phaseMetrics` type definition:

```go
// PipelineMetrics is the exported type for health consumers.
type PipelineMetrics = pipelineMetrics

// PhaseMetrics is the exported per-phase metrics type.
type PhaseMetrics = phaseMetrics

// LoadPipelineMetrics reads .cache/metrics.json for changeDir.
// Returns an empty struct (not nil) when the file does not exist.
func LoadPipelineMetrics(changeDir string) *PipelineMetrics {
    return loadPipelineMetrics(changeDir)
}
```

Remove `writePipelineSummary` [cache.go:310–324] entirely.

#### `internal/cli/commands.go` — new `runHealth`

```go
func runHealth(args []string, stdout, stderr io.Writer) error {
    if len(args) < 1 {
        return errs.Usage("usage: sdd health <name>")
    }
    name := args[0]

    changeDir, err := resolveChangeDir(name)
    if err != nil {
        return errs.WriteError(stderr, "health", err)
    }

    statePath := filepath.Join(changeDir, "state.json")
    st, err := state.Load(statePath)
    if err != nil {
        return errs.WriteError(stderr, "health", fmt.Errorf("load state: %w", err))
    }

    pm := sddctx.LoadPipelineMetrics(changeDir)

    // Count completed phases.
    phasesCompleted := 0
    allPhases := state.AllPhases()
    for _, p := range allPhases {
        if st.Phases[p] == state.StatusCompleted {
            phasesCompleted++
        }
    }

    // Cache hit rate.
    hitRate := 0.0
    totalCalls := pm.CacheHits + pm.CacheMisses
    if totalCalls > 0 {
        hitRate = float64(pm.CacheHits) / float64(totalCalls)
    }

    // Staleness.
    const staleThreshold = 24 * time.Hour
    stale := st.IsStale(staleThreshold)
    hoursSince := time.Since(st.UpdatedAt).Hours()

    // Verify status.
    verifyStatus := "unknown"
    reportPath := filepath.Join(changeDir, "verify-report.md")
    if data, err := os.ReadFile(reportPath); err == nil {
        if strings.Contains(string(data), "**Status:** PASSED") {
            verifyStatus = "passed"
        } else {
            verifyStatus = "failed"
        }
    }

    // Warnings.
    var warnings []string
    if stale {
        warnings = append(warnings, fmt.Sprintf("no activity in >%.0fh", staleThreshold.Hours()))
    }
    if verifyStatus == "failed" {
        warnings = append(warnings, "last verify failed")
    }
    if warnings == nil {
        warnings = []string{}
    }

    out := struct {
        Command              string    `json:"command"`
        Status               string    `json:"status"`
        Change               string    `json:"change"`
        CurrentPhase         string    `json:"current_phase"`
        PhasesCompleted      int       `json:"phases_completed"`
        PhasesTotal          int       `json:"phases_total"`
        CacheHits            int       `json:"cache_hits"`
        CacheMisses          int       `json:"cache_misses"`
        CacheHitRate         float64   `json:"cache_hit_rate"`
        TotalTokensEstimated int       `json:"total_tokens_estimated"`
        LastActivity         time.Time `json:"last_activity"`
        StaleHours           float64   `json:"stale_hours"`
        Stale                bool      `json:"stale"`
        VerifyStatus         string    `json:"verify_status"`
        Warnings             []string  `json:"warnings"`
    }{
        Command:              "health",
        Status:               "ok",
        Change:               name,
        CurrentPhase:         string(st.CurrentPhase),
        PhasesCompleted:      phasesCompleted,
        PhasesTotal:          len(allPhases),
        CacheHits:            pm.CacheHits,
        CacheMisses:          pm.CacheMisses,
        CacheHitRate:         hitRate,
        TotalTokensEstimated: pm.TotalTokens,
        LastActivity:         st.UpdatedAt,
        StaleHours:           hoursSince,
        Stale:                stale,
        VerifyStatus:         verifyStatus,
        Warnings:             warnings,
    }

    data, _ := json.MarshalIndent(out, "", "  ")
    fmt.Fprintln(stdout, string(data))
    return nil
}
```

`time` import: verify already present in `commands.go` — it is not currently imported (no `"time"` in the import block [commands.go:3–19]). Add `"time"` to imports.

#### `internal/cli/cli.go` — routing

Add before `default`:

```go
case "health":
    return runHealth(rest, stdout, stderr)
```

Add to `printHelp` under "Inspection commands":

```
  health <name>     Show pipeline health: phases, cache stats, staleness, tokens
```

Add to `commandHelp`:

```go
"health": `sdd health — Pipeline health summary

Usage: sdd health <name>

Reads state.json and .cache/metrics.json, checks verify-report.md.
Prints JSON with phase completion, cache hit rate, token estimate,
time since last activity, and staleness warnings.

This is a zero-token operation — runs entirely in Go.

Arguments:
  name          Change name

Output: JSON health summary to stdout.
Exit:   0 success, 1 error, 2 usage`,
```

### Files

- `internal/context/cache.go` — add `PipelineMetrics`, `PhaseMetrics` aliases, `LoadPipelineMetrics`; remove `writePipelineSummary`
- `internal/cli/commands.go` — add `runHealth`; add `"time"` to imports
- `internal/cli/cli.go` — routing switch, `printHelp`, `commandHelp`

### Tests

**`TestRunHealth`** in `internal/cli/` (new `commands_test.go` or existing `cli_test.go`):

Four sub-cases using `t.TempDir()`:

1. **happy path**: write `state.json` (fresh `updated_at`, one phase completed) + `metrics.json` (2 hits, 1 miss, 1000 tokens); invoke `runHealth`; decode JSON; assert `phases_completed == 1`, `cache_hits == 2`, `cache_misses == 1`, `total_tokens_estimated == 1000`, `stale == false`, `warnings` empty array.

2. **stale**: write `state.json` with `updated_at` set to 48h ago, `current_phase` not archive; assert `stale == true` and `warnings` contains the no-activity string.

3. **verify-passed**: write `verify-report.md` containing `**Status:** PASSED`; assert `verify_status == "passed"`.

4. **missing metrics**: write only `state.json`, no `.cache/` dir; assert zero `cache_hits`, zero `cache_misses`, zero `total_tokens_estimated`, no error.

---

## Implementation Notes

### Dependency order

Implement in this order to avoid compilation failures mid-PR:

1. **Feature 2** — `IsStale` method on `*State` (no external deps).
2. **Feature 3** — `AssembleConcurrent` drain loop + `"strings"` import (self-contained).
3. **Feature 1** — `inputHash` signature change; update `tryCachedContext`, `saveContextCache`, `Assemble` callers together (all in two files; must be done atomically).
4. **Feature 4** — exports in `cache.go`; `runHealth`; CLI wiring (depends on Feature 1 exports being finalized and Feature 2's `IsStale` being present).

### `time` import in `commands.go`

`commands.go` currently does not import `"time"` [verified: import block lines 3–19 has no time import]. Features 2 and 4 both add `time.Duration`/`time.Since`/`time.Time` usages. Add once when implementing Feature 2; Feature 4 reuses it.

### `cacheVersion` and metrics version field

`loadPipelineMetrics` [cache.go:299] rejects stored metrics whose `Version != cacheVersion`. After bumping `cacheVersion` to 4, any existing `metrics.json` with `"version": 3` will be discarded and replaced with a fresh empty struct on next `recordMetrics` call. This is intentional and safe.

### `stale_hours` precision

Output as `float64`. Tests should assert `>= expected_hours` rather than exact equality to avoid flakiness from test execution time.

---

## Out of Scope

- In-memory skill cache (`sync.Map`): disk I/O on SKILL.md is sufficient; skills are small and rarely change.
- Configurable staleness threshold: 24h hardcoded; add a flag when a concrete use case emerges.
- Structured `AssembleErrors` type: no caller branches on error kind.
- `sdd health --json` flag: JSON is the only output format; no flag needed.
