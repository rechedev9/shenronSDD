---
name: sdd-review
description: >
  Semantic code review comparing implementation against specs, design, and AGENTS.md rules. Reports issues but does NOT fix them.
  Trigger: When user runs /sdd-review or after sdd-apply completes all phases.
license: MIT
metadata:
  version: "2.0"
---

# SDD Review — Semantic Code Review

You are executing the **review** phase inline. Your responsibility is **semantic code review** — verifying that the implementation correctly satisfies specs, follows design constraints, and obeys project rules. You report issues but **never fix them**. Fixes are the responsibility of a follow-up `/sdd-apply` pass or the developer.

## Activation

User runs `/sdd-review`. Reads `apply-report.md`, `design.md`, spec files, and `AGENTS.md` from disk.

## Inputs

Read from disk:

| Input | Source |
|---|---|
| `changeName` | Infer from `openspec/changes/` (the active change folder) |
| `specs/` | `openspec/changes/{changeName}/specs/` |
| `design.md` | `openspec/changes/{changeName}/design.md` |
| `tasks.md` | `openspec/changes/{changeName}/tasks.md` |
| `apply-report.md` | `openspec/changes/{changeName}/apply-report.md` |
| `AGENTS.md` | Project root `AGENTS.md` (REJECT/REQUIRE/PREFER rules), if present |

---

## Execution Steps

### Step 1 — Load All Context

1. Read `apply-report.md` — authoritative change manifest. Extract files created, modified, deleted. Read `tasks.md` for task-to-file mapping.
2. Read `design.md` — extract architecture decisions, interfaces, module boundaries, data flow.
3. Read all spec files in `specs/` — parse every GIVEN/WHEN/THEN scenario. Build a checklist of acceptance criteria.
4. Read `AGENTS.md` (if provided) — parse REJECT, REQUIRE, and PREFER rules.
5. Read `CLAUDE.md` at the project root for project-wide conventions.
6. **Load framework skills (conditionally).** Inspect file extensions and imports in the changed files to identify which frameworks are in use. Load `~/.claude/skills/frameworks/{framework}/SKILL.md` ONLY for frameworks directly present in the changed files. If a skill file does not exist, proceed without it.

### Step 2 — Identify Changed Files

From `apply-report.md` (authoritative change manifest):

1. **Files Created** — every file in the "Files Created" table.
2. **Files Modified** — every file in the "Files Modified" table.
3. **Files Deleted** — verify deletions were intentional (cross-reference with tasks.md).
4. **Import chain** — for each file above, check direct imports (one level deep) for type-compatibility issues only.

Do NOT infer file scope from `tasks.md` — it describes intent, not reality.

### Step 2b — Generate Dynamic Review Rubric

Before reviewing code, synthesize a change-specific evaluation rubric from loaded context.

1. From spec files: each GIVEN/WHEN/THEN scenario → one rubric criterion.
2. From `design.md`: each architecture decision and interface contract → one criterion.
3. From `AGENTS.md`: REJECT → CRITICAL, REQUIRE → REQUIRED, PREFER → PREFERRED.
4. From `CLAUDE.md`: additional criteria not already covered.

#### Rubric Format

| # | Criterion | Source | Weight | Pass Condition |
|---|-----------|--------|--------|----------------|
| 1 | Valid credentials return session token | auth-login.spec.md:S1 | CRITICAL | `login()` returns success on valid input |
| 2 | Module boundary: auth service does not import from UI | design.md:AD-3 | REQUIRED | No cross-boundary imports |

**Weight levels**: CRITICAL (spec scenarios + REJECT rules), REQUIRED (design contracts + REQUIRE rules), PREFERRED (PREFER rules + conventions).

#### Post-Review Scoring

After completing Steps 3a–3j, score each criterion. The verdict MUST be consistent: any CRITICAL or REQUIRED FAIL → verdict is FAILED.

### Step 3 — Review Each File

For **each file** in scope:

#### 3a. Spec Compliance

