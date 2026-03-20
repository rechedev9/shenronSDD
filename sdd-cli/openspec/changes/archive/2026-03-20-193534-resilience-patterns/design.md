# Design: Resilience Patterns

Change: resilience-patterns
Date: 2026-03-20

---

## Overview

Four independent code changes. Implementation order follows dependency edges:
Feature 2 (IsStale) → Feature 3 (partial-failure) → Feature 1 (skill-hash) → Feature 4 (health command).

No new packages. All errors wrapped with `fmt.Errorf(...%w...)`. Tests are table-driven with `t.TempDir()`.

---

## Feature 2: IsStale on *State

### File: `internal/state/types.go`

**Current state:** `State` struct has `UpdatedAt time.Time` (line 38). `IsComplete()` exists (called at commands.go:336 and 354). No staleness predicate.

**Exact addition** — append after the closing brace of `NewState` (after line 57):

```go
// IsStale reports whether a non-complete change has had no activity
// for longer than threshold. Complete changes are never stale.
func (s *State) IsStale(threshold time.Duration) bool {
    if s.IsComplete() {
        return false
    }
    return time.Since(s.UpdatedAt) > threshold
}
```

`time` is already imported (line 3). No other imports needed.

### File: `internal/cli/commands.go`

**`runStatus` output struct** — current struct literal at lines 319–337. Add three fields after `IsComplete bool`:

```go
UpdatedAt  time.Time `json:"updated_at"`
Stale      bool      `json:"stale"`
StaleHours float64   `json:"stale_hours,omitempty"`
```

**`runStatus` populate block** — add before the `out := struct{...}` literal:

```go
const staleThreshold = 24 * time.Hour
stale := st.IsStale(staleThreshold)
staleHours := 0.0
if stale {
    staleHours = time.Since(st.UpdatedAt).Hours()
}
```

**`runStatus` struct initializer** — add to the initializer block:

```go
UpdatedAt:  st.UpdatedAt,
Stale:      stale,
StaleHours: staleHours,
```

**`runList` `changeInfo` struct** (lines 350–355) — add after `IsComplete`:

```go
UpdatedAt  time.Time `json:"updated_at"`
Stale      bool      `json:"stale"`
StaleHours float64   `json:"stale_hours,omitempty"`
```

**`runList` loop body** (lines 376–381) — add staleness locals before the `append`:

```go
listStale := st.IsStale(staleThreshold)
listStaleHours := 0.0
if listStale {
    listStaleHours = time.Since(st.UpdatedAt).Hours()
}
changes = append(changes, changeInfo{
    Name:         st.Name,
    CurrentPhase: string(st.CurrentPhase),
    Description:  st.Description,
    IsComplete:   st.IsComplete(),
    UpdatedAt:    st.UpdatedAt,
    Stale:        listStale,
    StaleHours:   listStaleHours,
})
```

`staleThreshold` is a package-level const in commands.go (defined once, reused by both runStatus and runList). `time` is already imported.

### Tests: `internal/state/types_test.go`

Table-driven, `t.Parallel()`:

| name | UpdatedAt offset | IsComplete | threshold | want IsStale |
|------|-----------------|------------|-----------|--------------|
| fresh_active | -30min | false | 1h | false |
| stale_active | -25h | false | 24h | true |
| complete_old | -48h | true | 1h | false |
| boundary | -1h - 1ns | false | 1h | true |

`IsComplete()` depends on `Phases` map. Set `Phases[PhaseArchive] = StatusCompleted` (and all prior = StatusCompleted) to force `IsComplete() == true`. Inject `UpdatedAt` directly via struct literal after `NewState`.

---

## Feature 3: Partial-Failure Accumulator

### File: `internal/context/context.go`

**Current drain loop** (lines 148–153):

```go
for i, r := range results {
    if r.err != nil {
        return fmt.Errorf("assemble %s: %w", phases[i], r.err)
    }
    w.Write(r.data)
}
return nil
```

