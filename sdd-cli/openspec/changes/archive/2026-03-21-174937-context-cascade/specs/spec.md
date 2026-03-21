# Spec: context-cascade (Phase 2)

Change: context-cascade
Phase: spec
Date: 2026-03-21
Domain: context-assembly

---

## Overview

Delta spec. Three targeted improvements to context assembly:

1. **Structured Decision Extraction** — replace `extractFirst` for proposal.md and design.md in `buildSummary` with a new `extractDecisions` function that surfaces `key: value` pairs and `## Decisions` / `## Architecture` section content.
2. **Completed Tasks in Review and Clean** — emit a `COMPLETED TASKS` section in `AssembleReview` and `AssembleClean` by calling the already-existing `extractCompletedTasks`.
3. **Cache Version Bump** — increment `cacheVersion` from `4` to `5` to invalidate stale entries produced by the old format.

No new dependencies. No new exported types. No breaking changes to existing command interfaces.

---

## 2.1 Structured Decision Extraction

### Problem

`buildSummary` calls `extractFirst(content, "##", 3)` for both proposal.md and design.md. This returns the first three non-empty lines after the first `##` heading — typically boilerplate prose, not actionable decisions. Design files often encode decisions as `key: value` pairs or under `## Decisions` / `## Architecture` sections, which `extractFirst` does not preferentially target.

### ADDED Requirements

**CA-01** `internal/context/summary.go` MUST define a new unexported function:

```
func extractDecisions(content string) string
```

**CA-02** `extractDecisions` MUST scan `content` for `key: value` pairs (single-line, colon-separated, not inside fenced code blocks) and return up to five such pairs joined by `"; "`.

**CA-03** `extractDecisions` MUST collect content lines under `## Decisions` or `## Architecture` headings (case-insensitive match on the heading text, stopping at the next `##` heading or end of content) and return up to three non-empty lines joined by `" "` when no `key: value` pairs are found.

**CA-04** When neither structured `key: value` pairs nor a `## Decisions` / `## Architecture` section is found, `extractDecisions` MUST fall back to `extractFirst(content, "##", 3)`.

