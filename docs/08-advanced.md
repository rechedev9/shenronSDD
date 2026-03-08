# Advanced Usage

Extending and customizing the SDD Workflow ecosystem.

---

## Creating Custom Skills

Skills are just markdown files with structured instructions. You can create new skills for any domain, framework, or workflow pattern.

### 1. Use the skill-creator skill

```
/sdd:explore "create a skill for [your framework]"
```

Or load the skill-creator directly:

```
Read ~/.claude/skills/frameworks/skill-creator/SKILL.md
Then create a skill for [your framework/domain]
```

### 2. Skill file structure

```markdown
---
name: your-skill-name
description: >
  One-line description of what this skill does.
  Trigger: When this skill should be loaded.
license: MIT
metadata:
  author: your-name
  version: "1.0"
---

# Skill Title

## Activation

This skill activates when:
- [Trigger condition 1]
- [Trigger condition 2]

## Input Envelope

[What the orchestrator passes to this sub-agent]

## Execution Steps

### Step 1: [First step]
[Instructions]

### Step 2: [Second step]
[Instructions]

## Rules and Constraints

1. [Hard constraint 1]
2. [Hard constraint 2]

## Error Handling

- [Error case 1]: [Action]
- [Error case 2]: [Action]

## Example Usage

[Realistic input/output example]
```

### 3. Register the skill

Add it to the framework skills lazy-loading table in `CLAUDE.md`:

```markdown
| Your Domain | Trigger description | `~/.claude/skills/frameworks/your-skill/SKILL.md` |
```

### 4. Evolve from learned patterns

If you've used `/learn` to capture patterns, cluster them into a skill with `/evolve`:

```
/evolve --execute
```

This turns 3+ related learned patterns into a structured SKILL.md automatically.

---

## Creating Custom Commands

Commands are markdown files in `~/.claude/commands/`. They define user-invokable slash commands.

### Command file format

```markdown
# /your-command — Short Description

What this command does in 1-2 sentences.

## Arguments
$ARGUMENTS — Argument description
Example: `/your-command arg1 arg2`

## Execution

### Step 1: [First action]
[Instructions for Claude]

### Step 2: [Second action]
[Instructions for Claude]

## Output
[What the user sees when complete]
```

### Example: Custom deployment command

```markdown
# /deploy — Deploy to Staging

Runs verification, builds, and deploys to the staging environment.

## Arguments
$ARGUMENTS — Optional: service name (default: all)

## Execution

### Step 1: Pre-deploy verification
```bash
bun run typecheck && bun run lint && bun test
```

### Step 2: Build
```bash
bun run build
```

### Step 3: Deploy
```bash
fly deploy --config fly.staging.toml
```

### Step 4: Health check
```bash
curl -f https://staging.example.com/health
```

## Output
Report deploy URL and health check status.
```

Save to `~/.claude/commands/deploy.md`. Invoke with `/deploy` or `/deploy api`.

---

## Multi-Project Workflows

### Shared skills across projects

Skills in `~/.claude/skills/` are global — available to all projects. This is intentional. A `react-19` skill should not be duplicated per project.

Project-specific patterns that repeat across sessions belong in learned skills:

```
/learn
# → creates ~/.claude/skills/learned/project-specific-pattern.md
```

### Multiple active changes

SDD supports multiple simultaneous changes in the same project:

```
openspec/changes/
  add-csv-export/    # Change 1 in progress
  fix-session-ttl/   # Change 2 in progress
  archive/
```

Use change names explicitly with `/sdd:continue`:

```
/sdd:continue add-csv-export
/sdd:continue fix-session-ttl
```

Without a name, `/sdd:continue` prompts you to pick from active changes.

### Blocking changes

Some changes must complete before others can start. Document this in proposals:

```markdown
## Dependencies

### Internal Dependencies
- `fix-session-ttl` must be merged before starting `add-remember-me` — the Remember Me feature depends on the fixed session TTL behavior
```

---

## Integrating SDD into CI/CD

### Pre-merge verification

Use `/sdd:verify` as a CI gate. The verify-report.md provides machine-readable results:

```yaml
# Example verify-report.md verdict
Verdict: PASS
```

A simple CI script:
```bash
# .github/workflows/sdd-verify.yml
- name: Run SDD verification
  run: |
    bun run typecheck
    bun run lint
    bun test
    # SDD verify-report is written by /sdd:verify in Claude Code
    # In CI, run the underlying commands directly
```

### Enforcing spec coverage

The `phases.verify.spec_test_coverage_fail` config key controls when verify fails based on scenario coverage. For strict projects:

```yaml
phases:
  verify:
    spec_test_coverage_fail: 20  # FAIL if >20% of scenarios lack tests
```

### AGENTS.md in CI

AGENTS.md rules are enforced by `sdd-review`. For automated enforcement in CI, consider running the review agent on every PR:

```bash
# After implementing and verifying:
# /sdd:review writes review-report.md
# Check the verdict:
grep "Status: FAILED" openspec/changes/*/review-report.md && exit 1 || exit 0
```

---

## Customizing Phase Behavior

### Adjusting the task breakdown

The `phases.tasks.max_files_per_task` setting controls granularity:

```yaml
phases:
  tasks:
    max_files_per_task: 1  # One file per task (maximum precision)
    # or
    max_files_per_task: 3  # Allow related files in one task
```

### Custom phase ordering in tasks

For non-standard architectures, the default 5-phase ordering can be adjusted:

```yaml
phases:
  tasks:
    phase_order:
      - database        # DB schema first (for data-heavy projects)
      - foundation      # Types and config
      - core
      - api             # API before UI (for backend-first teams)
      - frontend
      - testing
      - cleanup
```

### Skipping phases

For trivial changes, you can skip phases that add overhead:

```
# Skip explore for small, well-understood changes:
/sdd:new fix-typo --no-explore

# Skip clean for urgent patches:
/sdd:apply fix-session-ttl
/sdd:verify fix-session-ttl
/sdd:archive fix-session-ttl  # Skip clean
```

The orchestrator respects phase skips — missing artifacts mean that phase was intentionally omitted.

---

## SDD for Non-TypeScript Projects

SDD works with any tech stack. The `sdd-apply` skill adjusts based on `config.yaml`.

### Go project

```yaml
stack:
  runtime: go
  language: go

phases:
  verify:
    commands:
      typecheck: go build ./...
      lint: golangci-lint run
      test: go test ./...
      format_check: gofmt -l .
```

### Python project

```yaml
stack:
  runtime: python
  language: python
  frameworks:
    backend: django
    testing: pytest

conventions:
  error_handling:
    pattern: throw       # Python uses exceptions

phases:
  verify:
    commands:
      typecheck: mypy .
      lint: ruff check .
      test: pytest
      format_check: black --check .
```

### Rust project

```yaml
stack:
  runtime: rust
  language: rust

phases:
  verify:
    commands:
      typecheck: cargo check
      lint: cargo clippy
      test: cargo test
      format_check: cargo fmt --check
```

---

## Phase Delta Tracking & Analytics

### How it works

The orchestrator writes a `QualitySnapshot` to `quality-timeline.jsonl` after every sub-agent returns. This creates a per-change quality timeline that tracks how build health, issue counts, completeness, and scope evolve across the 11 phases.

### Viewing analytics

```
/sdd:analytics add-csv-export
```

This reads the JSONL file and computes:
- **Build health progression** — Did typecheck/lint/tests improve or regress between phases?
- **Issue density by phase** — Which phases introduced the most critical issues?
- **Completeness curve** — How did task/spec coverage grow over the pipeline?
- **Scope summary** — Total files created, modified, reviewed
- **Phase timing** — Duration between consecutive snapshots
- **Regressions** — Any metric that worsened (flagged for attention)

### Using analytics for process improvement

Over multiple changes, analytics reveal patterns:
- If `sdd-apply` consistently introduces type errors that `sdd-verify` catches, the design phase may need stronger interface definitions
- If review REJECT violations appear frequently, the AGENTS.md rules may need to be surfaced earlier (e.g., in the design SKILL.md)
- If completeness plateaus below 100%, task granularity may need adjustment

