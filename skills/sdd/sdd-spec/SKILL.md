---
name: sdd-spec
description: >
  Write delta specifications (ADDED/MODIFIED/REMOVED requirements) with Given/When/Then scenarios using RFC 2119 keywords.
  Trigger: When user runs /sdd:continue after proposal is approved, or after sdd-propose completes.
license: MIT
metadata:
  version: "1.0"
---

# SDD Spec

You are executing the **spec** phase inline. Specifications define WHAT the system must do after the change is applied, expressed as requirements with testable scenarios. You use RFC 2119 keywords for precision and Given/When/Then scenarios for verifiability.

## Activation

User runs `/sdd:continue` or `/sdd:spec` after the proposal is approved. Reads `openspec/changes/{changeName}/proposal.md` and `openspec/specs/` for existing context.

## Execution Steps

### Step 1: Load Project Context

1. Read `openspec/config.yaml` for:
   - Spec phase rules (`phases.specs`)
   - RFC 2119 keyword usage
   - Scenario format (Given/When/Then)
   - Minimum scenarios per requirement
2. Read project conventions relevant to specs (type strictness, error handling, testing patterns).

### Step 2: Read Proposal

1. Read `proposal.md` at the provided path.
2. Extract:
   - Intent (what the change accomplishes)
   - In-scope items (each becomes a spec domain or requirement group)
   - Affected areas (files and modules that will change)
   - Success criteria (each maps to one or more requirements)
   - Out-of-scope items (ensure specs do not accidentally cover these)
3. If the proposal has unresolved open questions, warn in the output but proceed with available information.

### Step 3: Read Existing Specs

1. If `openspec/specs/` contains existing spec files, read them for context.
2. Existing specs represent the CURRENT system behavior.
3. Delta specs describe CHANGES to this baseline:
   - **ADDED**: New behaviors not in the current system
   - **MODIFIED**: Changed behaviors (must reference what they replace)
   - **REMOVED**: Deleted behaviors (must explain why)
4. If no existing specs exist, all requirements are ADDED.

### Step 4: Identify Spec Domains

Group requirements by domain. A domain is a logical area of the system:

| Domain Type       | Example Domains                              |
|-------------------|----------------------------------------------|
| API               | auth-api, users-api, payments-api            |
| Data              | user-schema, session-schema, migration       |
| Business Logic    | auth-flow, permission-rules, pricing-engine  |
| UI                | login-form, dashboard-layout, settings-page  |
| Infrastructure    | database-connection, cache-layer, queue      |
| Integration       | oauth-provider, email-service, webhook       |

Each in-scope item from the proposal maps to one or more domains.

### Step 5: Write Delta Spec Files

For each domain, create a spec file:

**Path**: `openspec/changes/{change_name}/specs/{domain}/spec.md`

Each spec file follows this structure:

```markdown
# Delta Spec: {Domain Name (title case)}

**Change**: {change_name}
**Date**: {ISO 8601 timestamp}
**Status**: draft
**Depends On**: proposal.md

---

## Context

{Brief description of this domain's role in the change. Reference the proposal intent and relevant existing specs if any.}

## ADDED Requirements

### REQ-{DOMAIN}-{NNN}: {Requirement Title}

{Requirement description using RFC 2119 keywords.}

The system **MUST** {required behavior}.
The system **SHALL** {required behavior}.
The system **SHOULD** {recommended behavior}.
The system **MAY** {optional behavior}.

#### Scenario: {Scenario Title} · `code-based` · `critical`

- **WHEN** {action - specific trigger or input}
- **THEN** {outcome - specific, observable, verifiable result}

#### Scenario: {Edge Case Title} · `code-based` · `critical`

- **GIVEN** {non-obvious precondition that changes the outcome}
- **WHEN** {action that triggers edge case}
- **THEN** {expected handling of edge case}

---

## MODIFIED Requirements

### REQ-{DOMAIN}-{NNN}: {Requirement Title}

**Previously**: {Reference to existing spec or description of current behavior}

{New requirement description using RFC 2119 keywords.}

The system **MUST** now {changed behavior} instead of {old behavior}.

#### Scenario: {Scenario showing new behavior} · `code-based` · `critical`

- **WHEN** {action}
- **THEN** {new outcome, different from previous behavior}

---

## REMOVED Requirements

### REQ-{DOMAIN}-{NNN}: {Requirement Title}

**Reason**: {Why this requirement is being removed}

**Previously**: {What the system used to do}

**Migration**: {How existing users/data are affected, if applicable}

---

## Acceptance Criteria Summary

| Requirement ID       | Type     | Priority   | Scenarios |
|----------------------|----------|------------|-----------|
| REQ-{DOMAIN}-001    | ADDED    | MUST       | 2         |
| REQ-{DOMAIN}-002    | ADDED    | SHOULD     | 1         |
| REQ-{DOMAIN}-003    | MODIFIED | MUST       | 2         |

**Total Requirements**: {count}
**Total Scenarios**: {count}
```

