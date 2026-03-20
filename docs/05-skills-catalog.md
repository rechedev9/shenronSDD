---
summary: "How framework skills work, how to create new ones, skill anatomy."
read_when:
  - "Creating a new SKILL.md"
  - "Understanding how skills are loaded"
---

# Skills Catalog

Complete catalog of all skills in the SDD Workflow ecosystem.

Skills are `.md` files in `~/.claude/skills/`. Each skill contains either:
- A **SKILL.md** with structured instructions for a sub-agent
- A learned pattern from past sessions

## How Skills Are Now Loaded

**Prior model**: skills were loaded on demand by sub-agents during LLM execution, consuming tokens to read and interpret them at runtime.

**Current model**: the `sdd` Go binary assembles context before any LLM call. `sdd context` reads SKILL.md files from disk, resolves which ones apply (by phase, by detected stack), and injects the assembled text into the prompt payload. The LLM never "chooses" which skills to load — Go makes that decision deterministically, zero tokens.

```
sdd context --phase apply --change add-csv-export
# → reads config.yaml
# → resolves skills: [sdd-apply, typescript, react-19]
# → content-hash checks each SKILL.md (skip if unchanged since last run)
# → assembles final context blob
# → passes to Claude API
```

Skills are still the same SKILL.md format. What changed is who reads them and when: Go code, before the LLM call, not the LLM itself.

---

## SDD Phase Skills

11 skills that implement the SDD pipeline phases. Each is a complete sub-agent specification.

**Location:** `~/.claude/skills/sdd/{skill-name}/SKILL.md`
**Total lines:** ~3,835
**Loaded by:** `sdd context --phase <phase>` (Go assembler)

| Skill | Phase | Description | Lines |
|---|---|---|---|
| `sdd-init` | init | Bootstrap SDD: auto-detect stack, create openspec/, generate config.yaml and operational contracts | ~200 |
| `sdd-explore` | explore | Read-only codebase investigation with risk assessment and structured exploration protocol (hypothesis-driven file analysis) | ~360 |
| `sdd-propose` | propose | Write structured change proposal (WHAT + WHY) | ~294 |
| `sdd-spec` | spec | Write delta specs with RFC 2119 + Given/When/Then | ~332 |
| `sdd-design` | design | Write technical design with architecture decisions + interfaces | ~386 |
| `sdd-tasks` | tasks | Break design into phased, numbered implementation checklist | ~464 |
| `sdd-apply` | apply | Implement code in batches with build-fix loop, structured reading protocol, test generation governance, and EET | ~280 |
| `sdd-review` | review | Semantic code review with dynamic agentic rubric, function tracing, data flow analysis, and counter-hypothesis check | ~310 |
| `sdd-verify` | verify | Technical quality gate — in CLI mode, `sdd verify` runs build/lint/test directly in Go with progress logging (zero tokens) | ~310 |
| `sdd-clean` | clean | Dead code removal, duplicate elimination, simplification | ~285 |
| `sdd-archive` | archive | Merge delta specs, archive change, capture learnings | ~305 |

Each SKILL.md contains:
- Activation conditions
- Input envelope schema (what the assembler passes in)
- Step-by-step execution instructions
- Output envelope schema (what the sub-agent returns)
- Hard constraints and rules
- Error handling
- Example usage with input/output pair

### `sdd verify` — Go-native quality gate

The `sdd-verify` SKILL.md still defines the LLM-side review protocol. But the actual build/lint/test execution is handled by the Go binary, not the LLM:

```
sdd verify --change add-csv-export
# runs: go build ./... (or bun run typecheck, etc. from config.yaml)
# runs: golangci-lint run (or bun run lint)
# runs: go test ./... (or bun test)
# streams progress line-by-line
# writes verify-report.md with structured verdict
# zero LLM tokens consumed for the execution step
```

The LLM is only invoked if `sdd verify` detects failures and the `--diagnose` flag is set. Pure green runs are entirely deterministic Go.

---

## Framework Skills

14 skills with version-specific patterns for popular frameworks. Loaded by the Go assembler when the detected stack matches.

**Location:** `~/.claude/skills/frameworks/{framework}/SKILL.md`
**Total lines:** 3,029
**Loaded by:** `sdd context` — stack field in `config.yaml` drives selection (zero tokens)

### Frontend

#### `react-19`
**Trigger:** `stack.frameworks.frontend: react` in config.yaml
**Key patterns:**
- React Compiler is active — no manual `useMemo`/`useCallback` needed
- `use()` hook for promises and context
- Server Components default (Client Components are opt-in with `'use client'`)
- `useActionState` replaces `useFormState`
- No more `forwardRef` — refs passed directly as props
- Error boundaries: two-tier strategy (root boundary + feature boundaries)

