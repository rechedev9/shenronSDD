# Proposal: context-cascade

Phase 2 context cascade improvements: structured decision log, priority-weighted context inclusion, completed tasks summary promotion to review and clean phases.

## Problem

Three independent weaknesses in the context assembly pipeline:

1. **Weak decision extraction.** `buildSummary()` in `summary.go` uses `extractFirst()` to pull the first N lines after a heading match. For proposals and designs, this often yields prose preamble rather than actual decisions. The structured information (key/value pairs, `## Decisions` sections) that agents write is ignored.

2. **No completed-tasks signal in review/clean.** `extractCompletedTasks()` exists in `summary.go` and is used by `apply.go` to give the apply agent awareness of prior work. `review.go` and `clean.go` do not call it, so those agents lack the same signal about what has been implemented.

3. **Cache version stale.** Adding new sections to assemblers without bumping `cacheVersion` can serve stale cached context to subsequent runs.

## Proposed Changes

### 2.1 Structured Decision Log (`summary.go`)

Add `extractDecisions(content string) string` that:
- Scans for `key: value` pairs (single-line, colon-separated, not inside code fences).
- Also collects content under `## Decisions` or `## Architecture` headings.
- Falls back to `extractFirst()` if no structured data found.

Replace the `proposal.md` and `design.md` extraction calls in `buildSummary()` to use `extractDecisions()`.

### 2.2 Completed Tasks in Review and Clean (`review.go`, `clean.go`)

`review.go`: load `tasks.md` (already required), call `extractCompletedTasks()`, emit as a `COMPLETED TASKS` section before `GIT DIFF`.

`clean.go`: already loads `tasks.md` (required), call `extractCompletedTasks()`, emit as a `COMPLETED TASKS` section after `VERIFY REPORT`.

No new artifact dependencies are introduced — both files already require `tasks.md`.

### 2.3 Cache Version Bump (`cache.go`)

Bump `cacheVersion` from `4` to `5`. The new sections added to review and clean assemblers change their output format; stale caches must be invalidated.

## Affected Files

- `internal/context/summary.go` — add `extractDecisions()`; update `buildSummary()` calls
- `internal/context/review.go` — add `COMPLETED TASKS` section
- `internal/context/clean.go` — add `COMPLETED TASKS` section
- `internal/context/cache.go` — bump `cacheVersion` to 5

## Out of Scope

- Priority-weighted soft-limit context truncation (Approach C mtime-based summarization). Deferred: requires design for which assemblers check mtime and how summaries are formatted.
- Changes to `apply.go` — already has `COMPLETED TASKS`.
- Changes to `explore.go`, `propose.go`, `spec.go`, `design.go`, `tasks.go`.
- Any changes to the size guard in `context.go`.
- New artifact types or phase inputs.

## Success Criteria

- `go build ./...` passes.
- `go test ./...` passes (or existing test failures are pre-existing and unrelated).
- `buildSummary()` for a proposal containing `key: value` pairs returns those pairs, not preamble prose.
- `AssembleReview` output includes a `COMPLETED TASKS` section when `tasks.md` has checked items.
- `AssembleClean` output includes a `COMPLETED TASKS` section when `tasks.md` has checked items.
- Running `sdd context review` after the change produces a cache miss (version bump forces recomputation).

## Risk Assessment

Low. All changes are string processing on in-memory data already loaded. No new I/O paths. No new required artifacts. `extractDecisions()` has an explicit fallback to the existing `extractFirst()` behavior, so degraded input produces the same output as before. The cache version bump is a one-time invalidation with no user-visible side effect beyond a slightly slower first run.

## Rollback Plan

Revert the four changed files to their current state (git revert or manual restore). Bump `cacheVersion` to 6 in `cache.go` to force cache invalidation after rollback. No data is written by these changes that would need cleanup.
