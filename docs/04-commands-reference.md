# Commands Reference

All slash commands available in the SDD Workflow ecosystem.

Commands are stored as `.md` files in `~/.claude/commands/`. They are invoked in Claude Code using the `/command-name` syntax.

---

## SDD Pipeline Commands

These commands drive the SDD pipeline. They follow the phase order: init → explore → new → continue → apply → review → verify → clean → archive.

### `/sdd:init`

Bootstrap Spec-Driven Development for a project.

```
/sdd:init
/sdd:init --force       # Re-initialize, overwriting existing config
/sdd:init --dry-run     # Preview what would be created without writing
```

**What it does:**
- Reads `package.json`, `bun.lockb`, `tsconfig.json`, `CLAUDE.md`, `AGENTS.md`, `docker-compose.yml`
- Detects runtime (Bun/Node/Deno/Go/Python/Rust), frameworks, database, ORM, test runner
- Extracts conventions from `CLAUDE.md` (type strictness, error handling, testing patterns)
- Creates `openspec/` directory structure with `config.yaml`
- Generates `AGENTS.md` at project root if it doesn't exist (SDD context + code review rules extracted from `CLAUDE.md`)
- Generates `contracts` section in `config.yaml` with per-phase pre/post-conditions (PARCER operational contracts)

**Output:**
```
openspec/
  config.yaml
  specs/.gitkeep
  changes/.gitkeep
  changes/archive/.gitkeep
```

**Run when:** Starting SDD on a new project for the first time.

**Already initialized?** Reports current state without overwriting (unless `--force`).

---

### `/sdd:explore <topic>`

Investigate a codebase area before proposing changes.

```
/sdd:explore "how does authentication work?"
/sdd:explore "payment flow — preparing to add Stripe"
/sdd:explore "why is the dashboard slow?" --detail deep
```

**Arguments:**
- `<topic>` — Question or area to investigate (required)
- `--detail concise|standard|deep` — Analysis depth (default: standard)
- `--focus <paths>` — Directories/files to prioritize

**Detail levels:**
| Level | Output size | When to use |
|---|---|---|
| concise | 30–50 lines | Quick sanity check |
| standard | 80–150 lines | Normal investigation |
| deep | 150–300 lines | Complex refactors, security audits |

**Output:** `openspec/changes/{name}/exploration.md` with:
- Current state summary
- Relevant files table (path, purpose, complexity, test coverage)
- Dependency map
- 8-dimension risk assessment
- Approach comparison table
- Recommendation + open questions

**Run when:** Before starting any non-trivial change, or when you need to understand a codebase area.

---

### `/sdd:new <name> [description]`

Start a new SDD change (exploration + proposal in sequence).

```
/sdd:new add-csv-export
/sdd:new add-csv-export Export workout data as CSV files
/sdd:new fix-session-ttl Fix JWT/Redis TTL mismatch causing random logouts
```

**Arguments:**
- `<name>` — Change identifier in kebab-case (required)
- `[description]` — Intent description (prompted if omitted)

**What it does:**
1. Validates `openspec/` exists (suggests `/sdd:init` if not)
2. Runs `sdd-explore` sub-agent → creates `exploration.md`
3. Shows exploration summary → asks for approval
4. Runs `sdd-propose` sub-agent → creates `proposal.md`
5. Shows proposal summary → asks for approval

**Output:**
- `openspec/changes/{name}/exploration.md`
- `openspec/changes/{name}/proposal.md`

**Next step:** `/sdd:continue {name}` to generate specs + design.

---

### `/sdd:continue [name]`

Run the next dependency-ready phase for a change.

```
/sdd:continue
/sdd:continue add-csv-export
```

**Arguments:**
- `[name]` — Change name (auto-detected from `openspec/changes/` if omitted)

**Phase detection logic:**

| Artifacts present | Next phase |
|---|---|
| (none) | Suggests `/sdd:new` |
| `exploration.md` only | propose |
| `proposal.md` | spec + design (parallel) |
| `specs/` + `design.md` | tasks |
| `tasks.md` with unchecked items | apply |
| `tasks.md` all checked | review |
| `review-report.md` | verify |
| `verify-report.md` (PASS) | clean |
| `clean-report.md` | archive |

