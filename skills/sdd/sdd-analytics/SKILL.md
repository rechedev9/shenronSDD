---
name: sdd-analytics
description: >
  Quality analytics from phase delta tracking data. Reads quality-timeline.jsonl files, computes deltas between
  consecutive phases, and generates markdown reports showing quality curves, phase value rankings, and insights.
  Trigger: When user runs /sdd:analytics [name] with --mode single|aggregate|compare.
license: MIT
metadata:
  version: "1.0"
---

# SDD Analytics — Phase Delta Tracking

You are executing the **analytics** phase inline. Your responsibility is to **read quality timeline data**, compute deltas between consecutive phases, and produce **markdown reports** showing how each SDD phase affected quality. You never modify source code or specs — you only read `quality-timeline.jsonl` files and produce reports.

## Activation

User runs `/sdd:analytics [change-name] [--mode single|aggregate|compare]`. Reads `quality-timeline.jsonl` from disk.

## Inputs

Read from disk:

| Input | Source |
|---|---|
| `changeName` | CLI argument (for `single` mode), or omit for `aggregate` |
| `changeNames` | Multiple CLI arguments (for `compare` mode) |
| `mode` | Flag `--mode single|aggregate|compare` (default: `single`) |
| `quality-timeline.jsonl` | `openspec/changes/{changeName}/quality-timeline.jsonl` |

---

## QualitySnapshot Schema

Each line in `quality-timeline.jsonl` is a JSON object conforming to this schema:

```typescript
interface QualitySnapshot {
  /** Name of the SDD change */
  changeName: string
  /** SDD phase that produced this snapshot */
  phase: "explore" | "propose" | "spec" | "design" | "tasks" | "apply" | "review" | "verify" | "clean" | "archive"
  /** ISO 8601 timestamp of when the phase completed */
  timestamp: string
  /** Phase completion status: SUCCESS, PARTIAL, ERROR, SKIPPED */
  agentStatus: "SUCCESS" | "PARTIAL" | "ERROR" | "SKIPPED"
  /** Task and spec completion metrics */
  completeness: {
    tasksCompleted: number | null
    tasksTotal: number | null
    specsCovered: number | null
    specsTotal: number | null
  }
  /** Build tool results */
  buildHealth: {
    typecheck: "PASS" | "FAIL" | null
    typecheckErrors: number | null
    lint: "PASS" | "FAIL" | null
    lintErrors: number | null
    lintWarnings: number | null
    format: "PASS" | "FAIL" | null
    tests: "PASS" | "FAIL" | null
    testsPassed: number | null
    testsFailed: number | null
  }
  /** Static analysis counts */
  staticAnalysis: {
    bannedAny: number | null
    typeAssertions: number | null
    compilerSuppressions: number | null
    consoleUsage: number | null
    todoFixme: number | null
  }
  /** Security scan counts */
  security: {
    hardcodedSecrets: number | null
    injectionRisks: number | null
    xssVectors: number | null
    missingValidation: number | null
  }
  /** Issue counts from review */
  issues: {
    critical: number | null
    warning: number | null
    suggestion: number | null
  }
  /** Scope of the change */
  scope: {
    filesCreated: number | null
    filesModified: number | null
    filesTotal: number | null
  }
  /** Raw phase data passthrough — preserves phase-specific data for drilldown */
  phaseSpecific: Record<string, unknown>
}
```

---

## Extraction Mapping — JSONL Fields

Each SDD phase appends a JSONL line to `quality-timeline.jsonl` with fields that map directly to QualitySnapshot. The mapping is consistent across all phases — no per-phase conditional logic needed.

