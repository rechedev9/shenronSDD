---
summary: "Advanced topics: CLI architecture, caching, Context Cascade, pipeline metrics, Semi-Formal Reasoning, EET, Agentic Rubrics, PARCER Contracts."
read_when:
  - "Advanced SDD usage"
  - "Extending SDD with new techniques"
---

# Advanced Usage

Extending and customizing the SDD Workflow ecosystem.

---

## CLI Architecture: What the Go Binary Owns

The `sdd` binary is not an LLM wrapper. It is a deterministic orchestrator that handles
everything that does not require intelligence, so the LLM only handles what does.

```
sdd <command>
  ├── context    — assemble prompt context from SKILL.md files + config
  ├── verify     — run build/lint/test, write verify-report.md
  ├── health     — pipeline observability (quality-timeline.jsonl)
  └── init       — detect stack, write config.yaml (zero LLM calls)
```

Responsibilities split:

| Task | Go binary | LLM |
|---|---|---|
| Read SKILL.md files from disk | `sdd context` | — |
| Select skills based on stack + phase | `sdd context` | — |
| Content-hash cache (skip unchanged skills) | `sdd context` | — |
| Assemble final context blob | `sdd context` | — |
| Run shell commands (build, lint, test) | `sdd verify` | — |
| Parse command output → structured verdict | `sdd verify` | — |
| Stream progress during verify | `sdd verify` | — |
| Aggregate quality-timeline.jsonl | `sdd health` | — |
| Detect stack, write config.yaml | `sdd init` | — |
| Write spec, design, tasks | — | LLM |
| Generate code | — | LLM |
| Semantic review | — | LLM |
| Fault localization | — | LLM |
| Engram memory (mem_save, mem_context) | — | LLM via MCP |

---

## Content-Hash Caching Architecture

Every SKILL.md file has a content hash (SHA-256) stored in `openspec/.skill-cache`. When
`sdd context` runs, it computes the current hash of each candidate skill and compares it
to the cache.

```
sdd context --phase apply --change add-csv-export
→ candidate skills: [sdd-apply, typescript, react-19, zod-4]
→ hash(sdd-apply/SKILL.md)   = a3f...  cache = a3f...  → SKIP (unchanged)
→ hash(typescript/SKILL.md)  = 9b2...  cache = 9b2...  → SKIP (unchanged)
→ hash(react-19/SKILL.md)    = 7c1...  cache = 88d...  → INCLUDE (changed since last run)
→ hash(zod-4/SKILL.md)       = 4e5...  cache = (none)  → INCLUDE (new)

Assembled context: react-19 + zod-4 only (2 skills, not 4)
Cache updated for react-19 and zod-4.
```

**Why this matters**: In a typical workflow, most skills are unchanged between sessions.
Caching means only skills that were actually edited get re-sent to the LLM. On a stable
project, `sdd context` may send 0–1 skills after the first run.

**Cache file location:** `openspec/.skill-cache` (JSON, checked into `.gitignore` by `sdd init`)

**Cache invalidation:** Any edit to a SKILL.md file invalidates its cache entry. The cache
is per-project, not global — different projects maintain independent caches.

---

## Context Cascade

The Go assembler builds context in layered priority order. Each layer can override or
supplement the previous. Lower numbers = higher priority (later layers cannot override
earlier ones for conflicting keys).

```
Priority 1 (highest): Phase skill
  ~/.claude/skills/sdd/{phase}/SKILL.md
  Always included. Defines the sub-agent's core instructions.

Priority 2: Stack-matched framework skills
  ~/.claude/skills/frameworks/{framework}/SKILL.md
  Included when stack.frameworks.* matches in config.yaml.
  If react-19 and nextjs-15 both match, both are included.

Priority 3: Analysis skills (auto-pulled by phase skill)
  ~/.claude/skills/analysis/{skill}/SKILL.md
  Included when the phase SKILL.md declares a dependency on them.
  (e.g., sdd-verify declares dependency on build-validator)

Priority 4: Knowledge skills (project-configured)
  ~/.claude/skills/knowledge/{skill}/SKILL.md
  Included when listed in config.yaml skills.extra.

Priority 5 (lowest): Learned skills
  ~/.claude/skills/learned/*.md
  Included only when explicitly listed in config.yaml skills.extra.
  Not auto-included — too project-specific for general use.
```

**Conflict resolution**: If two skills define contradictory instructions for the same
scenario, the higher-priority skill wins. This means phase skills always take precedence
over framework skills — the phase defines correctness, the framework adds style.

