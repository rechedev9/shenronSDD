# Design: context-cascade

## Overview

Three targeted changes to `internal/context/`:

1. Add `extractDecisions()` to `summary.go`, replace `extractFirst()` calls for proposal and design in `buildSummary()`.
2. Add `COMPLETED TASKS` section before `TASKS` in `AssembleReview()` and `AssembleClean()`.
3. Bump `cacheVersion` from 4 to 5.

---

## File Changes

| File | Change |
|------|--------|
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/summary.go` | Add `extractDecisions()`; update two call sites in `buildSummary()` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/review.go` | Insert `COMPLETED TASKS` section before `TASKS` write |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/clean.go` | Insert `COMPLETED TASKS` section before `TASKS` write |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/cache.go` | `cacheVersion = 4` → `cacheVersion = 5` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context_test.go` | Add unit tests for `extractDecisions`; update section-presence assertions for review and clean |

---

## 1. `extractDecisions` Algorithm

### Signature

```go
func extractDecisions(content string) string
```

Unexported. Returns a single string — either a "; "-joined list of key-value decisions, a " "-joined block of header-scoped lines, or the `extractFirst` fallback.

### Full Algorithm

```
lines  = split content on "\n"
inFence = false
kvPairs = []string{}          // CA-02 bucket
headerLines = []string{}      // CA-03 bucket
inDecisionSection = false

for each line:
    trimmed = strings.TrimSpace(line)

    // CA-05: toggle fence tracking on ``` lines
    if strings.HasPrefix(trimmed, "```"):
        inFence = !inFence
        continue

    if inFence:
        continue

    // CA-03: detect decision/architecture section headers
    if strings.HasPrefix(trimmed, "## "):
        header = strings.ToLower(strings.TrimPrefix(trimmed, "## "))
        inDecisionSection = (header == "decisions" || header == "architecture")
        continue

    // CA-03: collect lines inside a decision/architecture section
    if inDecisionSection && trimmed != "":
        headerLines = append(headerLines, trimmed)
        if len(headerLines) >= 3:
            inDecisionSection = false   // cap; stop collecting more
        continue

    // CA-02: scan for key: value pairs anywhere outside fences
    if !inDecisionSection && strings.Contains(trimmed, ": "):
        parts = strings.SplitN(trimmed, ": ", 2)
        key   = strings.TrimSpace(parts[0])
        value = strings.TrimSpace(parts[1])
        // key must be a short label (no spaces, not a URL fragment)
        if isDecisionKey(key) && value != "":
            kvPairs = append(kvPairs, key+": "+value)
            if len(kvPairs) >= 5:
                break outer loop

// Result priority:
if len(kvPairs) > 0:
    return strings.Join(kvPairs, "; ")
if len(headerLines) > 0:
    return strings.Join(headerLines, " ")