**Parallel execution:** `sdd-spec` and `sdd-design` are the only phases that run simultaneously. All others are sequential.

**Run when:** After approving any phase artifact and wanting to proceed to the next step.

---

### `/sdd:ff <name> [description]`

Fast-forward all planning phases without stopping for approvals.

```
/sdd:ff add-dark-mode
/sdd:ff add-dark-mode Add dark mode toggle to settings page
```

**What it does (sequentially):**
1. `sdd-explore` → `exploration.md`
2. `sdd-propose` → `proposal.md`
3. `sdd-spec` + `sdd-design` (parallel) → `specs/` + `design.md`
4. `sdd-tasks` → `tasks.md`

No approval prompts between phases.

**Final output:** Consolidated planning summary showing all phase results.

**When to use:** Experienced users who trust the planning pipeline, or when the change is well-understood and requirements are clear.

**Not suitable for:** Large changes (10+ files), changes with external dependencies that need discussion, changes where approach is uncertain.

---

### `/sdd:apply [name] [flags]`

Implement code following specs and design, one phase at a time.

```
/sdd:apply add-csv-export
/sdd:apply add-csv-export --phase 1
/sdd:apply add-csv-export --phase 2
/sdd:apply add-csv-export --tdd
/sdd:apply add-csv-export --fix-only
/sdd:apply add-csv-export --dry-run
```

**Flags:**
| Flag | Description |
|---|---|
| `--phase N` | Implement only tasks in phase N (1–5) |
| `--tdd` | Write failing test first, then implement (Red-Green-Refactor) |
| `--fix-only` | Run build-fix loop only, no new implementation |
| `--dry-run` | Show what would be implemented without writing code |

**What it does for each task:**
1. Reads spec scenarios (acceptance criteria)
2. Reads design constraints (interfaces, data flow)
3. Reads existing code (matches patterns before writing)
4. Implements the task
5. Marks task `[x]` in `tasks.md`

**Build-fix loop** (runs after each batch):
```
bun run typecheck  → fix type errors (max 5 attempts each)
bun run lint       → fix lint errors (auto-fix first, then manual)
bun test           → fix test failures in touched files
bun run format:check → auto-format affected files
```

**v1.1 Enhancements:**
- **Test Generation Governance**: Standard mode does not generate speculative tests. Tests are only created when explicitly required by the task or when using `--tdd` mode with underspecified specs.
- **Experience-Driven Early Termination (EET)**: Before fix attempt #3+, the build-fix loop queries Engram memory for matching error patterns. If the current error matches a known dead-end from prior sessions, the loop aborts early. Failure patterns are saved to Engram on escalation for future queries.

**Hard rules:** No `any`, no `as Type`, no `@ts-ignore`, no `!` assertions. If design contradicts spec, follows spec and notes deviation.

---

### `/sdd:review [name]`

Semantic code review comparing implementation against specs and project rules.

```
/sdd:review add-csv-export
```

**What it checks:**
- **Spec compliance:** Every Given/When/Then scenario covered?
- **Design compliance:** Module boundaries, interfaces, data flow correct?
- **AGENTS.md rules:** REJECT violations block verdict; REQUIRE violations are blocking; PREFER are advisory
- **Pattern compliance:** Naming, error handling, import style match codebase
- **Security scan:** OWASP Top 10 patterns (injection, XSS, auth bypass, hardcoded secrets)

**Severity levels:**
| Severity | Criteria | Blocks verdict? |
|---|---|---|
| CRITICAL | Spec not satisfied, REJECT violated, security vulnerability | Yes |
| WARNING | REQUIRE violated, missing edge case, poor error handling | Yes (if REQUIRE) |
| SUGGESTION | PREFER not followed, minor naming, style preference | No |

**v1.1 Enhancements:**
- **Dynamic Agentic Rubric**: Before reviewing, generates a change-specific rubric from specs + design + AGENTS.md. Each criterion is scored post-review. Verdict must be consistent with rubric scores.
- **Function Tracing Table**: Every exported function touched by the change gets a row with File:Line, parameter types, return type, and verified behavior.
- **Data Flow Analysis**: Critical data paths traced from creation through transformations to consumption, with invariants documented.
- **Counter-Hypothesis Check**: For each critical function, actively searches for evidence the implementation could fail. Minimum one counter-hypothesis per critical function.