| QualitySnapshot Field | JSONL Source | Notes |
|---|---|---|
| `changeName` | `changeName` | null for init/analytics |
| `phase` | `phase` | e.g., "apply", "review", "verify" |
| `timestamp` | `timestamp` | ISO 8601, written by each phase |
| `agentStatus` | `agentStatus` | SUCCESS / PARTIAL / ERROR |
| `issues.critical` | `issueCount.critical` | 0 if no issues |
| `issues.warning` | `issueCount.warnings` | null if absent |
| `issues.suggestion` | `phaseSpecific.suggestionCount` | null if absent |
| `buildHealth.typecheck` | `buildHealth.typecheck` | null if phase doesn't build |
| `buildHealth.typecheckErrors` | `phaseSpecific.buildHealthDetail.typecheck.errorCount` | null if absent |
| `buildHealth.lint` | `buildHealth.lint` | null if phase doesn't build |
| `buildHealth.lintErrors` | `phaseSpecific.buildHealthDetail.lint.errorCount` | null if absent |
| `buildHealth.lintWarnings` | `phaseSpecific.buildHealthDetail.lint.warningCount` | null if absent |
| `buildHealth.tests` | `buildHealth.tests` | null if phase doesn't build |
| `buildHealth.testsPassed` | `phaseSpecific.buildHealthDetail.tests.passed` | null if absent |
| `buildHealth.testsFailed` | `phaseSpecific.buildHealthDetail.tests.failed` | null if absent |
| `buildHealth.format` | `buildHealth.format` | null if phase doesn't build |
| `completeness.tasks` | `completeness.tasksCompleted / tasksTotal` | null if phase doesn't track tasks |
| `completeness.specs` | `completeness.specsCovered / specsTotal` | null if phase doesn't track specs |
| `scope.filesCreated` | `phaseSpecific.filesCreated` | Count |
| `scope.filesModified` | `phaseSpecific.filesModified` | Count |
| `staticAnalysis` | `phaseSpecific.staticAnalysis` | null if absent — only verify populates |
| `security` | `phaseSpecific.security` | null if absent — only verify populates |
| `phaseSpecific` | `phaseSpecific` | Full passthrough |

**Null propagation**: Planning phases (explore, propose, spec, design, tasks) will have null buildHealth and zero-value metrics for tasks/specs that don't apply. This is correct — write them as-is for timeline completeness.

---

## Delta Computation Rules

Deltas measure the change between consecutive snapshots in the timeline.

### Numeric Fields

```
delta = snapshot[N].field - snapshot[N-1].field
```

Positive delta = improvement for: `completeness.*`, `testsPassed`, `scope.filesTotal`
Negative delta = improvement for: `issues.critical`, `issues.warning`, `typecheckErrors`, `lintErrors`, `testsFailed`, `staticAnalysis.*`, `security.*`

### Status Fields

Convert to numeric before computing delta:

```
PASS = 1, FAIL = 0, null = -1
```

A delta of `+1` (FAIL → PASS) is an improvement. A delta of `-1` (PASS → FAIL) is a regression.

### Null Propagation