**Observability**: Run `sdd context --dry-run` to see exactly which skills would be
included and in what order, without assembling the full blob.

```
sdd context --phase review --change add-csv-export --dry-run
Priority 1: sdd-review         (3,310 tokens)
Priority 2: typescript         (1,840 tokens)
Priority 2: react-19           (2,100 tokens)
Priority 2: zod-4              (  980 tokens)
Priority 3: build-validator    (1,200 tokens) [auto-pulled]
──────────────────────────────────────────────
Total context contribution:    9,430 tokens
Cached (skipped):              7,040 tokens (sdd-review + typescript — unchanged)
Net context to send:           2,280 tokens
```

---

## Per-Dimension TTL

`sdd health` tracks quality dimensions across the pipeline. Each dimension has an
independent TTL — how long a passing result remains valid before the next run must
re-verify it.

| Dimension | TTL | Rationale |
|---|---|---|
| `typecheck` | until source file changes | Type errors are deterministic |
| `lint` | until source file changes | Lint results are deterministic |
| `test` | until source or test file changes | Tests are deterministic |
| `build` | until source file changes | Build output is deterministic |
| `spec_coverage` | until spec or test file changes | Coverage depends on both |
| `review_verdict` | until implementation file changes | Review result tied to specific code |
| `engram_freshness` | 7 days | Memory may become stale as code evolves |

TTLs are stored per-change in `openspec/changes/{name}/.dimension-ttl.json`.

When you run `sdd verify`, it checks TTLs before executing commands:

```
sdd verify --change add-csv-export
→ typecheck TTL: valid (no source changes since last green run) → SKIP
→ lint TTL: valid → SKIP
→ test TTL: expired (test file edited 2 minutes ago) → RUN
→ build TTL: valid → SKIP

Running: bun test
  PASS  src/services/workout-export.test.ts (12 tests)
Verdict: PASS
```

Override TTLs to force a full re-run:

```
sdd verify --change add-csv-export --force
# ignores all TTLs, runs every command
```

---

## Smart-Skip Verify

Smart-skip is a coarser mechanism than TTL: it gates the entire `commands.build` step
based on whether any source file in the change's blast radius changed since the last
green verify run.

```yaml
# config.yaml
phases:
  verify:
    smart_skip:
      enabled: true   # default
```

How it works:

1. `sdd verify` reads `openspec/changes/{name}/apply-report.md` for the list of
   created/modified files.
2. Computes content hashes for all listed files + their direct imports.
3. Compares to hashes stored in `openspec/.verify-cache` from the last green run.
4. If all hashes match → skip `commands.build`, proceed to typecheck + lint + test.
5. If any hash changed → run full verify including build.

TypeCheck, lint, and test are never skipped by smart-skip (only `build`). Build is
typically the slowest step (minutes for large projects); the others are fast enough to
always run.

---

## `sdd verify` — Go-Native Quality Gate

`sdd verify` runs build/lint/test as native Go subprocess calls. It does not spawn an
LLM. It streams output line-by-line and writes a structured verdict.

```
sdd verify --change add-csv-export

[sdd verify] Running typecheck...
  bun run typecheck

  src/services/workout-export.ts: no errors

[sdd verify] typecheck PASS (1.2s)

[sdd verify] Running lint...
  bun run lint

  ✓ 12 files linted, 0 errors

[sdd verify] lint PASS (0.8s)

[sdd verify] Running tests...
  bun test src/

  src/services/workout-export.test.ts:
    ✓ includes header row
    ✓ returns headers for empty history
    ✓ renders null fields as empty strings
    ✓ rejects invalid date format
    ... (12 tests total)

[sdd verify] test PASS (2.1s)

[sdd verify] ─────────────────────────────────────
[sdd verify] Verdict: PASS
[sdd verify] typecheck: PASS | lint: PASS | test: PASS
[sdd verify] Duration: 4.1s
[sdd verify] Wrote: openspec/changes/add-csv-export/verify-report.md
```

On failure, `sdd verify` writes the failure output to verify-report.md and exits non-zero,
making it CI-safe:

```bash
sdd verify --change add-csv-export || exit 1
```

To invoke fault localization (LLM analysis of failures):

```
sdd verify --change add-csv-export --diagnose
# runs verify first; if failures found, invokes sdd-verify SKILL.md
# with fault localization protocol for each failing test
```

---

## `sdd health` — Pipeline Observability

`sdd health` reads `quality-timeline.jsonl` and renders pipeline metrics across all
phases of a change (or across all changes in the project).

