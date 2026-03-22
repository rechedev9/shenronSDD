---
name: sdd-clean
description: >
  Three-pass code cleanup: dead code removal, duplication & reuse analysis, and quality & efficiency review.
  Runs after verify passes. Trigger: When user runs /sdd-clean or after sdd-verify passes.
license: MIT
metadata:
  version: "3.0"
---

# SDD Clean — Code Cleanup

You are executing the **clean** phase inline. Your responsibility is to clean up code after implementation — removing dead code, eliminating duplicates, and simplifying complex expressions. You operate **only on files related to the current change** and verify that every removal is safe before committing it.

## Activation

User runs `/sdd-clean`. Reads `tasks.md` and `verify-report.md` from disk. Aborts if verify verdict is FAIL.

## Inputs

Read from disk:

| Input | Source |
|---|---|
| `changeName` | Infer from `openspec/changes/` (the active change folder) |
| `tasks.md` | `openspec/changes/{changeName}/tasks.md` |
| `verify-report.md` | `openspec/changes/{changeName}/verify-report.md` |

---

## Execution Steps

### Step 1 — Determine Scope

1. Read `openspec/config.yaml` and `CLAUDE.md` for project conventions. Extract build commands into `CMD_CHECK`, `CMD_LINT`, `CMD_TEST` variables. If config.yaml has no `commands` block, fall back to project root detection (same as sdd-apply Step 1).
2. Read `tasks.md` to identify all files created or modified in this change.
3. Build the **cleanup scope**:
   - **Primary**: Files directly created/modified in this change.
   - **Secondary**: Files that import from primary files (one level of dependents).
   - **Excluded**: Everything else. Do NOT refactor the whole project.
4. Read `verify-report.md`. If the verify verdict is FAIL, **abort** — do not clean broken code.

### Step 2 — Three-Pass Analysis

Analyze all files in scope using **three distinct passes**. Each pass adopts a different mental model.

> **Why three passes?** A single linear review develops confirmation bias. Three passes with distinct goals catch different classes of issues.

---

#### Pass 1 — Dead Code & Stale References

**Goal**: Find everything that exists but shouldn't.

**1a. Dead Code Detection**

| Check | Risk Level | Description |
|---|---|---|
| Unused imports | SAFE | Imports not referenced anywhere in the file |
| Unused local variables | SAFE | Variables declared but never read |
| Unused function parameters | CAREFUL | May be required by interface contract |
| Unused private functions | SAFE | Private/unexported functions not called within the file |
| Unused exported functions | CAREFUL | Must verify no external callers |
| Unreachable code | SAFE | Code after return/throw/break/continue |
| Dead branches | CAREFUL | Conditions always true/false |
| Commented-out code | SAFE | Code blocks that are commented out (not documentation) |

**1b. Documentation Synchronization**

For every function **modified in this change**, check for stale documentation. Fix misleading docs — do NOT add docs to undocumented functions.

| Check | Detection | Fix |
|---|---|---|
| **Stale param docs** | Doc lists a parameter that no longer exists, or is missing one added | Update param list to match signature |
| **Stale return docs** | Return description doesn't match actual return | Update return docs |
| **Misleading inline comments** | Comment describes logic that was changed | Rewrite to describe current behavior |
| **Orphaned TODO/FIXME** | TODO references a task completed in this change | Remove |
| **Stale symbol name in comment** | Comment references a renamed symbol | Update the reference |

Do NOT: add new docs, rewrite docs for style, touch files outside scope.

**Indirect caller audit**: If a public function's signature changed, grep for callers. Update stale doc references in callers within scope; note out-of-scope callers in the clean report.

---

#### Pass 2 — Duplication & Reuse

**Goal**: Find code that is repeated or reinvents something the codebase already provides. This pass looks **outward** — beyond the change scope.

**2a. Duplicate Detection (within change scope)**

- **Identical blocks**: 5+ lines of identical code in two or more places.
- **Near-identical blocks**: Same structure with only variable names different.
- **Repeated patterns**: Same sequence of operations in multiple functions.

For each: can it be extracted without premature abstraction? Same interface and semantics? Would shared function be MORE or LESS readable?

