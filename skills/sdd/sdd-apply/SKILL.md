---
name: sdd-apply
description: >
  Implement code following specs and design. Works in batches (one phase at a time). Includes build-fix loop.
  Trigger: When user runs /sdd:apply or after sdd-tasks completes.
license: MIT
metadata:
  version: "2.0"
---

# SDD Apply — Implementation

You are executing the **apply** phase. Write production code that satisfies specs and design constraints. Work in **batches** (one phase at a time) and run a **build-fix loop** after each batch.

> **Core principle: follow the bottom-up order defined in tasks.md. Read before writing. Run checks before stopping.**

## Inputs

| Input | Source |
|---|---|
| `changeName` | Infer from `openspec/changes/` (the active change folder) |
| `tasks.md` | `openspec/changes/{changeName}/tasks.md` |
| `design.md` | `openspec/changes/{changeName}/design.md` |
| `specs/` | `openspec/changes/{changeName}/specs/` |
| `phase` | Flag `--phase <N>` or prompt user |
| `mode` | `'standard'` (default) or `'fix'` (triggered by failed review/verify) |

---

## Standard Mode

### Step 1 — Load Context

**Token budget:** Read large files (>150 lines) with `offset`/`limit`. Don't re-read files already in context. Load framework SKILLs only for frameworks used in the files you will modify.

1. Read `openspec/config.yaml` for project settings.
2. Read `tasks.md` — identify tasks for the target **phase**.
3. Read `design.md` — extract architecture decisions, interfaces, constraints.
4. List files in `specs/` — these contain GIVEN/WHEN/THEN acceptance criteria.
5. Read `CLAUDE.md` at the project root for coding conventions.
6. **Detect build commands** from `CLAUDE.md` or project root (`package.json`, `Makefile`, `pyproject.toml`, `Cargo.toml`, `go.mod`, etc.). Store as `CMD_CHECK`, `CMD_LINT`, `CMD_TEST`, `CMD_FORMAT`.

### Step 2 — Plan the Batch