**CA-05** Lines inside fenced code blocks (delimited by ` ``` `) MUST be excluded from `key: value` scanning. `extractDecisions` MUST toggle a `inFence bool` flag on ` ``` ` prefixed lines.

**CA-06** `buildSummary` MUST replace the two `extractFirst` calls for proposal.md and design.md with `extractDecisions`:

- For proposal.md: `"Proposal: " + extractDecisions(string(data))`
- For design.md: `"Design: " + extractDecisions(string(data))`

**CA-07** The exploration.md and review-report.md extraction calls in `buildSummary` MUST remain unchanged (still use `extractFirst`).

### Scenarios

**WHEN** design.md contains `Language: Go` and `BuildTool: make` outside code fences
**THEN** `extractDecisions` returns `"Language: Go; BuildTool: make"`

**WHEN** design.md has a `## Decisions` section with three prose lines and no `key: value` pairs
**THEN** `extractDecisions` returns those three lines joined by `" "`

**WHEN** design.md has `key: value` pairs only inside ` ``` ` fenced blocks
**THEN** `extractDecisions` ignores them and falls back to section or `extractFirst` behavior

**WHEN** proposal.md has no `key: value` pairs and no `## Decisions` or `## Architecture` heading
**THEN** `extractDecisions` returns the same result as `extractFirst(content, "##", 3)`

**WHEN** design.md has both `key: value` pairs and a `## Decisions` section
**THEN** `extractDecisions` returns the `key: value` pairs (pairs take priority)

---

## 2.2 Completed Tasks in Review and Clean

### Problem

`AssembleReview` and `AssembleClean` load tasks.md but emit its raw content verbatim as the `TASKS` section. Reviewers and cleanup agents have to scan the full task list to identify what was actually done. `extractCompletedTasks` already exists in `summary.go` and produces a compact `"section: task"` summary of `- [x]` items.

### ADDED Requirements

**CA-08** `AssembleReview` [review.go] MUST call `extractCompletedTasks(string(tasks))` after loading tasks.md and emit the result as a `COMPLETED TASKS` section immediately before the `TASKS` section:

```go
writeSectionStr(w, "COMPLETED TASKS", extractCompletedTasks(string(tasks)))
writeSection(w, "TASKS", tasks)
```

**CA-09** `AssembleClean` [clean.go] MUST call `extractCompletedTasks(string(tasks))` after loading tasks.md and emit the result as a `COMPLETED TASKS` section immediately before the `TASKS` section:

```go
writeSectionStr(w, "COMPLETED TASKS", extractCompletedTasks(string(tasks)))
writeSection(w, "TASKS", tasks)
```

**CA-10** The existing `TASKS` section in both assemblers MUST be preserved unchanged; `COMPLETED TASKS` is an additional section, not a replacement.

**CA-11** When `extractCompletedTasks` returns `"(no tasks completed yet)"`, the `COMPLETED TASKS` section MUST still be emitted with that value.

### Scenarios

**WHEN** tasks.md contains `- [x] Write unit tests` under `## Phase: apply`
**THEN** the assembled context for review includes a `COMPLETED TASKS` section containing `"## Phase: apply: Write unit tests"`

**WHEN** tasks.md contains only unchecked `- [ ]` items
**THEN** the `COMPLETED TASKS` section contains `"(no tasks completed yet)"`

**WHEN** `AssembleReview` is called
**THEN** section order in the writer is: `SKILL`, `CHANGE`, `SPECIFICATIONS`, `DESIGN`, `COMPLETED TASKS`, `TASKS`, `GIT DIFF`, and optionally `PROJECT RULES`

**WHEN** `AssembleClean` is called
**THEN** section order in the writer is: `SKILL`, `CHANGE`, `PIPELINE CONTEXT`, `VERIFY REPORT`, `COMPLETED TASKS`, `TASKS`, and optionally `DESIGN`, `SPECIFICATIONS`

---

## 2.3 Cache Version Bump

### Problem

Adding `COMPLETED TASKS` to the review and clean assembler output changes the format of cached context entries for those phases. Any entry written with the old format (version 4) will be served stale until TTL expiry.

### ADDED Requirements

**CA-12** `cacheVersion` in `internal/context/cache.go` MUST be changed from `4` to `5`.

**CA-13** The comment above `cacheVersion` MUST list "adding new sections to assemblers" among the bump triggers — this condition is already present in the existing comment; no change required if it already reads that way.

### Scenarios

**WHEN** a `.cache/*.hash` file encodes a hash prefixed with `v4:`
**THEN** `tryCachedContext` returns a cache miss because `inputHash` now emits `v5:`, causing the stored hash to differ from the computed hash

**WHEN** `cacheVersion` is `5`
**THEN** `LoadPipelineMetrics` rejects any `metrics.json` with `"version": 4` and returns a fresh empty struct

---

## Affected Files

| File | Changes |
|------|---------|
| `internal/context/summary.go` | Add `extractDecisions`; update `buildSummary` calls for proposal.md and design.md |
| `internal/context/review.go` | Add `COMPLETED TASKS` section via `extractCompletedTasks` |
| `internal/context/clean.go` | Add `COMPLETED TASKS` section via `extractCompletedTasks` |
| `internal/context/cache.go` | Bump `cacheVersion` 4 → 5 |

---

## Out of Scope

- Changing `extractFirst` itself — it remains the fallback path inside `extractDecisions`.
- Adding `extractDecisions` to phases other than proposal and design.
- Configurable max pairs/lines counts — five pairs and three lines are hardcoded.
- Emitting `COMPLETED TASKS` in phases other than review and clean.

---

## Eval Definitions

| ID | Condition | Pass |
|----|-----------|------|
| CA-01 | `extractDecisions` is defined and unexported in `summary.go` | `grep -n 'func extractDecisions' summary.go` returns one match |
| CA-02 | `key: value` pairs outside fences are collected | unit test: input with two bare pairs returns both in output |
| CA-03 | `## Decisions` section content is returned when no pairs exist | unit test: section-only input returns section lines |
| CA-04 | Fallback to `extractFirst` when no structured data | unit test: plain prose returns same result as `extractFirst(content, "##", 3)` |
| CA-05 | Pairs inside code fences are ignored | unit test: fenced pair not present in output |
| CA-06 | `buildSummary` uses `extractDecisions` for proposal and design | `grep 'extractDecisions' summary.go` returns two call sites |
| CA-07 | `buildSummary` still uses `extractFirst` for exploration and review-report | `grep 'extractFirst' summary.go` returns two call sites |
| CA-08 | `AssembleReview` emits `COMPLETED TASKS` before `TASKS` | assembled output for review contains `COMPLETED TASKS` section; `TASKS` section follows it |
| CA-09 | `AssembleClean` emits `COMPLETED TASKS` before `TASKS` | assembled output for clean contains `COMPLETED TASKS` section; `TASKS` section follows it |
| CA-10 | Raw `TASKS` section still present in both assemblers | `TASKS` section appears in assembled output after `COMPLETED TASKS` |
| CA-11 | Empty task list produces sentinel value | tasks.md with no `- [x]` → `COMPLETED TASKS` section body is `(no tasks completed yet)` |
| CA-12 | `cacheVersion` is `5` | `grep 'cacheVersion = 5' cache.go` returns one match |
| CA-13 | Old v4 cache entries are rejected | `tryCachedContext` returns `(nil, false)` for a hash file written under version 4 |
