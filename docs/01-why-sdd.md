# Why SDD? The Six Problems with Standard AI Coding

> Part 1 of the [SDD Workflow Documentation](../README.md)

---

## Introduction

AI coding tools are powerful. A modern language model can generate working implementations
from natural language descriptions, refactor complex code, write tests, and explain
unfamiliar codebases. For small, self-contained tasks — a utility function, a script, a
standalone component — they work remarkably well.

But these tools have fundamental architectural limitations that cause predictable failures
at scale. When you move beyond toy examples and into real codebases — systems with dozens
of modules, established conventions, accumulated technical decisions, and production
constraints — the same tools that felt magical start producing inconsistent, unreliable,
and untraceable output. These failures are not random. They stem from six specific
problems in how standard AI coding workflows are structured. SDD was designed to address
each one.

---

## Problem 1: Context Window Pollution

### The standard approach

You open your AI coding tool. You point it at your project. The tool ingests your
codebase — hundreds of files, thousands of lines — into a single context window. You ask
it to add a feature. The agent now holds your entire project in memory alongside your
request, and it tries to understand everything at once before producing output.

### What goes wrong

The context window is finite. As it fills with code from every module, every config file,
every test, the signal-to-noise ratio drops. The model is spending attention on your
database migration scripts when it should be focused on your React components. It sees
your authentication middleware when it is trying to design a chart widget.

The consequences are concrete:

- **Wrong-file edits**: The agent modifies a file that matches a pattern but is not the
  right target. It finds `UserProfile` in `types.ts` when it should be editing
  `components/user-profile.tsx`.
- **Pattern conflicts**: The agent sees two different patterns for the same thing (because
  the codebase evolved over time) and picks the wrong one, or worse, invents a third
  pattern.
- **Attention dilution**: Critical details in the actual target area get less attention
  because the model is distributing its capacity across irrelevant code.

### How SDD solves this

Each SDD sub-agent gets **only** the context relevant to its phase:

| Phase | What the sub-agent sees |
|-------|------------------------|
| explore | The codebase (read-only) — but that is its entire job |
| propose | The explore report — not the code itself |
| spec | The proposal — focused on requirements |
| design | The proposal + spec — focused on technical approach |
| tasks | The spec + design — focused on breaking work into steps |
| apply | The tasks + spec + design + **only the specific file being modified** |
| review | The spec + design + the changed files — comparing against requirements |

The apply sub-agent never sees your entire codebase. It sees the specification of what to
build, the design of how to build it, and the specific file it is currently modifying.
Nothing else. This is why it produces more focused, consistent output: its context window
contains only signal, no noise.

---

## Problem 2: Hallucinations from Context Compression

### The standard approach

You have been working with your AI tool for 20 minutes. The context window is filling up.
The tool (or the platform behind it) begins compressing earlier parts of the conversation
to make room for new content. This compression is usually invisible to you.

### What goes wrong

Context compression is lossy. It is summarization, and summarization discards nuance.
Here is what typically happens:

**Original context (early in the session):**
> "We decided to use the Result<T, E> pattern instead of throwing exceptions because the
> payment processing module has three different failure modes (network timeout, validation
> error, insufficient funds) that need to be handled differently by the caller. Throwing
> would force try/catch nesting three levels deep."

**After compression:**
> "Result pattern is used for error handling in payments."

**What the model does next:**
When you later ask about error handling in a different module, the model knows to use
Result but has lost the reasoning. If you ask "should we use Result for this simple
config loader?" the model says yes — because it remembers Result is the pattern, but not
the specific constraints that motivated it. The decision has been flattened into a rule
without context.

Worse, when compressed context mingles with new context, the model can hallucinate
connections that do not exist. It might "remember" that a particular function returns a
Result when it actually throws, because the compression merged details from two different
discussions.

### How SDD solves this

Decisions are not stored in the conversation. They are written to files on disk:

```
openspec/changes/payment-refactor/
  proposal.md       ← WHY we are making this change
  specs/*/spec.md   ← WHAT the requirements are (with rationale)
  design.md         ← HOW we decided to implement it (with alternatives considered)
  tasks.md          ← The implementation plan
```

When a sub-agent needs to know why a decision was made, it reads the file. The file has
not been compressed. The file has not been summarized. The file contains exactly what was
written, with full context and rationale. Context compression in the conversation is
irrelevant because the source of truth is on disk, not in the context window.

---

## Problem 3: Session Amnesia

### The standard approach

You work on a feature for an hour. You make architectural decisions. You discover that a
particular approach does not work because of a database constraint. You establish naming
conventions. You fix a subtle bug related to timezone handling. Then you close your
session.

The next day, you open a new session and start working on a related feature.

### What goes wrong

The new session knows nothing about yesterday. Every decision is gone:

- The architectural choice you made? The new session proposes the alternative you rejected
  yesterday.
- The naming convention you established? The new session uses a different one.
- The database constraint you discovered? The new session runs into it again, wastes 15
  minutes, and rediscovers the same workaround.
- The timezone bug? The new session introduces it again in the new feature.