**Output:** `openspec/changes/{name}/review-report.md` with PASSED/FAILED verdict.

**Note:** This agent never fixes issues — it reports only. Fixes go back to `/sdd:apply --fix-only`.

---

### `/sdd:verify [name]`

Technical quality gate — build health, static analysis, security, completeness.

```
/sdd:verify add-csv-export
```

**What it runs:**
```
bun run typecheck    # TypeScript compilation
bun run lint         # ESLint
bun run format:check # Prettier
bun test             # Full test suite
```

**v1.1 Enhancement — Fault Localization:**
When tests fail, the verify agent produces structured diagnosis:
- **PREMISES**: Step-by-step test semantics (test identifier, arrange, act, assert)
- **DIVERGENCE CLAIMS**: Formal cross-references between test expectations and source code locations where behavior diverges, with confidence levels (HIGH/MEDIUM/LOW)

**Static analysis scan:**
- Banned `any` usage (CRITICAL)
- Type assertions `as Type` in non-test files (CRITICAL)
- Compiler suppressions `@ts-ignore`, `@ts-expect-error` (CRITICAL)
- `console.log` in source (WARNING)
- `TODO`/`FIXME` markers (WARNING)

**Security scan:**
- Hardcoded secrets (API keys, passwords)
- SQL injection patterns
- XSS vectors (`innerHTML`)
- Missing input validation on API routes

**Completeness check:**
- Tasks: X/Y marked `[x]`
- Spec scenarios: X/Y with corresponding tests
- Design interfaces: X/Y implemented

**Three verdicts:**
| Verdict | Condition |
|---|---|
| PASS | All checks pass, no CRITICAL issues |
| PASS WITH WARNINGS | All checks pass but WARNING-level issues exist |
| FAIL | Any CRITICAL issue (type error, test failure, security vuln) |

**Output:** `openspec/changes/{name}/verify-report.md`

---

### `/sdd:clean [name]`

Dead code removal, duplicate elimination, simplification.

```
/sdd:clean add-csv-export
```

**Scope:** Files from the current change + their direct dependents only. Never refactors the whole project.

**What it removes (SAFE — no verification needed):**
- Unused imports
- Unused local variables
- Unreachable code
- Commented-out code blocks

**What it removes (CAREFUL — verifies after each):**
- Unused exported functions (searches for callers first)
- Dead branches
- Duplicate logic (Rule of Three: 3+ occurrences)

**Simplifications applied:**
- `x === null || x === undefined ? default : x` → `x ?? default`
- `arr.filter(...).length > 0` → `arr.some(...)`
- `if (cond) { return true; } else { return false; }` → `return cond;`

**Verification:** Runs `bun run typecheck` + `bun test` after every significant removal. Reverts if they fail.

**Will not touch:** Test fixtures, feature flags, polyfills, generated code, public APIs.

**Output:** `openspec/changes/{name}/clean-report.md`

---

### `/sdd:archive [name]`

Close a completed change — merge specs, archive artifacts, capture learnings.

```
/sdd:archive add-csv-export
```

**Safety check:** Aborts immediately if `verify-report.md` verdict is FAIL or has unresolved REJECT violations.

**What it does:**
1. Merges delta specs into `openspec/specs/`:
   - ADDED → appended with `<!-- Added: date from change: name -->`
   - MODIFIED → replaces old version, old version preserved as comment
   - REMOVED → commented out (never deleted)
2. Moves change folder: `openspec/changes/{name}/` → `openspec/changes/archive/{date}-{name}/`
3. Creates `archive-manifest.md` with change summary and key decisions
4. Captures learnings to `~/.claude/skills/learned/` (reusable patterns only)
5. Saves to Engram memory if available

**Output:**
- Updated `openspec/specs/*.spec.md`
- `openspec/changes/archive/{date}-{name}/` (permanent audit trail)
- Optional: `~/.claude/skills/learned/{pattern}.md`

---

### `/sdd:analytics [name]`

Analyze quality trends from phase delta tracking data.

```
/sdd:analytics add-csv-export
```

