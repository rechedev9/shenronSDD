---
summary: "Token consumption analysis: before and after the Go CLI, with real measurements."
read_when:
  - Evaluating SDD cost efficiency
  - Comparing SDD vs manual AI coding
  - Optimizing token usage
---

# Token Economics

## The problem

AI coding agents waste tokens on deterministic work: reading state, loading skills, assembling context, running build commands. These operations don't need intelligence — they need `if/else`.

## Measured: before vs after

### Before (archivos .md, no CLI)

| Operation | Tokens | Frequency | Notes |
|-----------|--------|-----------|-------|
| Read SKILL.md | 3-8K | Per phase | 8 phases = ~40K |
| Read prior artifacts | 5-20K | Per phase, cumulative | Review phase reads ~20K |
| State discovery | 2-3K | Per phase | Scan directory, deduce phase |
| Run verify manually | ~30K | Once | Claude executes and parses output |
| Run archive manually | ~15K | Once | Claude moves files and writes manifest |
| **Total per pipeline** | **~458K** | | |

### After (Go CLI)

| Operation | Tokens | Frequency | Notes |
|-----------|--------|-----------|-------|
| Sub-agent explore | ~44K | Once | Sonnet |
| Sub-agent propose | ~16K | Once | Sonnet |
| Sub-agent spec | ~38K | Once | Sonnet |
| Sub-agent design | ~42K | Once | Sonnet (can run parallel with spec) |
| Sub-agent tasks | ~21K | Once | Sonnet |
| All Go operations | **0** | 14+ times | init, context, write, verify, archive, status, list, diff, health |
| **Total per pipeline** | **~161K** | | |

### Savings: 65% reduction

And the 161K goes 100% to reasoning (exploration, proposals, specs, design, tasks). Zero wasted on bookkeeping.

## Cache impact

On re-execution (e.g., sub-agent fails and retries):

| Scenario | Without cache | With cache |
|----------|--------------|------------|
| `sdd context` same inputs | Re-reads all files, re-assembles | 0ms, serves from `.cache/` |
| SKILL.md unchanged | Re-reads 3-8K file | Hash match, skip |
| Artifacts unchanged | Re-reads all | Content-hash match, skip |
| After commit (smart-skip) | Full verify run | `sdd verify` skips if no source changes |

## Per-dimension TTL

Different phases have different volatility:

| Phase | TTL | Rationale |
|-------|-----|-----------|
| propose | 4h | Proposals are stable once written |
| spec/design | 2h | Specs change during iteration |
| tasks | 1h | Task list updates during apply |
| apply | 30min | Implementation context changes rapidly |
| review/clean | 1h | Review findings are relatively stable |

## Pipeline metrics

Every `sdd context` run records to `metrics.json`:

```json
{
  "version": 4,
  "phases": {
    "spec": { "bytes": 14817, "tokens": 3704, "cached": true, "duration_ms": 0 },
    "design": { "bytes": 15200, "tokens": 3800, "cached": false, "duration_ms": 12 }
  },
  "total_bytes": 46758,
  "total_tokens": 11689,
  "cache_hits": 3,
  "cache_misses": 2
}
```

`sdd health <name>` surfaces this data for observability.

## Model selection strategy

Use the cheapest model that can do the job:

| Phase | Recommended model | Rationale |
|-------|-------------------|-----------|
| explore | Sonnet | Codebase scanning, not architecture |
| propose | Sonnet | Summarization, not invention |
| spec | Sonnet | Requirements extraction |
| design | **Opus** | Architecture decisions shape everything |
| tasks | Sonnet | Decomposition from design |
| apply | **Opus** | Production code quality |
| review | Sonnet | Analytical comparison |
| clean | Sonnet | Mechanical cleanup |
| verify | **Go (0 tokens)** | Deterministic |
| archive | **Go (0 tokens)** | Deterministic |
