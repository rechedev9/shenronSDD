---
name: sdd-design
description: >
  Create technical design document capturing HOW the change is implemented. Architecture decisions, data flow, interfaces.
  Trigger: When user runs /sdd-continue after proposal is approved, or after sdd-propose completes.
license: MIT
metadata:
  version: "1.0"
---

# SDD Design

You are executing the **design** phase inline. While the proposal captures WHAT and WHY, and specs capture the requirements, the design captures HOW the change will be implemented at the architecture and code-structure level. Your design must follow existing project patterns and conventions.

## Activation

User runs `/sdd-continue` or `/sdd-design` after the proposal is approved. Reads `openspec/changes/{changeName}/proposal.md`, existing spec files if present, and `exploration.md` if available.

## Execution Steps

### Step 1: Load Project Context

1. Read `openspec/config.yaml` for:
   - Technology stack (runtime, frameworks, ORM, database)
   - Architecture pattern (monorepo, workspaces)
   - Coding conventions (type strictness, error handling, file organization)
   - Design phase required sections (`phases.design.required_sections`)
2. Read `CLAUDE.md` for any additional rules not captured in config.
3. **Load framework skills.** Based on the tech stack identified in config.yaml, read the relevant skill file(s) from `~/.claude/skills/frameworks/` before designing components or interfaces for that domain. For example: React → `react-19/SKILL.md`, Next.js → `nextjs-15/SKILL.md`, Tailwind → `tailwind-4/SKILL.md`, Zod → `zod-4/SKILL.md`. If a skill file does not exist, proceed without it. This step is required so that interface definitions and data flow diagrams follow idiomatic patterns for the project's actual framework versions.
4. Note all constraints from `CLAUDE.md` that affect design decisions (type strictness, error handling patterns, file organization limits, etc.).

### Step 2: Read Proposal and Optional Specs

1. Read `proposal.md` at the provided path.
2. Extract:
   - Approach and key decisions
   - Affected areas with file paths
   - Dependencies (internal and external)
   - Success criteria
3. If `spec_paths` are provided, read spec files to align design with requirements:
   - Map each requirement to a design component
   - Ensure every MUST requirement has a clear implementation path
4. If `exploration_path` is provided, read for additional codebase context.

### Step 3: Analyze Existing Codebase Patterns

This is a critical step. The design MUST follow existing patterns. Read source code to understand:

1. **Project structure**: How are files organized? What naming conventions are used?
   - Use Glob to map the directory tree
   - Identify patterns: feature-based, layer-based, domain-driven
2. **Type patterns**: How are types/interfaces defined and shared?
   - Search for type definition files and shared type locations
   - Note generic patterns used (Result types, Option types, error enums, etc.)
3. **Error handling patterns**: How are errors created and propagated?
   - Find the project's error handling pattern and how it is used
   - Identify error type conventions (custom error types, error enums, etc.)
   - Note error boundary patterns if applicable (UI frameworks, middleware, etc.)
4. **API patterns**: How are endpoints defined?
   - Read existing route/controller files
   - Note middleware usage, validation patterns, response shapes
5. **Database patterns**: How are queries structured?
   - Read existing repository/service files
   - Note ORM usage, transaction patterns, migration conventions
6. **Testing patterns**: How are tests structured?
   - Read existing test files
   - Note test grouping patterns, assertion styles, and test runner conventions

### Step 4: Make Architecture Decisions

For each significant design choice, evaluate alternatives:

| Decision Criteria    | What to Evaluate                              |
|----------------------|-----------------------------------------------|
| Consistency          | Does this match existing project patterns?    |
| Type safety          | Can this be fully typed without escape hatches?|
| Testability          | Can each component be tested in isolation?    |
| Simplicity           | Is this the simplest solution that works?     |
| Performance          | Are there known performance implications?     |
| Maintainability      | Will future developers understand this?       |

Every decision must have at least 2 alternatives with rationale for the chosen option. If existing project code already establishes a pattern, "consistency with existing code" is a valid (and strong) rationale.

### Step 5: Design Data Flow

Create clear data flow descriptions showing how data moves through the system for each key operation:

```
User Action
  -> Frontend Component (file path)
    -> API Call (endpoint, method, payload type)
      -> Backend Handler (file path)
        -> Validation (schema, types)
          -> Business Logic (service file path)
            -> Database Query (repository file path)
              -> Response (type, shape)
            <- Error Path (error type per project conventions)
          <- Validation Error (shape)
        <- HTTP Response (status, body type)
      <- Frontend State Update (store/hook file path)
    <- UI Update (component re-render)
```

