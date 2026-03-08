# Skills Catalog

Complete catalog of all skills in the SDD Workflow ecosystem.

Skills are `.md` files in `~/.claude/skills/`. Each skill contains either:
- A **SKILL.md** with structured instructions for a sub-agent
- A learned pattern from past sessions

Skills are loaded on demand — not all at startup.

---

## SDD Phase Skills

11 skills that implement the SDD pipeline phases. Each is a complete sub-agent specification.

**Location:** `~/.claude/skills/sdd/{skill-name}/SKILL.md`
**Total lines:** ~3,835

| Skill | Phase | Description | Lines |
|---|---|---|---|
| `sdd-init` | init | Bootstrap SDD: detect stack, create openspec/, generate config.yaml, AGENTS.md, and operational contracts | ~370 |
| `sdd-explore` | explore | Read-only codebase investigation with risk assessment and structured exploration protocol (hypothesis-driven file analysis) | ~360 |
| `sdd-propose` | propose | Write structured change proposal (WHAT + WHY) | ~294 |
| `sdd-spec` | spec | Write delta specs with RFC 2119 + Given/When/Then | ~332 |
| `sdd-design` | design | Write technical design with architecture decisions + interfaces | ~386 |
| `sdd-tasks` | tasks | Break design into phased, numbered implementation checklist | ~464 |
| `sdd-apply` | apply | Implement code in batches with build-fix loop, structured reading protocol, test generation governance, and EET | ~280 |
| `sdd-review` | review | Semantic code review with dynamic agentic rubric, function tracing, data flow analysis, and counter-hypothesis check | ~310 |
| `sdd-verify` | verify | Technical quality gate with fault localization protocol for test failure diagnosis | ~310 |
| `sdd-clean` | clean | Dead code removal, duplicate elimination, simplification | ~285 |
| `sdd-archive` | archive | Merge delta specs, archive change, capture learnings | ~305 |

Each SKILL.md contains:
- Activation conditions
- Input envelope schema (what the orchestrator passes in)
- Step-by-step execution instructions
- Output envelope schema (what the sub-agent returns)
- Hard constraints and rules
- Error handling
- Example usage with input/output pair

---

## Framework Skills

14 skills with version-specific patterns for popular frameworks. Lazy-loaded when working in the relevant domain.

**Location:** `~/.claude/skills/frameworks/{framework}/SKILL.md`
**Total lines:** 3,029

### Frontend

#### `react-19`
**Trigger:** Writing `.tsx` components, React hooks
**Key patterns:**
- React Compiler is active — no manual `useMemo`/`useCallback` needed
- `use()` hook for promises and context
- Server Components default (Client Components are opt-in with `'use client'`)
- `useActionState` replaces `useFormState`
- No more `forwardRef` — refs passed directly as props
- Error boundaries: two-tier strategy (root boundary + feature boundaries)

#### `tailwind-4`
**Trigger:** Styling with Tailwind CSS classes
**Key patterns:**
- CSS-first configuration with `@theme` — no `tailwind.config.js`
- `@import "tailwindcss"` replaces `@tailwind base/components/utilities`
- CSS variables for theme tokens: `--color-primary: oklch(...)`
- `@layer` for custom utilities
- Lightning CSS processor (not PostCSS by default)

#### `zustand-5`
**Trigger:** State management with Zustand
**Key patterns:**
- Slices pattern for large stores
- `immer` middleware for complex state mutations
- `subscribeWithSelector` for fine-grained subscriptions
- `devtools` middleware configuration
- TypeScript: explicit store type with `StateCreator<T>`

### Meta-Framework

#### `nextjs-15`
**Trigger:** Next.js App Router development
**Key patterns:**
- App Router as default (not Pages Router)
- `async` Server Components for data fetching
- Server Actions with `'use server'`
- `metadata` export for SEO
- `loading.tsx` / `error.tsx` conventions
- `next/image` and `next/font` for optimization

### Type Safety & Validation

#### `typescript`
**Trigger:** Writing strict TypeScript patterns
**Key patterns:**
- `unknown` + type guards instead of `any`
- `satisfies` operator for type inference preservation
- `Result<T, E>` pattern for fallible operations
- Branded types for domain IDs
- `as const` for literal types
- Discriminated unions for exhaustive matching

#### `zod-4`
**Trigger:** Schema validation, parsing external data
**Key patterns:**
- `z.string()`, `z.number()`, `z.object()` base schemas
- `z.parse()` throws, `z.safeParse()` returns `{success, data, error}`
- `.brand()` for branded types
- `z.infer<typeof Schema>` for TypeScript types
- Schema composition with `.extend()`, `.merge()`, `.pick()`

### Testing