### Step 6: RFC 2119 Keyword Usage

Apply keywords precisely as defined in RFC 2119:

| Keyword          | Meaning                                          | Usage                                |
|------------------|--------------------------------------------------|--------------------------------------|
| **MUST**         | Absolute requirement                             | Core functionality, security rules   |
| **MUST NOT**     | Absolute prohibition                             | Security violations, data corruption |
| **SHALL**        | Same as MUST (used for variety)                  | Contractual obligations              |
| **SHALL NOT**    | Same as MUST NOT                                 | Contractual prohibitions             |
| **SHOULD**       | Recommended, but valid reasons to deviate exist  | Best practices, performance goals    |
| **SHOULD NOT**   | Discouraged, but valid reasons to include exist  | Anti-patterns with exceptions        |
| **MAY**          | Truly optional                                   | Nice-to-have features, extensions    |

Rules for keyword usage:
- Every MUST/SHALL requirement needs at least one scenario proving compliance.
- Every MUST NOT/SHALL NOT needs at least one scenario proving violation is handled.
- SHOULD requirements need scenarios but failures are warnings, not blockers.
- MAY requirements need scenarios to define behavior IF the option is implemented.

### Step 7: Scenario Quality Standards

Each scenario heading MUST include an inline eval type and criticality tag:

```
#### Scenario: {Title} · `{eval-type}` · `{criticality}`
```

- `eval-type`: `code-based` | `model-based` | `human-based` (same classification as Step 8b)
- `criticality`: `critical` | `standard`

This pre-annotates the Eval Definitions table at authoring time — Step 8b reads these tags directly instead of re-classifying.

**GIVEN is optional.** Only include it when the precondition is non-obvious or changes the outcome. Omitting it for straightforward cases keeps scenarios focused on the trigger and result.

- Include GIVEN when: a specific system state, user role, prior action, or data condition materially affects the scenario
- Omit GIVEN when: the precondition is implied by the requirement context (e.g., "GIVEN a user exists" for a user API requirement)

Each scenario must also be:

1. **Specific**: Use concrete values, not placeholders.
   - Bad: "WHEN a user submits the form"
   - Good: "WHEN a POST request is made to `/api/users` with body `{ email: 'test@example.com', role: 'admin' }`"

2. **Independent**: Each scenario tests one behavior path.
   - Bad: "THEN the user is created AND an email is sent AND the audit log is updated"
   - Good: Three separate scenarios for creation, email, and audit

3. **Verifiable**: The THEN clause must be observable and map to a single assertion.
   - Bad: "THEN the system handles the error gracefully"
   - Good: "THEN the API returns HTTP 400 with body `{ error: 'INVALID_EMAIL', message: 'Email format is invalid' }`"

4. **Aligned with project conventions**:
   - Reference the project's error handling patterns in THEN clauses where applicable (per CLAUDE.md)
   - For API specs: include HTTP status codes and response body shapes
   - For UI specs: reference user-visible states and interactions

### Step 8: Cross-Domain Consistency

After writing all domain specs:

1. Check for **requirement ID collisions** across domains.
2. Check for **contradictions** (Domain A says MUST, Domain B says MUST NOT for same behavior).
3. Check for **missing coverage** (proposal in-scope items without corresponding requirements).
4. Check for **scope creep** (requirements that cover out-of-scope items from proposal).

Document any issues found in the summary output warnings.

### Step 8b: Generate Eval Definitions

For each scenario across all domain specs, emit an eval definition. This enables `sdd-verify` to apply pass@k scoring instead of binary pass/fail.

**Eval type and criticality are already declared** in each scenario heading as inline tags (set in Step 7). Read them directly — do NOT re-classify from the THEN clause. If a scenario heading is missing its tags, treat it as `code-based` · `standard` and flag a warning.

For reference only (use when authoring scenarios, not when generating the table):

**Eval type** — pick based on the THEN clause:

| Type | Use when THEN clause... |
|------|------------------------|
| `code-based` | References observable, machine-checkable state (HTTP status, return value, DB row, error code) |
| `model-based` | Describes semantic behavior requiring judgment (UX quality, error message tone, accessibility) |
| `human-based` | Requires manual verification (visual design, business process approval) |

**Criticality** — pick based on RFC 2119 keyword and domain:

| Level | When | Verify threshold |
|-------|------|-----------------|
| `critical` | MUST/SHALL requirements, security scenarios, data integrity | pass^3 = 1.00 → **FAIL** if absent |
| `standard` | SHOULD requirements, performance goals, UX scenarios | pass@3 ≥ 0.90 → **PASS_WITH_WARNINGS** if absent |