Use ASCII diagrams for complex flows. Keep it readable without specialized tooling.

### Step 6: Define Interfaces and Contracts

Write type/interface definitions in the project's language. Use the actual syntax that will compile/type-check — not pseudocode. Follow the project's error handling pattern (per CLAUDE.md) for fallible operations.

For API endpoints, define request/response contracts:

```
POST /api/{resource}
  Request Body: Create{Thing}Input
  Response 201: { data: {Thing} }
  Response 400: { error: { code: string, message: string } }
  Response 401: { error: { code: 'UNAUTHORIZED' } }
```

For database schemas, define the migration:

```sql
-- Migration: add_{table_name}
CREATE TABLE {table_name} (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ...
);
```

### Step 7: Plan Testing Strategy

Map each requirement (from specs if available, or from proposal success criteria) to a testing approach:

| What to Test                  | Type        | Approach                          | File Path                    |
|-------------------------------|-------------|-----------------------------------|------------------------------|
| {Feature} input validation    | Unit        | Test each validation rule         | {path per project convention} |
| {Feature} business logic      | Unit        | Test success/error paths          | {path per project convention} |
| {Feature} API endpoint        | Integration | Test HTTP request/response cycle  | {path per project convention} |
| {Feature} error handling      | Unit        | Test all error variants           | {path per project convention} |
| {Feature} database operations | Integration | Test with real DB (test container) | {path per project convention} |

For each test, note:
- What dependencies need to be mocked vs. real
- What fixtures/factories are needed
- Whether it maps to a specific spec scenario (reference REQ-ID)

### Step 8: Write design.md

Create `openspec/changes/{change_name}/design.md`:

```markdown
# Technical Design: {Change Name (title case)}

**Change**: {change_name}
**Date**: {ISO 8601 timestamp}
**Status**: draft
**Depends On**: proposal.md

---

## Technical Approach

{2-3 paragraphs describing the overall technical strategy. How does this fit into the existing architecture? What patterns does it follow? What is the high-level implementation path?}

## Architecture Decisions

| # | Decision                | Choice              | Alternatives Considered      | Rationale                              |
|---|-------------------------|---------------------|------------------------------|----------------------------------------|
| 1 | {Decision point}        | {Chosen approach}   | {Alt 1}, {Alt 2}             | {Why chosen over alternatives}         |
| 2 | {Decision point}        | {Chosen approach}   | {Alt 1}, {Alt 2}             | {Why chosen over alternatives}         |
| 3 | {Decision point}        | {Chosen approach}   | {Alt 1}, {Alt 2}             | {Why chosen over alternatives}         |

## Data Flow

### {Operation 1 Name}

{ASCII diagram or step-by-step description showing data flow}

### {Operation 2 Name}

{ASCII diagram or step-by-step description showing data flow}

## File Changes

| # | File Path (absolute)           | Action  | Description                               |
|---|--------------------------------|---------|-------------------------------------------|
| 1 | {/abs/path/to/file.ts}        | create  | {What this new file contains}             |
| 2 | {/abs/path/to/existing.ts}    | modify  | {What changes and why}                    |
| 3 | {/abs/path/to/deprecated.ts}  | delete  | {Why this file is being removed}          |

**Summary**: {N} files created, {N} files modified, {N} files deleted

## Interfaces and Contracts

### Types

{TypeScript type definitions for new interfaces}

### API Contracts

{Request/response contracts for new or modified endpoints}

### Database Schema

{Migration SQL or ORM schema definitions, if applicable}

## Testing Strategy

| # | What to Test                  | Type         | File Path                          | Maps to Requirement |
|---|-------------------------------|--------------|------------------------------------|---------------------|
| 1 | {Test subject}                | unit         | {/abs/path/to/test file}       | REQ-{DOMAIN}-{NNN} |
| 2 | {Test subject}                | unit         | {/abs/path/to/test file}       | REQ-{DOMAIN}-{NNN} |
| 3 | {Test subject}                | integration  | {/abs/path/to/test file}       | REQ-{DOMAIN}-{NNN} |

### Test Dependencies

- **Mocks needed**: {list of dependencies to mock}
- **Fixtures needed**: {list of test data factories}
- **Infrastructure**: {test database, test containers, etc.}

## Migration and Rollout

{How to deploy this change safely. If no deployment changes needed, state "No migration or rollout steps required."}

### Migration Steps

1. {Step 1 - e.g., "Run database migration"}
2. {Step 2 - e.g., "Deploy backend with feature flag disabled"}
3. {Step 3 - e.g., "Enable feature flag for beta users"}

### Rollback Steps

{Reference the proposal's rollback plan and add technical details}

## Open Questions

{Any technical questions that arose during design. These should be resolved before implementation.}

- {Question 1}
- {Question 2}

---

**Next Step**: After both design and specs are complete, run `sdd-tasks` to create the implementation checklist.
```

