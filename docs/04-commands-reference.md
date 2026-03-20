---
summary: "Full reference for the sdd CLI binary and all slash commands, with options and usage examples."
read_when:
  - "Looking up a command or flag"
  - "Understanding how slash commands use the sdd binary under the hood"
  - "Creating a new slash command"
---

# Commands Reference

SDD exposes two interfaces to the same pipeline:

1. **`sdd` CLI** — The Go binary. Handles state, context assembly, verify, and archive. Invoked directly from the terminal or by slash commands internally.
2. **Slash commands** — Markdown files in `~/.claude/commands/` that Claude Code invokes with `/command-name`. They delegate deterministic work to `sdd` and Claude reasoning work to sub-agents.

Most users interact with the slash commands. Understanding `sdd` directly is useful when scripting, debugging, or inspecting pipeline state without Claude.

---

## The `sdd` CLI Binary

The `sdd` binary is the deterministic harness for the SDD pipeline. It handles all work that does not require reasoning, at zero token cost.

### Installation

```bash
# Install to PATH (typically /usr/local/bin or ~/go/bin)
go install github.com/your-org/sdd@latest

# Or build from source
git clone https://github.com/your-org/sdd
cd sdd && go build -o sdd . && mv sdd /usr/local/bin/
```

### `sdd init`

Bootstrap SDD for a project.

```bash
sdd init
sdd init --force       # Re-initialize, overwriting existing config
sdd init --dry-run     # Preview what would be created without writing
```

**What it does:**
- Scans manifest files, lockfiles, config files to auto-detect the tech stack
- Detects language, runtime, package manager, frameworks, database, ORM, test runner
- Extracts conventions from `CLAUDE.md` if present
- Creates `openspec/` directory structure with `config.yaml`
- Generates `contracts` section in `config.yaml` with per-phase PARCER operational pre/post-conditions

**Output:**
```
openspec/
  config.yaml
  specs/.gitkeep
  changes/.gitkeep
  changes/archive/.gitkeep
```

---

### `sdd status [name]`

Inspect pipeline state without invoking Claude.

```bash
sdd status                        # All active changes
sdd status add-csv-export         # Specific change detail
sdd status add-csv-export --json  # Machine-readable output
```

**Output (example):**
```
Change: add-csv-export
Phase:  apply (in progress)
  [x] exploration.md
  [x] proposal.md
  [x] specs/
  [x] design.md
  [x] tasks.md (12/19 complete)
  [ ] review-report.md
  [ ] verify-report.md
  [ ] clean-report.md
Next: continue apply or run /sdd:apply to resume
```

State is derived entirely from filesystem artifact presence — no database, no lockfile.

---

### `sdd context <name> <phase>`

Assemble and print the context that would be passed to a sub-agent for a given phase.

```bash
sdd context add-csv-export apply   # Show apply context (with Context Cascade)
sdd context add-csv-export review  # Show review context
sdd context add-csv-export apply --no-cache  # Force rebuild, ignore cache
```

**What this shows:**
- The exact artifact slice the sub-agent would receive
- The Context Cascade (accumulated decision history)
- Any framework skills that would be loaded for the current task
- Cache status (HIT / MISS, content-hash, TTL remaining)

Useful for debugging unexpected sub-agent behavior — you can see exactly what context a sub-agent received.

---

### `sdd verify <name>`

Run the full quality gate without invoking Claude.

```bash
sdd verify add-csv-export
sdd verify add-csv-export --check typecheck  # Single check only
sdd verify add-csv-export --check tests
sdd verify add-csv-export --json             # Machine-readable verdict
```

**What it runs (commands from config.yaml):**
```
typecheck    → zero errors required
lint         → zero violations required
format:check → zero formatting violations required
test         → all tests pass
```

**Static analysis (built-in Go regex scans):**
- Hardcoded secrets (common API key formats, passwords in strings)
- Banned constructs from config.yaml (e.g., `any` usage, `console.log`)
- Each violation reported with file path and line number

**Completeness check:**
- Tasks: X/Y marked `[x]`
- Spec scenarios: X/Y with corresponding tests
- Design interfaces: X/Y implemented

**Output:** Writes `openspec/changes/{name}/verify-report.md` and prints verdict to stdout.

**Exit codes:**
- `0` — PASS or PASS WITH WARNINGS
- `1` — FAIL (blocks downstream operations)

---