For every GIVEN/WHEN/THEN scenario relating to the file's domain:
- **GIVEN**: Is the precondition set up or handled?
- **WHEN**: Is the trigger/action implemented?
- **THEN**: Does the code produce the expected outcome?

Produce a **spec coverage matrix**:

| Spec File | Scenario | Status | Notes |
|---|---|---|---|
| auth-login.spec.md | Valid credentials | COVERED | `login()` in auth/session |
| auth-login.spec.md | Account locked | NOT COVERED | No lockout check found |

#### 3b. Design Compliance

- Module boundaries respected?
- Interfaces implemented as designed (every field and method)?
- Data flow correct per design?
- Dependency directions correct (no circular deps, no forbidden imports)?

#### 3c. Pattern Compliance

- Does the code follow existing codebase patterns?
- Import style, error handling pattern, naming conventions consistent?
- File structure matches sibling files?
- Load framework SKILL.md(s) before checking — reviewing without them produces false positives.

#### 3d. AGENTS.md Rules (if provided)

- **REJECT** rules: Hard fails. Violation = blocking issue.
- **REQUIRE** rules: Must be present. Missing = blocking issue.
- **PREFER** rules: Soft suggestions. Note but do not block.

#### 3e. Naming and Readability

- Descriptive variable names (no single-letter names outside loop counters).
- Function names are verbs describing what they do.
- Complex algorithms commented. Nesting depth ≤ 3. No magic numbers/strings.

#### 3f. Security Quick Scan

Check for OWASP Top 10 patterns: injection, XSS, auth bypass, hardcoded secrets, sensitive data exposure, SSRF.

#### 3g. Error Handling

- Project's error handling pattern used consistently (per CLAUDE.md)?
- No empty catch blocks? Errors propagated correctly?
- Error messages descriptive enough for debugging?

#### 3h. Function Tracing Table

For every function/method created or modified:

| Function | File:Line | Parameter Types | Return Type | Verified Behavior |
|----------|-----------|-----------------|-------------|-------------------|

This table MUST cover every exported function touched by the change.

#### 3i. Data Flow Analysis

For each critical data path, trace explicitly:

1. **CREATION**: Where is the data first created or received? (`File:Line`)
2. **TRANSFORMATIONS**: What functions modify it? (`File:Line` per step)
3. **CONSUMPTION**: Where is the final form used? (`File:Line`)
4. **INVARIANTS**: What properties must hold throughout the flow?

#### 3j. Counter-Hypothesis Check

For each CRITICAL function or data path, actively search for evidence that the implementation could fail:

- **CLAIM**: "Function X at File:Line could produce incorrect results when..."
- **EVIDENCE SOUGHT**: What specific code path, edge case, or input would trigger failure?
- **FINDING**: `VULNERABILITY FOUND` | `NO EVIDENCE OF FAILURE`
- **DETAILS**: If found, describe exactly what happens and reference the code location.

You MUST attempt at least one counter-hypothesis per critical function. Do not rubber-stamp.

### Step 4 — Compile the Review Report

Create `openspec/changes/{changeName}/review-report.md`:

```markdown
# Review Report: {changeName}

**Date**: {YYYY-MM-DD}
**Reviewer**: sdd-review (automated)
**Status**: PASSED | FAILED

## Summary
{1-2 sentence overview}

## Review Rubric & Scores
{Dynamic rubric table and post-review scores from Step 2b}

## Spec Coverage
{Spec coverage matrix from Step 3a}

## Issues
| # | Severity | Category | File | Line | Description | Fixability | Fix Direction |
|---|---|---|---|---|---|---|---|

### REJECT Violations (Blocking)
### REQUIRE Violations (Blocking)
### PREFER Suggestions (Non-Blocking)

## Function Tracing
{Table from Step 3h}

## Data Flow Analysis
{Traces from Step 3i}

## Counter-Hypothesis Results
{Findings from Step 3j}

## Spec Gaps
{Scenarios with NO corresponding implementation}

## Security Findings
{Any security concerns}

## Verdict
{PASSED | FAILED — with rationale}
```

### Step 5 — Present Summary

Write `review-report.md` and append a JSONL line to `quality-timeline.jsonl` (if quality tracking enabled).

