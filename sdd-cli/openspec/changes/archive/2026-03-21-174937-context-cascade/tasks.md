# Tasks: context-cascade

## Phase 1: Core function (extractDecisions)

- [ ] Add `extractDecisions(content string) string` to `internal/context/summary.go`
- [ ] Toggle `inFence bool` on lines starting with ` ``` ` to skip fenced code blocks
- [ ] Outside fences, collect `key: value` pairs where key has no spaces and is not a URL/list marker; accumulate up to 5 pairs
- [ ] If 5 pairs collected, return them joined by `"; "`
- [ ] If fewer than 5 pairs (or zero), scan for `## Decisions` or `## Architecture` headings (case-insensitive) and collect up to 3 content lines joined by `" "`
- [ ] If neither strategy yields output, fall back to `extractFirst(content, "##", 3)`

## Phase 2: Integration

- [ ] In `buildSummary()` (`summary.go`), replace `extractFirst(string(data), "##", 3)` with `extractDecisions(string(data))` for the `proposal.md` block
- [ ] In `buildSummary()` (`summary.go`), replace `extractFirst(string(data), "##", 3)` with `extractDecisions(string(data))` for the `design.md` block
- [ ] Confirm `exploration.md` and `review-report.md` blocks in `buildSummary()` still use `extractFirst()` (no change needed)
- [ ] In `review.go` `AssembleReview()`, add `writeSectionStr(w, "COMPLETED TASKS", extractCompletedTasks(string(tasks)))` immediately before the existing `writeSection(w, "TASKS", tasks)` call
- [ ] In `clean.go` `AssembleClean()`, add `writeSectionStr(w, "COMPLETED TASKS", extractCompletedTasks(string(tasks)))` immediately before the existing `writeSection(w, "TASKS", tasks)` call

## Phase 3: Cache bump + tests

- [ ] In `cache.go`, change `const cacheVersion = 4` to `const cacheVersion = 5`
- [ ] Create `internal/context/summary_test.go` with package `context`
- [ ] Add test: kv pairs extracted — input has several `key: value` lines, expect up to 5 joined by `"; "`
- [ ] Add test: fenced kv pairs ignored — `key: value` inside ` ``` ` fence not collected
- [ ] Add test: `## Decisions` heading — no kv pairs present, lines under heading returned (up to 3, space-joined)
- [ ] Add test: `## Architecture` heading — same as above for alternate heading name
- [ ] Add test: fallback to `extractFirst` — no kv pairs and no matching heading, verify result equals `extractFirst(content, "##", 3)`
- [ ] Add test: mixed content — kv pairs outside fence take priority over heading lines
- [ ] Add test: `AssembleReview` output contains `=== COMPLETED TASKS ===` section when tasks.md has `- [x]` items
- [ ] Add test: `AssembleClean` output contains `=== COMPLETED TASKS ===` section when tasks.md has `- [x]` items

## Phase 4: Verify

- [ ] Run `go build ./...` from repo root — zero errors
- [ ] Run `go test ./internal/context/...` — all tests pass including new summary_test.go
- [ ] Run `go vet ./internal/context/...` — zero warnings
