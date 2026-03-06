# Standard AI Coding vs SDD: Four Scenarios

Both approaches use the same AI model. The difference is structure: how context is managed,
how decisions are recorded, and how quality is verified. This document walks through four
concrete scenarios to show where standard AI coding workflows encounter problems — and how
SDD addresses each one.

None of this is a criticism of AI assistants. The underlying models are capable. The issue
is that capability without structure produces inconsistent outcomes at scale.

---

## Introduction

When you prompt an AI assistant to write code, you get code. Fast. Often good. The problems
emerge later: when you return to the feature three sessions from now, when a teammate tries
to understand why a decision was made, or when a bug turns out to have been caused by an
assumption the AI made and never documented.

SDD (Spec-Driven Development) is a workflow layer on top of AI coding. It imposes structured
phases — explore, propose, spec, design, apply, review, verify, clean, archive — and uses
persistent memory (Engram) to carry decisions across sessions. The goal is not to slow down
development. The goal is to prevent the class of failures that appear only after the fact.

The four scenarios below are representative. Each one shows a realistic failure mode and
how the SDD workflow addresses it.

---

## Scenario 1: Adding a New Feature

**Feature**: Add CSV export to a workout tracking app.

This is a common, well-scoped feature. It should be straightforward. In practice, it
produces several quiet failures that only surface in follow-up work.

### Standard Workflow

```
User: "Add CSV export for workout data"

AI: [reads 40 files to understand the codebase]
AI: writes WorkoutExportService
AI: writes CSVFormatter using the "csv-stringify" library
AI: modifies WorkoutController to add the /export route
AI: writes tests for the happy path
AI: "Done! Here's your CSV export feature."
```

Problems discovered later:

```
- The project already had "csv-parse" installed, which includes csv-stringify.
  The AI installed a second CSV library that was not needed.

- WorkoutController now handles routing, business logic, AND formatting.
  It violates the Single Responsibility Principle. Nobody flagged this.

- Workouts with missing "duration" fields cause a runtime crash.
  The AI tested normal workouts. Edge cases were not in the prompt.

- Three sessions later, user asks to add PDF export.
  AI opens a new session with no memory of the CSV feature.
  It writes a new ExportService with different interfaces.
  WorkoutController now has two export code paths that don't share logic.
  Merging them requires a refactor nobody planned for.
```

### SDD Workflow

```
/sdd:new add-csv-export "Export workout data as CSV files"
```

**Phase: explore**

The explore sub-agent reads the codebase with a specific mandate: map dependencies,
find existing patterns, assess risk.

```
explore finds:
- "csv-parse" v3.0.1 already installed (package.json, line 12)
- WorkoutRepository.findByUser() and WorkoutRepository.findByDateRange() exist
- WorkoutController currently handles only routing (clean SRP)
- No existing export abstraction

Risk assessment:
- Blast radius: LOW (new service + new route, no existing logic modified)
- Existing pattern: services in src/services/, thin controllers
- Missing field risk: workouts schema allows nullable duration, sets, and reps
```

**Phase: propose**

The propose sub-agent produces a structured proposal that is shown to the user for
approval before any code is written.

```
Intent:
  Allow users to download their workout history as a CSV file.

Scope (IN):
  - GET /api/workouts/export endpoint
  - CSV format with headers: date, exercise, sets, reps, duration, notes
  - Optional query params: startDate, endDate

Scope (OUT):
  - PDF export (explicitly deferred — separate change)
  - Bulk export across users (admin feature, separate change)

Approach:
  - WorkoutExportService: single class, single responsibility
  - Reuse installed csv-parse library (no new dependencies)
  - WorkoutController: thin delegation only — one method added

Rollback:
  - Remove WorkoutExportService, revert WorkoutController (3 files, ~50 lines)
  - No database changes required
```

**Phase: spec (parallel with design)**

