---
name: sdd-verify
description: >
  Technical quality gate. Runs build checks, tests, static analysis, security audit. Compares implementation completeness against tasks/specs.
  Trigger: When user runs /sdd:verify or after sdd-review passes.
license: MIT
metadata:
  version: "2.0"
---

# SDD Verify — Technical Quality Gate

You are executing the **verify** phase inline. Your responsibility is to run **all technical quality checks** and produce a definitive pass/fail verdict. You check build health, test coverage, static analysis, security, and completeness against the task/spec plan. You **never fix issues** — you only report them with enough detail for a follow-up `/sdd:apply` fix pass or the developer to act.

## Activation

User runs `/sdd:verify [--fuzz]`. Reads `tasks.md`, spec files, `design.md`, and optionally `review-report.md` from disk.

## Inputs

Read from disk:

| Input | Source |
|---|---|
| `changeName` | Infer from `openspec/changes/` (the active change folder) |
| `tasks.md` | `openspec/changes/{changeName}/tasks.md` |
| `specs/` | `openspec/changes/{changeName}/specs/` |
| `design.md` | `openspec/changes/{changeName}/design.md` |
| `review-report.md` | `openspec/changes/{changeName}/review-report.md` (optional, if review ran) |
| `--fuzz` flag | Passed via CLI when user runs `/sdd:verify --fuzz` |

---

## Execution Steps

### Step 0 — Token Budget (MANDATORY before anything else)

- quality-timeline.jsonl: Bash("tail -n 5 openspec/changes/{changeName}/quality-timeline.jsonl"). NEVER use the Read tool on this file — Read loads the full file into context.
- Large source files (>150 lines): use `offset`/`limit` to read only the relevant section.
- Do NOT re-read a file already in context.
- Framework SKILL.md: load ONLY for frameworks present in the changed files, not the full tech stack.

### Step 0b — Load Build Commands

Read `openspec/config.yaml` and extract the top-level `commands` block. Store as variables for all subsequent steps:

- `CMD_CHECK` ← `commands.typecheck` or `commands.check`
- `CMD_LINT` ← `commands.lint`
- `CMD_TEST` ← `commands.test`
- `CMD_FORMAT_CHECK` ← `commands.format_check`

**Fallback**: If `config.yaml` does not exist or has no `commands` block, detect the toolchain from project root files (`package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`, `Makefile`, etc.) and infer commands. Use `CLAUDE.md` if it documents check commands.

### Step 1 — Completeness Check

1. Read `tasks.md`. Count total tasks, completed (`[x]`), and incomplete (`[ ]`).
2. Read all spec files in `specs/`. Count total GIVEN/WHEN/THEN scenarios. For each, search the codebase for a corresponding test. Count covered vs. uncovered.
3. Read `design.md`. Extract all interface/contract definitions. For each, check if it exists in the codebase.
4. If `review-report.md` exists and contains REJECT violations, the verdict is automatically **FAIL** — but continue running all checks for the full report.

### Step 1b — Eval-Driven Assessment

Read the `## Eval Definitions` sections from all spec files. If no eval definitions exist (specs pre-date EDD), skip this step.

For each eval definition row:

| Eval Type | How to check | Failure maps to |
|-----------|-------------|-----------------|
| `code-based` | Search changed files and test files for a test whose name contains keywords from the scenario title | Missing = CRITICAL (critical) or WARNING (standard) |
| `model-based` | Semantically assess whether the implementation plausibly satisfies the THEN clause | Not satisfied = WARNING regardless of criticality |
| `human-based` | Note as requiring manual verification — do NOT fail the verdict | Note only |

**Threshold translation for single-run verify:**
- `pass^3 = 1.00` (critical) → test MUST exist → absence = **FAIL**
- `pass@3 ≥ 0.90` (standard) → test SHOULD exist → absence = **PASS_WITH_WARNINGS**

### Step 2 — Build / Compile Check

Run: `{CMD_CHECK}`

- Capture full output (stdout and stderr).
- Count errors. For each: extract file path, line number, error code, message.
- Classify: PASS (0 errors) or FAIL (1+ errors).

### Step 3 — Lint