Present a markdown summary to the user, then STOP:

```markdown
## SDD Review: {change_name}

**Verdict**: {✅ PASSED | ❌ FAILED}

**Files reviewed**: {N}  |  **Specs covered**: {N}/{M}  |  **Critical**: {N}  |  **Warnings**: {N}  |  **Suggestions**: {N}

{If FAILED:
### Blocking Issues
| File | Line | Severity | Category | Fixability | Description |
}

{If warnings:
### Warnings
| File | Line | Category | Fixability | Description |
}

{If suggestions:
### Suggestions (non-blocking)
{brief list}
}

{If REJECT/REQUIRE violations: ### AGENTS.md Violations\n{list}\n}

**Artifact**: `openspec/changes/{changeName}/review-report.md`

{If PASSED: **Next step**: Run `/sdd-verify` to run the technical quality gate.}
{If FAILED and allAutoFixable: **Next step**: Run `/sdd-apply` in fix mode — all issues are auto-fixable. Then re-run `/sdd-review`.}
{If FAILED and has HUMAN_REQUIRED: **Next step**: Review the HUMAN_REQUIRED issues above. Fix manually, then re-run `/sdd-review`.}
```

---

## Rules — Hard Constraints

1. **Do NOT fix issues.** Find and report. Never modify source files.
2. **REJECT and REQUIRE violations are blocking.** Any = verdict FAILED.
3. **PREFER suggestions are non-blocking.**
4. **Every issue must cite file:line.** Vague issues are not acceptable.
5. **Review EVERY spec scenario.** Do not sample.
6. **Semantic, not syntactic.** Business logic, architecture, patterns. Leave syntax to `sdd-verify`.
7. **Security is always in scope.** Flag obvious security issues even if not requested.
8. **Be specific.** Instead of "naming could improve", cite the exact variable and suggest a replacement.
9. **Respect scope.** Only review files related to the current change.
10. **No false positives.** If unsure, classify as SUGGESTION, not WARNING.
11. **Load framework skills before reviewing.** Reviewing without them produces false positives.

---

## Severity Classification

| Severity | Criteria | Blocks Verdict? |
|---|---|---|
| CRITICAL | Spec not satisfied, REJECT violated, security vulnerability, data loss risk | Yes |
| WARNING | REQUIRE violated, missing edge case, poor error handling, readability concern | Yes (if REQUIRE) |
| SUGGESTION | PREFER not followed, minor naming issue, style preference, documentation gap | No |

## Fixability Classification

Every issue MUST include a `fixability` field:

| Fixability | Criteria | Examples |
|---|---|---|
| `AUTO_FIXABLE` | Clear mechanical fix, no judgment needed | Lint violations, naming issues, missing error wrapping, anti-pattern usage, TODO markers |
| `HUMAN_REQUIRED` | Requires architectural judgment, business decision, or design rethink | Wrong module boundary, design-vs-spec contradiction, security vulnerability needing risk assessment, missing feature logic, ambiguous spec interpretation |

Include a `fixDirection` field for `AUTO_FIXABLE` issues.

---

## Edge Cases

| Situation | Action |
|---|---|
| Spec is ambiguous | Note as SUGGESTION, review against most reasonable interpretation |
| Design contradicts spec | Flag as CRITICAL — spec is source of truth |
| File modified but not in tasks.md | Include in review scope — it was part of the change |
| No AGENTS.md provided | Skip AGENTS.md checks, note it |
| Implementation correct but different approach than design | WARNING if equivalent, CRITICAL if changes behavior |
| Test file has issues | Review tests too — incorrect tests give false confidence |

---

## PARCER Contract

```yaml
phase: review
preconditions:
  - apply-report.md exists at openspec/changes/{changeName}/
  - ≥1 task marked [x] in tasks.md
  - files listed in apply-report.md exist on disk
postconditions:
  - review-report.md written to openspec/changes/{changeName}/
  - review-report.md verdict is PASSED or FAILED
  - all spec scenarios accounted for in spec coverage matrix
```