```
REQ-EXPORT-001 [MUST]:
  Generated CSV MUST include a header row:
  date,exercise,sets,reps,duration_minutes,notes

REQ-EXPORT-002 [MUST]:
  Empty workout history MUST produce an empty CSV with headers only.
  Response: 200 OK, Content-Type: text/csv, body: header row only.
  NOT a 404 or error response.

REQ-EXPORT-003 [MUST]:
  Nullable fields (sets, reps, duration) MUST be rendered as empty strings,
  not "null" or "undefined".

REQ-EXPORT-004 [SHOULD]:
  Endpoint SHOULD accept startDate and endDate query params (ISO 8601).
  Invalid date formats MUST return 400 with a descriptive error message.

REQ-EXPORT-005 [MUST]:
  Response MUST set Content-Disposition: attachment; filename="workouts.csv"
```

**Phase: design (parallel with spec)**

```
WorkoutExportService:
  - exportToCsv(userId: string, options: ExportOptions): Promise<Result<string, ExportError>>
  - Uses csv-parse/stringify (existing dependency — no new install needed)
  - Returns Result<T, E> — never throws

ExportOptions interface:
  interface ExportOptions {
    readonly startDate?: Date;
    readonly endDate?: Date;
  }

ExportError interface:
  type ExportError =
    | { readonly kind: "INVALID_DATE_RANGE"; readonly message: string }
    | { readonly kind: "USER_NOT_FOUND"; readonly message: string }
    | { readonly kind: "SERIALIZATION_FAILED"; readonly message: string }

WorkoutController change:
  - One new method: handleExport(req, res)
  - Delegates entirely to WorkoutExportService
  - No business logic in controller
```

**Phase: apply**

```
WorkoutExportService created:
- exportToCsv implemented with Result<string, ExportError>
- Nullable field handling: (workout.duration ?? "").toString()
- Empty history: returns Ok("date,exercise,sets,reps,duration_minutes,notes\n")

Tests cover:
- REQ-EXPORT-001: header row present (it("includes header row", ...))
- REQ-EXPORT-002: empty history → 200 with headers (it("returns headers for empty history", ...))
- REQ-EXPORT-003: null duration → empty string, not "null" (it("renders null fields as empty strings", ...))
- REQ-EXPORT-004: invalid date → Err with INVALID_DATE_RANGE (it("rejects invalid date format", ...))

bun run typecheck: PASS (0 errors)
bun test: PASS (12 tests, 12 passed)
```

**Phase: review**

```
AGENTS.md check: "REQUIRE: All new services must have unit tests" — PASS (12 tests)
Spec coverage:
  REQ-EXPORT-001: covered by test "includes header row"           ✓
  REQ-EXPORT-002: covered by test "returns headers for empty..."  ✓
  REQ-EXPORT-003: covered by test "renders null fields..."        ✓
  REQ-EXPORT-004: covered by test "rejects invalid date format"   ✓
  REQ-EXPORT-005: covered by controller integration test          ✓
SRP check: WorkoutController.handleExport is 6 lines, no business logic ✓
```

**Phase: archive**

```
Engram saves:
  key: "decision/csv-export-library"
  content: "csv-parse used for CSV generation (already installed, no new dep needed)"

  key: "architecture/export-service-pattern"
  content: "WorkoutExportService: thin service, Result<T,E>, controller delegates only"
```

Three sessions later, when the user requests PDF export:

```
mem_context("export") loads:
→ "csv-parse used for CSV generation"
→ "WorkoutExportService: Result<T,E> pattern, thin service layer"
→ "ExportOptions interface defined in src/types/export.ts"

New design extends ExportOptions, implements PdfExportService with same interface.
No conflicts. No re-investigation.
```

**What SDD prevented**: Duplicate library installation, SRP violation in the controller,
unhandled nullable fields, incompatible follow-on implementation.

---

## Scenario 2: Fixing a Complex Bug

**Bug**: "Users are getting logged out randomly."

Session-persistent bugs are where standard AI workflows break down most visibly. The
investigation has to happen in context, and context doesn't persist across sessions.

### Standard Workflow

```
Session 1:
User: "Users are getting logged out randomly"
AI: [reads auth.middleware.ts, auth.service.ts, jwt.utils.ts]
AI: "I think it might be the JWT expiry. Let me check the token refresh logic..."
AI: [reads session.service.ts, redis.client.ts]
AI: "Found it! The token refresh logic has a race condition in refreshIfExpiring()."
AI: modifies auth.middleware.ts — adds mutex around the refresh check

Session ends. Context is gone.
```

