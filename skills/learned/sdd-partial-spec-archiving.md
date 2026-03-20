---
name: sdd-partial-spec-archiving
source: sdd-archive
date: 2026-02-22
change: exploracion-de-mejoras
---

# SDD Partial Spec Archiving Pattern

## Context

When archiving an SDD change where some spec requirements are intentionally deferred or only partially implemented, the archive must document the partial state without blocking the archive on unfinished work.

## Pattern

1. **Verify verdict first** — only archive if verify-report says PASS or PASS WITH WARNINGS. A genuine partial implementation that verify agrees with is still archive-worthy.

2. **Note deferred scenarios inline in the merged spec** — inside the main spec file for the requirement, add an `**IMPLEMENTATION STATUS**` note directly under the requirement header explaining which scenarios are deferred and why. Do not delete or hide the deferred scenarios; keep them in full so the future implementer has the full acceptance criteria.

3. **Track deferred specs separately** — if an entire initiative is deferred (e.g., Initiative 5 hook consolidation), create its main spec file in `openspec/specs/` with a prominent `**Status**: DEFERRED` header and implementation gate conditions. This pre-populates the spec for the next change.

4. **Archive manifest deferred work section** — the archive-manifest.md must list each deferred item with: which requirement IDs are affected, what specifically is missing, and the recommended next action (e.g., a specific SDD command to run).

## Example

For REQ-CORR-007 (guest mode), scenarios 1 and 4 were FULLY COVERED, scenarios 2 and 3 were DEFERRED. The merged spec includes:

```markdown
**IMPLEMENTATION STATUS**: Scenarios 2 and 3 (guest mutation persistence and page-refresh survival)
are DEFERRED pending full wiring of the `isGuest` mutation branch in `use-program.ts`.
Scenarios 1 and 4 are FULLY COVERED as of 2026-02-22.
```

## Anti-pattern

Do NOT remove deferred scenarios from the merged spec. Do NOT create a separate "deferred.spec.md" alongside the main spec — keep everything in one domain file. Do NOT archive a change if verify-report has CRITICAL failures (even partial implementations must pass typecheck + lint + tests).
