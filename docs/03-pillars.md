# The Five Architecture Pillars

## Introduction

SDD rests on five architecture pillars. Each solves a specific failure mode of standard AI coding workflows. These are not abstract design principles — each pillar exists because a concrete, recurring problem was observed in AI-assisted development, and each pillar is implemented as a specific, inspectable mechanism.

The five pillars are:

1. **Agent Teams Lite** — Sub-agent isolation prevents context pollution
2. **Engram Persistent Memory** — SQLite-backed memory prevents session amnesia
3. **Agent Review Rules** — File-based review rules eliminate circular self-review
4. **Framework Skills** — On-demand skill files correct training data cutoffs
5. **Semi-Formal Reasoning** — Structured hypothesis-evidence-observation cycles prevent shallow analysis

Understanding these pillars explains why SDD is structured the way it is, and why the orchestrator protocol rules (never read source code, never write implementation, always delegate) are not arbitrary restrictions but load-bearing design decisions.

---

## Pillar 1: Agent Teams Lite

### The Problem

Monolithic AI sessions accumulate context that pollutes reasoning. When a single Claude session has read 50 source files, 3 config files, and the last hour of conversation history, its effective context window is largely consumed by information that is not relevant to the current decision.

This creates two failure modes:

**Noise-induced errors**: The model makes decisions influenced by code that is superficially similar to the code under discussion but actually unrelated. A function named `validateUser` in a billing module biases how the model thinks about a different `validateUser` in an auth module — even when the two are semantically distinct.

**Decision quality degradation**: Research consistently shows that AI models make better decisions with focused, relevant context than with large, noisy context. An agent that reads 5 relevant files for a specific task outperforms an agent that has read 200 files "for background."

### The Solution

The Claude Code Task tool creates fresh sub-processes — referred to as "sub-agents" — with clean context windows. The orchestrator tracks state and passes only the necessary artifact paths to each sub-agent. Each sub-agent reads only what it needs for its specific phase.

### How It Works

```
Orchestrator (Claude Code)
├── Knows: current phase, artifact paths, user approvals
├── Does NOT: read source code, write implementation, make decisions
└── Delegates via Task tool:
    ├── sdd-explore  → reads only the relevant code files for the topic
    ├── sdd-spec     → reads only proposal.md
    ├── sdd-design   → reads only proposal.md + optionally exploration.md
    ├── sdd-apply    → reads only tasks.md + design.md + the specific file being modified
    └── sdd-review   → reads only tasks.md + specs + design + AGENTS.md
```

**sdd-apply** never sees 200 unrelated files. When implementing `AuthService.login()`, it reads:
- The spec scenario for REQ-AUTH-001 (what "correct" means)
- The TypeScript interface for `AuthService` from design.md (what the signature must be)
- The current content of `src/services/auth.service.ts` (what pattern to match)

That is it. No routing logic, no UI components, no billing code, no migration files — nothing that would bias the implementation toward unrelated patterns.

### The Orchestrator Protocol Rules

These rules are not suggestions. They are the mechanism by which context isolation is maintained:

| Rule | Reason |
|------|--------|
| NEVER execute phase work inline | Inline work uses the orchestrator's context, defeating isolation |
| NEVER read source code directly | Source code in the orchestrator's context leaks into all subsequent decisions |
| NEVER write implementation code | Writing code requires reading the file, which loads it into context |
| NEVER write specs or proposals | Design decisions made inline are not sub-agent-isolated |
| ONLY track state, present summaries, ask for approval, launch sub-agents | This is the entire job of the orchestrator |
| Between sub-agent calls: show what was done, ask to proceed | Forces explicit checkpoints, prevents runaway execution |
| Pass file PATHS to sub-agents, not file contents | Paths are small; contents are large and context-polluting |

### Why It Matters in Practice

Consider a change that touches auth, billing, and user management — three separate domains. With a monolithic session:
- Context contains auth code + billing code + user management code simultaneously
- Auth decisions get influenced by billing patterns (or vice versa)
- The model's attention is divided across the full surface area

With sub-agents:
- sdd-spec writes auth requirements reading only the auth-relevant proposal
- sdd-design writes auth interfaces reading only auth exploration and auth spec
- sdd-apply implements billing reading only billing tasks and the billing file