You can mitigate this by writing context into your CLAUDE.md file, but this is manual,
incomplete, and does not scale. You end up with a CLAUDE.md that is either too sparse
(missing critical context) or too bloated (a dumping ground of everything, which causes
its own context pollution problems).

### How SDD solves this

SDD uses Engram, a persistent memory system backed by SQLite with FTS5 full-text search.
Engram operates through an MCP (Model Context Protocol) server that provides memory tools
to Claude Code.

The protocol is proactive, not reactive:

**Session start:**
```
mem_context → loads relevant decisions, patterns, bugs, and conventions
             from ALL prior sessions
```

**During the session (automatic, after every significant event):**
```
mem_save("decision/error-handling-payments",
         "Using Result<T, E> for payment module because three distinct
          failure modes need caller-level differentiation. Throwing would
          cause 3-level try/catch nesting.")

mem_save("bug/timezone-chart-data",
         "Chart data timestamps were in UTC but the display assumed local
          time. Root cause: API returns UTC, frontend date-fns format()
          defaults to local. Fix: explicit timezone parameter in all
          format() calls.")

mem_save("pattern/component-naming",
         "Dashboard components use kebab-case files with PascalCase exports.
          Example: dashboard-chart.tsx exports DashboardChart.")
```

**Next session start:**
```
mem_context → "Found 3 relevant entries:
  - Result pattern for payments (and why)
  - Timezone bug in chart data (root cause + fix)
  - Component naming convention (with example)"
```

The new session starts with full context from every prior session. No manual CLAUDE.md
maintenance. No re-discovery of known constraints. No re-investigation of solved bugs.

---

## Problem 4: No Traceability

### The standard approach

You ask the AI to add a feature. It writes code. The code works. You ship it.

Three months later, someone (maybe you) needs to modify that feature. They look at the
code and ask: "Why was it done this way? Why this data structure? Why this API shape?
Why is this field optional instead of required?"

### What goes wrong

There is no record. The conversation where the decisions were made is long gone. The AI
did not leave comments explaining its reasoning (and AI-generated comments are often
superficial anyway). The code is the only artifact, and code shows WHAT but not WHY.

This creates a cascade of problems:

- **Breaking changes**: A developer modifies the code without understanding the
  constraints that shaped it. The modification violates an assumption that is not
  documented anywhere.
- **Duplicated effort**: Someone implements a similar feature elsewhere and makes
  different decisions, creating inconsistency.
- **Review difficulty**: Code reviewers cannot evaluate whether the implementation is
  correct because they do not have the requirements it was built against.

### How SDD solves this

SDD produces a full traceability chain for every change:

```
proposal.md     →  WHAT is changing and WHY
                    (business context, user impact, motivation)
        |
        v
specs/*/spec.md →  WHAT must be true when the change is complete
                    (RFC 2119 requirements, Given/When/Then scenarios)
        |
        v
design.md       →  HOW the change will be implemented
                    (architecture decisions, alternatives considered, interfaces)
        |
        v
tasks.md        →  The implementation plan
                    (phased, numbered, verifiable steps)
        |
        v
[implementation]→  The actual code changes
        |
        v
review-report   →  Did the implementation match the spec?
                    (compliance check by independent sub-agent)
        |
        v
verify-report   →  Does the implementation pass quality gates?
                    (typecheck, lint, tests, security)
        |
        v
archive/        →  Historical record with learnings
                    (merged specs, captured patterns, noted risks)
```

Three months later, when someone asks "why was it done this way?", the answer is in the
archived design document. When they ask "what were the requirements?", the answer is in
the archived spec. When they ask "did anyone verify this?", the answer is in the
archived review and verify reports.

---

## Problem 5: No Formal Specs = No Formal Verification

### The standard approach

You ask the AI to write a feature. It writes the code. Then you ask it to review the
code. It reviews the code and says: "The implementation looks correct. The code is
well-structured and follows best practices."

### What goes wrong

This is self-review, and self-review is structurally flawed. The same model that wrote
the code is reviewing it. It has the same biases, the same blind spots, the same
assumptions. It does not have an independent specification to verify against — it is
comparing the code against its own understanding of what the code should do, which is
exactly the understanding that produced the code in the first place.

Concrete failures from self-review:

- **Missing edge cases**: The model did not consider a null input when writing the code,
  and it does not consider it when reviewing either.
- **Incorrect error handling**: The model assumes a function cannot fail (because it wrote
  it to not fail), so the review does not flag missing error handling.
- **Spec drift**: Without a written spec, the model's understanding of the requirements
  shifts subtly during implementation. The review compares against the shifted
  understanding, not the original intent.
- **Circular validation**: "Is this code correct?" "Yes, because it does what it does."
  Without external requirements, correctness is undefined.

### How SDD solves this

SDD separates specification from implementation and implementation from review. Three
different sub-agents handle these three tasks:

1. **sdd-spec** writes the requirements BEFORE any code exists. It defines what MUST be
   true, what SHOULD be true, and what MAY be true, using RFC 2119 keywords. It writes
   Given/When/Then scenarios that are concrete and testable.