**What it does:**
- Reads `openspec/changes/{name}/quality-timeline.jsonl`
- Computes phase-over-phase deltas for build health, issue counts, completeness, and scope
- Produces a trend report with:
  - Build health progression (typecheck/lint/test pass rates over phases)
  - Issue density by phase (critical/warning counts)
  - Completeness curve (task and spec scenario coverage over time)
  - Phase timing estimates
- Highlights regressions (any metric that worsened between phases)

**Output:** Formatted report to stdout (not written to a file).

**Run when:** After completing a change to review quality trends, or mid-pipeline to check progress.

---

## Utility Commands

Standalone commands usable outside the SDD pipeline.

### `/verify [mode]`

Comprehensive project verification — all quality checks in one command.

```
/verify              # full mode (default)
/verify quick        # build + types only
/verify pre-commit   # full minus git status
/verify pre-pr       # full + security scan
/verify healthcheck  # environment diagnostics
/verify scan         # full autonomous audit with fix loop
```

| Mode | What it runs |
|---|---|
| quick | build, typecheck |
| full | build, typecheck, lint, tests, console.log scan, git status |
| pre-commit | same as full minus git status |
| pre-pr | full + secrets scan + dependency audit + static analysis |
| healthcheck | runtimes, git, project, hooks, processes |
| scan | healthcheck + fix loop (max 3) + code review |

---

### `/build-fix [mode] [scope]`

Diagnose and fix build errors automatically.

```
/build-fix           # fix everything
/build-fix types     # TypeScript errors only
/build-fix lint      # ESLint errors only
/build-fix all src/auth/  # scope to directory
```

**Fix pipeline:** Review → Fix (priority: types → lint → tests → static → format) → Verify. Max 3 loops, 5 attempts per unique error.

---

### `/commit-push-pr`

Commit staged changes, push to remote, and open a pull request.

```
/commit-push-pr
```

**What it does:**
1. Reviews git status and diffs
2. Runs `bun run typecheck && bun run lint && bun test`
3. Creates commit with message following repo style
4. Pushes to remote (refuses to push directly to `main`/`master`)
5. Creates PR with `gh pr create`

**Safety:** Checks for sensitive files (.env, credentials). Refuses force-push to main.

---

### `/code-review [files]`

Standalone code review with security audit.

```
/code-review
/code-review src/auth/
/code-review src/auth/login.ts src/auth/session.ts
```

Checks: type safety, immutability, file organization, error handling, code smells, async patterns, security (OWASP Top 10).

---

### `/learn`

Extract reusable patterns from the current session.

```
/learn
```

Reviews the session for extractable patterns: error resolution, debugging techniques, workarounds, project-specific discoveries. Asks for confirmation before saving to `~/.claude/skills/learned/`.

---

### `/evolve [flags]`

Cluster learned patterns into skills, commands, or agents.

```
/evolve              # preview clusters
/evolve --execute    # create evolved structures
/evolve --threshold 5  # require 5+ related patterns
```

Reads all `~/.claude/skills/learned/` files, groups by domain similarity, and generates new SKILL.md, command, or agent files for clusters of 3+ related patterns.

---

### `/instinct [action]`

Manage learned patterns.

```
/instinct            # show all (default: status)
/instinct status --domain testing
/instinct export
/instinct import instincts-export-20260222.yaml
```

| Action | Description |
|---|---|
| `status` | Show all learned patterns grouped by domain |
| `export` | Export patterns to shareable YAML file |
| `import <file>` | Import patterns from YAML/JSON/markdown |

---

## Command Files Location

All commands are `.md` files in `~/.claude/commands/`:

```
~/.claude/commands/
  sdd-init.md
  sdd-explore.md
  sdd-new.md
  sdd-continue.md
  sdd-ff.md
  sdd-apply.md
  sdd-review.md
  sdd-verify.md
  sdd-clean.md
  sdd-archive.md
  sdd-analytics.md
  verify.md
  build-fix.md
  commit-push-pr.md
  code-review.md
  learn.md
  evolve.md
  instinct.md
```

---

## Navigation

- [← 03-pillars.md](./03-pillars.md) — Architecture deep-dive
- [→ 05-skills-catalog.md](./05-skills-catalog.md) — Full skills catalog