Each sub-agent is an expert in exactly one thing at a time. The orchestrator coordinates, but never muddies the water.

---

## Pillar 2: Engram Persistent Memory

### The Problem

AI sessions have context limits. When a long session approaches its limit, Claude automatically compresses ("compacts") prior context. Decisions made early in a session — including their rationale — get summarized into shorter form. The rationale is usually what gets lost.

**Example**: In session 1, the team decides to use `Result<T, E>` instead of exceptions because a specific third-party library throws unpredictably and wrapping it in try/catch everywhere produced duplicated boilerplate. That constraint — the specific library's behavior — is the reason for the decision.

After compaction, the context might read: "Using Result<T,E> pattern for error handling." The constraint is gone. In session 2 or 3, when someone asks "why can't we just throw here?", the model has no context to answer correctly.

**Session amnesia** is the related problem: starting a new session means re-discovering everything. The previous session's discoveries about which files are tightly coupled, which functions have surprising side effects, and which assumptions were wrong — all of that is gone.

### The Solution

Engram is a SQLite + FTS5 (full-text search) database exposed as an MCP (Model Context Protocol) server. Decisions, patterns, bugs, and domain knowledge are saved as structured observations with topic keys. Every new session starts by loading relevant context from Engram via `mem_context`.

This is persistent, searchable, structured memory that survives session boundaries, context compaction, and process restarts.

### The Protocol

**SESSION START** — at the beginning of every new session, before any task work:
```
mem_context()
```
This loads observations relevant to the current project and topic, recovering decisions and patterns from prior sessions. The model does not start blind.

**PROACTIVE SAVE** — after any significant event, without waiting for the user to ask:

```typescript
// After making an architecture decision:
mem_save({
  topic_key: "decision/auth-error-handling",
  content: `
    Decision: Use Result<T,E> for all AuthService methods instead of throwing.
    Rationale: third-party library 'auth-vendor' throws unpredictably on network
    timeouts. Wrapping in try/catch at every call site produced 40+ duplicated
    error handling blocks. Centralizing with Result<T,E> reduced this to 1.
    Constraint: Do NOT switch to exceptions without addressing the auth-vendor
    timeout behavior first.
    Date: 2026-02-22
  `
})
```

Trigger `mem_save` IMMEDIATELY after ANY of:
- Decision made (architecture, convention, tool choice)
- Bug fixed — include the root cause, not just the fix
- Convention documented or updated
- Non-obvious discovery, gotcha, or edge case found
- Pattern established (naming, structure, approach)
- User preference or constraint learned
- Feature implemented with a non-obvious approach

**SESSION CLOSE** — before reporting "done", call:
```
mem_session_summary({
  goal: "Implement login endpoint per add-login-endpoint spec",
  discoveries: [
    "AuthService already has a findByEmail method — reused it",
    "bun test has a race condition in parallel test runs — use --no-parallel flag"
  ],
  accomplished: [
    "Created login.route.ts with Zod validation",
    "Added login() to AuthService with Result<T,E> pattern",
    "22 new tests, all passing"
  ],
  next_steps: [
    "sdd:review — review-report not yet generated",
    "Address timing-safe password comparison (flagged in review notes)"
  ],
  relevant_files: [
    "src/routes/auth/login.route.ts",
    "src/services/auth.service.ts",
    "openspec/changes/add-login-endpoint/tasks.md"
  ]
})
```

**COMPACTION RECOVERY** — when context compaction is detected:
1. Immediately call `mem_session_summary` with the compacted content to preserve it
2. Call `mem_context` to reload context from prior sessions
3. Then continue working

### Topic Key Families

Topic keys use a hierarchical namespace to enable targeted retrieval:

| Family | Examples | Content |
|--------|----------|---------|
| `architecture/*` | `architecture/auth-jwt-strategy`, `architecture/database-connection-pooling` | System design decisions with rationale |
| `bug/*` | `bug/bun-test-parallel-race`, `bug/drizzle-null-handling` | Root cause + fix + prevention |
| `decision/*` | `decision/result-pattern-choice`, `decision/monorepo-structure` | Trade-off decisions with constraints |
| `pattern/*` | `pattern/route-handler-structure`, `pattern/zod-schema-naming` | Reusable implementation patterns |
| `config/*` | `config/tsconfig-paths`, `config/bun-test-flags` | Configuration gotchas and explanations |
| `discovery/*` | `discovery/auth-vendor-timeout-behavior`, `discovery/next15-cache-defaults` | Non-obvious findings |
| `learning/*` | `learning/sdd-apply-batch-size`, `learning/spec-scenario-granularity` | Lessons learned from SDD sessions |

Use `mem_suggest_topic_key` before saving evolving topics to get the recommended key and avoid creating duplicate observations for the same topic. Same topic = same key = upsert, not duplicate.

### What Gets Saved vs. What Does Not

**Save**:
- Stable patterns confirmed across multiple interactions
- Key architectural decisions with their rationale and constraints
- Non-obvious discoveries and gotchas — especially things that wasted time
- Solutions to recurring problems
- User preferences and project-specific constraints

