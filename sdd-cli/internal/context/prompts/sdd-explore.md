---
name: sdd-explore
description: >
  Investigate a codebase area or idea. Read-only analysis with risk assessment.
  Trigger: When user runs /sdd-explore or needs to understand a part of the codebase before proposing changes.
license: MIT
metadata:
  version: "1.0"
---

# SDD Explore

You are executing the **explore** phase inline. Your output enables informed decision-making before any code changes are proposed. You produce structured, evidence-based analysis with concrete file paths and risk assessments.

## Activation

User runs `/sdd-explore [topic]` optionally with `--change-name <name>` and `--detail-level concise|standard|deep` (default: standard).

## Execution Steps

### Step 1: Load Project Context

1. Read `openspec/config.yaml` for:
   - Technology stack details
   - Architecture patterns
   - Coding conventions and constraints
   - Known directory structure
2. If `config.yaml` does not exist, warn and proceed with manual detection.
3. Read `CLAUDE.md` for additional project rules that may affect analysis.
4. **Query memory for prior knowledge** (if `config.yaml → capabilities.memory_enabled` is `true`):
   - Run `mem_search` with a natural language query describing the topic. Semantic matching is automatic — no synonym expansion needed.
   - Pass the project name via the `project` parameter to filter results to this project.
   - Look for prior explorations, architectural decisions, discovered patterns, or bugfix history related to the topic.
   - Review returned results by score — higher scores indicate stronger semantic relevance. Discard results that reference deleted files or deprecated patterns.
   - If no matches found, proceed normally — this step is non-blocking.

### Step 2: Identify Search Strategy

Based on the `topic`, determine the search strategy:

| Topic Type             | Search Strategy                                    |
|------------------------|----------------------------------------------------|
| Feature area           | Glob for feature directory, Grep for imports/usage |
| Bug investigation      | Grep for error messages, stack trace patterns      |
| Dependency analysis    | Read package.json, Grep for import statements      |
| Performance concern    | Grep for hot paths, database queries, renders      |
| Security audit         | Grep for user input handling, auth patterns        |
| Architecture question  | Glob for directory structure, Read entry points    |
| Data flow              | Grep for type definitions, function signatures     |

### Step 3: Execute Broad Search

1. Use **Glob** to find relevant files:
   - Search for files matching the topic keywords
   - Search for related configuration files
   - Search for test files that reveal expected behavior
2. Use **Grep** to find relevant code patterns:
   - Search for type definitions related to the topic
   - Search for function names and exports
   - Search for imports and usage of relevant modules
   - Search for comments and documentation within code
3. If `focus_paths` is provided, prioritize those paths but do not ignore related files outside them.

### Step 4: Deep Analysis (Structured Exploration Protocol)

For each relevant file discovered, you MUST complete the following reasoning template internally before moving to the next file. This is not optional — skipping it produces shallow, exploration-free analysis.

#### 4a. Pre-Read Hypothesis

Before opening any file, fill:

- **HYPOTHESIS**: What do you expect to find in this file, and why do you believe it is relevant to the topic? (1-2 sentences)
- **EVIDENCE**: What specific evidence (from tests, prior files read, grep results, or the topic itself) led you to this file?
- **CONFIDENCE**: `HIGH` | `MEDIUM` | `LOW` — How certain are you about this hypothesis? A LOW confidence signals a speculative read; HIGH means strong prior evidence supports your expectation. Declaring confidence calibrates whether a subsequent CONFIRMED or REFUTED result is surprising or expected.

#### 4b. Read and Observe

1. **Read the file** to understand its purpose and structure.
2. **Map exports and imports** to understand the dependency graph.
3. **Identify interfaces and types** that define contracts.
4. **Trace data flow** from entry point through transformations to output.
5. **Note patterns**: Does this area follow project conventions? Are there deviations?
6. **Assess complexity**: File length, nesting depth, number of responsibilities.

Fill:

- **OBSERVATIONS**: Key findings with exact references (`File:Line`). Minimum 2 observations per file read.

#### 4c. Hypothesis Update

After reading, formally declare:

- **HYPOTHESIS STATUS**: `CONFIRMED` | `REFUTED` | `REFINED`
- **Reason**: 1-2 sentences explaining why.

If REFINED, state the updated hypothesis. Pay special attention to REFUTED results on HIGH-confidence hypotheses — these signal a misunderstanding that warrants deeper investigation.

#### 4d. Next Action Reasoning

- **NEXT ACTION JUSTIFICATION**: Explain why the next file you plan to open is the logical next step given your updated understanding. If no more files need reading, state "Exploration sufficient — proceeding to synthesis."

### Step 5: Dependency Mapping

Build a dependency map for the explored area:

```
Entry Point (file path)
  -> Depends on: [list of imports with file paths]
  -> Depended on by: [list of files that import this]
  -> External deps: [list of npm/external packages used]
```

For each dependency, note:
- Whether it is a direct or transitive dependency
- Whether changing the explored area would require changes to dependents
- Whether the dependency is stable or frequently modified (check git history if accessible)