### Step 9: Validate Design Completeness

Before returning, validate:

1. **Every file from the proposal's affected areas** appears in the File Changes table.
2. **Every success criterion** has a corresponding testing strategy entry.
3. **Architecture decisions** each have at least 2 alternatives.
4. **Interfaces use project conventions** (per CLAUDE.md type safety and error handling rules).
5. **File paths are absolute** throughout the document.
6. **New files follow project naming conventions** (kebab-case, colocated tests, etc.).
7. **The design does not exceed project file length limits** (plan to split large files).

### Step 10: Present Summary

Present a markdown summary to the user, then STOP. Do not proceed automatically.

**On success, output:**

```markdown
## SDD Design: {change_name}

**Decisions**: {N}  |  **File changes**: {create}✚ {modify}✎ {delete}✗  |  **Interfaces**: {N}

### Design Written
`openspec/changes/{change_name}/design.md`

### File Changes Planned
| Action | Count |
|--------|-------|
| Create | {N} |
| Modify | {N} |
| Delete | {N} |

### Interfaces Defined
{List interface names — 1 per line}

### Testing Strategy
- **Test cases planned**: {N}
- **Has database migration**: {yes | no}

{If open questions: ### Open Questions ({N})\n{questions — must be resolved before implementation}\n}
{If warnings: ### ⚠ Warnings\n- {warning}\n}

**Next step**: Review `openspec/changes/{change_name}/design.md`. When both design and spec are complete, run `/sdd-tasks` to generate the implementation checklist.
```

## Rules and Constraints

1. **Design MUST follow existing project patterns.** Read code first, then design. Never introduce patterns that conflict with the codebase.
2. **Include actual type/interface definitions** in the project's language, not pseudo-code. They must compile/type-check.
3. **Architecture decisions MUST have at least 2 alternatives** with rationale. Single-option "decisions" are not decisions.
4. **File Changes MUST list EVERY file** that will be created, modified, or deleted. No surprises during implementation.
5. **Testing strategy MUST cover each requirement** from specs (if available) or each success criterion from proposal.
6. **Design can run in PARALLEL with sdd-spec.** Both depend only on the proposal.
7. **Follow the project's error handling pattern** (per CLAUDE.md). Define error variants explicitly.
8. **All file paths must be absolute.** Never use relative paths.
9. **Never modify source code.** Design artifacts go in `openspec/changes/{change_name}/`.
10. **Respect file organization rules.** If the project limits files to 600 lines, plan to split accordingly.
11. **Interface definitions must be strict.** Use the project's immutability patterns, explicit return types, and no type-system escape hatches (per CLAUDE.md).
12. **Data flow diagrams must be ASCII-based.** Do not reference external diagramming tools. The design must be readable in any text editor.
13. **Load framework skills before designing.** Read `~/.claude/skills/frameworks/{framework}/SKILL.md` for every active framework in the project before defining interfaces, data flow, or architecture decisions for that domain. Designing without the framework skill risks specifying deprecated APIs or non-idiomatic patterns that sdd-apply will then implement incorrectly.

## Error Handling

- If `openspec/config.yaml` does not exist: return `status: error` recommending `sdd-init`.
- If `proposal.md` does not exist: return `status: error` with message.
- If existing codebase patterns are inconsistent: note in warnings and follow the most common pattern.
- If the design reveals the proposal scope is insufficient: note in `open_questions` and warn.
- All errors include the phase name (`design`) and a human-readable message.

## PARCER Contract

```yaml
phase: design
preconditions:
  - proposal.md exists and was approved
postconditions:
  - design.md written with all required sections
  - ≥1 interface definition with typed fields
```
