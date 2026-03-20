---
name: sdd-propose
description: >
  Create a change proposal from exploration analysis. Produces proposal.md with intent, scope, approach, risks, and rollback plan.
  Trigger: When user runs /sdd:new or after sdd-explore completes successfully.
license: MIT
metadata:
  version: "1.0"
---

# SDD Propose

You are executing the **propose** phase inline. A proposal is the human-readable contract that must be approved before any specifications, design, or implementation work begins. It captures the WHAT, WHY, and high-level HOW of a change.

## Activation

User runs `/sdd:new <change-name> "<intent>"`. The change name must be kebab-case. Reads `openspec/changes/{changeName}/exploration.md` if it exists.

## Execution Steps

### Step 1: Load Project Context

1. Read `openspec/config.yaml` for:
   - Project stack and architecture
   - Coding conventions and constraints
   - Required proposal sections (from `phases.proposal.required_sections`)
2. If `config.yaml` does not exist, return `status: error` recommending `sdd-init` first.

### Step 2: Load Exploration Results

1. If `exploration_path` is provided, read the `exploration.md` file.
2. If `exploration_data` is provided, use the inline data.
3. If neither is provided:
   - Check if `openspec/changes/{change_name}/exploration.md` exists (may have been created by a prior explore phase).
   - If not found, proceed without exploration data but add a warning that the proposal may lack depth.
4. Extract from exploration:
   - Current state summary
   - Relevant files and their purposes
   - Risk assessment
   - Recommended approach
   - Open questions

### Step 2b: Requirement Clarification Gate

This step ensures that critical ambiguities are resolved before writing the proposal. It prevents building proposals on assumptions that could invalidate the entire approach.

#### 2b-1. Check for BLOCKING Questions

1. If exploration data is available, read the `blocking_questions` field (or the "Clarification Required (BLOCKING)" section of exploration.md).
2. If no exploration data is available, perform lightweight ambiguity detection on the `intent`:

| Check | Trigger | Example |
|---|---|---|
| Vague scope | Intent uses words like "improve", "refactor", "fix" without specifics | "Improve the dashboard" — improve what? performance? UX? data accuracy? |
| Multiple interpretations | Intent could lead to fundamentally different approaches | "Add notifications" — email? push? in-app? all three? |
| Missing constraints | Intent doesn't specify boundaries that affect architecture | "Support multiple users" — how many? 10 or 10 million? |
| Undefined integration | Intent references external systems without details | "Connect to payment provider" — which one? what API version? |

3. Generate any new BLOCKING questions found that weren't in the exploration.

#### 2b-2. Evaluate Clarification Status

Three possible states:

**State A: No BLOCKING questions** → Proceed to Step 3.

**State B: BLOCKING questions exist AND `clarification_answers` provided** → Validate that every BLOCKING question has a corresponding answer. If any answer is missing or unclear, add it to the questions list and return `needs_clarification`. Otherwise, incorporate answers into the proposal context and proceed to Step 3.

**State C: BLOCKING questions exist AND NO `clarification_answers`** → Return early with:

```yaml
phase: propose
status: needs_clarification
data:
  change_name: <string>
  blocking_questions:
    - question: <specific question>
      why_it_matters: <consequence>
      options:
        - label: "A"
          description: <option>
          consequence: <result>
        - label: "B"
          description: <option>
          consequence: <result>
      recommendation: <default if forced to choose>
  deferred_questions:
    - <string>
  message: "Cannot write proposal — {N} blocking question(s) require your input. Please answer and re-invoke."
```

Present these questions to the user. When they provide answers, re-run `/sdd:new` with the answers incorporated.

#### 2b-3. Incorporate Answers

When `clarification_answers` are provided:
1. Use the answers to resolve approach decisions, scope boundaries, and constraints.
2. Reference the answers explicitly in the proposal (e.g., in Key Decisions: "Per user clarification: using WebSocket over SSE for real-time updates").
3. If an answer creates new questions (rare), classify them. New BLOCKING questions trigger another `needs_clarification` return (max 2 clarification rounds to prevent fatigue).

### Step 3: Validate Change Name

1. Ensure `change_name` is kebab-case (lowercase, hyphens, no spaces).
2. Check if `openspec/changes/{change_name}/` already exists:
   - If `proposal.md` exists, return `status: error` with message "Proposal already exists. Use sdd-continue to proceed or choose a different change name."
   - If directory exists but no `proposal.md`, it is safe to proceed (exploration may have been written).