**Replacement:**

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
return nil
```

**Import change:** `"strings"` is not in the current import block (lines 11–22 of context.go). Add it to the import block between `"sync"` and `"time"`.

No changes to `AssembleConcurrent`'s signature or to `runContext`.

**Behavioral contract (document in function comment):** All successful phase buffers are written to `w` before the error is returned. Partial output on stdout when some phases fail is intentional — Claude receives maximum available context.

### Tests: `internal/context/assembler_concurrent_test.go`

Table-driven with a stub `Assembler` that either writes fixed bytes or returns a sentinel error:

| name | phase 0 | phase 1 | want writer contains | want err |
|------|---------|---------|---------------------|----------|
| both_succeed | ok "AAA" | ok "BBB" | "AAABBB" | nil |
| first_fails | err | ok "BBB" | "BBB" | contains "phase 0" |
| second_fails | ok "AAA" | err | "AAA" | contains "phase 1" |
| both_fail | err | err | "" | contains "2 phase(s) failed" |

Use `bytes.Buffer` as `w`. Inject phase assemblers via a test-local `dispatchers` override or by calling `AssembleConcurrent` through a wrapper that swaps `dispatchers` for the duration of the test (use `t.Cleanup` to restore).

---

## Feature 1: Skill-Hash Caching

### File: `internal/context/cache.go`

**`cacheVersion`** — bump line 21 from `3` to `4`.

**`inputHash` new signature** (replaces line 67):

```go
func inputHash(changeDir, skillsPath, phaseName string, inputs []string) string
```

**`inputHash` body change** — after `fmt.Fprintf(h, "v%d:", cacheVersion)` (line 72) and before the `sorted` slice construction, insert:

```go
if skillsPath != "" && phaseName != "" {
    skillPath := filepath.Join(skillsPath, phaseName, "SKILL.md")
    if data, err := os.ReadFile(skillPath); err == nil {
        fmt.Fprintf(h, "skill:%d:", len(data))
        h.Write(data)
    }
    // Missing SKILL.md: contribute nothing — safe cache miss on first call.
}
```

`filepath` and `os` are already imported (lines 7–8).

**`tryCachedContext` new signature** (replaces line 118):

```go
func tryCachedContext(changeDir, skillsPath, phase string) ([]byte, bool)
```

**`tryCachedContext` guard change** (replaces lines 119–122):

```go
inputs, ok := phaseInputs[phase]
if !ok {
    return nil, false
}
// Skip only when there is truly nothing to hash (no inputs and no skill).
if len(inputs) == 0 && skillsPath == "" {
    return nil, false
}
```

**`tryCachedContext` `inputHash` call** (replaces line 137):

```go
currentHash := inputHash(changeDir, skillsPath, phase, inputs)
```

**`saveContextCache` new signature** (replaces line 169):

```go
func saveContextCache(changeDir, skillsPath, phase string, content []byte) error
```

**`saveContextCache` guard change** (replaces lines 170–173):

```go
inputs, ok := phaseInputs[phase]
if !ok {
    return nil
}
if len(inputs) == 0 && skillsPath == "" {
    return nil
}
```

**`saveContextCache` `inputHash` call** (replaces line 180):

```go
hash := inputHash(changeDir, skillsPath, phase, inputs)
```

### File: `internal/context/context.go`

**`Assemble` — `tryCachedContext` call** (line 63):

```go
if cached, ok := tryCachedContext(p.ChangeDir, p.SkillsPath, phaseStr); ok {
```

**`Assemble` — `saveContextCache` call** (line 90):

```go
_ = saveContextCache(p.ChangeDir, p.SkillsPath, phaseStr, content)
```

`p.SkillsPath` is already a field on `Params` (context.go line 33). No struct change.

### Tests: `internal/context/cache_test.go`

Table-driven, `t.TempDir()`:

**Test: skill hash invalidates cache**
1. Create `skillsPath/explore/SKILL.md` with content "v1".
2. Call `saveContextCache(changeDir, skillsPath, "explore", []byte("ctx1"))`.
3. Call `tryCachedContext(changeDir, skillsPath, "explore")` — assert hit, content = "ctx1".
4. Overwrite SKILL.md with "v2".
5. Call `tryCachedContext` again — assert miss (hash changed).

**Test: explore phase now cached**
1. Create `skillsPath/explore/SKILL.md`.
2. `saveContextCache` for "explore" — assert no error.
3. `tryCachedContext` — assert hit.

**Test: empty skillsPath + no inputs still skips**
1. Call `tryCachedContext(changeDir, "", "explore")` with no SKILL.md — assert miss.

**Test: non-explore phases unaffected by missing SKILL.md**
1. Write `changeDir/exploration.md`.
2. `saveContextCache(changeDir, "", "propose", content)` — assert saved.
3. `tryCachedContext(changeDir, "", "propose")` — assert hit (skill hash contributes nothing when skillsPath is empty, but artifact hash is valid).

---

## Feature 4: Pipeline Health Summary

### File: `internal/context/cache.go`

**Export type aliases** — append after `writePipelineSummary` definition (or at end of file after deletion):

```go
// PipelineMetrics is the exported metrics type for health consumers.
type PipelineMetrics = pipelineMetrics

// PhaseMetrics is the exported per-phase metrics type.
type PhaseMetrics = phaseMetrics

// LoadPipelineMetrics reads .cache/metrics.json for changeDir.
// Returns an empty struct (not nil) when the file does not exist.
func LoadPipelineMetrics(changeDir string) *PipelineMetrics {
    return loadPipelineMetrics(changeDir)
}
```

**Delete `writePipelineSummary`** (lines 309–324). Confirmed no callers via grep — dead code.

### File: `internal/cli/commands.go`

**New import** — `sddctx` alias already present (line 16). Add `"time"` if not present — it is already imported transitively via `state` but commands.go does not import it directly. Verify: current imports (lines 3–19) do not include `"time"`. Add it.

**New `runHealth` function** — append after `runDiff`:

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

    // Phases completed.
    phasesCompleted := 0
    for _, p := range state.AllPhases() {
        if st.Phases[p] == state.StatusCompleted {
            phasesCompleted++
        }
    }
    phasesTotal := len(state.AllPhases())

    // Cache hit rate.
    hitRate := 0.0
    if total := pm.CacheHits + pm.CacheMisses; total > 0 {
        hitRate = float64(pm.CacheHits) / float64(total)
    }

    // Staleness.
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
        PhasesCompleted      int       `json:"phases_completed"`
        PhasesTotal          int       `json:"phases_total"`
        CurrentPhase         string    `json:"current_phase"`
        CacheHitRate         float64   `json:"cache_hit_rate"`
        TotalTokensEstimated int       `json:"total_tokens_estimated"`
        LastActivity         time.Time `json:"last_activity"`
        HoursSinceActivity   float64   `json:"hours_since_activity"`
        VerifyStatus         string    `json:"verify_status"`
        Stale                bool      `json:"stale"`
        Warnings             []string  `json:"warnings"`
    }{
        Command:              "health",
        Status:               "ok",
        Change:               name,
        PhasesCompleted:      phasesCompleted,
        PhasesTotal:          phasesTotal,
        CurrentPhase:         string(st.CurrentPhase),
        CacheHitRate:         hitRate,
        TotalTokensEstimated: pm.TotalTokens,
        LastActivity:         st.UpdatedAt,
        HoursSinceActivity:   hoursSince,
        VerifyStatus:         verifyStatus,
        Stale:                stale,
        Warnings:             warnings,
    }
    data, _ := json.MarshalIndent(out, "", "  ")
    fmt.Fprintln(stdout, string(data))
    return nil
}
```

`staleThreshold` is the `const` already defined for `runStatus`/`runList` (shared, defined once at package scope in commands.go — not inside any function).

All required imports (`os`, `strings`, `filepath`, `fmt`, `time`, `json`, `state`, `sddctx`, `errs`) are already present in commands.go. Only `"time"` needs adding if absent.

### File: `internal/cli/cli.go`

**Routing switch** — add `case "health":` before `default:`:

```go
case "health":
    return runHealth(args, stdout, stderr)
```

**`printHelp` inspection section** — add line:

```
  health <name>     Show pipeline health: phases, cache stats, staleness, tokens
```

**`commandHelp` map** — add entry:

```go
"health": `sdd health — Pipeline health summary

Usage: sdd health <name>

Reads state.json and .cache/metrics.json, checks verify-report.md.
Prints JSON with phase completion, cache hit rate, token estimate,
time since last activity, and staleness warnings.

Zero-token operation — runs entirely in Go, no Claude invocation.

Arguments:
  name          Change name

Output: JSON health summary to stdout.
Exit:   0 success, 1 error, 2 usage`,
```

### Tests: `internal/cli/commands_test.go`

Table-driven, `t.TempDir()`:

| name | setup | want fields |
|------|-------|-------------|
| healthy | state (3 phases completed, updatedAt=now), metrics (3 hits, 0 misses), verify-report PASSED | phases_completed=3, cache_hit_rate=1.0, verify_status="passed", stale=false, warnings=[] |
| stale | state (updatedAt = now-48h, not complete), no metrics | stale=true, warnings contains ">24h", cache_hit_rate=0.0 |
| verify_failed | state (fresh), verify-report without PASSED | verify_status="failed", warnings contains "last verify failed" |
| missing_metrics | state only, no .cache dir | cache_hit_rate=0.0, total_tokens_estimated=0, no error |
| no_args | args=[] | exit code 2 (usage error) |

Write state.json directly via `state.Save`. Write metrics.json via `json.MarshalIndent` of a `sddctx.PipelineMetrics` value. Capture stdout with `bytes.Buffer`.

---

## Shared constant placement

`staleThreshold` must be a package-level constant in `internal/cli/commands.go`, not inside any function, so it is accessible to `runStatus`, `runList`, and `runHealth` without duplication:

```go
// staleThreshold is the inactivity duration after which a non-complete
// change is considered stale. Applies to runStatus, runList, runHealth.
const staleThreshold = 24 * time.Hour
```

---

## File summary

| File | Change |
|------|--------|
| `internal/state/types.go` | Add `IsStale` method |
| `internal/context/cache.go` | New `inputHash` params, update `tryCachedContext`/`saveContextCache`, bump `cacheVersion` to 4, export `PipelineMetrics`/`PhaseMetrics`/`LoadPipelineMetrics`, delete `writePipelineSummary` |
| `internal/context/context.go` | Update `Assemble` call sites for new cache signatures; replace `AssembleConcurrent` drain loop; add `"strings"` import |
| `internal/cli/commands.go` | Add `staleThreshold` const; add stale fields to `runStatus` and `runList`; add `runHealth`; add `"time"` import |
| `internal/cli/cli.go` | Add `case "health"` routing, `printHelp` entry, `commandHelp` entry |
| `internal/state/types_test.go` | `IsStale` table tests |
| `internal/context/cache_test.go` | Skill-hash table tests |
| `internal/context/assembler_concurrent_test.go` | Partial-failure table tests |
| `internal/cli/commands_test.go` | `runHealth` table tests |