Add an `## Eval Definitions` section at the end of each domain spec file (after Acceptance Criteria Summary):

```markdown
## Eval Definitions

| Scenario | Eval Type | Criticality | Threshold |
|----------|-----------|-------------|-----------|
| REQ-{DOMAIN}-001 › {Scenario Title} | code-based | critical | pass^3 = 1.00 |
| REQ-{DOMAIN}-001 › {Edge Case Title} | code-based | critical | pass^3 = 1.00 |
| REQ-{DOMAIN}-002 › {Scenario Title} | code-based | standard | pass@3 ≥ 0.90 |
| REQ-{DOMAIN}-003 › {Scenario Title} | model-based | standard | pass@3 ≥ 0.90 |
```

Every scenario in the Acceptance Criteria Summary MUST have a corresponding row here. No orphaned scenarios.

### Step 9: Present Summary

Present a markdown summary to the user, then STOP. Do not proceed automatically.

**On success, output:**

```markdown
## SDD Spec: {change_name}

**Domains**: {N}  |  **Requirements**: {N}  |  **Scenarios**: {N}

### Spec Files Written
{For each domain:}
- `openspec/changes/{change_name}/specs/{domain}/spec.md` — {N} requirements, {N} scenarios

### Coverage
| Domain | MUST | SHOULD | MAY | Scenarios |
|--------|------|--------|-----|-----------|
| {domain} | {N} | {N} | {N} | {N} |

### Eval Definitions
- **code-based**: {N}  |  **model-based**: {N}  |  **human-based**: {N}
- **critical**: {N}  |  **standard**: {N}

{If consistency issues: ### ⚠ Consistency Issues\n- {issue}\n}
{If warnings: ### ⚠ Warnings\n- {warning}\n}

**Next step**: Review the spec files in your editor. When satisfied, proceed to `/sdd:design` (if not already running in parallel) and then `/sdd:tasks` once both spec and design are complete.
```

## Rules and Constraints

1. **Use RFC 2119 keywords precisely.** MUST means MUST. Do not use MUST for recommended behaviors.
2. **Every requirement needs at least one testable scenario.** No exceptions. Requirements without scenarios are incomplete.
3. **Scenarios must be concrete.** Use specific values, HTTP codes, type shapes, error messages. No vague "works correctly" assertions.
4. **Delta specs describe CHANGES, not the entire system.** Only specify what is ADDED, MODIFIED, or REMOVED.
5. **If existing specs exist in `openspec/specs/`**, reference them for context. MODIFIED requirements MUST include a "Previously:" reference.
6. **REMOVED requirements MUST include a reason** and migration notes if applicable.
7. **Never modify source code.** Specs are written to `openspec/changes/{change_name}/specs/`.
8. **Never write specs for out-of-scope items.** If the proposal says "OAuth2 token refresh is out of scope", do not write refresh specs.
9. **Specs can run in PARALLEL with sdd-design.** Both depend only on the proposal. Neither depends on the other.
10. **Requirement IDs must be unique** across all domains within the change. Use the format `REQ-{DOMAIN}-{NNN}`.
11. **Align scenarios with project error handling patterns** (per CLAUDE.md). THEN clauses should reference the project's success/error return conventions where applicable.
12. **Include negative scenarios.** For every happy path, include at least one error/edge case scenario showing what happens when things go wrong.
13. **Domain names MUST be kebab-case.** All domain identifiers and spec directory names MUST use lowercase-with-hyphens (e.g., `auth-api`, `user-profile`, `payment-flow`). Explicitly banned: `snake_case` (e.g., `auth_api`), `camelCase` (e.g., `authApi`), `PascalCase` (e.g., `AuthApi`). This ensures consistent directory naming across `openspec/changes/{change}/specs/{domain}/` and `openspec/specs/{domain}.spec.md`.

## Error Handling

- If `openspec/config.yaml` does not exist: return `status: error` recommending `sdd-init`.
- If `proposal.md` does not exist at the given path: return `status: error` with message.
- If proposal has `status: rejected`: return `status: error` with message "Proposal was rejected. Create a new proposal."
- If a domain spec file already exists: overwrite it (specs are regenerated, not appended).
- All errors include the phase name (`spec`) and a human-readable message.

## PARCER Contract

```yaml
phase: spec
preconditions:
  - proposal.md exists and was approved
postconditions:
  - ≥1 spec file in openspec/changes/{changeName}/specs/
  - each spec contains ≥1 GIVEN/WHEN/THEN scenario
  - specs/ directory contains ≥1 spec file with ≥1 scenario
```
