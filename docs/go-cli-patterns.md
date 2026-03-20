---
summary: "14 Go CLI patterns adopted in the sdd binary, with sources and implementation details."
read_when:
  - Looking for patterns to implement
  - Understanding why a pattern was chosen
  - Researching Go CLI best practices
---

# Go CLI Patterns in SDD

14 patterns extracted from production Go CLIs, applied across 3 SDD pipeline iterations.

## Implemented

| # | Pattern | Location in sdd-cli | Impact |
|---|---------|---------------------|--------|
| 1 | Content-hash cache | `context/cache.go` | Skip re-assembly if artifacts unchanged |
| 2 | Versioned cache structs | `cache.go:cacheVersion` | Auto-invalidate on format change |
| 3 | Inline metrics stderr | `cache.go:writeMetrics` | Per-phase token tracking |
| 4 | Pre-flight size guard | `context.go:maxContextBytes` | Reject >100KB contexts |
| 5 | Atomic writes (temp+rename) | `verify.go`, `archive.go` | Prevent partial writes |
| 6 | Error classification | `errs/errs.go` | Typed errors: usage, transport, internal |
| 7 | Progress logging | `verify.go:Run()` | `sdd: verify build: ok (281ms)` |
| 8 | Per-dimension TTL | `cache.go:phaseTTL` | apply=30min, spec=2h, propose=4h |
| 9 | Incomplete-batch resume | `state.go` | Crash recovery via Recover+nextReady |
| 10 | Skill-hash in cache | `cache.go:inputHash` | SKILL.md edits invalidate cache |
| 11 | Zombie detection | `types.go:IsStale` | Warn on 24h+ inactive changes |
| 12 | Partial-failure accumulator | `context.go:AssembleConcurrent` | Write successes, collect errors |
| 13 | Smart-skip verify | `commands.go:shouldSkipVerify` | Skip if no source files changed |
| 14 | Concurrent assembly | `context.go:AssembleConcurrent` | spec+design in parallel |

## Investigated but not adopted

| Pattern | Reason deferred |
|---------|-----------------|
| Structured slog logging | Too many call sites, marginal benefit over fmt.Fprintf |
| In-memory skill cache (sync.Map) | Disk hash sufficient for CLI (not a long-running daemon) |
| Heartbeat goroutine | CLI is short-lived, no need for liveness check |
| Idle-exit timer | Not applicable to CLI tool |
| ETag HTTP conditional requests | No HTTP calls in sdd (all local files) |
| Bounded semaphore channel | Only 2 concurrent phases, WaitGroup sufficient |