#### `tailwind-4`
**Trigger:** `stack.css_framework: tailwind` in config.yaml
**Key patterns:**
- CSS-first configuration with `@theme` — no `tailwind.config.js`
- `@import "tailwindcss"` replaces `@tailwind base/components/utilities`
- CSS variables for theme tokens: `--color-primary: oklch(...)`
- `@layer` for custom utilities
- Lightning CSS processor (not PostCSS by default)

#### `zustand-5`
**Trigger:** `stack.state_management: zustand` in config.yaml
**Key patterns:**
- Slices pattern for large stores
- `immer` middleware for complex state mutations
- `subscribeWithSelector` for fine-grained subscriptions
- `devtools` middleware configuration
- TypeScript: explicit store type with `StateCreator<T>`

### Meta-Framework

#### `nextjs-15`
**Trigger:** `stack.frameworks.meta_framework: nextjs` in config.yaml
**Key patterns:**
- App Router as default (not Pages Router)
- `async` Server Components for data fetching
- Server Actions with `'use server'`
- `metadata` export for SEO
- `loading.tsx` / `error.tsx` conventions
- `next/image` and `next/font` for optimization

### Type Safety & Validation

#### `typescript`
**Trigger:** `stack.language: typescript` in config.yaml
**Key patterns:**
- `unknown` + type guards instead of `any`
- `satisfies` operator for type inference preservation
- `Result<T, E>` pattern for fallible operations
- Branded types for domain IDs
- `as const` for literal types
- Discriminated unions for exhaustive matching

#### `zod-4`
**Trigger:** `zod` present in detected dependencies
**Key patterns:**
- `z.string()`, `z.number()`, `z.object()` base schemas
- `z.parse()` throws, `z.safeParse()` returns `{success, data, error}`
- `.brand()` for branded types
- `z.infer<typeof Schema>` for TypeScript types
- Schema composition with `.extend()`, `.merge()`, `.pick()`

### Testing

#### `playwright`
**Trigger:** `playwright` present in detected dependencies
**Key patterns:**
- `page.getByRole()` for accessible locators (preferred over CSS)
- `page.getByText()`, `page.getByLabel()` for semantic queries
- Web-first assertions: `await expect(locator).toBeVisible()`
- Fixtures for shared test state
- `test.describe()` blocks for organization
- `page.waitForURL()` for navigation assertions

### AI Integration

#### `ai-sdk-5`
**Trigger:** `@ai-sdk/` packages present in detected dependencies
**Key patterns:**
- `streamText()` for streaming text responses
- `generateObject()` for structured outputs with Zod schemas
- `tool()` for defining AI tools
- `useChat()` hook for React streaming UI
- Provider configuration pattern
- Error handling for rate limits and timeouts

### Backend

#### `django-drf`
**Trigger:** `stack.frameworks.backend: django` in config.yaml
**Key patterns:**
- `ModelSerializer` for CRUD operations
- `ViewSet` + `Router` for standard REST endpoints
- `Permission` classes for authorization
- `IsAuthenticated`, `IsAdminUser`, custom permissions
- Pagination classes
- Filtering with `django-filter`

### DevOps & Tooling

#### `github-pr`
**Trigger:** Loaded for `sdd-archive` phase when `git.remote: github` is detected
**Key patterns:**
- `gh pr create` with `--title` and `--body` (HEREDOC for multiline)
- PR template sections: Summary, Test Plan, Breaking Changes
- `gh pr view`, `gh pr checks`, `gh pr merge`
- Branch protection checks before push
- Draft PR workflow

### Testing (Python)

#### `pytest`
**Trigger:** `stack.frameworks.testing: pytest` in config.yaml
**Key patterns:**
- `@pytest.fixture` for shared test state
- `@pytest.mark.parametrize` for data-driven tests
- `@pytest.mark.asyncio` for async tests
- `conftest.py` for shared fixtures
- `mock.patch` and `pytest-mock` for mocking
- `pytest.raises()` for exception testing

### Project Management

#### `jira-epic`
**Trigger:** `integrations.jira` configured in config.yaml
**Key patterns:**
- Maps proposal sections to Epic fields
- Story points estimation from change size
- Acceptance criteria from proposal success criteria
- Sprint and release assignment

#### `jira-task`
**Trigger:** `integrations.jira` configured in config.yaml
**Key patterns:**
- Maps tasks.md items to Jira subtasks
- Phase grouping into Jira stories
- Links to parent Epic
- Assignee and sprint from project config

### Meta