```
sdd health --change add-csv-export

Change: add-csv-export
Phases completed: explore → propose → spec → design → tasks → apply → review → verify

Build health progression:
  explore:  n/a
  apply:    typecheck PASS | lint PASS | test FAIL (3 failures)
  verify:   typecheck PASS | lint PASS | test PASS

Issue density by phase:
  apply:    3 test failures introduced
  review:   0 new issues (1 existing issue noted)
  verify:   0 issues (all resolved)

Spec coverage:
  REQ-EXPORT-001: covered ✓
  REQ-EXPORT-002: covered ✓
  REQ-EXPORT-003: covered ✓
  REQ-EXPORT-004: covered ✓
  REQ-EXPORT-005: covered ✓
  Coverage: 5/5 (100%)

Phase timing:
  explore:  8 min
  propose:  4 min
  spec:     6 min  ─┐ parallel
  design:   7 min  ─┘
  tasks:    3 min
  apply:   22 min
  review:  11 min
  verify:   4 min
  Total:   65 min

Regressions: none
```

For project-wide metrics:

```
sdd health
# shows metrics across all active changes + archived changes
# highlights: phases with high issue density, changes exceeding time norms
```

`sdd health` is read-only — it never modifies files. Use it at any point during a change
to understand pipeline state.

---

## Concurrent Spec + Design Assembly

`sdd context` supports concurrent assembly for phases that can run in parallel. Spec and
design are the canonical parallel pair — they both require only the proposal as input.

```
sdd context --phase spec,design --change add-csv-export
# assembles context for BOTH sdd-spec and sdd-design
# Go runs both SKILL.md resolutions concurrently
# returns a merged context blob suitable for parallel sub-agent execution
```

The assembled blob contains both skill sets, with a phase-routing header that tells the
LLM which instructions apply to which sub-task:

```
[CONTEXT: spec phase]
{sdd-spec SKILL.md content}

[CONTEXT: design phase]
{sdd-design SKILL.md content}

[SHARED CONTEXT]
{typescript, react-19, zod-4 skills — shared by both phases}
```

The orchestrator then invokes two sub-agents concurrently (or one sub-agent that works
both tasks). The Go binary handles the context assembly — the LLM does not need to figure
out which skills apply to which phase.

---

## Pipeline Metrics

`quality-timeline.jsonl` is the append-only audit trail. Every phase completion appends
one JSON object:

```json
{
  "change": "add-csv-export",
  "phase": "apply",
  "timestamp": "2026-03-20T14:22:11Z",
  "duration_seconds": 1340,
  "build_health": {
    "typecheck": "PASS",
    "lint": "PASS",
    "test": "FAIL",
    "test_failures": 3
  },
  "spec_coverage": {
    "total": 5,
    "covered": 2,
    "percent": 40
  },
  "tasks_completed": 4,
  "tasks_total": 7,
  "files_created": ["src/services/workout-export.ts"],
  "files_modified": ["src/controllers/workout.controller.ts"],
  "issues": [
    {"severity": "CRITICAL", "source": "test", "message": "renders null fields as \"null\""}
  ],
  "context_tokens": 42300,
  "skill_cache_hits": 3,
  "skill_cache_misses": 1
}
```

The `context_tokens` and `skill_cache_hits` fields are populated by the Go assembler
before the LLM call. They let you track token consumption and cache effectiveness
over time without any LLM involvement.

Inspect directly:

```bash
cat openspec/changes/add-csv-export/quality-timeline.jsonl | jq .
```

Or use `sdd health` for the rendered view.

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

[What the assembler passes to this sub-agent]

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

For automatic inclusion by `sdd context`, add it to the appropriate category:

- **Framework skill** (stack-matched): place in `~/.claude/skills/frameworks/{name}/SKILL.md`.
  The Go assembler matches against `config.yaml` stack fields.
- **Always-included project skill**: add to `config.yaml` under `skills.extra`.
- **Manual-only skill**: place anywhere and load with `/your-skill` slash command.

For framework skills that should auto-load, the `description.Trigger` frontmatter field
is read by the assembler to determine when to include the skill.

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
sdd verify --force
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

Use `sdd verify` as a CI gate. It exits non-zero on failure and writes a machine-readable verdict:

```yaml
# .github/workflows/sdd-verify.yml
- name: Install sdd
  run: go install github.com/your-org/sdd@latest

- name: Run SDD verification
  run: sdd verify --change ${{ env.CHANGE_NAME }} --force
```

`--force` bypasses TTL caching so CI always runs a full verification.

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

### Phase ordering in tasks