Run: `{CMD_LINT}`

- Capture full output.
- Count errors and warnings separately. For each error: file, line, rule, message.
- Classify: PASS (0 errors, warnings OK) or FAIL (1+ errors).

### Step 4 — Formatting

Run: `{CMD_FORMAT_CHECK}`

- Classify: PASS (no formatting issues) or FAIL (files need formatting).
- List files that need formatting.

### Step 5 — Tests

Run: `{CMD_TEST}`

- Extract: total tests, passed, failed, skipped.
- For each failure: test name, file path, error message, stack trace.
- Classify: PASS (0 failures) or FAIL (1+ failures).

### Step 5b — Fault Localization Protocol (when tests fail)

When Step 5 detects failures, diagnose each using this structured protocol before continuing. If all tests pass, skip.

#### PREMISES (Test Semantics)

For each failing test:

1. **Test identifier**: Test path and file location.
2. **Setup (Arrange)**: What preconditions does the test establish?
3. **Action (Act)**: What function or operation does the test invoke? Include the exact call signature.
4. **Assertion (Assert)**: What is the expected outcome? Quote the exact assertion.

#### DIVERGENCE CLAIMS

For each failing test, generate one or more divergence claims:

- **CLAIM**: A formal statement cross-referencing a test premise with a specific code location.
  Format: "Test expects [expected behavior] (test file:line), but implementation at [source file:line] produces [actual behavior] because [root cause]."
- **EVIDENCE**: The specific line(s) of source code that cause the divergence.
- **CONFIDENCE**: `HIGH` | `MEDIUM` | `LOW` — based on whether the root cause is certain or hypothetical.

Include all divergence claims in the verify report under a "Fault Localization" section. This structured diagnosis enables `sdd-apply` to fix issues precisely.

### Step 6 — Static Analysis

Scan all files created/modified in this change for anti-patterns. Load the relevant framework SKILL.md(s) and `CLAUDE.md` to determine which patterns are prohibited in this project. Check for:

1. **Language-specific anti-patterns** — per framework SKILL.md and CLAUDE.md (e.g., type system bypasses, compiler suppression directives, unsafe casts).
2. **Unstructured logging** — raw print/log statements instead of structured logger (per CLAUDE.md).
3. **TODO / FIXME markers** — unresolved work items (WARNING).
4. **Dynamic code execution** — `eval()`, `new Function()`, or equivalent (CRITICAL security risk).
5. **DOM injection** — unsanitized HTML insertion (CRITICAL XSS risk).

For each finding: file, line, pattern, severity (CRITICAL or WARNING), description.

### Step 7 — Security Scan

Scan for hardcoded secrets and dangerous patterns:

| Pattern | Severity | Description |
|---|---|---|
| API key patterns (long alphanumeric strings near `key`, `token`, `secret`) | CRITICAL | Possible hardcoded secret |
| Hardcoded password literals | CRITICAL | Hardcoded password |
| `.env` file committed | CRITICAL | Environment file should be gitignored |
| SQL/query string concatenation with user input | CRITICAL | Injection risk |
| Unvalidated URLs from user input passed to HTTP calls | WARNING | SSRF risk |
| Missing input validation on API route handlers | WARNING | Injection risk |

### Step 7b — Dynamic Security Testing / Fuzz (Optional)

Activates when: `--fuzz` flag is passed, OR Step 7 found security issues (auto-escalation), OR the change touches API handlers, auth logic, input parsers, or database operations.

If none of these conditions are met, skip entirely.

#### 7b-1. Identify Fuzz Targets

Scan changed files for functions that handle **external input boundaries**:

| Target Type | Detection Heuristic | Priority |
|---|---|---|
| API route handlers | Functions inside route/endpoint definitions | HIGH |
| Input parsers/validators | Functions that parse or validate external data | HIGH |
| Auth/session logic | Functions in files matching `*auth*`, `*session*`, `*login*`, `*token*` | HIGH |
| Database operations | Functions that call query builders or execute queries | MEDIUM |
| File/path handlers | Functions that accept file path parameters from external sources | MEDIUM |

**Hard limits:** Max **5 target functions** per change. Prioritize HIGH over MEDIUM.