#### `playwright`
**Trigger:** Writing E2E tests
**Key patterns:**
- `page.getByRole()` for accessible locators (preferred over CSS)
- `page.getByText()`, `page.getByLabel()` for semantic queries
- Web-first assertions: `await expect(locator).toBeVisible()`
- Fixtures for shared test state
- `test.describe()` blocks for organization
- `page.waitForURL()` for navigation assertions

### AI Integration

#### `ai-sdk-5`
**Trigger:** Vercel AI SDK integration
**Key patterns:**
- `streamText()` for streaming text responses
- `generateObject()` for structured outputs with Zod schemas
- `tool()` for defining AI tools
- `useChat()` hook for React streaming UI
- Provider configuration pattern
- Error handling for rate limits and timeouts

### Backend

#### `django-drf`
**Trigger:** Python REST APIs with Django REST Framework
**Key patterns:**
- `ModelSerializer` for CRUD operations
- `ViewSet` + `Router` for standard REST endpoints
- `Permission` classes for authorization
- `IsAuthenticated`, `IsAdminUser`, custom permissions
- Pagination classes
- Filtering with `django-filter`

### DevOps & Tooling

#### `github-pr`
**Trigger:** Creating pull requests via GitHub CLI
**Key patterns:**
- `gh pr create` with `--title` and `--body` (HEREDOC for multiline)
- PR template sections: Summary, Test Plan, Breaking Changes
- `gh pr view`, `gh pr checks`, `gh pr merge`
- Branch protection checks before push
- Draft PR workflow

### Testing (Python)

#### `pytest`
**Trigger:** Python testing with pytest
**Key patterns:**
- `@pytest.fixture` for shared test state
- `@pytest.mark.parametrize` for data-driven tests
- `@pytest.mark.asyncio` for async tests
- `conftest.py` for shared fixtures
- `mock.patch` and `pytest-mock` for mocking
- `pytest.raises()` for exception testing

### Project Management

#### `jira-epic`
**Trigger:** Creating Jira Epics from SDD proposals
**Key patterns:**
- Maps proposal sections to Epic fields
- Story points estimation from change size
- Acceptance criteria from proposal success criteria
- Sprint and release assignment

#### `jira-task`
**Trigger:** Creating Jira Tasks from SDD task lists
**Key patterns:**
- Maps tasks.md items to Jira subtasks
- Phase grouping into Jira stories
- Links to parent Epic
- Assignee and sprint from project config

### Meta

#### `skill-creator`
**Trigger:** Creating new SKILL.md files from patterns
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

Skills used internally by SDD sub-agents for specialized analysis tasks.

**Location:** `~/.claude/skills/analysis/{skill-name}/SKILL.md`
**Total lines:** ~500

| Skill | Used by | Description |
|---|---|---|
| `architect` | sdd-design | Architecture blueprint generation |
| `build-validator` | sdd-verify | Build health verification |
| `code-simplifier` | sdd-clean | Code simplification patterns |
| `verify-app` | /verify | Application health checks |

---

## Knowledge Skills

Reference skills that document project conventions. Not sub-agent instructions — pure reference.

**Location:** `~/.claude/skills/knowledge/{skill-name}/SKILL.md`
**Total lines:** ~600

| Skill | Description |
|---|---|
| `type-strictness` | TypeScript strictness rules: banned patterns, allowed patterns, test exceptions |
| `error-handling` | Error handling conventions: Result<T,E>, catch narrowing, error boundaries |
| `testing-patterns` | Testing conventions: bun:test, describe/it, AAA pattern, DI over mocking |

These are loaded via `/type-strictness`, `/error-handling`, `/testing-patterns` commands.

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

| Category | Count | Lines | Location |
|---|---|---|---|
| SDD phase skills | 11 | ~3,835 | `~/.claude/skills/sdd/` |
| Framework skills | 14 | 3,029 | `~/.claude/skills/frameworks/` |
| Analysis skills | 4 | ~500 | `~/.claude/skills/analysis/` |
| Knowledge skills | 3 | ~600 | `~/.claude/skills/knowledge/` |
| Workflow skills | 4 | ~517 | `~/.claude/skills/workflows/` |
| Learned skills | 4+ | grows | `~/.claude/skills/learned/` |
| **Total** | **40+** | **~8,481+** | |

---

## Skill File Format

All SKILL.md files use YAML frontmatter:

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
[What the orchestrator passes in — YAML schema]

## Execution Steps
[Step-by-step instructions]

## Rules and Constraints
[Hard limits the sub-agent must follow]

## Error Handling
[How to handle failure cases]

## Example Usage
[Realistic input/output example]
```

---

## Navigation

- [← 04-commands-reference.md](./04-commands-reference.md) — All slash commands
- [→ 06-comparisons.md](./06-comparisons.md) — Standard vs SDD workflows