### `sdd archive <name>`

Archive a completed change — spec merge, file move, manifest write.

```bash
sdd archive add-csv-export
sdd archive add-csv-export --dry-run   # Preview what would happen
sdd archive add-csv-export --skip-learnings  # Skip optional learnings extraction
```

**Safety gates (checked before any file operation):**
- Reads `verify-report.md` verdict. Exits 1 if FAIL.
- Reads `review-report.md` for unresolved REJECT violations. Exits 1 if any remain.

**What it does (all Go, zero tokens):**
1. Merges delta specs into `openspec/specs/`:
   - ADDED → appended with `<!-- Added: date from change: name -->`
   - MODIFIED → replaces old version, old preserved as comment
   - REMOVED → commented out (never deleted)
2. Moves change folder: `openspec/changes/{name}/` → `openspec/changes/archive/{date}-{name}/`
3. Writes `archive-manifest.md` with change summary and key decisions
4. Saves to Engram memory if available (zero tokens — uses the MCP CLI interface)

Optional: `sdd archive` can invoke a Claude sub-agent for learnings extraction. This is the only archive sub-step that uses tokens.

---

### `sdd cascade <name>`

Print the Context Cascade for a change — the accumulated decision history that every sub-agent receives.

```bash
sdd cascade add-csv-export
sdd cascade add-csv-export --phase apply  # Phase-specific view
```

**What it shows:**
- Every architectural decision recorded in pipeline state for this change
- The format each sub-agent sees it in (prepended before their artifact slice)

---

### `sdd cache`

Manage the context assembly cache.

```bash
sdd cache status           # Cache hit rate, size, oldest entry
sdd cache clear            # Clear all cached context
sdd cache clear add-csv-export  # Clear cache for one change
```

Cache entries are keyed by content-hash of the input artifacts + a configurable TTL (default: 1 hour). When artifacts change (e.g., after apply updates tasks.md), the cache entry is automatically invalidated on next access.

---

### `sdd timeline <name>`

Print the quality-timeline.jsonl for a change in human-readable form.

```bash
sdd timeline add-csv-export
sdd timeline add-csv-export --phase apply  # Single phase
sdd timeline add-csv-export --json         # Raw JSONL passthrough
```

---

## SDD Pipeline Slash Commands

These commands drive the SDD pipeline. They follow the phase order: init → explore → new → continue → apply → review → verify → clean → archive.

Each slash command delegates to `sdd` for deterministic work and invokes Claude sub-agents for reasoning work. The split is noted for each command.

### `/sdd:init`

Bootstrap Spec-Driven Development for a project.

```
/sdd:init
/sdd:init --force       # Re-initialize, overwriting existing config
/sdd:init --dry-run     # Preview what would be created without writing
```

**Under the hood:** Calls `sdd init`. Pure Go — zero token cost.

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

**Under the hood:**
1. `sdd status` — checks openspec/ is initialized
2. `sdd context ... explore` — assembles context (config.yaml + topic-matched files + Context Cascade)
3. Launches Claude sub-agent (Sonnet) with assembled context
4. Sub-agent writes `exploration.md`

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

**Under the hood:**
1. `sdd init` check — validates `openspec/` exists (suggests `/sdd:init` if not)
2. `sdd context {name} explore` — assembles explore context
3. Claude sub-agent (Sonnet) → writes `exploration.md`
4. Shows exploration summary → asks for approval
5. `sdd context {name} propose` — assembles propose context (exploration.md + Context Cascade)
6. Claude sub-agent (Sonnet) → writes `proposal.md`
7. Shows proposal summary → asks for approval

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

**Under the hood:** `sdd status {name}` determines the next phase. The result drives which sub-agent (or Go operation) to invoke:

| Artifacts present | Next phase | Executor |
|---|---|---|
| (none) | Suggests `/sdd:new` | — |
| `exploration.md` only | propose | Claude (Sonnet) |
| `proposal.md` | spec + design (parallel) | Claude (Sonnet + Opus) |
| `specs/` + `design.md` | tasks | Claude (Sonnet) |
| `tasks.md` with unchecked items | apply | Claude (Opus) |
| `tasks.md` all checked | review | Claude (Sonnet) |
| `review-report.md` | verify | `sdd verify` (Go) |
| `verify-report.md` (PASS) | clean | Claude (Sonnet) |
| `clean-report.md` | archive | `sdd archive` (Go) |