```
Next day:
User: "The random logout bug is still happening"
AI: [reads files again — no memory of yesterday's investigation]
AI: "I think it might be the JWT expiry. Let me check the middleware..."

(Same investigation. Same red herring. 45 minutes of context rebuild.)
```

```
Session 3:
User: "Still happening"
AI: "Let me look at Redis..."
AI: [finds it]
AI: "The Redis session TTL is 55 minutes, but the JWT TTL is 1 hour.
     Users who are active during the last 5 minutes have a valid JWT
     but their Redis session has already expired."

Root cause found — 3 sessions in.
The race condition fix from Session 1 was unnecessary.
Nobody wrote down the investigation path. Nobody will prevent this next time.
```

### SDD Workflow

```
/sdd:explore "random user logouts — investigate auth session flow"
```

The explore sub-agent performs a structured investigation:

```
explore sub-agent maps the full auth flow:
  login → JWT issued (1h TTL) → middleware validates JWT → Redis session lookup → API response

Findings:
  auth.middleware.ts line 47:
    const session = await redis.get(`session:${userId}`);
    if (!session) throw new UnauthorizedError("Session expired");
    // ^ This runs even when JWT is still valid

  redis.client.ts line 23:
    SESSION_TTL_SECONDS: 60 * 55  // 55 minutes

  jwt.utils.ts line 15:
    JWT_TTL_SECONDS: 60 * 60  // 60 minutes (1 hour)

  Gap: JWT valid for 60 min, Redis session valid for 55 min.
  Users active between minute 55 and minute 60 have a valid JWT
  but no Redis session. Middleware rejects them. They are logged out.

Confidence: HIGH — root cause identified with specific file and line references.
Red herring logged: token refresh race condition is a separate issue (not this bug).
```

Engram saves immediately after explore:

```
mem_save({
  topic_key: "bug/random-logout-root-cause",
  content: `
    Root cause: JWT TTL (1h, jwt.utils.ts:15) > Redis session TTL (55min, redis.client.ts:23).
    Users active in minutes 55-60 have valid JWT but expired Redis session.
    Middleware at auth.middleware.ts:47 rejects on Redis miss regardless of JWT validity.
    Red herring: token refresh race condition is unrelated.
  `
})
```

```
/sdd:new fix-session-ttl-mismatch "Fix JWT/Redis TTL mismatch causing random logouts"

propose:
  Root cause: documented (TTL mismatch, not race condition)
  Fix options presented:
    Option A: Align Redis TTL to match JWT TTL (55min → 60min)
              Simple. Low risk. Recommended.
    Option B: Extend session on activity (sliding window)
              Complex. Better UX. Separate change.
    Option C: Remove Redis session check, rely on JWT only
              Security regression. Rejected.

  Approved: Option A
  Rollback: change SESSION_TTL_SECONDS back to 3300 (one-line revert)

apply:
  redis.client.ts line 23: SESSION_TTL_SECONDS: 60 * 60  // now matches JWT TTL
  Integration test added: simulates 57-minute session, verifies no logout
  bun test: PASS
```

Future session — user asks to implement "remember me" (30-day sessions):

```
mem_context("session TTL") loads:
→ "JWT TTL (1h) > Redis TTL (55min) bug fixed — now both aligned at 1h"
→ "Root cause: Redis miss on valid JWT causes logout"
→ "Option B (sliding window) deferred — flagged as follow-on"

Design for "remember me":
→ Extends BOTH JWT TTL and Redis TTL (same class of bug prevented)
→ Sliding window from deferred Option B is implemented now
→ No new investigation needed
```

**What SDD prevented**: Repeated root-cause investigation, wasted session time on red
herrings, the same TTL class of bug recurring in the follow-on feature.

---

## Scenario 3: Refactoring a Module

**Task**: "The auth module has gotten too big. Let's split it up."

Refactoring is where implicit knowledge causes the most damage. The code that needs to
be moved has consumers. Some of those consumers are not obvious.

### Standard Workflow

