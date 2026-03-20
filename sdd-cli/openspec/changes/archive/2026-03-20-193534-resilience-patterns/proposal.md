# Proposal: Resilience Patterns

Change: resilience-patterns
Date: 2026-03-20

---

## Summary

Four targeted improvements to sdd-cli's correctness, observability, and fault tolerance. All are independent; each can be rolled back without affecting the others. No new dependencies. No breaking changes to existing command output.

Implement in dependency order: Feature 2 → Feature 3 → Feature 1 → Feature 4.

---

## Feature 1: Skill-Hash Caching

### Problem

`inputHash` [cache.go:67–92] computes the cache key from phase artifacts listed in `phaseInputs` [cache.go:54–63]. SKILL.md is not in `phaseInputs`; it is read raw by every assembler on every cache miss and on every `explore` call (explore has `"explore": {}` — zero inputs — so `tryCachedContext` returns `(nil, false)` immediately at [cache.go:119–122], bypassing the cache entirely).

This is a correctness bug: if a workflow author updates a SKILL.md, the cached context for that phase reflects the old skill until TTL expiry or an artifact change forces a miss.

### Decision

Add the SKILL.md content to `inputHash` for each phase by threading `skillsPath` and `phaseName` into the hash computation. This fixes cache correctness and enables caching for phases with no artifact inputs (explore).

No in-memory skill cache. Disk hash is sufficient. The extra `os.ReadFile` on the SKILL.md path only runs when computing a hash (cache miss path and `saveContextCache`), not on cache hits.

### Design

**`cache.go` — signature change to `inputHash`:**

```go
// inputHash computes a SHA256 hash of all input artifacts for a phase,
// including the SKILL.md content for cache correctness.
func inputHash(changeDir, skillsPath, phaseName string, inputs []string) string
```

Inside `inputHash`, after hashing the version prefix and before iterating `inputs`, hash the skill file:

```go
if skillsPath != "" && phaseName != "" {
    skillPath := filepath.Join(skillsPath, phaseName, "SKILL.md")
    if data, err := os.ReadFile(skillPath); err == nil {
        fmt.Fprintf(h, "skill:%d:", len(data))
        h.Write(data)
    }
    // Missing SKILL.md: cache miss (safe fallback)
}
```

**`cache.go` — update call sites:**

- `tryCachedContext(changeDir, phase string)` → `tryCachedContext(changeDir, skillsPath, phase string)`
- `saveContextCache(changeDir, phase string, content []byte)` → `saveContextCache(changeDir, skillsPath, phase string, content []byte)`
- Both call `inputHash` with the new signature.

**`context.go` — update callers:**

- `Assemble` [context.go:53] calls `tryCachedContext` and `saveContextCache` — pass `p.SkillsPath` and `phaseStr`.
- `p.SkillsPath` is already on `Params` [context.go:33]; no struct change needed.

**`phaseInputs` for explore:** Leave as `{}`. The skill hash entry in `inputHash` now covers explore's cache key. The check `len(inputs) == 0` in `tryCachedContext` [cache.go:120] must be relaxed: cache is skipped only when both inputs are empty AND skillsPath is empty. Change the guard:

```go
if !ok {
    return nil, false
}
// Only skip if there is truly nothing to hash.
if len(inputs) == 0 && skillsPath == "" {
    return nil, false
}
```

This enables explore-phase caching for the first time.

**`cacheVersion` bump:** Increment from 3 → 4. Invalidates all existing cache entries, which is correct since the hash format changed.

### Files

- `internal/context/cache.go` — `inputHash`, `tryCachedContext`, `saveContextCache`, `cacheVersion`
- `internal/context/context.go` — `Assemble` call sites

### Tests

- Table-driven test with temp dir containing a SKILL.md; verify cache hits after first assembly, cache miss after SKILL.md content changes.
- Verify explore phase now produces a cache hit on second call.

### Risk

Low. Cache is best-effort; all errors are silently ignored. `cacheVersion` bump ensures no stale entries survive. The only behavioral change: explore assembly is now cached.

---

## Feature 2: Zombie Change Detection

### Problem