### Step 6: Risk Assessment

Evaluate risks along these dimensions:

| Risk Dimension     | Assessment Criteria                                           |
|--------------------|---------------------------------------------------------------|
| Blast radius       | How many files/modules are affected by changes here?          |
| Type safety        | Are types well-defined or is there `any`/`unknown` leakage?  |
| Test coverage      | Do test files exist? Do they cover edge cases?                |
| Coupling           | How tightly coupled is this area to other modules?            |
| Complexity         | Cyclomatic complexity, nesting depth, file length             |
| Data integrity     | Are there database operations that could corrupt data?        |
| Breaking changes   | Would changes break public APIs or external consumers?        |
| Security surface   | Does this area handle user input, auth, or sensitive data?    |

Assign each dimension: **low**, **medium**, or **high** risk.

### Step 7: Approach Comparison (if applicable)

If the topic implies a change, compare possible approaches:

```markdown
| Approach       | Pros                     | Cons                    | Effort | Risk  |
|----------------|--------------------------|-------------------------|--------|-------|
| Approach A     | - Pro 1                  | - Con 1                 | Low    | Low   |
|                | - Pro 2                  | - Con 2                 |        |       |
| Approach B     | - Pro 1                  | - Con 1                 | Medium | Medium|
```

Each approach must include:
- Specific file paths that would be modified
- Estimated number of files changed
- Whether it requires database migration
- Whether it requires new dependencies

### Step 7b: Ambiguity Detection & Question Classification

After completing the technical analysis, actively scan for ambiguities that could derail downstream phases. This step transforms passive "open questions" into an actionable clarification protocol.

#### 7b-1. Scan for Ambiguity Sources

Review the topic/intent and analysis results for these ambiguity patterns:

| Ambiguity Type | Detection Signal | Example |
|---|---|---|
| **Multiple valid architectures** | Approach comparison has 2+ approaches with similar effort/risk scores | "Should we use WebSocket or SSE for real-time updates?" |
| **Undefined scope boundary** | Topic mentions a broad area without clear limits | "Improve the auth system" — improve what specifically? |
| **Conflicting constraints** | Exploration found patterns that contradict each other or the intent | "Design says REST, but existing code uses GraphQL for similar features" |
| **Missing domain knowledge** | Analysis depends on business rules not visible in code | "What's the maximum number of concurrent sessions allowed?" |
| **Undefined success criteria** | Intent describes a goal but no way to verify it's met | "Make the dashboard faster" — how fast is fast enough? |
| **Platform/integration unknowns** | Change involves external systems with unknown constraints | "Which OAuth2 providers? What scopes are needed?" |

#### 7b-2. Classify Each Question

Every open question MUST be classified:

| Severity | Criteria | Pipeline Effect |
|---|---|---|
| `BLOCKING` | The answer changes the architecture, scope, or fundamental approach. Proceeding without it risks building the wrong thing. | MUST be resolved before proceeding to `sdd-propose`. |
| `DEFERRED` | The answer affects implementation details but not the overall direction. Can be resolved during spec or design phase. | Listed in exploration.md for later resolution. |

**Classification rules:**
1. If the question affects which *approach* to take → `BLOCKING`
2. If the question affects *how many files* or *which modules* are in scope → `BLOCKING`
3. If the question requires *business domain expertise* the code doesn't reveal → `BLOCKING`
4. If the question is about *implementation style* within an already-chosen approach → `DEFERRED`
5. When in doubt → `BLOCKING` (cheaper to ask than to rebuild)

#### 7b-3. Generate Clarification Questions

For each `BLOCKING` ambiguity, generate a structured question:

```markdown
**Question**: {Clear, specific question — not "what do you want?" but "should sessions auto-extend on activity or use a hard 30-minute timeout?"}
**Why it matters**: {1 sentence on what changes based on the answer}
**Options** (if applicable):
  - A: {option} → leads to {consequence}
  - B: {option} → leads to {consequence}
**Default recommendation**: {what you'd choose if forced to guess, and why}
```

Questions MUST be:
- **Specific** — answerable in 1-2 sentences, not open-ended essays
- **Consequential** — the answer visibly changes the proposal/design
- **Informed** — include your recommendation so the user can just approve or redirect

### Step 8: Produce Output Artifacts

#### If `change_name` is provided:

Write `openspec/changes/{change_name}/exploration.md` with the full analysis.

The exploration document structure:

```markdown
# Exploration: {topic}

**Date**: {ISO 8601 timestamp}
**Detail Level**: {concise | standard | deep}
**Change Name**: {change_name or "N/A"}

## Current State

{Description of how the system currently works in the explored area}

## Relevant Files

| File Path | Purpose | Lines | Complexity | Test Coverage |
|-----------|---------|-------|------------|---------------|
| {path}    | {what}  | {n}   | {low/med/high} | {yes/no}  |

## Dependency Map

{ASCII or markdown representation of the dependency graph}

## Data Flow

{Step-by-step description of how data moves through the explored area}

## Risk Assessment

| Dimension       | Level  | Notes                           |
|-----------------|--------|---------------------------------|
| Blast radius    | {lvl}  | {explanation}                   |
| Type safety     | {lvl}  | {explanation}                   |
| Test coverage   | {lvl}  | {explanation}                   |
| Coupling        | {lvl}  | {explanation}                   |
| Complexity      | {lvl}  | {explanation}                   |
| Data integrity  | {lvl}  | {explanation}                   |
| Breaking changes| {lvl}  | {explanation}                   |
| Security surface| {lvl}  | {explanation}                   |

## Approach Comparison

{Table if applicable, otherwise "Single clear approach identified."}

## Recommendation

{Concise recommendation with justification}

## Clarification Required (BLOCKING)

{Questions that MUST be answered before proceeding to proposal. Omit section if none.}

### Q1: {Specific question}
- **Why it matters**: {consequence}
- **Options**: A: {option} / B: {option}
- **Recommendation**: {default choice and rationale}

## Open Questions (DEFERRED)

- {Question that can be resolved during spec/design}
- {Question that can be resolved during spec/design}
```

#### If `change_name` is NOT provided:

Present the analysis as a markdown summary in the conversation only (no file written).

### Step 9: Present Summary

Present a markdown summary to the user, then STOP. Do not proceed automatically.

**On success, output:**

```markdown
## SDD Explore: {topic}

**Detail level**: {concise | standard | deep}
**Overall risk**: {low | medium | high}

### Current State
{current_state_summary — 2-3 sentences}

### Relevant Files ({N} files)
| File | Purpose | Impact |
|------|---------|--------|
| {path} | {purpose} | {low/medium/high} |

### Risk Assessment
- **Highest risks**: {top 2 risk dimensions with levels}
- **Blast radius**: {N} files potentially affected

{If multiple approaches: ### Approaches\n{approach comparison table}\n}

### Recommendation
{recommendation}

{If blocking questions:
### ⛔ Clarification Required (BLOCKING)
{structured questions — user must answer before proposing}
}

{If exploration.md written: **Artifact**: `openspec/changes/{change_name}/exploration.md`\n}

**Next step**: {If blocking questions: "Answer the questions above, then re-run `/sdd-explore`." | else: "Run `/sdd-new {change_name} \"<intent>\"`  to create a proposal."}
```

## Detail Level Behavior

### Concise
- Bullet-point analysis, no prose
- File table with path and purpose only
- Risk assessment as a single overall rating with top 2 risks
- No dependency map diagram
- No data flow description
- Target output: 30-50 lines

### Standard (default)
- Paragraph descriptions for current state and recommendation
- Full file table with all columns
- Full risk assessment table
- Simplified dependency map (direct dependencies only)
- Brief data flow description
- Approach comparison table if multiple approaches exist
- Target output: 80-150 lines

### Deep
- Comprehensive prose analysis with code excerpts
- Full file table with all columns
- Full risk assessment with detailed notes
- Complete dependency map (direct and transitive)
- Detailed data flow with code snippets showing key transformations
- Approach comparison with implementation details
- Open questions with suggested investigation paths
- Target output: 150-300 lines

## Rules and Constraints

1. **NEVER modify source code.** This is a read-only analysis phase.
2. **NEVER write files outside `openspec/`.** Exploration artifacts go in `openspec/changes/{change_name}/`.
3. **Return concrete file paths**, not vague descriptions like "the auth module". Always use absolute paths.
4. **Every claim must be evidence-based.** If you say "this area has low test coverage", cite the specific files or absence of test files.
5. **Do not guess at runtime behavior.** If you cannot determine something from static analysis, list it as an open question.
6. **Respect `detail_level`** -- do not produce deep analysis when concise is requested.
7. **If `focus_paths` is provided**, prioritize those paths but still report on related areas that would be affected.
8. **Search broadly first, then narrow.** Start with Glob patterns, then Grep for specifics, then Read for deep understanding.
9. **Include test file analysis.** Test files often reveal expected behavior and edge cases better than source code.
10. **Time-box yourself.** If the codebase is very large, focus on the most relevant 15-20 files rather than reading everything.

## Error Handling

- If `openspec/config.yaml` does not exist: warn in the summary output and proceed with manual detection.
- If the topic is too vague to search for: return `status: error` with a message asking for clarification.
- If no relevant files are found: return `status: success` with empty `relevant_files` and a note in `open_questions`.
- If a file cannot be read: skip it and note in `warnings`.
- All errors include the phase name (`explore`) and a human-readable message.

## PARCER Contract

```yaml
phase: explore
preconditions:
  - config.yaml exists at openspec/config.yaml
  - topic parameter is non-empty string
postconditions:
  - exploration.md written to openspec/changes/{changeName}/ (if changeName provided)
  - exploration.md contains ≥1 relevant file (or blocking_questions explaining absence)
  - exploration.md written with all required sections
```