```
User: "Refactor the auth module — it's too big"
AI: [reads auth/ directory — 8 files, ~2000 lines]
AI: "I'll split it into auth-core, auth-oauth, and auth-session"

AI:
- creates src/auth-core/
- creates src/auth-oauth/
- creates src/auth-session/
- moves files and updates named imports

Problems found after the fact:

Problem 1 — broken imports:
  api/routes/admin.ts imported from 'auth' (barrel export)
  api/routes/webhook.ts imported from 'auth' (barrel export)
  api/middleware/rate-limit.ts imported from 'auth' (barrel export)
  → TypeScript errors on next build. Three files broken.

Problem 2 — dynamic import:
  api/middleware/oauth.ts line 88:
    const handler = await import(`../../auth/${provider}.handler`);
  This is a string-based dynamic import. TypeScript does not flag it.
  After refactor, the files are at ../../auth-oauth/${provider}.handler
  → Runtime failure. Silent at build time.

Problem 3 — file size:
  auth-core.ts: 850 lines
  (CLAUDE.md mandates 800-line hard limit)
  Nobody checked.

Problem 4 — test coverage gap:
  auth.test.ts tested auth/index.ts barrel exports
  After refactor, auth/index.ts is gone
  Tests pass (they import from the new paths) but coverage for
  auth-session is now 40% — tests were testing the barrel, not the module
```

### SDD Workflow

```
/sdd:explore "auth module structure — preparing for split"
```

The explore sub-agent uses Grep to find all consumers, not just the ones that are obvious:

```
explore sub-agent:

Step 1 — map the module:
  auth/ directory: 8 files, 2047 lines total
  auth/index.ts: 18 re-exports (barrel)
  auth/auth.service.ts: 780 lines (near 800-line limit — flagged)

Step 2 — find all consumers (Grep for all import patterns):
  Grep: "from '.*auth"
  Found 14 import sites across the codebase:
    api/routes/admin.ts          (imports AuthService, AdminAuth)
    api/routes/webhook.ts        (imports verifySignature)
    api/routes/user.ts           (imports requireAuth)
    api/middleware/rate-limit.ts (imports getAuthContext)
    api/middleware/cors.ts       (imports isPublicRoute)
    ... (9 more)

Step 3 — find dynamic imports (Grep for import() with string interpolation):
  Grep: "import\(`"
  Found:
    api/middleware/oauth.ts line 88:
      const handler = await import(`../../auth/${provider}.handler`);
  → FLAGGED AS HIGH RISK: dynamic import. Cannot safely rename files
    in the auth/providers/ directory.

Step 4 — file size projection:
  If split into auth-session, auth-oauth, auth-tokens:
    auth-session.ts: ~320 lines (safe)
    auth-oauth.ts:   ~180 lines (safe, because providers/ cannot be moved)
    auth-tokens.ts:  ~280 lines (safe)
    auth-core.ts:    ~430 lines (safe)
  → Splitting auth.service.ts (780 lines) avoids hitting the 800-line limit