`runStatus` [commands.go:286–342] and `runList` [commands.go:344–399] omit `updated_at` from their output. A change abandoned mid-pipeline is indistinguishable from one actively in progress. `state.Save` [state.go:149–170] does not stamp `UpdatedAt` directly — it serializes whatever is in the struct. `Advance` [state.go:75–86] sets `s.UpdatedAt = time.Now().UTC()` at line 81. Confirmed: `UpdatedAt` is reliably fresh after any `Advance` call, and set to `time.Now().UTC()` at `NewState` [types.go:44].

### Decision

Add `IsStale(threshold time.Duration) bool` method to `*State`. Add `stale: true` and `stale_hours: N` to `runStatus` and `runList` output. Default threshold 24h. No configurable thresholds.

### Design

**`internal/state/types.go` — new method:**

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

**`internal/cli/commands.go` — `runStatus` output struct:** Add three fields:

```go
UpdatedAt   time.Time `json:"updated_at"`
Stale       bool      `json:"stale"`
StaleHours  float64   `json:"stale_hours,omitempty"`
```

Populate after loading state:

```go
const staleThreshold = 24 * time.Hour
stale := st.IsStale(staleThreshold)
staleHours := 0.0
if stale {
    staleHours = time.Since(st.UpdatedAt).Hours()
}
```

**`internal/cli/commands.go` — `runList` `changeInfo` struct:** Add:

```go
UpdatedAt  time.Time `json:"updated_at"`
Stale      bool      `json:"stale"`
StaleHours float64   `json:"stale_hours,omitempty"`
```

Populate in the loop body before appending to `changes`.

### Files

- `internal/state/types.go` — `IsStale` method
- `internal/cli/commands.go` — `runStatus` output struct, `runList` changeInfo struct

### Tests

- `IsStale`: inject a past `UpdatedAt` via struct literal; verify `IsStale(1*time.Hour)` returns true when >1h old, false when <1h old, false when `IsComplete()`.
- `runStatus` / `runList`: write a state with old `UpdatedAt` to a temp dir; verify JSON output contains `"stale": true`.

### Risk

Low. Read-only change. Output fields are additive; existing consumers are unaffected (JSON unknown fields are ignored).

---

## Feature 3: Partial-Failure Accumulator

### Problem

`AssembleConcurrent` drain loop [context.go:148–153] returns on first error, discarding successful results from all subsequent phases. In the only current call site — `runContext` [commands.go:197–203] with spec+design parallel window — a spec failure silently drops design context that was successfully assembled.

### Decision

Write all successful results to `w` before returning. Collect all errors into a multi-error. Return a combined error after all successful results have been written. No structured `AssembleErrors` type (YAGNI — no caller currently branches on error kind; the string message is sufficient).

### Design

**`internal/context/context.go` — replace drain loop:**

```go
// Write in input order; collect errors.
var errs []string
var succeeded []string
for i, r := range results {
    if r.err != nil {
        errs = append(errs, fmt.Sprintf("%s: %v", phases[i], r.err))
        continue
    }
    w.Write(r.data)
    succeeded = append(succeeded, string(phases[i]))
}
if len(errs) > 0 {
    return fmt.Errorf("concurrent assembly: %d phase(s) failed:\n%s",
        len(errs), strings.Join(errs, "\n"))
}
```

Add `"strings"` to imports in context.go (verify it is not already present — it is not in the current import block).

**Caller behavior:** `runContext` [commands.go:197–203] calls `errs.WriteError` on the returned error, which writes a JSON error envelope to stderr and returns the error to `main`. Partial stdout content has already been written at that point. Claude receives partial context on failure — better than nothing, consistent with the principle of delivering maximum available information.

No changes to `runContext` itself. The semantic change (partial stdout before error) is intentional and documented here.

### Files

- `internal/context/context.go` — `AssembleConcurrent` drain loop, add `"strings"` import

### Tests

- Table-driven test with two phases; first succeeds, second fails. Assert: writer contains first phase's content; returned error names the failed phase.
- Both fail: assert empty writer, error names both.
- Both succeed: assert full content, nil error.

### Risk

Low-medium. Behavioral change: partial output is now written to stdout before the error is returned. Existing callers only read stdout on success (Claude session). The change is strictly additive from the caller's perspective — more information, not less.

---

## Feature 4: Pipeline Health Summary (`sdd health`)

### Problem