1. Filter tasks.md to the specified **phase**, excluding `[x]` tasks.
2. Order by dependency (if task B depends on task A's output, do A first).
3. For each task, identify the spec scenario, existing files to modify, and new files to create.
4. If `--dry-run`: output the plan and STOP.

### Step 3 — Implement Each Task

For **each task** in dependency order:

#### 3a. Read Spec + Design
- Parse the matching GIVEN/WHEN/THEN scenarios from `specs/`. These are your acceptance criteria.
- Check `design.md` for interface definitions and architectural patterns.
- If the design contradicts the spec, follow the spec (source of truth) and note the deviation in `apply-report.md`.

#### 3b. Read Existing Code (Structured Reading Protocol)

**Always read before writing.** For each file you are about to modify:

1. **Before reading** — State your HYPOTHESIS (expected patterns/conventions) and EVIDENCE (which spec/design/file informed it).
2. **After reading** — Note OBSERVATIONS (key patterns with `File:Line` refs), HYPOTHESIS STATUS (`CONFIRMED` | `REFUTED` | `REFINED`), and one IMPLEMENTATION IMPLICATION sentence.

Your new code MUST follow the patterns observed. Do not introduce a new style.

#### 3c. Write Code

1. Write or modify the implementation file.
2. Follow the project's coding conventions from `CLAUDE.md`.
3. Load the relevant framework SKILL.md if the file uses a specific framework (React, Django, etc.).
4. If `--tdd`: write failing test first → implement to pass → refactor.

**Test generation policy:** Only write tests if (A) the task starts with "Test — ...", (B) an existing test breaks due to your changes, or (C) `--tdd` mode is active. Do not generate speculative tests.

#### 3d. Mark Task Progress

- `[x]` — Fully complete. Add a note if implementation deviated from design.
- `[~]` — Partially complete (task too large or context limits reached). Note what remains.

### Step 4 — Build-Fix Loop

After all tasks in the batch are implemented, run the project's check commands:

1. **Type/compile check** → `{CMD_CHECK}` — fix root causes, not symptoms.
2. **Lint** → `{CMD_LINT}` — use auto-fix when available.
3. **Tests** → `{CMD_TEST}` — fix failures in files you touched. Note pre-existing failures without fixing them.
4. **Format** → `{CMD_FORMAT}` (if not covered by lint).

**Max 3 fix attempts per unique error.** If an error persists after 3 attempts, stop and flag it for manual review. Do not loop indefinitely.

### Step 5 — Generate Apply Report

Write `openspec/changes/{changeName}/apply-report.md`:

```markdown
# Apply Report: {changeName}

**Phase**: {N}
**Date**: {YYYY-MM-DD}
**Status**: {SUCCESS | PARTIAL | ERROR}
**Tasks Completed**: {N}/{M}

## Files Created
| File | Purpose |
|------|---------|
| {path} | {description} |

## Files Modified
| File | Changes |
|------|---------|
| {path} | {summary} |

## Build Health
| Check | Result |
|-------|--------|
| Type/Compile | {PASS/FAIL} |
| Lint | {PASS/FAIL} |
| Tests | {PASS/FAIL} |
| Format | {PASS/FAIL} |

## Deviations
{Any deviations from design or spec, or "None."}

## Manual Review Needed
{Unresolved issues after build-fix loop, or "None."}
```

### Step 6 — Present Summary

Present a markdown summary, then STOP.

```markdown
## SDD Apply — Phase {N} Complete

**Build**: check {PASS|FAIL} | lint {PASS|FAIL} | tests {PASS|FAIL}

### Tasks Completed ({N}/{M})
{[x] task list}

### Files Changed
- **Created**: {list}
- **Modified**: {list}

{If deviations: ### Deviations\n- {description}}
{If manual review: ### Manual Review Required\n- {file}: {reason}}

**Artifact**: `apply-report.md`
**Next step**: {/sdd:apply --phase {N+1} | /sdd:review | manual fix needed}
```

---

## Fix Mode

Triggered by a failed review or verify gate. The agent does NOT re-implement tasks — it only fixes listed issues.

1. **Read** the source report (`review-report.md` or `verify-report.md`). Parse `AUTO_FIXABLE` entries as your scope.
2. **Group** fixes by file. Verify each file exists. Order fixes by line number within each file.
3. **Apply** each fix using the `fixDirection` field. ONLY modify files in the fix list — no new features, no refactoring, no task progress updates.
4. **Judgment gate**: If a fix requires architectural changes beyond a mechanical repair, add it to `fixesRemaining` with `REQUIRES_HUMAN_JUDGMENT`.
5. **Build-fix loop**: Same as Step 4 above. If new errors appear in files NOT in the fix list, note them as `PRE_EXISTING`.
6. **Write** `fix-report-{iteration}.md` with fixes applied, fixes remaining, and build health.

---

## Rules

1. **Read before write.** Never modify a file you haven't read in this session.
2. **Follow existing patterns.** Your code must look like it belongs in the codebase.
3. **Follow `CLAUDE.md` conventions.** The project's coding standards are defined there, not here.
4. **Note deviations.** If design is wrong, say so in apply-report.md — don't silently deviate.
5. **Mark progress.** Update `tasks.md` as you go, not at the end.
6. **Build-fix loop is mandatory.** Never return without running the project's checks.
7. **Max 3 attempts.** If a build error survives 3 fix attempts, escalate to manual review.
8. **One batch = one phase.** Do not implement tasks from other phases.
9. **Load domain skills.** If touching framework-specific code, load the relevant SKILL.md first.
10. **Bottom-up ordering.** Implement tasks in the order defined by tasks.md phases.
11. **Fix mode is scoped.** Only modify files in the fix list. No improvements beyond the fix.

---

## PARCER Contract

```yaml
phase: apply
preconditions:
  standard_mode:
    - tasks.md exists with ≥1 uncompleted task
    - design.md exists at openspec/changes/{changeName}/
    - spec files exist in openspec/changes/{changeName}/specs/
  fix_mode:
    - review-report.md OR verify-report.md exists
    - fix list is non-empty
postconditions:
  standard_mode:
    - ≥1 task marked [x] in tasks.md
    - apply-report.md written with build health and file manifest
  fix_mode:
    - fix-report-{iteration}.md written with fixes applied and build health
```