Phases are determined dynamically by bottom-up analysis — the agent creates as many or as few phases as the change requires. Phase 1 always contains the lowest-level work (types, schemas, config); each subsequent phase builds on the previous. No task may reference a file created in a later phase.

You can hint at domain-specific ordering via config:

```yaml
phases:
  tasks:
    ordering_hint: "database schemas before types, API before frontend"
```

### Skipping phases

For trivial changes, you can skip phases that add overhead:

```
# Skip explore for small, well-understood changes:
/sdd:new fix-typo --no-explore

# Skip clean for urgent patches:
/sdd:apply fix-session-ttl
sdd verify --change fix-session-ttl
/sdd:archive fix-session-ttl  # Skip clean
```

The orchestrator respects phase skips — missing artifacts mean that phase was intentionally omitted.

---

## Multi-Language Support

SDD is language-agnostic. All phase SKILLs use `{CMD_CHECK}`, `{CMD_LINT}`, `{CMD_TEST}` variables resolved from the `commands` block in `config.yaml`. The Go assembler substitutes these before sending context to the LLM. Here are example configurations for different stacks:

### Go project

```yaml
stack:
  runtime: go
  language: go

commands:
  build: go build ./...
  typecheck: go vet ./...
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

commands:
  build: python -m build
  typecheck: mypy .
  lint: ruff check .
  test: pytest
  format_check: black --check .

conventions:
  error_handling:
    pattern: throw       # Python uses exceptions
```

### Rust project

```yaml
stack:
  runtime: rust
  language: rust

commands:
  build: cargo build
  typecheck: cargo check
  lint: cargo clippy
  test: cargo test
  format_check: cargo fmt --check
```

---

## Phase Delta Tracking & Analytics

### How it works

The Go binary writes a `QualitySnapshot` to `quality-timeline.jsonl` after every sub-agent returns. This creates a per-change quality timeline that tracks how build health, issue counts, completeness, and scope evolve across the 11 phases.

### Viewing analytics

```
sdd health --change add-csv-export
```

This reads the JSONL file and computes:
- **Build health progression** — Did typecheck/lint/tests improve or regress between phases?
- **Issue density by phase** — Which phases introduced the most critical issues?
- **Completeness curve** — How did task/spec coverage grow over the pipeline?
- **Scope summary** — Total files created, modified, reviewed
- **Phase timing** — Duration between consecutive snapshots
- **Regressions** — Any metric that worsened (flagged for attention)
- **Cache effectiveness** — skill_cache_hits vs misses per phase

### Using analytics for process improvement

Over multiple changes, analytics reveal patterns:
- If `sdd-apply` consistently introduces type errors that `sdd verify` catches, the design phase may need stronger interface definitions
- If review REJECT violations appear frequently, the AGENTS.md rules may need to be surfaced earlier (e.g., in the design SKILL.md)
- If completeness plateaus below 100%, task granularity may need adjustment
- If skill_cache_misses are high, skills are being edited frequently — consider stabilizing them

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

7. sdd verify --change feature-name
   → typecheck, lint, tests pass (Go — zero tokens)

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

When tests fail, the verify agent must diagnose each failure structurally (only invoked when `sdd verify --diagnose` is used):

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

The build-fix loop in sdd-apply (Step 4) includes an intelligent early-stop mechanism that leverages Engram persistent memory.

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

The review phase (sdd-review) generates a change-specific evaluation rubric before reviewing any code. This anchors the review to the actual requirements rather than generic best practices.

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

Version 1.1 adds formal pre/post-conditions for every SDD phase, inspired by the PARCER governance framework. These are defined in `openspec/config.yaml` and validated by the Go orchestrator (not the LLM).

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

### Go Orchestrator Validation

Before dispatching any sub-agent, the Go orchestrator validates preconditions:

1. Load `contracts.{phase}` from config.yaml
2. Check each precondition (file exists, field non-empty, prior phase completed)
3. **If any fails** → block launch, report unmet preconditions, suggest which phase to run first
4. **After sub-agent returns** → check postconditions, log warnings to quality timeline if failed

Postcondition failures do not block the next phase — they are recorded as warnings in `quality-timeline.jsonl` for analytics.

### Legacy compatibility

If `contracts` section does not exist in config.yaml (projects initialized before v1.1), contract validation is skipped entirely. Run `sdd init --force` to regenerate config.yaml with contracts.

---

## Navigation

- [← 07-configuration.md](./07-configuration.md) — Configuration reference
- [→ 09-troubleshooting.md](./09-troubleshooting.md) — Troubleshooting guide
- [↑ README.md](../README.md) — Back to start