Risk summary:
  HIGH: dynamic import at oauth.ts:88 — auth/providers/* must stay in place
  MED:  14 import consumers must be updated (automated but error-prone)
  LOW:  no database schema changes, full git revert possible
```

The explore report is presented to the user before any code is written.

```
/sdd:new refactor-auth-split "Split auth module into auth-session, auth-oauth, auth-tokens"

propose:
  Scope IN:
    - Split auth.service.ts (780 lines) into auth-session.ts and auth-tokens.ts
    - Update all 14 import consumers
    - Maintain auth/index.ts as backward-compatible barrel (with deprecation notice)
  Scope OUT:
    - auth/providers/* directory — MUST NOT be moved (dynamic import constraint)
    - auth-oauth split — deferred to separate change (providers can't be renamed)

design:
  Constraint documented: auth/providers/*.handler.ts paths are runtime strings.
  These paths CANNOT change without changing oauth.ts:88.
  A separate change will address that.

  File changes table (all 14 consumers listed explicitly):
    api/routes/admin.ts         → update to import from auth-session
    api/routes/webhook.ts       → update to import from auth-tokens
    api/middleware/rate-limit.ts → update to import from auth-session
    ... (11 more)

  auth/index.ts preserved as barrel:
    // @deprecated — import directly from auth-session, auth-tokens, or auth-core
    export * from '../auth-session';
    export * from '../auth-tokens';
    export * from '../auth-core';
  → Existing imports keep working. Migration can be incremental.

apply:
  Creates auth-session.ts (321 lines) ✓
  Creates auth-tokens.ts (284 lines) ✓
  Creates auth-core.ts (428 lines) ✓
  Updates all 14 consumers
  Preserves auth/index.ts as deprecated barrel
  auth/providers/* untouched (dynamic import constraint respected)
  bun run typecheck: PASS (0 errors)
  bun test: PASS (all 47 auth tests pass)

clean:
  Removes 3 unused re-exports from auth/index.ts
  Consolidates duplicate validateJwt helper (was in auth.service.ts AND auth.utils.ts)
```

**What SDD prevented**: Runtime failure from dynamic import, missed import consumers,
file size limit violation, test coverage regression.

---

## Scenario 4: Long Session — Context Exhaustion

**Task**: Building a complete billing module over multiple days.

This is where standard AI workflows are structurally unable to maintain consistency.
Context windows end. Sessions end. The knowledge built on Day 1 is not available on Day 3.

### Standard Workflow

```
Day 1 (Session 1):
User: "We need to add Stripe-based subscription billing"
AI: "I'll use a webhook-first architecture. Here's the plan..."
AI: implements stripe.service.ts
AI: creates webhook.handler.ts
AI: "For security, I'll validate Stripe signatures using the raw request body.
     We store the signature secret in STRIPE_WEBHOOK_SECRET env var."

[Session ends — 8 hours of architectural decisions, gone]
```

```
Day 2 (Session 2):
User: "Continue with billing — we need the subscription management endpoints"
AI: [reads stripe.service.ts, webhook.handler.ts]
AI: "I see you have a Stripe integration started. Let me understand the architecture..."
AI: [re-reads 20 files to reconstruct context — ~30 minutes of work]
AI: implements subscription.service.ts
AI: "I'll throw errors from the service for Stripe API failures"
    (Day 1 used Result<T,E> — AI doesn't know this)

Code inconsistency introduced: webhook handler uses Result<T,E>,
subscription service throws. Nobody notices yet.
```

```
Day 3 (Session 3):
User: "The webhook handler isn't validating Stripe signatures properly"
AI: [no memory of Day 1 security discussion]
AI: "I'll add signature validation using stripe.webhooks.constructEvent()"
AI: validates using req.body (parsed JSON) — not the raw body
    (Stripe signature validation requires the raw, unparsed request body.
     This is a well-known gotcha. It was discussed on Day 1. Nobody wrote it down.)

Result: signature validation silently fails for all requests.
Security gap in production. Discovered in security review two weeks later.
```

```
Day 4 — code review:
Reviewer: "Why does the webhook handler use Result<T,E> but subscription.service throws?"
Reviewer: "Why does this error log say [billing] but that one says [stripe-billing]?"
Reviewer: "The signature validation isn't using raw body — this won't work"
Author: "I don't know, different sessions"
```

### SDD Workflow

```
Day 1 (Session 1):
/sdd:new add-billing-module "Add Stripe-based subscription billing"
```

**Phase: explore**

```
explore:
→ No existing payment code found
→ Existing service pattern: Result<T,E>, structured logger with [module] prefix
→ Existing env var pattern: src/config/env.ts with zod validation
→ Risk: webhook signature validation requires raw body (flagged — common Stripe gotcha)
```

**Phase: propose + spec + design**

```
propose approved:
  Architecture: webhook-first (subscriptions driven by Stripe events, not API calls)
  Security: Stripe signature validation using raw body (express.raw() before express.json())

spec includes:
  REQ-BILLING-007 [MUST]:
    Webhook endpoint MUST validate Stripe signature before processing any event.
    MUST use raw request body (Buffer), not parsed JSON.
    Invalid signatures MUST return 400. MUST NOT process the event.

design includes:
  webhook.handler.ts:
    router.post('/webhook',
      express.raw({ type: 'application/json' }),  // raw body BEFORE json parser
      webhookHandler
    )

  Error handling: ALL services use Result<T,E> (consistent with existing pattern)
  Log prefix: [billing] for all billing module logs
  Env vars: STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET added to src/config/env.ts
```

Engram saves after each phase:

```
mem_save: "decision/billing-stripe-signature-raw-body"
  content: "Stripe webhook signature validation REQUIRES raw Buffer body.
            express.raw() must be applied BEFORE express.json() on /webhook route.
            Parsed JSON body causes silent validation failure — Stripe gotcha."

mem_save: "decision/billing-error-handling-pattern"
  content: "All billing services use Result<T,E>. Never throw from service layer.
            Consistent with existing codebase pattern."

mem_save: "decision/billing-log-prefix"
  content: "All billing module logs use [billing] prefix."

mem_save: "architecture/billing-stripe-webhook-first"
  content: "Subscriptions are driven by Stripe webhook events, not API calls.
            Source of truth: Stripe. Local DB is a projection."
```

**Day 2 (Session 2)**:

```
mem_context("billing stripe") loads:
→ "webhook-first architecture"
→ "Stripe signature: raw body required (express.raw() before express.json())"
→ "Error handling: Result<T,E> — no throws from services"
→ "Log prefix: [billing]"

/sdd:apply add-billing-module --phase 2

apply sub-agent reads design.md (single source of truth):
→ subscription.service.ts: uses Result<T,E> (consistent)
→ logs use [billing] prefix (consistent)
→ No re-investigation needed
→ 0 minutes spent reconstructing context
```

**Day 3 (Session 3)**:

```
mem_context("webhook signature stripe") loads:
→ "Stripe signature: raw Buffer required. express.raw() before express.json()."
→ "Gotcha: parsed JSON body causes silent failure."

Fix applied:
  webhook.handler.ts: express.raw({ type: 'application/json' }) confirmed in place
  Validation: stripe.webhooks.constructEvent(rawBody, sig, secret) ✓

bun test: 23 tests, 23 passed
Security check: raw body usage verified
```

**Day 4 — code review**:

```
Reviewer: "Result<T,E> throughout — consistent."
Reviewer: "All logs: [billing] prefix — consistent."
Reviewer: "Signature validation: raw body, correctly placed — good."
Author: "All decisions are in openspec/changes/add-billing-module/design.md"
```

**What SDD prevented**: Daily context reconstruction (~30 min/session), inconsistent error
handling across files, known Stripe gotcha applied incorrectly, security gap in production.

---

## Summary Table

| Scenario | Standard Failure | SDD Solution |
|---|---|---|
| New feature | Duplicate library, SRP violation, missing edge cases, incompatible follow-on | explore finds existing deps; spec captures edge cases; archive saves decisions for follow-on |
| Bug fix | Repeated investigation (3 sessions), lost root cause, wasted work on red herrings | explore documents full investigation; Engram persists root cause across sessions |
| Refactoring | Broken dynamic import, missed consumers, file size violation, test regression | explore maps all consumers and dynamic imports; design accounts for constraints |
| Long session | Daily context loss, inconsistent error handling, forgotten security decisions | Engram loads prior decisions; design.md is the session-independent source of truth |

---

## What SDD Doesn't Solve

This is worth being explicit about.

**Model hallucinations**: SDD structures the workflow, but it cannot fix errors in the
underlying model's outputs. A spec written by a hallucinating model is a hallucinated spec.
Review phases catch some of this, but not all.

**Trivial changes**: A one-line typo fix does not need a spec. SDD has overhead. For
changes that take five minutes, the workflow takes longer than the change. Use judgment.
The `/verify` command exists for quick quality gates outside full SDD.

**Team buy-in**: If one developer bypasses the spec process and pushes directly to main,
traceability breaks. SDD is a team convention, not a technical enforcement. A developer who
skips the propose phase because "it's just a small change" undermines the archive's value
for everyone who comes after.

**Engram availability**: Persistent memory depends on the Engram MCP server running. If it
is not running, `mem_save` calls fail silently and the workflow continues — but the memory
benefits are lost for that session. Decisions made without Engram are not recovered in
future sessions. The design.md file is the fallback: it persists in the repo even when
Engram is unavailable.

**Specification quality**: A spec is only as good as the person writing it. SDD enforces
that a spec exists and is approved before implementation. It does not enforce that the spec
is correct. The explore and propose phases reduce the risk of bad specs (by grounding them
in actual codebase findings), but they do not eliminate it.

---

## Navigation

- Previous: [05 - Skills Catalog](./05-skills-catalog.md)
- Next: [07 - Configuration](./07-configuration.md)
- Back to: [README](../README.md)