### Step 4: Assess Change Size

Determine the change size based on exploration data and intent:

| Size   | Criteria                                          | Recommendation           |
|--------|---------------------------------------------------|--------------------------|
| Small  | 1-3 files modified, no new dependencies, no DB    | Single proposal is fine  |
| Medium | 4-10 files, may add dependencies, minor DB change | Single proposal is fine  |
| Large  | 10+ files, new dependencies, DB migration, new API| Recommend splitting      |

If the change is **large** and `size_hint` was not explicitly set to `large`:
- Include a warning in the proposal recommending it be split into smaller changes
- Suggest concrete split points (e.g., "Phase 1: Add database schema, Phase 2: Add API endpoints, Phase 3: Add UI")

### Step 5: Create Change Directory

Create `openspec/changes/{change_name}/` if it does not already exist.

### Step 6: Write proposal.md

Write `openspec/changes/{change_name}/proposal.md` with the following required sections:

```markdown
# Proposal: {Change Name (title case)}

**Change ID**: {change_name}
**Date**: {ISO 8601 timestamp}
**Status**: draft

---

## Intent

{What is being changed and why, in 1-3 sentences. Focus on the problem being solved and the value delivered. Do not describe implementation details here.}

## Scope

### In Scope

- {Specific deliverable 1}
- {Specific deliverable 2}
- {Specific deliverable 3}

### Out of Scope

- {Explicitly excluded item 1 -- and brief reason why}
- {Explicitly excluded item 2 -- and brief reason why}

## Approach

{High-level strategy for implementing this change. 1-2 paragraphs describing the overall approach without getting into file-level details. Reference the recommended approach from exploration if available.}

### Key Decisions

| Decision                | Choice            | Rationale                          |
|-------------------------|-------------------|------------------------------------|
| {Decision point 1}     | {Chosen approach} | {Why this was chosen}              |
| {Decision point 2}     | {Chosen approach} | {Why this was chosen}              |

## Affected Areas

| Module / Area           | File Path                        | Change Type     | Risk Level |
|-------------------------|----------------------------------|-----------------|------------|
| {Module name}           | {absolute file path}             | create/modify/delete | low/medium/high |
| {Module name}           | {absolute file path}             | create/modify/delete | low/medium/high |

**Total files affected**: {count}
**New files**: {count}
**Modified files**: {count}
**Deleted files**: {count}

## Risks

| Risk                    | Probability | Impact  | Mitigation                         |
|-------------------------|-------------|---------|------------------------------------|
| {Risk description}      | low/medium/high | low/medium/high | {How to prevent or handle this} |
| {Risk description}      | low/medium/high | low/medium/high | {How to prevent or handle this} |

**Overall Risk Level**: {low | medium | high}

## Rollback Plan

{MANDATORY section. Describe exactly how to undo this change if something goes wrong.}

### Steps to Rollback

1. {Specific step 1 with git command or file operation}
2. {Specific step 2}
3. {Specific step 3}

### Rollback Verification

- {How to verify the rollback was successful}
- {What state the system should be in after rollback}

## Dependencies

### Internal Dependencies

- {Module or file that must exist/be modified first}

### External Dependencies

| Package              | Version  | Purpose                    | Already Installed |
|----------------------|----------|----------------------------|-------------------|
| {package-name}       | {semver} | {why it is needed}         | yes/no            |

### Infrastructure Dependencies

- {Database migration needed: yes/no}
- {New environment variables: list or none}
- {New services: list or none}

## Success Criteria

All of the following must be true for this change to be considered complete:

- [ ] {Measurable criterion 1, e.g., "All new endpoints return typed responses with proper error handling"}
- [ ] {Measurable criterion 2, e.g., "Unit tests cover all new functions with >80% branch coverage"}
- [ ] {Measurable criterion 3, e.g., "Project's build/typecheck command passes with zero errors"}
- [ ] {Measurable criterion 4, e.g., "Project's lint command passes with zero warnings"}
- [ ] {Measurable criterion 5, e.g., "No type-system escape hatches or compiler suppressions in new code"}
- [ ] {Measurable criterion 6, e.g., "Rollback plan tested and verified"}

## Open Questions

{List any unresolved questions from exploration or new questions raised during proposal writing. These MUST be answered before moving to the spec phase.}

- {Question 1}
- {Question 2}

---

**Next Step**: Review and approve this proposal, then run `sdd-spec` and `sdd-design` (can run in parallel).
```

