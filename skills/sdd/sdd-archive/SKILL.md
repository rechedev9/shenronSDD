---
name: sdd-archive
description: >
  Close a completed change. Merge delta specs into main specs, move change folder to archive, capture learnings.
  Trigger: When user runs /sdd:archive or after sdd-verify passes with no CRITICAL issues.
license: MIT
metadata:
  version: "1.0"
---

# SDD Archive — Change Closure

You are executing the **archive** phase inline. Your responsibility is to **close a completed change** by merging delta specs into the main spec source of truth, archiving the change folder for audit trail, and capturing any learnings for future sessions. You are the final step in the SDD pipeline.

## Activation

User runs `/sdd:archive`. Reads `proposal.md`, `verify-report.md`, and optionally `review-report.md` from disk. Aborts if verify verdict is FAIL with no clean-report override.

## Inputs

Read from disk:

| Input | Source |
|---|---|
| `changeName` | Infer from `openspec/changes/` (the active change folder) |
| `proposal.md` | `openspec/changes/{changeName}/proposal.md` |
| `verify-report.md` | `openspec/changes/{changeName}/verify-report.md` |
| `review-report.md` | `openspec/changes/{changeName}/review-report.md` (optional) |

---

## Execution Steps

### Step 1 — Safety Check

1. Read `verify-report.md`.
2. Parse the **verdict** field.
3. If verdict is **FAIL** or there are any **CRITICAL** issues:
   - **Check for clean-report override:** If `openspec/changes/{changeName}/clean-report.md` exists, read it. If the clean-report's build status shows all checks PASS and it reports all FAIL conditions resolved, the clean-report **supersedes** the stale verify-report for archive eligibility — proceed to step 4.
   - Otherwise, **ABORT immediately.** Do not archive a failing change.
   - Stop immediately and present an error message explaining why archiving was aborted.
4. If verdict is **PASS_WITH_WARNINGS**:
   - Proceed but include warnings in the archive summary.
   - Note that the change was archived with known warnings.
5. If `review-report.md` exists, check for unresolved REJECT violations:
   - If any REJECT violations are unresolved: **ABORT.** REJECT violations are blocking.

### Step 2 — Read Change Artifacts

1. Read the full contents of the change folder:
   - `openspec/changes/{changeName}/proposal.md` — read for intent, scope, success criteria, and rollback plan
   - `openspec/changes/{changeName}/exploration.md` (if exists)
   - `openspec/changes/{changeName}/tasks.md`
   - `openspec/changes/{changeName}/design.md`
   - `openspec/changes/{changeName}/specs/` (all spec files)
   - `openspec/changes/{changeName}/verify-report.md`
   - `openspec/changes/{changeName}/review-report.md` (if exists)
2. Parse each spec file to identify:
   - **Domain**: Which domain/feature area does this spec belong to?
   - **Delta type**: Is this an ADDED, MODIFIED, or REMOVED requirement?
   - **Requirement name/ID**: Unique identifier for matching against main specs.

### Step 3 — Merge Delta Specs into Main Specs

The main specs live in `openspec/specs/`. Each file represents a domain (e.g., `auth.spec.md`, `billing.spec.md`).

#### 3a. ADDED Requirements

For each new requirement in the delta specs:

1. Identify the target domain (from spec metadata or folder structure).
2. Check if `openspec/specs/{domain}.spec.md` exists.
   - If YES: Append the new requirement to the appropriate section of the existing spec.
   - If NO: Create `openspec/specs/{domain}.spec.md` with the new requirement as the initial content.
3. Preserve the GIVEN/WHEN/THEN format exactly as written in the delta spec.
4. Add a metadata comment: `<!-- Added: {YYYY-MM-DD} from change: {changeName} -->`

#### 3b. MODIFIED Requirements

For each modified requirement:

1. Find the matching requirement in `openspec/specs/{domain}.spec.md` by name or ID.
2. **Replace** the old requirement with the updated version from the delta spec.
3. Add a metadata comment: `<!-- Modified: {YYYY-MM-DD} from change: {changeName} -->`
4. Keep the old version as a comment block (for audit trail):
   ```markdown
   <!-- Previous version (before {changeName}):
   [old requirement text]
   -->
   ```

#### 3c. REMOVED Requirements

For each removed requirement:

1. Find the matching requirement in `openspec/specs/{domain}.spec.md`.
2. **Warn before removing.** Removal is destructive — note it prominently in the archive summary.
3. Comment out the requirement rather than deleting it:
   ```markdown
   <!-- Removed: {YYYY-MM-DD} from change: {changeName}
   [removed requirement text]
   -->
   ```
4. If the entire spec file would be empty after removal, keep the file with a header noting it was deprecated.

#### 3d. Conflict Resolution

When replacing or appending content in a main spec file, the target section may have been altered by another change merged since the delta was created:

1. **No conflict** — The target section matches expectations (or the delta is purely ADDED). Apply normally.
2. **Resolvable conflict** — The target section has been modified but the semantic intent of both versions is compatible (e.g., another change added a scenario to the same requirement). The agent MUST intelligently merge the semantic intent of both versions, preserving all non-contradictory content from each.
3. **Irreconcilable conflict** — There is a direct logical contradiction that cannot be safely merged (e.g., one version says "MUST return 200" and the delta says "MUST return 204" for the same scenario). The agent MUST:
   - Abort the merge for **that specific file only** (other domain merges continue).
   - Leave standard git-style conflict markers in the file:
     ```
     <<<<<<< main-spec (current)
     [existing content]
     =======
     [delta content from {changeName}]
     >>>>>>> delta ({changeName})
     ```
   - Set the archive status to `PARTIAL`.
   - Add the conflicted file to `phaseSpecificData.warnings` with reason `"MERGE_CONFLICT_REQUIRES_HUMAN"`.

#### 3e. No Main Spec Exists

If the delta introduces specs for a domain that has no main spec file:

1. Create `openspec/specs/{domain}.spec.md`.
2. Add a header with domain name, creation date, and source change.
3. Copy all delta specs for that domain as the initial content.

### Step 4 — Archive the Change

1. Create the archive directory if it does not exist:
   ```
   openspec/changes/archive/
   ```
2. Move the entire change folder:
   ```
   openspec/changes/{changeName}/ -> openspec/changes/archive/{YYYY-MM-DD}-{changeName}/
   ```
   Where `{YYYY-MM-DD}` is today's date in ISO 8601 format.
3. Create an archive manifest inside the archived folder:
   ```markdown
   # Archive Manifest: {changeName}

   **Archived**: {YYYY-MM-DD}
   **Verdict**: {PASS | PASS_WITH_WARNINGS}
   **Tasks Completed**: {X}/{Y}
   **Specs Merged**: {list of domains updated}
   **Warnings**: {count, if any}

   ## Change Summary
   {Brief description of what was done and why — reference proposal.md's Intent for the "what" and "why"}

   ## Key Decisions
   {Important architectural or design decisions made during this change}

   ## Files Created
   {list}

   ## Files Modified
   {list}
   ```

### Step 5 — Capture Learnings

Review the entire change lifecycle for patterns worth remembering:

#### 5a. Pattern Detection

Look for:
- **Recurring challenges**: Did the same type of error come up multiple times? (e.g., "always need to handle null for this API")
- **Design decisions**: Were there trade-offs that would apply to future changes?
- **Process improvements**: Did the SDD pipeline itself need workarounds?
- **Domain knowledge**: Facts about the codebase that would help future agents.
- **Gotchas**: Surprising behaviors, edge cases, or non-obvious constraints.

#### 5b. Save Learnings to Skills

If a significant, reusable pattern is found:

1. Create a learning file at `~/.claude/skills/learned/{pattern-name}.md`:
   ```markdown
   ---
   name: {pattern-name}
   source: sdd-archive
   date: {YYYY-MM-DD}
   change: {changeName}
   ---

   # {Pattern Name}

   ## Context
   {When does this pattern apply?}

   ## Pattern
   {What should you do?}

   ## Example
   {Concrete code or process example}

   ## Anti-pattern
   {What to avoid}
   ```

2. Only save learnings that are:
   - **Reusable**: Applicable beyond this specific change.
   - **Non-obvious**: Not something covered by CLAUDE.md or standard conventions.
   - **Actionable**: Provides a clear recommendation, not just an observation.

#### 5c. Memory Integration (Conditional on `memory_enabled`)

After capturing learnings, persist them to the memory RAG server for cross-session recall:

1. Build a structured summary containing:
   - Key decisions and their trade-offs (from `proposal.md` and `design.md`)
   - Gotchas and non-obvious constraints discovered during implementation
   - Domain-specific knowledge that would help future agents
2. Call `mem_save` for each distinct learning:
   - `topic_key`: Use hierarchical format — `decision/{changeName}-{topic}`, `pattern/{pattern-name}`, `discovery/{domain}-{insight}`
   - `content`: Write naturally using `**What** / **Why** / **Where** / **Learned**` format. No keyword enrichment needed — semantic search handles vocabulary matching automatically.
   - `project`: Pass the project name for namespace isolation.
   - `tags`: Categorize as `["decision"]`, `["pattern"]`, `["discovery"]`, or `["learning"]`.

If `config.yaml → capabilities.memory_enabled` is `false`, skip this entire step. If `true` but tools fail at runtime, note the failure in the archive summary and proceed.

#### 5d. Memory Pruning (Conditional on `memory_enabled`)

After saving new learnings, prune memories rendered obsolete by this change:

1. **Identify decay candidates** — Review the change's key decisions (`design.md` Architecture Decisions) and the delta specs (MODIFIED and REMOVED requirements). For each, run `mem_search` with a natural language query describing the superseded decision or removed behavior.

2. **Evaluate each candidate** — For every retrieved memory, classify:
   - **OBSOLETE**: The memory describes a pattern, decision, or bugfix that this change directly contradicts or replaces. → **Delete** via `mem_delete`.
   - **SUPERSEDED**: The memory is partially outdated. → **Delete** via `mem_delete`, then **save** a corrected version via `mem_save` with the same topic_key.
   - **STILL VALID**: The memory is not affected by this change. → Leave untouched.

3. **Pruning guardrails**:
   - Never delete memories with `decision/*` topic keys without finding a replacement decision in the current change.
   - Never delete memories less than 7 days old — they may be from a concurrent change still in progress.
   - Log every deletion in `phaseSpecificData.memoryPruning`: `{ deleted: [topic_keys], replaced: [topic_keys], preserved: count }`.

If `memory_enabled` is `false`, skip entirely.

### Step 6 — Present Summary

Append one final JSONL line to `openspec/changes/{changeName}/quality-timeline.jsonl` (if quality tracking enabled):
```json
{ "changeName": "...", "phase": "archive", "timestamp": "...", "agentStatus": "SUCCESS|ERROR", "completeness": null, "buildHealth": null, "issueCount": { "critical": 0 }, "phaseSpecific": { "specsMerged": N, "learningsSaved": N } }
```

Present a markdown summary to the user, then STOP:

```markdown
## SDD Archive: {change_name} ✅

**Change closed and archived.**

### Specs Merged into Main
{For each domain:}
- `openspec/specs/{domain}.spec.md` — {N} added, {N} modified, {N} removed

### Change Summary
{changeSummary.description}

**Key decisions**: {keyDecisions list}
**Files created**: {N}  |  **Files modified**: {N}

{If learnings: ### Learnings Saved to Memory ({N})\n{learning names and summaries}\n}
{If memoryPruning.deleted: ### Memory Pruned ({N} stale entries removed)\n}
{If warnings: ### ⚠ Warnings\n{warnings list}\n}

### Archive Location
`openspec/changes/archive/{change_name}/`

**The SDD pipeline for `{change_name}` is complete.** Run `/sdd:analytics {change_name}` to view quality metrics for this change.
```