**Parallel execution:** `sdd-spec` and `sdd-design` are the only Claude phases that run simultaneously. `sdd` assembles their respective context slices concurrently and the orchestrator launches both as Task tool calls in parallel.

**Run when:** After approving any phase artifact and wanting to proceed to the next step.

---

### `/sdd:ff <name> [description]`

Fast-forward all planning phases without stopping for approvals.

```
/sdd:ff add-dark-mode
/sdd:ff add-dark-mode Add dark mode toggle to settings page
```

**Under the hood (sequential):**
1. `sdd context ... explore` + Claude (Sonnet) → `exploration.md`
2. `sdd context ... propose` + Claude (Sonnet) → `proposal.md`
3. `sdd context ... spec` + `sdd context ... design` (parallel) + Claude (Sonnet + Opus) → `specs/` + `design.md`
4. `sdd context ... tasks` + Claude (Sonnet) → `tasks.md`

`sdd` manages state transitions and context assembly between each step. No approval prompts between phases.

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

**Under the hood:**
1. `sdd context {name} apply` — assembles context for the current task: tasks.md + design.md + spec artifacts + the specific file being modified + Context Cascade + relevant SKILL.md(s)
2. Claude sub-agent (Opus) reads assembled context and implements the task
3. Marks task `[x]` in `tasks.md`
4. `sdd verify --check typecheck,lint,tests` — runs build-fix loop (Go-native, zero tokens)
5. Repeats for next task in the same phase

**What it does for each task:**
1. Reads spec scenarios (acceptance criteria) — from assembled context
2. Reads design constraints (interfaces, data flow) — from assembled context
3. Reads existing code (matches patterns before writing) — file provided by context assembly
4. Implements the task
5. Marks task `[x]` in `tasks.md`

**Build-fix loop** (runs after each batch, executed by `sdd` in Go):
```
bun run typecheck  → fix type errors (max 5 attempts each)
bun run lint       → fix lint errors (auto-fix first, then manual)
bun test           → fix test failures in touched files
bun run format:check → auto-format affected files
```

**v1.1 Enhancements:**
- **Test Generation Governance**: Standard mode does not generate speculative tests. Tests are only created when explicitly required by the task or when using `--tdd` mode with underspecified specs.
- **Experience-Driven Early Termination (EET)**: Before fix attempt #3+, queries Engram memory for matching error patterns. If the current error matches a known dead-end from prior sessions, the loop aborts early. Failure patterns are saved to Engram on escalation for future queries.

**Hard rules:** No `any`, no `as Type`, no `@ts-ignore`, no `!` assertions. If design contradicts spec, follows spec and notes deviation.

---

### `/sdd:review [name]`

Semantic code review comparing implementation against specs and project rules.

```
/sdd:review add-csv-export
```

**Under the hood:**
1. `sdd context {name} review` — assembles context: changed source files + spec artifacts + design.md + AGENTS.md + Context Cascade. Does NOT include apply conversation history.
2. Claude sub-agent (Sonnet) evaluates code cold against the assembled context
3. Writes `review-report.md` with PASSED/FAILED verdict

**What it checks:**
- **Spec compliance:** Every Given/When/Then scenario covered?
- **Design compliance:** Module boundaries, interfaces, data flow correct?
- **Convention rules:** REJECT violations block verdict; REQUIRE violations are blocking; PREFER are advisory
- **Pattern compliance:** Naming, error handling, import style match codebase
- **Security scan:** OWASP Top 10 patterns (injection, XSS, auth bypass, hardcoded secrets)

**Severity levels:**
| Severity | Criteria | Blocks verdict? |
|---|---|---|
| CRITICAL | Spec not satisfied, REJECT violated, security vulnerability | Yes |
| WARNING | REQUIRE violated, missing edge case, poor error handling | Yes (if REQUIRE) |
| SUGGESTION | PREFER not followed, minor naming, style preference | No |

**v1.1 Enhancements:**
- **Dynamic Agentic Rubric**: Before reviewing, generates a change-specific rubric from specs + design + project conventions. Each criterion is scored post-review. Verdict must be consistent with rubric scores.
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

**Under the hood:** Calls `sdd verify {name}` directly. Pure Go — zero token cost.

**What it runs:**
```
bun run typecheck    # TypeScript compilation
bun run lint         # ESLint
bun run format:check # Prettier
bun test             # Full test suite
```