### Step 7: Validate Proposal Completeness

Before returning, validate that the proposal has:

1. **Intent** -- not empty, not just restating the change name
2. **Scope** -- both in-scope AND out-of-scope items listed
3. **Approach** -- at least one key decision with rationale
4. **Affected Areas** -- at least one file path (absolute, not relative)
5. **Risks** -- at least one risk identified (every change has risks)
6. **Rollback Plan** -- contains specific steps (not "revert the commit")
7. **Dependencies** -- section present even if empty ("None" is acceptable)
8. **Success Criteria** -- at least 3 measurable criteria including type safety and test passing

If any section is missing or inadequate, add it with a `[TODO]` marker and include a warning in the summary output.

### Step 8: Present Summary

Present a markdown summary to the user, then STOP. Do not proceed automatically.

If blocking clarification questions exist (status: PARTIAL), output the questions and stop — do not write `proposal.md` until answers are provided.

**On success, output:**

```markdown
## SDD Propose: {change_name}

**Size**: {small | medium | large}  |  **Risk**: {low | medium | high}  |  **Files affected**: {N}

### Proposal Written
`openspec/changes/{change_name}/proposal.md`

### Scope
- **In scope**: {N} items
- **Out of scope**: {N} items explicitly excluded

### Key Decisions
{decision table summary — 1-2 rows}

{If open questions: ### Open Questions ({N})\n{questions list}\n}
{If warnings: ### ⚠ Warnings\n{warnings list}\n}
{If large change: ### ⚠ Size Warning\nThis change is large. Consider splitting into: {split suggestions}\n}

**Next step**: Review `openspec/changes/{change_name}/proposal.md`. When satisfied, run `/sdd:spec` and `/sdd:design` (these can run in parallel, each in its own session).
```

**On PARTIAL (blocking questions):**

```markdown
## SDD Propose: Clarification Needed

Cannot write proposal — {N} blocking question(s) require your input.

### Q1: {specific question}
- **Why it matters**: {consequence}
- **Options**: A: {option} → {consequence} | B: {option} → {consequence}
- **Recommendation**: {default}

Re-run `/sdd:new {change_name} "<intent>"` after answering.
```

## Rules and Constraints

1. **The proposal is a HUMAN-READABLE document** for review and approval. Write clearly, not in code-speak.
2. **Rollback Plan is NEVER optional.** Every change must be reversible. If it truly cannot be rolled back (e.g., destructive database migration), state that explicitly with mitigation.
3. **Scope must explicitly state what is OUT of scope.** This prevents scope creep and miscommunication.
4. **Success criteria must be verifiable.** Not "code works well" but "all tests pass, no type errors, API returns 200 for valid input."
5. **If the change is too large** (10+ files, multiple domains), recommend splitting with concrete split suggestions.
6. **All file paths must be absolute.** Never use relative paths in the affected areas table.
7. **Never modify source code.** The proposal phase only writes to `openspec/changes/{change_name}/`.
8. **Never skip risk assessment.** Even "simple" changes have risks (regression, performance, compatibility).
9. **Reference exploration data** when available. The proposal should build on exploration findings, not ignore them.
10. **Open questions must be answerable.** Not "will this work?" but "should we use WebSocket or SSE for real-time updates?"
11. **Respect project conventions** from `config.yaml` and `CLAUDE.md`. Success criteria must include type safety and error handling checks consistent with the project's rules.
12. **Do not prescribe implementation details.** The proposal says WHAT and WHY, not exactly HOW at the code level. That is the design phase's job.

## Error Handling

- If `openspec/config.yaml` does not exist: return `status: error` with message "Run sdd-init first."
- If `change_name` is not kebab-case: return `status: error` with corrected suggestion.
- If `proposal.md` already exists: return `status: error` with guidance to use sdd-continue.
- If intent is empty or not provided: return `status: error` requesting intent description.
- All errors include the phase name (`propose`) and a human-readable message.

## PARCER Contract

```yaml
phase: propose
preconditions:
  - exploration.md exists at openspec/changes/{changeName}/
postconditions:
  - proposal.md written with all required_sections from config.yaml phases.proposal
  - proposal.md contains change size and risk level assessment
  - proposal.md written with all required sections, or clarification questions presented
```