#### `skill-creator`
**Trigger:** Explicitly loaded when creating new SKILL.md files
**Key patterns:**
- SKILL.md frontmatter format (name, description, license, metadata)
- Activation conditions section
- Input/output envelope schemas
- Execution steps with numbered sub-steps
- Rules and constraints section
- Error handling section
- Example usage with realistic input/output

---

## Analysis Skills

Skills used internally by SDD sub-agents for specialized analysis tasks. The Go assembler includes these automatically when the parent phase skill is loaded.

**Location:** `~/.claude/skills/analysis/{skill-name}/SKILL.md`
**Total lines:** ~500

| Skill | Used by | Description |
|---|---|---|
| `architect` | sdd-design | Architecture blueprint generation |
| `build-validator` | sdd-verify | Build health verification |
| `code-simplifier` | sdd-clean | Code simplification patterns |
| `verify-app` | sdd verify | Application health checks |

---

## Knowledge Skills

Reference skills that document project conventions. Not sub-agent instructions — pure reference. Loaded by the Go assembler when relevant phase + stack combination is detected.

**Location:** `~/.claude/skills/knowledge/{skill-name}/SKILL.md`
**Total lines:** ~600

| Skill | Description |
|---|---|
| `type-strictness` | TypeScript strictness rules: banned patterns, allowed patterns, test exceptions |
| `error-handling` | Error handling conventions: Result<T,E>, catch narrowing, error boundaries |
| `testing-patterns` | Testing conventions: bun:test, describe/it, AAA pattern, DI over mocking |

These are also loadable via `/type-strictness`, `/error-handling`, `/testing-patterns` slash commands for manual consultation.

---

## Workflow Skills

Skills that implement utility commands.

**Location:** `~/.claude/skills/workflows/{skill-name}/SKILL.md`
**Total lines:** ~517

| Skill | Command | Description |
|---|---|---|
| `build-fix` | `/build-fix` | Diagnose and fix build errors with retry loop |
| `code-review` | `/code-review` | Comprehensive code review with security audit |
| `commit-push-pr` | `/commit-push-pr` | Commit, push, and open PR workflow |
| `tdd` | Internal | Test-Driven Development with Red-Green-Refactor |

---

## Learned Skills

Skills extracted from past sessions using `/learn`. These grow over time as patterns are discovered.

**Location:** `~/.claude/skills/learned/`

| Skill | Context |
|---|---|
| `error-boundary-architecture` | Two-tier React error boundary strategy |
| `lefthook-format-recovery` | Recovering from lefthook pre-commit format failures |
| `replacing-type-assertions` | Patterns for removing unsafe `as Type` assertions |
| `variant-prop-component-unification` | Unifying components with variant props |

**Creating new learned skills:** Run `/learn` after solving a non-trivial problem. The skill is saved to `~/.claude/skills/learned/` and picked up by future sessions.

**Promoting to framework skills:** Run `/evolve` to cluster related learned patterns into a new SKILL.md under the appropriate category.

---

## Skills at a Glance

| Category | Count | Lines | Location | Loaded by |
|---|---|---|---|---|
| SDD phase skills | 11 | ~3,835 | `~/.claude/skills/sdd/` | `sdd context --phase <name>` |
| Framework skills | 14 | 3,029 | `~/.claude/skills/frameworks/` | `sdd context` (stack-driven) |
| Analysis skills | 4 | ~500 | `~/.claude/skills/analysis/` | auto-included with parent phase |
| Knowledge skills | 3 | ~600 | `~/.claude/skills/knowledge/` | `sdd context` or slash command |
| Workflow skills | 4 | ~517 | `~/.claude/skills/workflows/` | slash command |
| Learned skills | 4+ | grows | `~/.claude/skills/learned/` | `sdd context` (opt-in) |
| **Total** | **40+** | **~8,481+** | | |

---

## Skill File Format

All SKILL.md files use YAML frontmatter. The format is unchanged from the pre-CLI era — the Go assembler reads the same structure.

```yaml
---
name: sdd-apply
description: >
  Implement code following specs and design. Works in batches.
  Trigger: When user runs /sdd:apply or after sdd-tasks completes.
license: MIT
metadata:
  version: "1.0"
---

# Skill Name

## Activation
[When this skill activates]

## Input Envelope
[What the assembler passes in — YAML schema]

## Execution Steps
[Step-by-step instructions]

## Rules and Constraints
[Hard limits the sub-agent must follow]

## Error Handling
[How to handle failure cases]

## Example Usage
[Realistic input/output example]
```

The `description.Trigger` field is now also read by the Go assembler to decide automatic inclusion — not just documentation.

---

## Navigation

- [← 04-commands-reference.md](./04-commands-reference.md) — All slash commands
- [→ 06-comparisons.md](./06-comparisons.md) — Standard vs SDD workflows
