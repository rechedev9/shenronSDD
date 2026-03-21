# Exploration: Context Cascade Improvements

**Date**: 2026-03-21T12:00:00Z
**Detail Level**: deep
**Change Name**: context-cascade

## Current State

The context assembly pipeline in `internal/context/` builds per-phase context by loading SKILL.md files, prior artifacts, and project metadata. A `buildSummary()` function in `summary.go` extracts first-line excerpts from artifacts to carry decisions forward. The `extractCompletedTasks()` function exists but is only used in `apply.go`. The size guard in `context.go:80` hard-rejects contexts exceeding 100KB with no graceful degradation.

## Relevant Files

| File Path | Purpose | Lines | Complexity | Test Coverage |
|-----------|---------|-------|------------|---------------|
| `internal/context/summary.go` | `buildSummary()`, `extractFirst()`, `extractCompletedTasks()`, `projectContext()` | 152 | medium | no |
| `internal/context/context.go` | `Assemble()`, size guard, `Params`, dispatchers | 196 | medium | yes |
| `internal/context/apply.go` | `AssembleApply()`, `extractCurrentTask()` — uses `extractCompletedTasks()` | 96 | low | no |
| `internal/context/review.go` | `AssembleReview()` — no completed tasks summary | 101 | low | no |
| `internal/context/clean.go` | `AssembleClean()` — no completed tasks summary | 52 | low | no |
| `internal/context/cache.go` | `phaseInputs`, `inputHash()`, cache TTL, `maxContextBytes` | 335 | medium | yes |

## Dependency Map

```
summary.go
  -> Used by: apply.go (buildSummary, extractCompletedTasks)
  -> Used by: clean.go (buildSummary)
  -> Used by: propose.go, spec.go, design.go, tasks.go (buildSummary)

context.go
  -> Used by: cli/commands.go (Assemble, AssembleConcurrent)
  -> Depends on: cache.go (tryCachedContext, saveContextCache, maxContextBytes)

review.go
  -> Depends on: context.go (loadSkill, loadArtifact, writeSection)
  -> Does NOT use: summary.go extractCompletedTasks

clean.go
  -> Depends on: context.go (loadSkill, loadArtifact, writeSection)
  -> Uses: summary.go buildSummary
  -> Does NOT use: summary.go extractCompletedTasks
```

## Risk Assessment

| Dimension | Level | Notes |
|-----------|-------|-------|
| Blast radius | medium | Changes to summary.go affect all phases that call buildSummary() |
| Type safety | low | All string operations, no type-unsafe code |
| Test coverage | low | summary.go has no tests; context_test.go tests Assemble() |
| Coupling | low | Each assembler is independent; summary.go is a shared utility |
| Complexity | low | All functions are simple string processing |
| Data integrity | low | Read-only on artifacts, no state mutations |
| Breaking changes | medium | Changing buildSummary() output format affects cache invalidation (need cacheVersion bump) |
| Security surface | low | No user input handling |

## Approach Comparison

### 2.1 Structured Decision Log

| Approach | Pros | Cons | Effort | Risk |
|----------|------|------|--------|------|
| A: Extract `key: value` lines from artifacts | Simple regex, works with LLM-written docs | Depends on artifact formatting conventions | Low | Low |
| B: Add explicit `## Decisions` section convention | Cleaner extraction, unambiguous | Requires SKILL.md changes to instruct LLM output format | Medium | Low |

**Recommendation**: Approach A with B as enhancement. Extract `key: value` pairs from artifacts. Also look for lines under `## Decisions` or `## Architecture` headers. Fallback to current `extractFirst()` if no structured data found.

### 2.2 Priority-Weighted Context Inclusion

| Approach | Pros | Cons | Effort | Risk |
|----------|------|------|--------|------|
| A: Score + truncate in Assemble() | Centralized, all phases benefit | Requires assemblers to return scored sections | High | Medium |
| B: Score at artifact level in assemblers | Each assembler controls its own truncation | Duplicated logic per assembler | Medium | Low |
| C: Summarize low-priority artifacts before assembly | Simple, uses existing buildSummary pattern | Less granular control | Low | Low |

**Recommendation**: Approach C. When an artifact is old (mtime > 24h) AND not a direct input to the current phase, include only a summary instead of full content. This uses the existing `buildSummary()` pattern and doesn't require restructuring assemblers. The size guard becomes a soft limit that triggers summarization rather than hard rejection.

### 2.3 Completed Tasks Summary Promotion

Single clear approach: Add `extractCompletedTasks()` call to `AssembleReview()` and `AssembleClean()`, writing a `COMPLETED TASKS` section. Trivial change.

## Recommendation

All three items are independent and can be implemented in parallel:
- **2.1**: Modify `buildSummary()` in `summary.go` to extract key-value decisions
- **2.2**: Add priority scoring to `context.go` Assemble() path with soft size guard
- **2.3**: Add `extractCompletedTasks()` calls to `review.go` and `clean.go`

Bump `cacheVersion` from 4 to 5 since summary format changes.