### Manual inspection

The JSONL format is human-readable. Each line is a self-contained JSON object:

```bash
cat openspec/changes/add-csv-export/quality-timeline.jsonl | jq .
```

---

## Extending Engram Memory

### Custom topic key families

Beyond the default families (`architecture/*`, `bug/*`, `decision/*`, `pattern/*`, `config/*`, `discovery/*`, `learning/*`), create project-specific families:

```
mem_save(
  topic_key: "domain/billing/stripe-webhook-handling",
  content: "Always validate Stripe-Signature header before processing webhook body..."
)
```

### Searching memory

Use `mem_search` to find relevant context before starting work:

```
mem_search("oauth login implementation")
mem_search("stripe billing architecture")
```

Use `mem_context` at session start to load all relevant memories at once.

### Memory cleanup

Remove stale or incorrect memories with `mem_delete`. Update evolving topics with `mem_update` (not `mem_save`, which would create a duplicate — use `mem_suggest_topic_key` first to find the existing key).

---

## Team Usage

### Shared CLAUDE.md

The primary project conventions file. Keep it in the repo:

```
.
├── CLAUDE.md       # Team conventions — versioned
├── AGENTS.md       # AI review rules — versioned
└── openspec/
    ├── config.yaml
    └── specs/      # Living specifications — versioned
```

### Versioning specs