**Rule of Three**: Only consolidate if 3+ occurrences, OR 2 truly identical occurrences.

**2b. Codebase Reuse Search (beyond change scope)**

For every new function or inline logic block, search existing codebase for pre-existing utilities:

1. Search helper/util directories.
2. Search for similar function signatures.
3. Search for similar names.
4. Check adjacent modules.

Classify matches: **REPLACE** (use existing), **EXTEND** (add a parameter), or skip (superficially similar but semantically different).

**2c. Cross-File Helper Consolidation**

When the same helper function is defined in multiple test files:
1. Check if a shared helpers directory exists.
2. Identical across 2+ files → extract to shared helpers.
3. Differs slightly → parameterize the shared version.
4. Verify all callers still work after extraction.

---

#### Pass 3 — Quality & Efficiency

**Goal**: Find code that works but is suboptimal.

**3a. Complexity Analysis**

| Metric | Threshold | Action |
|---|---|---|
| Function length | > 50 lines | Flag for potential split |
| Nesting depth | > 3 levels | Flag for early-return refactor |
| Parameter count | > 5 params | Flag for options object pattern |
| Cyclomatic complexity | > 10 | Flag for decomposition |

**3b. Simplification Opportunities**

Look for language-idiomatic simplifications: redundant null checks replaceable by null-coalescing or optional chaining, boolean return patterns, pointless try-catch, redundant type annotations. Consult the relevant framework SKILL.md for language-specific simplification patterns.

**3c. Efficiency Analysis**

| Check | What to look for | Fix |
|---|---|---|
| **Redundant computations** | Same value computed multiple times | Extract to a variable or memoize |
| **Unnecessary waits/timeouts** | Hard-coded delays, excessive timeouts | Reduce to minimum or use event-based waits |
| **N+1 patterns** | Loop making an API/DB call per iteration | Batch into a single call |
| **Missed concurrency** | Independent async calls run sequentially | Run concurrently where safe |
| **Hot-path bloat** | Blocking work added to startup or per-request paths | Defer, lazy-load, or move off hot path |
| **Unbounded data structures** | Collections that grow without limit or cleanup | Add size limits or cleanup logic |
| **Listener/subscription leaks** | Event listeners added without cleanup | Add cleanup in teardown/dispose |

**Efficiency in test code**: Flag excessive timeouts in optional-presence checks.

### Step 3 — Apply Changes Safely

#### 3a. Risk-Based Approach

**SAFE removals** (apply directly, verify after batch): unused imports, unused locals, unreachable code, commented-out code.

**CAREFUL removals** (verify after each): unused exports (search callers first), unused params (check interface), dead branches, duplicate consolidation.

**RISKY removals** (extra verification): public API changes, dynamic import targets, config-referenced functions.

#### 3b. Verification After Changes

After each significant removal or batch of SAFE removals:

1. Run `{CMD_CHECK}` — if it fails, **REVERT** and note it.
2. Run `{CMD_TEST}` for affected files — if tests fail, **REVERT** and note it.

#### 3c. Documentation Fixes

Apply all doc fixes from Pass 1b as a batch per file. Doc fixes are behavior-neutral, so they don't require individual verification. Preserve existing documentation style.

#### 3d. Consolidating Duplicates

1. Create the shared function with a descriptive name, explicit types, and the same error handling pattern.
2. Replace each occurrence. Verify with build checks after EACH replacement.
3. If the shared function would need more than 2 generic parameters, do NOT consolidate — the abstraction is too complex.

### Step 4 — Broader Scope Check

After cleaning primary files, check secondary scope (direct dependents):
- Exports from primary files no longer used by any dependent?
- Types/interfaces replaced by new ones?
- Old implementations superseded?

### Step 5 — Final Verification

Run the full quality suite:

```
{CMD_CHECK}
{CMD_LINT}
{CMD_TEST}
```

All must pass. If any fail, identify which cleanup caused it and revert.

### Step 6 — Produce Clean Report

Create `openspec/changes/{changeName}/clean-report.md`:

```markdown
# Clean Report: {changeName}

**Date**: {YYYY-MM-DD}
**Status**: SUCCESS | ERROR

## Files Cleaned
{List of each file and actions taken}

## Lines Removed
{Total count and per-file breakdown}

## Actions Taken

### Pass 1 — Dead Code & Stale References
- Unused imports removed: {count}
- Dead functions removed: {count}
- Stale docs fixed: {count}

### Pass 2 — Duplication & Reuse
- Duplicates consolidated: {count}
- Replaced with existing utility: {count}
- Helpers extracted to shared module: {count}

### Pass 3 — Quality & Efficiency
- Complexity reductions: {count}
- Efficiency improvements: {count}
- Reverted changes: {count and reasons}

## Documentation Synchronization
| File | Function | Fix Type | Description |
|---|---|---|---|

## Build Status
- Build/Compile: {PASS | FAIL}
- Lint: {PASS | FAIL}
- Tests: {PASS | FAIL}
```

### Step 7 — Present Summary

Write `clean-report.md` and append a JSONL line to `quality-timeline.jsonl` (if quality tracking enabled).

Present a markdown summary to the user, then STOP:

```markdown
## SDD Clean: {change_name}

**Build after cleanup**: check {PASS|FAIL}  |  lint {PASS|FAIL}  |  tests {PASS|FAIL}

### Cleanup Summary
- **Files cleaned**: {N}
- **Lines removed**: {N}  |  **Unused imports**: {N}  |  **Dead functions**: {N}
- **Duplicates consolidated**: {N}  |  **Helpers extracted**: {N}
- **Complexity reductions**: {N}  |  **Efficiency improvements**: {N}

### Files Modified
{For each file: `{path}` — {list of actions}}

{If reverted: ### Reverted Actions ({N})\n{list — these were unsafe to remove}\n}

**Artifact**: `openspec/changes/{changeName}/clean-report.md`

{If SUCCESS: **Next step**: Run `/sdd-archive` to close the change and merge delta specs into main specs.}
{If ERROR: The cleanup was aborted — verify verdict was FAIL. Fix first, then re-run `/sdd-clean`.}
```

---

## Rules — Hard Constraints

1. **Scope is limited.** Only clean files from the current change + direct dependents.
2. **Verify after removal.** Every CAREFUL and RISKY removal must be followed by build + tests.
3. **Revert on failure.** If removal breaks the build or tests, REVERT immediately.
4. **No premature abstraction.** Three similar lines are better than a clever abstraction nobody understands.
5. **Rule of Three.** Do not extract unless 3+ occurrences (or 2 identical).
6. **Preserve public API.** Do not remove or rename exports that might be used outside scope.
7. **Never remove dynamically referenced code.**
8. **Abort if verify failed.** Do NOT clean broken code.
9. **No new features.** Cleanup must not change behavior. Note bugs, don't fix them here.
10. **Respect existing tests.** All tests must still pass. Remove tests only alongside the dead code they test.
11. **Produce clean-report.md.** Always write the artifact.
12. **Respect framework idioms.** Do NOT flag framework-idiomatic patterns as dead code or simplification candidates. Load the relevant framework SKILL.md to understand what is idiomatic.
13. **Handoff to sdd-archive.** Include build health in clean-report.md so sdd-archive has a trustworthy quality snapshot.

---

## What NOT to Clean

- **Configuration files** — too risky without understanding full impact.
- **Third-party / vendored code** — never touch.
- **Generated code** — regenerated, not hand-edited.
- **Test fixtures and mocks** — may look dead but serve a testing purpose.
- **Feature flags** — intentionally dormant code.
- **Polyfills and compatibility code** — may be needed in other environments.

---

## Edge Cases

| Situation | Action |
|---|---|
| Unused export in barrel/index file | Check all consumers before removing |
| Function parameter unused but required by interface | Keep it, add `_` prefix if linter allows |
| Commented-out code has NOTE explaining why | Keep it — serves as documentation |
| Duplicate code with subtle differences | Do NOT consolidate — differences are intentional |
| Cleanup reduces a file below 20 lines | Consider merging into parent module |

---

## PARCER Contract

```yaml
phase: clean
preconditions:
  - verify verdict is PASS or PASS_WITH_WARNINGS
postconditions:
  - no orphaned imports or dead code in changed files
  - clean-report.md confirms build PASS
  - clean-report.md confirms tests PASS
```