- If either `snapshot[N]` or `snapshot[N-1]` has `null` for a field, the delta is `null` (unknown).
- `null` deltas are displayed as `—` in reports.
- The first snapshot in a timeline has no delta (it's the baseline).

### Composite Quality Score

An optional aggregate score combining key metrics (used for the quality curve chart):

```
score = (completeness_ratio * 30) + (build_health_ratio * 30) + (issue_penalty * 20) + (security_penalty * 20)

where:
  completeness_ratio = tasksCompleted / tasksTotal  (0 if null)
  build_health_ratio = (pass_count / check_count)   (typecheck, lint, format, tests)
  issue_penalty = max(0, 20 - (critical * 10 + warning * 3))  (floor at 0)
  security_penalty = max(0, 20 - (hardcodedSecrets * 20 + injectionRisks * 10 + xssVectors * 10))
```

Score ranges from 0 to 100. First snapshot without enough data defaults to `null`.

---

## Report Modes

### Mode: `single`

Analyzes one change's quality timeline.

**Input**: `changeName` — reads `openspec/changes/{changeName}/quality-timeline.jsonl`

**Output**: `openspec/analytics/{changeName}-report.md`

Report structure:

```markdown
# Quality Report: {changeName}

**Generated**: {YYYY-MM-DD HH:mm}
**Phases Tracked**: {N}
**Timeline Span**: {first timestamp} → {last timestamp}

## Quality Curve

| Phase | Score | Δ | Status | Completeness | Build | Issues | Security |
|---|---|---|---|---|---|---|---|
| explore | — | — | SUCCESS | —/— | —/—/—/— | —/—/— | —/—/—/— |
| propose | — | — | SUCCESS | —/— | —/—/—/— | —/—/— | —/—/—/— |
| spec | 10 | — | SUCCESS | 0/15 | —/—/—/— | —/—/— | —/—/—/— |
| tasks | 15 | +5 | SUCCESS | 0/12 | —/—/—/— | —/—/— | —/—/—/— |
| apply (b1) | 45 | +30 | SUCCESS | 6/12 | ✓/✗/✓/✓ | 0/2/0 | 0/0/0/0 |
| apply (b2) | 68 | +23 | SUCCESS | 12/12 | ✓/✓/✓/✓ | 0/1/0 | 0/0/0/0 |
| review | 62 | -6 | SUCCESS | 12/12 | ✓/✓/✓/✓ | 2/5/3 | 0/0/0/0 |
| verify | 70 | +8 | SUCCESS | 12/12 | ✓/✓/✓/✓ | 0/2/1 | 0/0/0/0 |
| clean | 75 | +5 | SUCCESS | 12/12 | ✓/✓/✓/✓ | 0/1/0 | 0/0/0/0 |

Legend: Completeness = tasks/specs, Build = typecheck/lint/format/tests, Issues = critical/warning/suggestion, Security = secrets/injection/xss/validation

## Phase Deltas

| Phase | Score Δ | Tasks Δ | Type Errors Δ | Lint Errors Δ | Test Failures Δ | Critical Issues Δ |
|---|---|---|---|---|---|---|
| spec → tasks | +5 | — | — | — | — | — |
| tasks → apply(b1) | +30 | +6 | — | — | — | — |
| apply(b1) → apply(b2) | +23 | +6 | -3 | -2 | 0 | 0 |
| apply(b2) → review | -6 | 0 | 0 | 0 | 0 | +2 |
| review → verify | +8 | 0 | 0 | 0 | 0 | -2 |
| verify → clean | +5 | 0 | 0 | -1 | 0 | 0 |

## Insights

- **Highest value phase**: apply (b1) — Score Δ: +30
- **Regressions detected**: review (-6) — found 2 critical issues not visible before
- **Biggest cleanup**: apply (b2) — resolved 3 type errors, 2 lint errors
- **Final quality score**: 75/100

## Phase Value Ranking

| Rank | Phase | Score Δ | Primary Contribution |
|---|---|---|---|
| 1 | apply (b1) | +30 | Initial implementation |
| 2 | apply (b2) | +23 | Build fixes + task completion |
| 3 | verify | +8 | Quality gate enforcement |
| 4 | clean | +5 | Dead code removal |
| 5 | spec | +5 | Established spec baseline |
| 6 | review | -6 | Surfaced hidden issues (negative Δ expected) |
```

### Mode: `aggregate`

Analyzes all tracked changes in the project.

**Input**: Scans `openspec/changes/*/quality-timeline.jsonl` (excludes `archive/`)

**Output**: `openspec/analytics/aggregate-report.md`

Report structure:

```markdown
# Aggregate Quality Report

**Generated**: {YYYY-MM-DD HH:mm}
**Changes Analyzed**: {N}
**Total Phase Snapshots**: {M}

## Average Phase Deltas

| Phase | Avg Score Δ | Median Score Δ | Std Dev | Occurrences |
|---|---|---|---|---|
| explore | — | — | — | 5 |
| propose | — | — | — | 5 |
| spec | +3.2 | +3 | 1.1 | 5 |
| tasks | +4.8 | +5 | 0.9 | 5 |
| apply | +24.6 | +25 | 8.2 | 12 |
| review | -3.1 | -2 | 4.5 | 5 |
| verify | +6.2 | +6 | 2.3 | 5 |
| clean | +3.8 | +4 | 1.5 | 4 |

## Phase Value Ranking (by average Δ)

1. **apply** — Avg +24.6 (highest value producer)
2. **verify** — Avg +6.2 (consistent quality improver)
3. **tasks** — Avg +4.8 (planning baseline)
4. **clean** — Avg +3.8 (reliable cleanup)
5. **spec** — Avg +3.2 (establishes measurement)
6. **review** — Avg -3.1 (surfaces issues — negative expected)

## Insights

- {Generated observations about patterns across changes}
- {Phases that consistently regress vs improve}
- {Changes with unusual quality curves}

## Per-Change Summary

| Change | Phases | Final Score | Duration | Top Phase |
|---|---|---|---|---|
| auth-system | 9 | 82 | 3h 20m | apply (+35) |
| user-profile | 7 | 71 | 2h 10m | apply (+28) |
```

### Mode: `compare`

Side-by-side comparison of multiple changes.

**Input**: `changeNames` — list of 2-4 change names

**Output**: `openspec/analytics/compare-report.md`

Report structure:

```markdown
# Comparison Report

**Generated**: {YYYY-MM-DD HH:mm}
**Changes Compared**: {change1}, {change2}, ...

## Quality Curves

| Phase | {change1} | {change2} | ... |
|---|---|---|---|
| spec | 10 | 12 | ... |
| tasks | 15 | 18 | ... |
| apply | 68 | 55 | ... |
| review | 62 | 52 | ... |
| verify | 70 | 65 | ... |
| clean | 75 | 70 | ... |

## Phase Delta Comparison

| Phase | {change1} Δ | {change2} Δ | Winner |
|---|---|---|---|
| apply | +53 | +37 | {change1} |
| review | -6 | -3 | {change2} |
| verify | +8 | +13 | {change2} |
| clean | +5 | +5 | Tie |

## Insights

- {Which change had a smoother quality curve}
- {Which phases differed most between changes}
- {Correlation between spec quality and final score}
```

---

## Execution Steps

### Step 1 — Load Timeline Data

1. Based on `mode`, locate the relevant `quality-timeline.jsonl` file(s):
   - `single`: `openspec/changes/{changeName}/quality-timeline.jsonl`
   - `aggregate`: All `openspec/changes/*/quality-timeline.jsonl` (exclude `archive/`)
   - `compare`: `openspec/changes/{name}/quality-timeline.jsonl` for each name in `changeNames`
2. Read each file line by line. Parse each line as JSON into a `QualitySnapshot`.
3. If a line fails to parse, skip it and log a warning in the report.
4. If no timeline files exist, report gracefully: "No quality-timeline.jsonl found for {changeName}. Run a full SDD pipeline to generate tracking data."

### Step 2 — Compute Deltas

For each change's timeline (sorted by timestamp):
1. The first snapshot is the **baseline** — it has no deltas.
2. For each subsequent snapshot, compute deltas using the rules above.
3. Compute the composite quality score for each snapshot.
4. Store deltas alongside snapshots for report generation.

### Step 3 — Generate Report

Based on `mode`, generate the appropriate report using the templates above.

1. Create the `openspec/analytics/` directory if it doesn't exist.
2. Write the report to the appropriate path.
3. Replace all template placeholders with computed values.
4. Generate the Insights section by analyzing patterns:
   - Identify the highest-value phase (largest positive Δ).
   - Identify regressions (negative Δ).
   - Note phases that were skipped or failed.
   - For `aggregate`: identify outlier changes and consistent patterns.
   - For `compare`: identify which change had smoother progression.

### Step 4 — Present Summary

Write the analytics report to `openspec/analytics/{changeName}-analytics.md` (single/compare) or `openspec/analytics/aggregate-{date}.md` (aggregate).

Present a markdown summary to the user, then STOP:

```markdown
## SDD Analytics: {change_name | "Aggregate"} ({mode})

**Changes analyzed**: {N}  |  **Snapshots**: {N}  |  **Timeline span**: {Xh Ym}

### Quality Curve
| Phase | Score | Delta | Highest Value Phase? |
|-------|-------|-------|---------------------|
| spec | {N} | — | |
| tasks | {N} | +{N} | |
| apply | {N} | +{N} | {✓ if highest} |
| review | {N} | {±N} | |
| verify | {N} | +{N} | |
| clean | {N} | +{N} | |

**Final score**: {N}/100  |  **Highest value phase**: {phase} (+{delta})  |  **Regressions**: {N}

{If regressions: ### Regressions Detected\n{list: phase → score drop + cause}\n}

{If compare mode:
### Comparison: {change_a} vs {change_b}
| Metric | {change_a} | {change_b} |
|--------|-----------|-----------|
| Final score | {N} | {N} |
| Smoothest progression | {yes|no} | {yes|no} |
}

**Report**: `openspec/analytics/{reportPath}`
```

---

## Rules — Hard Constraints

1. **Never modify source code, specs, or timeline files.** You only read and produce reports.
2. **Never block on malformed data.** If a JSONL line is invalid, skip it and note the warning.
3. **Null means unknown, not zero.** Display `null` deltas as `—`, never as `0`.
4. **Review's negative delta is expected.** Do not flag review regressions as problems — review *discovers* issues, which correctly lowers the score.
5. **Always create the analytics directory.** If `openspec/analytics/` doesn't exist, create it.
6. **Respect the phaseSpecific passthrough.** When a user asks for drilldown details, reference `phaseSpecific` data.
7. **No timeline = graceful message.** Never error out because a timeline file is missing.
8. **Aggregate excludes archive.** Only scan `openspec/changes/*/` excluding `openspec/changes/archive/`.
9. **Timestamps must be sorted.** If snapshots are out of order (unlikely), sort by timestamp before computing deltas.
10. **Reports are markdown only.** No HTML, no JSON output files. The JSONL timeline is the programmatic interface.

---

## Edge Cases

| Situation | Action |
|---|---|
| No `quality-timeline.jsonl` exists for the requested change | Report: "No quality-timeline.jsonl found. Run a full SDD pipeline to generate tracking data." Return `status: "SUCCESS"` with empty results. |
| Timeline has only 1 snapshot | Report the baseline with no deltas. Note: "Only one phase tracked — deltas require at least two snapshots." |
| Planning phases (explore, propose) have all-null metrics | Display `—` for all metric columns. These phases produce no quality metrics by design. |
| Apply has multiple batches | Each batch is a separate row. Label as `apply (b1)`, `apply (b2)`, etc. |
| A phase was SKIPPED | Include in timeline with `agentStatus: "SKIPPED"` and all metrics as `null`. |
| A phase returned ERROR | Include in timeline with `agentStatus: "ERROR"`. Extract whatever metrics are available. |
| Aggregate mode with only 1 change | Produce the aggregate report anyway — it's still useful as a baseline. |
| Compare mode with a change that has no timeline | Include the change in the report with "No data" in all cells. |
| Score computation has insufficient data | Report score as `null` (`—`). Don't default to 0. |
| Timeline file has duplicate timestamps for same phase | Keep both — they may represent retries. Sort by timestamp. |

---

## PARCER Contract

```yaml
phase: analytics
preconditions: []
postconditions:
  - report written to openspec/analytics/
  - analytics report contains quality curve and insights
```