`pipelineMetrics` [cache.go:229–237] accumulates token usage in `.cache/metrics.json`. `loadPipelineMetrics` [cache.go:289–307] reads it. `writePipelineSummary` [cache.go:310–324] is dead code (no callers). There is no command to get a structured view of a change's health — completed phases, cache efficiency, staleness, token budget — without invoking Claude.

### Decision

New `sdd health <name>` command. Zero-token (pure Go). Reads state, metrics, and verify-report. Emits JSON to stdout. Wire `loadPipelineMetrics` export to the CLI by exporting it from `internal/context`.

No configurable thresholds. `writePipelineSummary` is removed (superseded).

### Design

**`internal/context/cache.go` — export types and loader:**

```go
// PipelineMetrics is the exported type for health consumers.
type PipelineMetrics = pipelineMetrics   // type alias, zero cost

// PhaseMetrics is the exported per-phase metrics type.
type PhaseMetrics = phaseMetrics

// LoadPipelineMetrics reads .cache/metrics.json for changeDir.
// Returns an empty struct (not nil) when the file does not exist.
func LoadPipelineMetrics(changeDir string) *PipelineMetrics {
    return loadPipelineMetrics(changeDir)
}
```

Remove `writePipelineSummary` (dead code).

**`internal/cli/commands.go` — new `runHealth`:**

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

    // Compute phases completed.
    phasesCompleted := 0
    phasesTotal := len(state.AllPhases())
    for _, p := range state.AllPhases() {
        if st.Phases[p] == state.StatusCompleted {
            phasesCompleted++
        }
    }

    // Cache hit rate.
    hitRate := 0.0
    total := pm.CacheHits + pm.CacheMisses
    if total > 0 {
        hitRate = float64(pm.CacheHits) / float64(total)
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
        Command               string    `json:"command"`
        Status                string    `json:"status"`
        Change                string    `json:"change"`
        PhasesCompleted       int       `json:"phases_completed"`
        PhasesTotal           int       `json:"phases_total"`
        CurrentPhase          string    `json:"current_phase"`
        CacheHitRate          float64   `json:"cache_hit_rate"`
        TotalTokensEstimated  int       `json:"total_tokens_estimated"`
        LastActivity          time.Time `json:"last_activity"`
        HoursSinceActivity    float64   `json:"hours_since_activity"`
        VerifyStatus          string    `json:"verify_status"`
        Stale                 bool      `json:"stale"`
        Warnings              []string  `json:"warnings"`
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

**`internal/cli/cli.go` — routing:**

Add `case "health":` to the switch before `default`. Add to `printHelp` under "Inspection commands":

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

- `internal/context/cache.go` — export `LoadPipelineMetrics`, `PipelineMetrics`, `PhaseMetrics`; remove `writePipelineSummary`
- `internal/cli/commands.go` — new `runHealth`
- `internal/cli/cli.go` — routing switch, `printHelp`, `commandHelp`

### Tests

- `runHealth`: write a temp dir with state.json + metrics.json; assert JSON fields match expected values.
- Stale path: state with `UpdatedAt` 48h ago, `IsComplete() == false`; assert `"stale": true` and warnings contain the stale message.
- Verify-passed path: write a verify-report.md with `**Status:** PASSED`; assert `"verify_status": "passed"`.
- Missing metrics.json: assert zero cache stats, no error.

### Risk

Low. Entirely additive. Exporting `LoadPipelineMetrics` adds surface to `internal/context` but breaks nothing. Removing `writePipelineSummary` (dead code) has no callers to break — confirmed by grep.

---

## Rollback

Each feature is independent. Rollback per-feature by reverting its specific file set:

| Feature | Files to revert |
|---------|----------------|
| 1 Skill-hash | `cache.go` (inputHash, tryCachedContext, saveContextCache, cacheVersion), `context.go` (Assemble call sites) |
| 2 Zombie detection | `types.go` (IsStale), `commands.go` (runStatus output, runList changeInfo) |
| 3 Partial-failure | `context.go` (AssembleConcurrent drain loop) |
| 4 Health command | `cache.go` (exports, remove writePipelineSummary), `commands.go` (runHealth), `cli.go` (routing, help) |

---

## Out of Scope

- In-memory skill cache (process-lifetime sync.Map): disk hash is sufficient; skill files are small and rarely change.
- Configurable staleness threshold: 24h hardcoded; add a flag when a real use case emerges.
- `AssembleErrors` structured type: no caller currently branches on error kind.