**v1.1 Enhancement — Fault Localization:**
When tests fail, `sdd` produces structured diagnosis:
- **PREMISES**: Step-by-step test semantics (test identifier, arrange, act, assert)
- **DIVERGENCE CLAIMS**: Formal cross-references between test expectations and source code locations where behavior diverges, with confidence levels (HIGH/MEDIUM/LOW)

For complex test failures requiring semantic interpretation, `sdd` may invoke a Claude sub-agent to produce the divergence claims. Command execution and report writing are always Go-native.

**Static analysis scan (Go regex):**
- Banned `any` usage (CRITICAL)
- Type assertions `as Type` in non-test files (CRITICAL)
- Compiler suppressions `@ts-ignore`, `@ts-expect-error` (CRITICAL)
- `console.log` in source (WARNING)
- `TODO`/`FIXME` markers (WARNING)

**Security scan (Go regex):**
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

**Under the hood:**
1. `sdd status` — confirms verify-report is PASS (Go gate; blocks clean if FAIL)
2. `sdd context {name} clean` — assembles context for clean sub-agent
3. Claude sub-agent (Sonnet) identifies and removes dead code
4. After each removal: `sdd verify --check typecheck,tests` (Go) confirms no regression

**Scope:** Files from the current change + their direct dependents only. Never refactors the whole project.

**What it removes (SAFE — no verification needed):**
- Unused imports
- Unused local variables
- Unreachable code
- Commented-out code blocks

**What it removes (CAREFUL — sdd verifies after each):**
- Unused exported functions (searches for callers first)
- Dead branches
- Duplicate logic (Rule of Three: 3+ occurrences)

**Simplifications applied:**
- `x === null || x === undefined ? default : x` → `x ?? default`
- `arr.filter(...).length > 0` → `arr.some(...)`
- `if (cond) { return true; } else { return false; }` → `return cond;`

**Will not touch:** Test fixtures, feature flags, polyfills, generated code, public APIs.

**Output:** `openspec/changes/{name}/clean-report.md`

---

### `/sdd:archive [name]`

Close a completed change — merge specs, archive artifacts, capture learnings.

```
/sdd:archive add-csv-export
```

**Under the hood:** Calls `sdd archive {name}`. Primarily Go — zero token cost for the core operations. Optional learnings extraction may invoke a Claude sub-agent.

**Safety check (Go gate):** Reads verify-report.md verdict. Exits immediately if FAIL or if unresolved REJECT violations remain in review-report.md. No files are touched until gates pass.

**What it does:**
1. Merges delta specs into `openspec/specs/` (Go):
   - ADDED → appended with `<!-- Added: date from change: name -->`
   - MODIFIED → replaces old version, old version preserved as comment
   - REMOVED → commented out (never deleted)
2. Moves change folder: `openspec/changes/{name}/` → `openspec/changes/archive/{date}-{name}/` (Go, atomic rename)
3. Creates `archive-manifest.md` with change summary and key decisions (Go)
4. Captures learnings to `~/.claude/skills/learned/` — optional, may use Claude sub-agent
5. Saves session summary to Engram memory if available

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

**Under the hood:** Calls `sdd timeline {name}` for the raw data, then invokes a Claude sub-agent (Sonnet) to produce the trend analysis narrative.

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

**Under the hood:** Calls `sdd verify` for the command execution and report parsing. Claude is invoked only if a `scan` mode fix-loop is requested.

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

## File Locations

### `sdd` binary

```
/usr/local/bin/sdd          # System install (typical)
~/go/bin/sdd                # go install destination
```

### Slash command files

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

### How slash commands use `sdd`

Each slash command `.md` file contains a SKILL.md-style instruction block that tells Claude Code how to execute it. The pattern is always the same:

```markdown
1. Run `sdd status {name}` to determine current state
2. Run `sdd context {name} {phase}` to assemble sub-agent context
3. Launch sub-agent via Task tool with the assembled context
4. Run `sdd verify` or `sdd archive` for deterministic post-processing
```

The slash command is the user-facing interface. The `sdd` binary is the implementation. Claude is the reasoning engine. None of the three knows more than it needs to.

---

## Navigation

- [← 03-pillars.md](./03-pillars.md) — Architecture deep-dive
- [→ 05-skills-catalog.md](./05-skills-catalog.md) — Full skills catalog