#### 7b-2. Generate Fuzz Test Cases

For each target, generate adversarial test cases across these categories:

| Category | What It Tests |
|---|---|
| **Boundary values** | Empty inputs, max-length inputs, numeric extremes, special values |
| **Injection payloads** | SQL injection, XSS, path traversal, command injection |
| **Type coercion** | Wrong types, null/undefined, prototype pollution attempts |
| **Malformed data** | Truncated input, invalid encoding, deeply nested structures |
| **Auth bypass** | Empty/expired/malformed tokens, missing auth headers |

**Per function:** Max **10 test cases**, covering at least 3 of the 5 categories.

#### 7b-3. Write Temporary Fuzz Test File

Write fuzz tests to `{targetDir}/{feature}.fuzz.test.{ext}` using the project's test runner and language conventions. Load the relevant framework SKILL.md for test patterns.

**Test expectations:** Fuzz tests assert:
1. **No unhandled throws** — the function must not crash on adversarial input
2. **No raw error leakage** — error messages must not expose internals
3. **Input validation triggers** — adversarial input must be rejected, not silently accepted
4. **No prototype pollution** — objects with `__proto__` keys must not modify global prototypes

#### 7b-4. Run Fuzz Tests

Run: `{CMD_TEST} {fuzz test file}`

For each failure, classify:

| Finding Type | Severity |
|---|---|
| Unhandled throw on adversarial input | CRITICAL |
| Injection payload accepted without validation | CRITICAL |
| Internal error details leaked | WARNING |
| Prototype pollution possible | CRITICAL |
| No validation triggered | WARNING |

#### 7b-5. Cleanup and Report

1. **Findings exist:** Keep the fuzz test file. Add findings to the report under `## Dynamic Security Testing (Fuzz)`.
2. **No findings:** Delete the fuzz test file. Note clean result in the report.

### Step 8 — Dependency Audit (if available)

Attempt to run a dependency audit using the detected package manager. If not available, skip and note it. If available, count vulnerabilities by severity.

### Step 9 — Compile Verify Report

Create `openspec/changes/{changeName}/verify-report.md`:

```markdown
# Verification Report: {changeName}

**Date**: {YYYY-MM-DD}
**Verifier**: sdd-verify (automated)
**Verdict**: PASS | PASS_WITH_WARNINGS | FAIL

## Completeness
- Tasks: {completed}/{total} completed
- Spec Scenarios: {covered}/{total} have corresponding tests
- Design Interfaces: {implemented}/{total} implemented

## Build Health
| Check | Status | Details |
|---|---|---|
| Build/Compile | PASS/FAIL | {N} errors |
| Lint | PASS/FAIL | {N} errors, {M} warnings |
| Formatting | PASS/FAIL | {N} files need formatting |
| Tests | PASS/FAIL | {passed} passed, {failed} failed, {skipped} skipped |

## Static Analysis
{Findings from Step 6, grouped by severity}

## Security
{Findings from Step 7}

## Dynamic Security Testing (Fuzz)
{Results from Step 7b, or "skipped"}

## Eval-Driven Assessment
{Results from Step 1b, or "skipped (no eval definitions)"}

## Fault Localization (if tests failed)
{Structured premises and divergence claims from Step 5b}

## Issues Detail
| # | Severity | Category | File | Line | Description | Fixability | Fix Direction |
|---|---|---|---|---|---|---|---|

## Verdict Rationale
{Explanation of why the verdict is PASS, PASS_WITH_WARNINGS, or FAIL}
```

### Step 10 — Present Summary

Write `verify-report.md` and append a JSONL line to `quality-timeline.jsonl` (if quality tracking enabled).

Present a markdown summary to the user, then STOP:

```markdown
## SDD Verify: {change_name}

**Verdict**: {✅ PASS | ⚠️ PASS_WITH_WARNINGS | ❌ FAIL}

### Build Health
| Check | Result | Details |
|-------|--------|---------|
| build | {PASS/FAIL} | {N} errors |
| lint | {PASS/FAIL} | {N} errors, {N} warnings |
| tests | {PASS/FAIL} | {passed}/{total} passed, {N} failed |
| format | {PASS/FAIL} | {N} files need formatting |

### Completeness
- **Tasks**: {N}/{M} complete  |  **Specs**: {N}/{M} covered
- **Interfaces**: {N}/{M} implemented

{If security findings: ### Security\n{summary}\n}
{If eval-driven: ### Eval Results\n- Critical: {N}/{M} passing  |  Standard: {N}/{M} passing\n}
{If FAIL: ### Critical Issues\n{issue list with file:line and fixability}\n}
{If warnings: ### Warnings\n{issue list}\n}

**Artifact**: `openspec/changes/{changeName}/verify-report.md`

{If PASS: **Next step**: Run `/sdd:clean` to remove dead code, or `/sdd:archive` to close the change.}
{If PASS_WITH_WARNINGS: **Next step**: Review warnings above. Run `/sdd:clean` or `/sdd:archive` when satisfied.}
{If FAIL and allAutoFixable: **Next step**: Run `/sdd:apply` in fix mode — all issues are auto-fixable.}
{If FAIL and has HUMAN_REQUIRED: **Next step**: Manually fix the HUMAN_REQUIRED issues above, then re-run `/sdd:verify`.}
```

---

## Rules — Hard Constraints

1. **Never fix issues.** Report only. Fixing is `sdd-apply`'s job or the developer's.
2. **Capture full command output.** Always capture stderr too.
3. **CRITICAL issues = FAIL verdict.** No exceptions.
4. **WARNING issues = PASS_WITH_WARNINGS.** The change can proceed but issues should be addressed.
5. **SUGGESTION issues = informational.** They do not affect the verdict.
6. **Review-report REJECT violations = automatic FAIL.**
7. **Be precise.** Every issue must have a file path and line number.
8. **Run ALL checks.** Even if build fails, still run lint, tests, and static analysis. The full picture is needed.
9. **Distinguish pre-existing issues.** Failures in files NOT touched by this change are "pre-existing."
10. **Completeness matters.** Passing builds but incomplete tasks = still incomplete.

---

## Fixability Classification

Every issue MUST include a `fixability` field:

| Fixability | Criteria | Examples |
|---|---|---|
| `AUTO_FIXABLE` | Clear mechanical fix derivable from the error and code context | Build errors, lint violations, formatting issues, anti-pattern usage, TODO markers |
| `HUMAN_REQUIRED` | Requires architectural judgment, business decision, or design rethink | Missing feature logic, security vulnerabilities needing risk assessment, pre-existing failures blocking the build |

Include a `fixDirection` field for `AUTO_FIXABLE` issues: a 1-sentence instruction for `sdd-apply`.

---

## Verdict Decision Matrix

| Condition | Verdict |
|---|---|
| All checks PASS, all tasks complete, no CRITICAL issues | PASS |
| All checks PASS but has WARNING-level issues | PASS_WITH_WARNINGS |
| Any CRITICAL issue (build error, test failure, security vuln, REJECT violation) | FAIL |
| Tasks incomplete (not all marked [x]) | FAIL |
| Spec scenarios without tests > 20% of total | PASS_WITH_WARNINGS |
| Spec scenarios without tests > 50% of total | FAIL |

---

## Edge Cases

| Situation | Action |
|---|---|
| Build command not found | Note as CRITICAL — build infrastructure missing |
| Tests take > 5 minutes | Let them run up to 10 minutes, then timeout and note it |
| No test files exist at all | Flag as CRITICAL — untested code cannot pass verification |
| `review-report.md` not provided | Skip review-report checks, note that semantic review was skipped |
| Dependency audit not available | Skip and note — do not count as failure |
| Static analysis finds issues in vendor/dependency dirs | Ignore — only scan project source |

---

## PARCER Contract

```yaml
phase: verify
preconditions:
  - review-report.md exists at openspec/changes/{changeName}/ (or review explicitly skipped)
  - implementation files exist on disk
postconditions:
  - verify-report.md written to openspec/changes/{changeName}/
  - verify-report.md contains all build checks
  - verify-report.md verdict is PASS, PASS_WITH_WARNINGS, or FAIL
```