**Do not save**:
- Session-specific context (the current task name, today's date, task status)
- Incomplete information — save after the conclusion is reached, not during investigation
- Anything already documented in `CLAUDE.md` — Engram supplements the docs, does not duplicate them

### The Result

A project with 6 months of Engram memory behaves qualitatively differently from a fresh session. When starting work on a billing feature, `mem_context()` surfaces:
- The decision made 3 months ago to use Stripe Checkout (not Elements) and the specific reason
- The bug discovered last month where webhook idempotency keys were not being checked
- The pattern established for Result<T,E> in payment flows
- The gotcha about Stripe's webhook signature verification failing in local environments

All of that context loads before any code is read, before any tool is called, before any decision is made.

---

## Pillar 3: Agent Review Rules

### The Problem

"AI, review your own code" is circular. The reviewer shares biases with the author. If the author chose not to validate input because they forgot the requirement, the reviewer — sharing the same context — is likely to miss the same gap. If the author made a category error (e.g., "direct DB access is fine in routes, everyone does it"), the reviewer will apply the same mental model.

There is also the problem of undefined standards. "Review for best practices" produces subjective, inconsistent results. Different sessions may enforce different conventions. There is no stable, inspectable contract that defines what "correct" means.

### The Solution

`AGENTS.md` is a project-level file in the repository containing keyword-prefixed rules for AI code review. Rules are written once by the team, committed to the repo, and enforced automatically by `sdd-review` — which only reads specs and code that it did not write.

Three rule levels with explicit semantics:

```markdown
REJECT: Direct database access outside /src/data/ — bypass the repository layer
REQUIRE: All API routes MUST validate input with Zod before processing
PREFER: Use branded types for domain IDs (UserId, OrderId) over plain strings
```

### Rule Semantics

| Prefix | Verdict Impact | Enforcement |
|--------|---------------|-------------|
| `REJECT` | Any match → FAILED | Hard block. Merge cannot proceed. |
| `REQUIRE` | Missing → FAILED | Must be present and satisfied. |
| `PREFER` | Advisory | Noted in report. Does not block merge. |

`REJECT` rules are written for patterns the team has decided are categorically unacceptable. No amount of context justifies a violation. If you need an exception, the rule itself must be updated (via a PR), not bypassed.

`REQUIRE` rules state positive requirements — things that must be present. Missing input validation, missing error handling, missing test coverage for a specific code path.

`PREFER` rules capture team style preferences and upgrade paths. They inform without blocking, making it possible to gradually move a codebase toward better patterns.

### Example AGENTS.md File

```markdown
# AGENTS.md — AI Code Review Rules

## Security

REJECT: Direct SQL string concatenation in queries — use parameterized queries
REJECT: JWT secrets hardcoded in source files — use environment variables
REJECT: Passwords stored without hashing — use PasswordService.hash()
REQUIRE: All authentication middleware must be applied before business logic handlers
REQUIRE: User-owned resources must verify ownership before access (e.g., userId === resource.userId)

## Architecture

REJECT: Direct database access outside src/data/ — all DB access goes through repositories
REJECT: Business logic in route handlers — route handlers validate + delegate only
REQUIRE: All Result<T,E> errors must be handled at the route level (no unhandled Err propagation to the client)

## TypeScript

REJECT: Usage of `any` type in production code
REJECT: Type assertions (`as Type`) in production code
REJECT: Compiler suppressions (`@ts-ignore`, `@ts-expect-error`) in production code
REQUIRE: All catch clauses must use `unknown` type with explicit narrowing
REQUIRE: All public functions must have explicit return types

## Testing

REQUIRE: New public functions must have at least one unit test
REQUIRE: New API routes must have at least one integration test covering the happy path
PREFER: Use branded types (UserId, OrderId) for domain entity identifiers
PREFER: Error responses should include a machine-readable error code field

## Code Quality

REJECT: `console.log` in production code — use the structured logger
REJECT: Magic numbers and magic strings — define named constants
```

### Why File-Based Rules Work

| Property | Why It Matters |
|----------|---------------|
| **Versioned** | Rules are committed to the repo. Changes to rules are reviewed like code. |
| **Transparent** | No "the AI decided" ambiguity. Every rule citation in a review report has a corresponding line in AGENTS.md. |
| **Composable** | Add or remove rules without touching the review agent. |
| **Auditable** | Review reports cite specific rules and violations. Disputes are resolved by reading AGENTS.md. |
| **Stable** | The same rules apply consistently across all sessions, all reviewers, all time. |

### Integration Points

Rules do not exist in isolation. They are wired into the SDD pipeline at multiple points:

- **`sdd-init`**: Detects `AGENTS.md` and records its path in `config.yaml`. If absent, logs a warning recommending its creation.
- **`sdd-review`**: Loads `AGENTS.md` and checks every REJECT and REQUIRE rule against the implementation. Reports each violation with file path and line number.
- **`sdd-verify`**: Reads `review-report.md` and counts unresolved REJECT violations. Any REJECT violation that was not resolved between review and verify causes an automatic FAIL verdict.
- **`sdd-archive`**: Reads `review-report.md` for REJECT violations. Aborts archiving if any remain unresolved.

### Separation From the Author

The critical property of Agent Review Rules is that `sdd-review` did not write the code it reviews. It receives:
- The spec (what the code should do)
- The design (how the code was supposed to be structured)
- The AGENTS.md rules (what is categorically prohibited or required)
- The implemented source code (what was actually written)

It does not receive the conversation history, the task list progress, or any context about what the developer was thinking. It evaluates the code against the contract, cold.

This is a structural guarantee — not a prompt-level instruction — that the reviewer cannot be biased by the author's reasoning.

---

## Pillar 4: Framework Skills

### The Problem

AI models have training data cutoffs. A model trained primarily on React 18 data will suggest patterns that are wrong for React 19:

- Adds `useMemo` and `useCallback` everywhere (React 19's compiler handles this automatically)
- Uses `forwardRef` for ref passing (React 19 passes refs as regular props)
- Wraps context providers in `useMemo` (not needed with React 19's compiler)

A model trained on Tailwind 3 data will suggest patterns that break in Tailwind 4:

- Creates `tailwind.config.js` (Tailwind 4 uses CSS-first configuration)
- Uses `@apply` directives from the old plugin system
- Configures colors in `theme.extend` (now defined with `@theme` in CSS)

A model trained on Next.js 13/14 data will make errors in Next.js 15:

- Uses the `pages/` directory instead of `app/`
- Uses `getServerSideProps` instead of async server components
- Uses the old metadata approach instead of the `metadata` export API

These are not hallucinations — they are correct answers for the wrong version. The model cannot know which version the project uses unless it is told explicitly.

### The Solution

Framework-specific SKILL.md files live at `~/.claude/skills/frameworks/{framework}/SKILL.md`. Each file contains:
- Correct patterns for the current version (not the training data version)
- Common mistakes to avoid (the wrong-version patterns)
- Project-specific conventions where the project deviates from framework defaults
- Examples showing before (wrong) and after (correct)

Skills are loaded on demand — only when the agent is about to work in that domain. A session implementing a database repository does not load the React 19 skill. A session styling a component does not load the Drizzle skill.

### The 14 Framework Skills

| Skill | Domain | Key Focus |
|-------|--------|-----------|
| `react-19` | React components | React compiler (no manual `useMemo`), `use()` hook, ref as prop, Server Components |
| `tailwind-4` | Styling | CSS-first config (`@theme`), no `tailwind.config.js`, `@layer` syntax |
| `typescript` | Strict TypeScript | No `any`, no `as Type`, `Result<T,E>`, explicit return types, branded types |
| `zod-4` | Schema validation | `z.string()`, `z.parse()` vs `z.safeParse()`, schema composition, error shapes |
| `zustand-5` | State management | Slices pattern, immer integration, `subscribeWithSelector`, devtools |
| `playwright` | E2E testing | `page.getByRole()`, web-first assertions, fixtures, parallel test setup |
| `nextjs-15` | App Router | Server/Client Components boundary, server actions, metadata API |
| `ai-sdk-5` | Vercel AI integration | `streamText()`, `generateObject()`, tool use, streaming patterns |
| `github-pr` | Pull requests | `gh` CLI, PR templates, review workflow, branch naming |
| `django-drf` | Python REST APIs | Serializers, ViewSets, permissions, authentication backends |
| `pytest` | Python testing | Fixtures, `parametrize`, markers, async tests with `pytest-asyncio` |
| `jira-epic` | Project management | Epic creation, acceptance criteria, story points |
| `jira-task` | Project management | Task creation from SDD proposals, linking to epics |
| `skill-creator` | Meta | Creating new SKILL.md files from observed patterns |

### Loading Trigger Configuration

Skills are loaded based on the domain being worked in, as configured in `CLAUDE.md`:

```markdown
| Domain     | Trigger                                  | Skill Path                                    |
|------------|------------------------------------------|-----------------------------------------------|
| React 19   | Writing .tsx components, React hooks     | ~/.claude/skills/frameworks/react-19/SKILL.md |
| Tailwind 4 | Styling with Tailwind classes            | ~/.claude/skills/frameworks/tailwind-4/SKILL.md |
| TypeScript | Writing strict TypeScript patterns       | ~/.claude/skills/frameworks/typescript/SKILL.md |
| Zod 4      | Schema validation, parsing               | ~/.claude/skills/frameworks/zod-4/SKILL.md |
| Zustand 5  | State management                         | ~/.claude/skills/frameworks/zustand-5/SKILL.md |
| Next.js 15 | App Router, Server Components            | ~/.claude/skills/frameworks/nextjs-15/SKILL.md |
```

When `sdd-apply` is about to implement a file that ends in `.tsx`, it first reads the react-19 SKILL.md. When it is about to implement a Zod schema, it reads the zod-4 SKILL.md. The skill file is consumed before any code is written.

### Example: What a Skill File Corrects

A fragment from the hypothetical `react-19/SKILL.md`:

```markdown
## React 19 — Critical Pattern Corrections

### useMemo and useCallback — DO NOT ADD

React 19 includes an automatic compiler that handles memoization. Manually adding
useMemo and useCallback creates unnecessary complexity and can interfere with the
compiler's optimizations.

WRONG (React 18 pattern):
```tsx
const memoizedValue = useMemo(() => expensiveComputation(a, b), [a, b]);
const stableCallback = useCallback(() => doSomething(x), [x]);
```

CORRECT (React 19):
```tsx
const value = expensiveComputation(a, b);   // compiler handles it
const handleClick = () => doSomething(x);   // compiler handles it
```

### Ref Forwarding — forwardRef Is Removed

React 19 passes refs as regular props. forwardRef is deprecated and removed.

WRONG (React 18 pattern):
```tsx
const MyInput = forwardRef<HTMLInputElement, InputProps>((props, ref) => (
  <input ref={ref} {...props} />
));
```

CORRECT (React 19):
```tsx
function MyInput({ ref, ...props }: InputProps & { ref?: React.Ref<HTMLInputElement> }) {
  return <input ref={ref} {...props} />;
}
```

### The use() Hook

React 19 introduces use() for reading resources (Promises, Context) inside render:

```tsx
// Reading a promise inside a component (requires Suspense boundary above)
function UserProfile({ userPromise }: { userPromise: Promise<User> }) {
  const user = use(userPromise);  // suspends until resolved
  return <div>{user.name}</div>;
}
```
```

### Self-Improving Protocol

Skills follow a feedback loop that makes them more complete over time:

1. **Before writing code**, read the relevant SKILL.md — it is the primary source of truth for that framework
2. **During implementation**, prefer SKILL.md over internet search. If the SKILL.md covers the topic, do not search the internet
3. **If the SKILL.md doesn't answer the question**, search the internet — then update the SKILL.md with the finding. Internet search during implementation signals an incomplete spec
4. **After implementation**, if new gotchas or patterns were discovered, append them to the SKILL.md

This protocol is inspired by [antirez's clean room methodology](https://antirez.com/latest/0): curate documentation as a prerequisite, not a supplement. If the agent needs to search the internet during implementation, the spec is incomplete — fix the spec, not just the code. Over time, each SKILL.md converges toward completeness, and internet search during implementation drops to zero.

### Why Lazy Loading

Skills add 200–400 lines of context each. Loading all 14 at session start would:
- Consume 3,000–6,000 lines of context before any work begins
- Include irrelevant information (Django patterns when working in a Next.js project)
- Potentially cause conflicts (pytest fixtures pattern influencing bun:test code)

Loading only the relevant skill for the specific file being modified keeps context focused. An agent implementing a server action loads `nextjs-15` skill. An agent implementing a Zod schema loads `zod-4` skill. An agent implementing a repository loads `typescript` skill. No cross-contamination.

### Skill Evolution

Skills are not static. The ecosystem grows through use:

1. `/learn` — extracts reusable patterns observed in the current session
2. `/evolve` — clusters learned patterns into new or updated skill files
3. `/instruct status|import|export` — manages the learned pattern library

When a project discovers that a framework has a specific behavior in their environment (a gotcha with `bun test --watch` and hot module replacement, a quirk of Next.js 15 caching in production), that discovery can be saved via Engram and then evolved into a project-specific skill file that prevents the same discovery from needing to happen twice.

Over time, the skill files become a curated, project-specific extension of the model's knowledge — covering the exact versions, configurations, and constraints of the actual project.

---

## Pillar 5: Semi-Formal Reasoning

### The Problem

AI agents process information sequentially but do not naturally externalize their reasoning. When an explore agent reads 15 files, it holds observations in its hidden state — but those observations are unstructured, unordered, and prone to being overwritten by newer information. When a review agent checks code against specs, it reads both and produces a verdict — but the intermediate reasoning (which functions were traced, which data flows were verified, which failure modes were considered) is invisible and often shallow.

Two concrete failure modes emerge from this:

**Confirmation bias in exploration**: The agent forms an early hypothesis about how a module works, then reads subsequent files through that lens. Contradictory evidence is weighted less heavily than confirming evidence — not through malice, but because the model's attention mechanism naturally reinforces patterns it has already activated.

**Rubber-stamp reviews**: The agent reads the spec, reads the code, and produces "PASSED — implementation matches requirements" without genuinely testing the correspondence. The review is a formality, not a rigorous check. Critical edge cases, data flow invariants, and potential failure modes are not examined because the agent was not structurally required to examine them.

### The Solution

Semi-formal reasoning injects mandatory reasoning templates at key points in four SDD phases. The agent must fill these templates as part of its execution — they are not optional annotations but required steps that gate progress to the next action.

**Structured Exploration Protocol (sdd-explore, Step 4)**:

Before reading any file, the agent declares:
- A **hypothesis** about what it expects to find
- The **evidence** that led it to this file
- A **confidence level** (HIGH / MEDIUM / LOW)

After reading, it must formally update:
- **Observations** with exact File:Line references
- **Hypothesis status**: CONFIRMED, REFUTED, or REFINED
- **Next action justification**: Why the next file is the logical next step

The confidence field is critical. A HIGH-confidence hypothesis that gets REFUTED signals a fundamental misunderstanding — the agent must investigate deeper, not move on. A LOW-confidence hypothesis that gets CONFIRMED is a genuine discovery worth highlighting. Without the confidence declaration, the agent treats all reads as equally informative, which they are not.

**Semi-Formal Certificate (sdd-review, Steps 3h–3j)**:

The review agent must produce three formal structures:

1. **Function Tracing Table**: Every exported function touched by the change gets a row with File:Line, parameter types, return type, and verified behavior. This forces the agent to actually look up each function instead of reasoning about what it "probably" does.

2. **Data Flow Analysis**: For each critical data path, trace creation → transformations → consumption → invariants, with File:Line at every step. This catches a class of bugs where data is transformed incorrectly at an intermediate step — bugs that are invisible when you only check inputs and outputs.

3. **Counter-Hypothesis Check**: For each critical function, the agent must actively search for evidence that the implementation is wrong. The claim format — "Function X at File:Line could fail when..." — forces adversarial thinking. The agent is not asking "is this correct?" (which invites confirmation) but "how could this be wrong?" (which invites scrutiny).

**Fault Localization Protocol (sdd-verify, Step 5b)**:

When tests fail, the agent must decompose each failure into PREMISES (what the test does, step by step) and DIVERGENCE CLAIMS (where exactly the test's expectation diverges from the code's behavior, with specific File:Line references and a confidence level). This transforms vague "test failed" reports into precise diagnostic maps that sdd-apply can act on directly.

### Why "Semi-Formal"

Full formal verification (proof-based reasoning, theorem provers) is impractical for general-purpose software engineering. Informal reasoning ("the code looks right") is insufficient for AI agents whose internal reasoning is opaque. Semi-formal reasoning occupies the middle ground: structured templates that enforce rigor without requiring mathematical proof. The templates are designed to be light enough that they do not significantly increase token consumption, but structured enough that they prevent the most common reasoning failures.

### Relationship to Other Pillars

Semi-formal reasoning complements but does not replace the other four pillars:

- **Agent Teams Lite** (Pillar 1) ensures each agent has focused context. Semi-formal reasoning ensures the agent *uses* that context rigorously.
- **Engram Memory** (Pillar 2) persists decisions across sessions. Semi-formal reasoning creates better decisions to persist.
- **Agent Review Rules** (Pillar 3) provides mechanical REJECT/REQUIRE checks. Semi-formal reasoning adds semantic verification (function tracing, data flow, counter-hypotheses) on top.
- **Framework Skills** (Pillar 4) provides version-correct patterns. Semi-formal reasoning ensures those patterns are applied with understanding, not just pattern-matching.

---

## How the Pillars Work Together

The five pillars are not independent features. They compose. A concrete scenario shows how all five activate in a single change.

### Scenario: Implementing OAuth2 Login

The team is adding GitHub OAuth2 authentication to a Next.js 15 application.

**Step 1: Session start — Pillar 2 (Engram) activates**

```
mem_context()
```

Engram returns:
- `decision/auth-strategy`: "Using NextAuth.js v5 was rejected — it requires pages/ router and we use app/. Decision: implement OAuth2 flow manually with server actions."
- `bug/github-oauth-callback-urls`: "GitHub OAuth apps require exact URL match including trailing slash. Development and production callback URLs must both be registered."
- `pattern/server-action-result`: "Server actions return Result<T,E>, never throw. Caller handles Err in UI."

The model starts with 6 months of auth-related project knowledge before reading a single file.

**Step 2: sdd-review phase — Pillar 3 (Agent Review Rules) activates**

`sdd-review` loads `AGENTS.md` and checks the implementation against rules including:

```
REJECT: JWT secrets hardcoded in source files — use environment variables
REJECT: User-owned resources must verify ownership before access
REQUIRE: All authentication middleware must be applied before business logic handlers
```

The implementation used `process.env.GITHUB_CLIENT_SECRET` correctly, but the reviewer finds that the callback handler does not verify that the returned GitHub user matches the expected state parameter. This is flagged as a `REQUIRE` violation (auth middleware must validate before business logic), producing a FAILED verdict.

The author — the `sdd-apply` agent — would likely not have flagged this because it was focused on making the flow work, not on the adversarial case. The reviewer found it because it has no investment in the implementation working.

**Step 3: sdd-apply implementation — Pillar 4 (Framework Skills) activates**

`sdd-apply` is implementing the OAuth callback as a Next.js 15 server action in a `.tsx` file. Before writing code, it reads:
- `~/.claude/skills/frameworks/nextjs-15/SKILL.md` — learns the correct server action pattern, the `redirect()` behavior in App Router, the `cookies()` API for session storage
- `~/.claude/skills/frameworks/react-19/SKILL.md` — learns not to add `useMemo` to the login button component it will create

Without the nextjs-15 skill, the model might have implemented the callback as a route handler instead of a server action, or used the old `useRouter().push()` pattern instead of the App Router `redirect()`.

**Step 4: sdd-explore isolation — Pillar 1 (Agent Teams Lite) activates**

`sdd-explore` runs as an isolated sub-agent. It reads only the auth-related files:
- `src/auth/` (12 files)
- `src/middleware.ts` (references auth)
- `package.json` (to confirm no existing OAuth library)

It does not receive the orchestrator's conversation history, the Engram context summary, or any information about billing, user management, or UI components. Its blast radius assessment is based on the actual dependency graph of the auth module — not contaminated by unrelated context.

**The composition**: Engram provided historical context before investigation began. Agent Teams Lite isolated each phase's reasoning. Skills corrected version-specific patterns during implementation. Agent Review Rules verified the result against explicit, team-agreed rules.

Each pillar addressed a failure mode that the others did not:
- Engram addresses session amnesia (no other pillar can prevent re-discovering old decisions)
- Agent Teams Lite addresses context pollution (no other pillar can reduce in-session noise)
- Agent Review Rules addresses circular self-review (no other pillar can eliminate author bias)
- Skills address training cutoffs (no other pillar can inject current framework knowledge)
- Semi-Formal Reasoning addresses shallow analysis (no other pillar can force rigorous intermediate reasoning)

Remove any one pillar and a specific category of quality problems returns.

---

## Model Strategy: Opus where it matters, Sonnet everywhere else

SDD's architecture enables a cost-efficient model split. Because each sub-agent has a **focused, minimal context** (SKILL.md + 3-5 artifacts, never the full codebase), Sonnet performs at Opus quality for most phases. Opus is reserved only for the two phases where reasoning depth directly impacts output quality.

### Decision matrix

| Agent | Model | Reasoning |
|---|---|---|
| `sdd-explore` | Sonnet | Reads files and fills a structured template — pattern matching, not deep reasoning |
| `sdd-propose` | Sonnet | Structured document with clear sections derived from exploration data |
| `sdd-spec` | Sonnet | RFC 2119 + Given/When/Then format — template-driven, verifiable output |
| `sdd-tasks` | Sonnet | Converting design file changes into a numbered checklist |
| `sdd-review` | Sonnet | Checklist comparison of implementation against specs and AGENTS.md rules |
| `sdd-verify` | Sonnet | Runs commands, counts errors, fills a report — nearly deterministic |
| `sdd-clean` | Sonnet | Pattern matching for dead code and simplification opportunities |
| `sdd-archive` | Sonnet | File operations and spec merging — procedural |
| **`sdd-design`** | **Opus** | Makes architecture decisions that shape the entire implementation. Trade-offs between approaches require deep contextual reasoning. Wrong decisions here compound through every subsequent phase. |
| **`sdd-apply`** | **Opus** | Writes production code. Must navigate strict TypeScript, match existing patterns, satisfy spec scenarios, and handle edge cases — the highest cognitive load in the pipeline. |

### Why Sonnet is sufficient for 8 of 10 agents

The SDD structure compensates for model differences:

- Each agent reads a SKILL.md that defines exactly what to do and what format to produce
- Inputs are artifacts on disk (exploration.md, proposal.md) — not compressed summaries
- Outputs are validated by the next phase (sdd-review checks sdd-apply's work)
- The orchestrator (Opus) retains overall judgment and can reject bad outputs

A Sonnet agent following a detailed SKILL.md with clean inputs produces better results than an Opus agent with a polluted context and vague instructions.

### Cost impact

For a typical SDD change (full pipeline, ~10 sub-agent calls):

- **All Opus**: baseline cost
- **Sonnet for 8 agents, Opus for 2**: ~60–70% cost reduction
- **No quality loss**: the two Opus agents cover the phases where reasoning depth is the binding constraint

---

## Navigation

- Previous: [The SDD Pipeline](./02-pipeline.md)
- Next: [Commands Reference](./04-commands-reference.md)