// CA-04: fallback
return extractFirst(content, "##", 3)
```

### `isDecisionKey` helper (inline, not exported)

```go
// Returns true if s looks like a short label: no spaces, 1–30 chars,
// not starting with http/- or a digit.
func isDecisionKey(s string) bool {
    if len(s) == 0 || len(s) > 30 {
        return false
    }
    if strings.ContainsAny(s, " \t") {
        return false
    }
    if strings.HasPrefix(s, "http") || strings.HasPrefix(s, "-") {
        return false
    }
    return true
}
```

This keeps the function self-contained in `summary.go`. No new file required.

### Updated `buildSummary` Call Sites

Replace:
```go
if intent := extractFirst(string(data), "##", 3); intent != "" {
    sections = append(sections, "Proposal: "+intent)
}
```
With:
```go
if intent := extractDecisions(string(data)); intent != "" {
    sections = append(sections, "Proposal: "+intent)
}
```

Same substitution for the `design.md` block (`decision` → `extractDecisions`).

The `exploration.md` and `review-report.md` calls in `buildSummary` are **not** changed — they use `extractFirst` with `"##"` and `"Verdict"` respectively, which are appropriate for their formats.

---

## 2. Section Ordering Changes

### `AssembleReview` (review.go)

Current order:
```
SKILL → CHANGE → SPECIFICATIONS → DESIGN → TASKS → GIT DIFF → [PROJECT RULES]
```

New order:
```
SKILL → CHANGE → SPECIFICATIONS → DESIGN → COMPLETED TASKS → TASKS → GIT DIFF → [PROJECT RULES]
```

Implementation — insert after `writeSection(w, "DESIGN", design)` and before `writeSection(w, "TASKS", tasks)`:

```go
completedTasks := extractCompletedTasks(string(tasks))
writeSectionStr(w, "COMPLETED TASKS", completedTasks)
writeSection(w, "TASKS", tasks)
```

`tasks` is `[]byte`; cast to `string` for `extractCompletedTasks`. No new load needed — tasks is already loaded above.

### `AssembleClean` (clean.go)

Current order:
```
SKILL → CHANGE → [PIPELINE CONTEXT] → VERIFY REPORT → TASKS → [DESIGN] → [SPECIFICATIONS]
```

New order:
```
SKILL → CHANGE → [PIPELINE CONTEXT] → VERIFY REPORT → COMPLETED TASKS → TASKS → [DESIGN] → [SPECIFICATIONS]
```

Implementation — insert after `writeSection(w, "VERIFY REPORT", verifyReport)` and before `writeSection(w, "TASKS", tasks)`:

```go
completedTasks := extractCompletedTasks(string(tasks))
writeSectionStr(w, "COMPLETED TASKS", completedTasks)
writeSection(w, "TASKS", tasks)
```

`extractCompletedTasks` already handles the empty/no-completed case by returning `"(no tasks completed yet)"`, so no guard needed.

---

## 3. Cache Version Bump

`cache.go` line 21: `const cacheVersion = 4` → `const cacheVersion = 5`

This invalidates all existing `.cache/*.hash` files, forcing re-assembly on next run. No migration logic needed — `tryCachedContext` treats version mismatches as cache misses by recomputing `inputHash` with the new version prefix `"v5:"`.

---

## Architecture Decisions

### Decision 1: Where to place `isDecisionKey`

**Option A** — Inline anonymous check in `extractDecisions` body.
- Pro: no extra symbol; function is self-contained.
- Con: duplicates logic if ever reused; harder to unit-test.

**Option B** — Unexported helper `isDecisionKey(s string) bool` in same file.
- Pro: independently testable; named intent.
- Con: adds one more unexported symbol.

**Option C** — Inline regex `regexp.MustCompile`.
- Pro: expressive.
- Con: runtime cost; overkill for a simple predicate; adds regex dep.

**Chosen: Option B.** The logic (length, no-spaces, no-http-prefix) is non-trivial enough that naming it aids readability and test coverage.

### Decision 2: Where `COMPLETED TASKS` goes in the section sequence

**Option A** — After TASKS (appended summary).
- Pro: reviewer sees full task list first, then summary.
- Con: completed context arrives after the incomplete list; AI must backtrack.

**Option B** — Before TASKS (prefix summary).
- Pro: completed context frames the incomplete tasks; matches how humans scan status before detail.
- Con: slight redundancy if all tasks are complete.

**Option C** — Replace TASKS with a merged section.
- Pro: single section; no duplication.
- Con: breaks existing section-label assertions in tests; more invasive refactor.

**Chosen: Option B (before TASKS).** Matches the spec (CA-08/09 both say "before TASKS") and the cognitive pattern: state what's done before listing what remains.

### Decision 3: `extractDecisions` result when both kvPairs and headerLines are populated

**Option A** — kvPairs wins (current design). Header section lines are secondary.
- Pro: `key: value` pairs are the most structured signal in a design doc.
- Con: a doc with one stray `key: value` line suppresses richer section content.

**Option B** — headerLines wins.
- Pro: explicit section markup is more intentional than ad-hoc `key: value` lines.
- Con: many design docs lack `## Decisions` headers.

**Option C** — Merge both, deduplicate.
- Pro: most complete.
- Con: combined string may exceed token budget; complexity increase.

**Chosen: Option A.** `key: value` scanning is the primary signal per CA-02. The `## Decisions` / `## Architecture` collector is a secondary enrichment path. Fallback chain (kv → header → extractFirst) covers all document styles without merging logic.

---

## Testing Strategy

### Unit Tests (all in `context_test.go`, `package context`)

#### `TestExtractDecisions` — table-driven

Test cases:

| Case | Input | Expected output |
|------|-------|-----------------|
| kv pair present | `"approach: middleware\nfallback: noop"` | `"approach: middleware; fallback: noop"` |
| kv inside code fence skipped | `"```\nkey: val\n```\nother: x"` | `"other: x"` |
| header section collected | `"## Decisions\nUse adapter pattern\nNo ORM"` | `"Use adapter pattern No ORM"` |
| kv cap at 5 | 6 `key: val` lines | 5 pairs in output |
| header cap at 3 lines | 4 lines under `## Decisions` | 3 lines joined |
| fallback to extractFirst | plain prose, no kv, no decision header | first section content |
| empty input | `""` | `""` (fallback returns empty) |
| architecture header | `"## Architecture\nLayer separation"` | `"Layer separation"` |

#### `TestIsDecisionKey`

Cover: empty string, >30 chars, contains space, starts with `http`, starts with `-`, valid short label.

#### `TestAssembleReviewCompletedTasks`

Extend existing `TestAssembleReview`:

```go
os.WriteFile(filepath.Join(changeDir, "tasks.md"), []byte(`# Tasks
## Phase 1
- [x] Done task
- [ ] Pending task
`), 0o644)
// Assert:
if !strings.Contains(out, "--- COMPLETED TASKS ---") { ... }
if !strings.Contains(out, "Done task") { ... }
```

Also verify ordering: `COMPLETED TASKS` appears before `TASKS` in the output string via index comparison.

#### `TestAssembleCleanCompletedTasks`

Same pattern as above against `AssembleClean`. Requires `verify-report.md` and `tasks.md` fixtures with at least one `[x]` item.

#### `TestAssembleCleanNoCompletedTasks`

`tasks.md` with only `- [ ]` items. Assert `--- COMPLETED TASKS ---` is present and content is `(no tasks completed yet)`.

### Regression Tests

- `TestAssembleReview` — existing test passes unchanged (section labels checked remain valid; `COMPLETED TASKS` is additive).
- `TestAssembleClean` — same.

No new integration tests needed. The cache version bump has no dedicated test; the existing cache tests in `cache.go` test via `inputHash` which embeds the version string automatically.

---

## Implementation Sequence

1. `summary.go` — add `isDecisionKey`, add `extractDecisions`, update two `buildSummary` call sites.
2. `review.go` — insert `COMPLETED TASKS` write.
3. `clean.go` — insert `COMPLETED TASKS` write.
4. `cache.go` — bump `cacheVersion`.
5. `context_test.go` — add tests.
6. Run `go test ./internal/context/...` to verify.