2. **sdd-apply** implements the code. It reads the spec but does not write it. It is a
   different sub-agent with a different context.

3. **sdd-review** compares the implementation against the spec. It reads both the spec
   and the code, but it did not write either one. It is checking: "Does the code satisfy
   requirement X?" — not "Does the code look good to me?"

This separation is the same principle behind code review in human teams: the reviewer
is not the author. In SDD, the separation is enforced architecturally. The review
sub-agent literally cannot have the authorship bias because it was not present during
implementation — it was not even instantiated yet.

Additionally, the AGENTS.md file (the agent review rules) provides mechanical
rules that the review sub-agent checks line by line:

```markdown
- REJECT: any use of `any` type in production code
- REQUIRE: explicit return types on all exported functions
```

These are not subjective assessments. They are binary checks. Either the code uses `any`
or it does not. Either functions have return types or they do not. The review sub-agent
applies these checks without judgment, creating a baseline of mechanical verification
that supplements the semantic review.

---

## Problem 6: Framework Version Mistakes

### The standard approach

You are building a React 19 application with Tailwind 4 and Next.js 15. You ask the AI
to add a component. The AI generates code.

### What goes wrong

The model's training data has a cutoff. Even recent models may have been trained primarily
on React 18 code (because React 18 had years of ecosystem output while React 19 is
newer). The result:

- **React 18 patterns in React 19**: Wrapping everything in `useMemo` and `useCallback`
  when React 19's compiler handles memoization automatically. Using `useEffect` for data
  fetching when `use()` is the React 19 pattern. Missing Server Components opportunities.
- **Tailwind 3 syntax in Tailwind 4**: Using `tailwind.config.js` when Tailwind 4 uses
  CSS-first configuration with `@theme`. Using `@apply` excessively when Tailwind 4
  discourages it.
- **Old Next.js patterns**: Using `getServerSideProps` when Next.js 15 uses Server
  Components and Server Actions. Using the Pages Router when the project uses App Router.
- **Deprecated APIs**: Using APIs that were deprecated or removed in the current version
  of a library. The code compiles but produces warnings, or worse, breaks silently at
  runtime.

These are not hallucinations in the traditional sense — the code the model generates was
correct for the previous version. The model simply does not know (or does not prioritize)
the version-specific differences.

### How SDD solves this

SDD uses framework skills — version-specific SKILL.md files that are loaded on demand
when the code touches a particular framework. Each skill contains:

**Correct patterns for the current version:**
```markdown
## React 19 — Component Patterns

- DO NOT use useMemo/useCallback for memoization — the React 19 compiler
  handles this automatically
- USE the `use()` hook for data fetching in client components
- USE Server Components by default; add "use client" only when needed
- USE Server Actions for form submissions and mutations
```

**Banned patterns from previous versions:**
```markdown
## Banned Patterns (React 18 holdovers)

- REJECT: useMemo for derived values (compiler handles this)
- REJECT: useCallback wrapping event handlers (compiler handles this)
- REJECT: useEffect for data fetching (use `use()` or Server Components)
- REJECT: getServerSideProps / getStaticProps (Next.js 15 uses RSC)
```

**Correct imports and configuration:**
```markdown
## Imports

- `import { use, Suspense } from 'react'` — NOT from 'react-dom'
- `import { useFormStatus } from 'react-dom'` — for form states
```

Skills are loaded based on file extension and import detection. The trigger table in
CLAUDE.md maps domains to skills:

| Trigger | Skill Loaded |
|---------|-------------|
| Writing `.tsx` files | React 19 skill |
| Importing from `@tailwindcss` | Tailwind 4 skill |
| Files in `app/` directory (Next.js) | Next.js 15 skill |
| Using Zod schemas | Zod 4 skill |
| Using Zustand stores | Zustand 5 skill |

When the apply sub-agent starts writing a React component, it reads the React 19 skill
first. The skill overrides stale training data with current, version-specific patterns.
Skills can be updated independently of the model — when React 20 ships, you update the
skill file, not the model.

---

## Conclusion

SDD does not make AI smarter. It does not use a better model. It does not use prompt
engineering tricks or temperature tuning. It uses the same Claude model that powers
standard workflows.

What SDD changes is the **architecture** of the interaction:

| Standard workflow | SDD workflow |
|-------------------|-------------|
| One agent, entire codebase | Fresh sub-agent per phase, focused context |
| Decisions in conversation (compressed) | Decisions in files on disk (permanent) |
| Session ends, knowledge ends | Engram memory persists across sessions |
| No specs, no traceability | Full artifact chain: proposal to archive |
| Self-review (circular) | Independent review against written specs |
| Stale framework knowledge | Version-specific skills, updated on demand |

The same model produces dramatically better results when given constraints, traceability,
and fresh context at each step. SDD is the workflow that provides those conditions.

---

**Next:** [02 - Pipeline](02-pipeline.md) — Deep dive into all 11 phases with examples and artifact formats.

**Back to:** [README](../README.md)