If aborted (`ERROR`): output a short message explaining why (FAIL verdict, unresolved REJECT violations, etc.) and what to fix before retrying.

---

## Rules — Hard Constraints

1. **NEVER archive a FAIL verdict** — unless `clean-report.md` exists and supersedes it (see Step 1.3). If verify-report says FAIL or has CRITICAL issues and no clean-report resolves them, abort immediately.
2. **NEVER archive with unresolved REJECT violations.** If review-report has REJECT violations, abort.
3. **Spec merge is additive by default.** For REMOVED requirements, warn prominently — do not silently delete.
4. **Archive is permanent.** Never delete archived changes. They serve as an audit trail.
5. **Date format is ISO 8601.** Always use `YYYY-MM-DD`.
6. **Main specs are the source of truth after merge.** The delta specs in the archive are historical artifacts.
7. **Learnings are selective.** Do not force patterns. Only save genuinely useful, reusable, non-obvious insights — but always persist them to memory when found (Step 5c).
8. **Preserve previous versions.** When modifying a main spec requirement, keep the old version as a comment.
9. **One domain per spec file.** Do not merge requirements from different domains into the same main spec file.
10. **No code changes.** This agent does NOT modify source code. Only spec files, archive folders, and learnings.

---

## Spec Merge Conflict Resolution

| Situation | Action |
|---|---|
| Delta adds a requirement that already exists in main spec | Treat as MODIFIED — update the existing requirement |
| Delta modifies a requirement that doesn't exist in main spec | Treat as ADDED — append to the domain spec |
| Delta removes a requirement that doesn't exist in main spec | Ignore — nothing to remove, note it in the archive summary |
| Two delta specs modify the same main requirement | Apply in order (by spec filename alphabetically), note potential conflict |
| Domain name in delta doesn't match any existing spec | Create a new spec file for the domain |

---

## Edge Cases

| Situation | Action |
|---|---|
| `openspec/specs/` directory doesn't exist | Create it before merging |
| `openspec/changes/archive/` directory doesn't exist | Create it before archiving |
| Change folder is empty (all artifacts deleted) | Abort — nothing to archive |
| Verify report has PASS_WITH_WARNINGS and 10+ warnings | Archive but prominently note the warning count |
| Learning pattern name conflicts with existing file | Append a version number (e.g., `pattern-name-v2.md`) |
| `memory_enabled: false` in config.yaml | Skip Step 5c entirely — no memory calls attempted |
| `memory_enabled: true` but memory tools fail at runtime | Note failure in archive summary, proceed with archive |
| Multiple changes archive on the same date with the same name | Append a counter: `{YYYY-MM-DD}-{changeName}-2` |
| Change was partially implemented (not all tasks [x]) | If verify PASSED (meaning partial was intentional), archive. Note incomplete tasks |

---

## Archive Folder Structure

After archiving, the structure should look like:

```
openspec/
  specs/                          # Main specs (source of truth)
    auth.spec.md                  # Updated with merged deltas
    billing.spec.md
  changes/
    active-change/                # Still in progress (not archived)
      tasks.md
      design.md
      specs/
    archive/
      2026-02-22-auth-lockout/    # Archived change
        proposal.md
        exploration.md
        tasks.md
        design.md
        specs/
        verify-report.md
        review-report.md
        archive-manifest.md       # Created during archive
```

---

## PARCER Contract

```yaml
phase: archive
preconditions:
  - clean phase completed successfully
  - verify verdict is PASS or PASS_WITH_WARNINGS (no CRITICAL issues)
postconditions:
  - change directory moved to openspec/changes/archive/{date}-{changeName}/
  - archive-manifest.md written
  - delta specs merged into openspec/specs/
```