`openspec/specs/` contains the living specifications for your system. Check them into git:
- They document WHAT the system does (not HOW — that's the code)
- They evolve as requirements change (via `/sdd:archive` which merges delta specs)
- They serve as onboarding material for new team members

### PR workflow with SDD

Typical team workflow:

```
1. /sdd:new feature-name "Intent description"
   → exploration.md, proposal.md

2. Team reviews proposal.md in PR (not code yet — just the plan)

3. /sdd:continue feature-name
   → spec.md, design.md (parallel)

4. Team reviews spec.md and design.md
   "Do these specs match what we want to build?"

5. /sdd:apply feature-name --phase 1
   → implements foundation tasks

6. /sdd:review feature-name
   → checks against specs and AGENTS.md

7. /sdd:verify feature-name
   → typecheck, lint, tests pass

8. /commit-push-pr
   → PR created with full context (proposal → spec → design → implementation)

9. /sdd:archive feature-name
   → specs merged into openspec/specs/
```

The PR body can reference `openspec/changes/{name}/proposal.md` for reviewers who want the full context.

---

## When to Skip SDD

SDD adds structure and overhead. Not every change needs all 11 phases.

| Change type | Recommended approach |
|---|---|
| Typo fix | Direct edit — no SDD |
| One-line bug fix | Direct edit — no SDD |
| Config change | Direct edit — no SDD |
| Simple feature (1-2 files) | `/sdd:ff` (fast-forward, no pauses) |
| Standard feature (3-10 files) | Full SDD with `/sdd:new` + `/sdd:continue` |
| Complex feature (10+ files) | Full SDD + consider splitting into smaller changes |
| Multi-session feature | Full SDD with Engram memory enabled |
| Security-sensitive change | Full SDD + careful AGENTS.md rules |

The guiding principle: if you can hold the entire change in your head and describe it in one sentence, skip SDD. If you need to think it through, SDD pays for itself.

---

## Semi-Formal Reasoning

Version 1.1 injects structured reasoning templates into four SDD phases. These are internal cognitive scaffolds — the agent fills them while working, not visible in output artifacts. They address two failure modes: shallow exploration (reading files without purpose) and rubber-stamp review (affirming code without genuinely testing it).

### Structured Exploration Protocol (sdd-explore, Step 4)

When the explore sub-agent reads a file, it must complete a hypothesis cycle:

```
PRE-READ:
  HYPOTHESIS: What do I expect to find, and why?
  EVIDENCE: What prior file, grep result, or test led me here?
  CONFIDENCE: HIGH | MEDIUM | LOW

POST-READ:
  OBSERVATIONS: Key findings with File:Line references (minimum 2 per file)
  HYPOTHESIS STATUS: CONFIRMED | REFUTED | REFINED
  REASON: Why (1-2 sentences)

TRANSITION:
  NEXT ACTION JUSTIFICATION: Why the next file is the logical next step
```

The CONFIDENCE field creates a calibration signal. A HIGH-confidence hypothesis that gets REFUTED demands deeper investigation. A LOW-confidence one that gets CONFIRMED is a genuine discovery.

### Structured Reading Protocol (sdd-apply, Step 3c)

A lightweight version of the exploration protocol, applied before modifying any existing file:

```
HYPOTHESIS: What patterns and conventions does this file use?
EVIDENCE: Which spec, design constraint, or prior file informed this?
OBSERVATIONS: Key patterns with File:Line references
HYPOTHESIS STATUS: CONFIRMED | REFUTED | REFINED
IMPLEMENTATION IMPLICATION: How observations constrain the code to write
```

This prevents the common failure mode where an agent writes new code that contradicts existing patterns in the same file.

### Semi-Formal Certificate (sdd-review, Steps 3h–3j)

Three additions to the review process:

**Function Tracing Table (Step 3h):**

| Function | File:Line | Parameter Types | Return Type | Verified Behavior |
|----------|-----------|-----------------|-------------|-------------------|
| `login` | `session.ts:42` | `(email: string, pw: string)` | `Promise<Result<Session, AuthError>>` | Err on invalid creds |

Every exported function touched by the change gets a row.

**Data Flow Analysis (Step 3i):**

```
credentials (LoginRequest)
  → CREATED at auth.controller.ts:15 (parsed from request body)
  → VALIDATED at auth.controller.ts:18 (Zod safeParse)
  → CONSUMED at auth.service.ts:42 (password comparison)
  → INVARIANT: email is always lowercase after validation
```

Traces data creation, transformation, consumption, and invariants.

**Counter-Hypothesis Check (Step 3j):**

For each critical function, the review agent must actively try to break the implementation:

```
CLAIM: "verifyPassword at auth.service.ts:48 could fail when..."
EVIDENCE SOUGHT: What edge case or code path would trigger failure?
FINDING: VULNERABILITY FOUND | NO EVIDENCE OF FAILURE
DETAILS: [specific code location and explanation]
```

Minimum one counter-hypothesis per critical function. This shifts review from passive verification to active adversarial analysis.

### Fault Localization Protocol (sdd-verify, Step 5b)

When tests fail, the verify agent must diagnose each failure structurally:

**PREMISES (Test Semantics):**
1. Test identifier (`describe > it` path)
2. Setup (Arrange): preconditions, variables, mocks
3. Action (Act): function call with exact signature
4. Assertion (Assert): expected outcome, quoted verbatim

**DIVERGENCE CLAIMS:**
```
CLAIM: Test expects Result.ok to be true for valid credentials
  (auth.test.ts:25), but verifyPassword() at auth.service.ts:48
  returns false because it compares raw password against hash
  without bcrypt.compare().
EVIDENCE: auth.service.ts:48 — return password === storedHash
CONFIDENCE: HIGH
```

This enables sdd-apply to fix issues precisely, without re-investigating the failure.

---

## Experience-Driven Early Termination (EET)

The build-fix loop in sdd-apply (Step 4) now includes an intelligent early-stop mechanism that leverages Engram persistent memory.

### How it works

1. **Error Signature**: Each build error is normalized into a fingerprint: `{errorCode}:{file}:{category}` (e.g., `TS2345:src/auth/session.ts:type-mismatch`)
2. **Engram Query**: Before fix attempt #3 (and every subsequent attempt), the agent queries Engram for matching `bug/*` or `pattern/build-fix-loop` entries
3. **Trajectory Evaluation**: If a prior session encountered the same error and exhausted ≥3 similar fix attempts without resolution, the current fix cycle triggers **early termination**
4. **Escalation Memory**: When any fix cycle exhausts all 5 attempts, the failure pattern is saved to Engram under `bug/build-fix/{errorSignature}` — feeding future EET queries

### Behavior

```
Attempt 1 → fix → still fails
Attempt 2 → fix → still fails
Attempt 3 → EET check:
  mem_search("TS2345:src/auth/session.ts:type-mismatch")
  → Match found: same error, 4 failed attempts in prior session
  → EARLY TERMINATION: abort, return FAILED with earlyTermination.reason
```

The max-5-attempts hard ceiling remains. EET is an additional smart stop that saves tokens when the trajectory is recognized as unproductive.

### When Engram is unavailable

If Engram MCP tools are not running, EET checks are skipped silently. The build-fix loop falls back to the standard max-5-attempts behavior.

---

## Dynamic Agentic Rubric

The review phase (sdd-review) now generates a change-specific evaluation rubric before reviewing any code. This anchors the review to the actual requirements rather than generic best practices.

### Rubric Generation (Step 2b)

The agent reads all specs, design.md, AGENTS.md, and CLAUDE.md, then produces a rubric table:

| # | Criterion | Source | Weight | Pass Condition |
|---|-----------|--------|--------|----------------|
| 1 | Valid credentials return session token | auth-login.spec.md:S1 | CRITICAL | `login()` returns `Ok(Session)` |
| 2 | Module boundary respected | design.md:AD-3 | REQUIRED | No cross-boundary imports |
| 3 | No `any` types | AGENTS.md:REJECT | CRITICAL | Zero `any` in changed files |

**Weights:**
- `CRITICAL` — Failure = review FAILED (spec scenarios + REJECT rules)
- `REQUIRED` — Failure = review FAILED (design contracts + REQUIRE rules)
- `PREFERRED` — Failure = noted as SUGGESTION (PREFER rules)

### Post-Review Scoring

After completing all review steps (3a–3j), each rubric criterion is scored:

| # | Criterion | Score | Evidence |
|---|-----------|-------|----------|
| 1 | Valid credentials return session | PASS | `login()` at session.ts:42 returns `Ok(session)` |
| 3 | No `any` types | FAIL | `any` at handler.ts:18 |

The verdict must be consistent with the rubric. If any CRITICAL or REQUIRED criterion is FAIL, the verdict is FAILED — regardless of other findings.

### Why this matters

Without a rubric, the review agent's evaluation criteria are implicit — baked into the SKILL.md instructions. The agent might prioritize code style over spec compliance, or miss a REJECT rule because it was not top of mind. The rubric makes criteria explicit and orderable before the review begins.

---

## PARCER Operational Contracts

Version 1.1 adds formal pre/post-conditions for every SDD phase, inspired by the PARCER governance framework. These are defined in `openspec/config.yaml` and validated by the orchestrator.

### Contract Structure

```yaml
contracts:
  explore:
    preconditions:
      - config.yaml exists at openspec/config.yaml
      - topic is non-empty string
    postconditions:
      - exploration.md written (if changeName provided)
      - envelope contains relevant_files with ≥1 entry
      - envelope status is success or error
  apply:
    preconditions:
      - tasks.md exists with ≥1 uncompleted task
      - design.md exists
      - spec files exist in specs/
    postconditions:
      - ≥1 task marked [x] in tasks.md
      - buildStatus included in envelope
      - all created/modified files listed
  # ... (all 10 phases have contracts)
```

### Orchestrator Validation

Before dispatching any sub-agent, the orchestrator validates preconditions:

1. Load `contracts.{phase}` from config.yaml
2. Check each precondition (file exists, field non-empty, prior phase completed)
3. **If any fails** → block launch, report unmet preconditions, suggest which phase to run first
4. **After sub-agent returns** → check postconditions, log warnings to quality timeline if failed

Postcondition failures do not block the next phase — they are recorded as warnings in `quality-timeline.jsonl` for analytics.

### Legacy compatibility

If `contracts` section does not exist in config.yaml (projects initialized before v1.1), contract validation is skipped entirely. Run `/sdd:init --force` to regenerate config.yaml with contracts.

---

## Navigation

- [← 07-configuration.md](./07-configuration.md) — Configuration reference
- [→ 09-troubleshooting.md](./09-troubleshooting.md) — Troubleshooting guide
- [↑ README.md](../README.md) — Back to start
