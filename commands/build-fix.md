# Build Fix — Diagnose, Fix, and Verify

Automatically diagnose and fix build errors with a retry loop. Runs without asking questions.

## Arguments
$ARGUMENTS — Mode:
- `types` — Fix TypeScript errors only
- `lint` — Fix ESLint errors only
- `all` — Fix everything (default)
- Optionally append file scope: `all src/auth/`

## Fix Pipeline

### Step 1: Review
Run checks and compile issue report:
```bash
bun run typecheck 2>&1 | tail -50
bun run lint 2>&1 | tail -50
bun run format:check 2>&1 | tail -20
bun test 2>&1 | tail -50
```

Then use Grep for static analysis:
- `\bany\b` in `*.ts` (exclude test files)
- `\bas\s+(?!const\b)` in `*.ts` (exclude test files)
- `@ts-ignore|@ts-expect-error`
- `console\.log\(` in `src/`

Compile results:
```
CRITICAL: [blocking issues]
WARNING: [should-fix issues]
COUNTS: X critical, Y warning
```

**If 0 critical and 0 warning → skip to Step 3.**

### Step 2: Fix (with retry loop)

Work through issues in priority order:
1. **TypeScript errors** (types inform everything else)
   - Add missing type annotations
   - Replace `any` with proper types
   - Remove unsafe `as Type` assertions (`as const` is safe)
   - Add null checks for possibly undefined values
2. **Lint errors** — Run `bun run lint:fix` first, then fix remaining manually
   - Unused variables (remove or use)
   - Missing return types
   - Floating promises (add `await` or `void`)
3. **Test failures**
4. **Static analysis violations**
5. **Formatting** (always last) — `bun run prettier --write <path>`

**Retry policy per issue:**
- Attempts 1–2: Fix the obvious problem
- Attempt 3: Re-read file from scratch, understand broader context
- Attempt 4: Try a fundamentally different approach
- Attempt 5: Flag with `// TODO(#manual): <description>`, move on

### Step 3: Verify

Run all checks — ALL must pass:
```bash
bun run typecheck && bun run lint && bun run format:check && bun test
```

**If any fail:** loop back to Step 2 with remaining issues. Maximum 3 loops.

## Common Fix Patterns

```typescript
// Bad: implicit any → Good: explicit types
const fn = (x) => x;
const fn = (x: string): string => x;

// Bad: type assertion → Good: type guard
const value = data as User;
function isUser(data: unknown): data is User {
  return typeof data === 'object' && data !== null && 'id' in data;
}

// Bad: non-null assertion → Good: null check
const name = user!.name;
const name = user?.name ?? 'default';

// Bad: floating promise → Good: awaited
fetchData();
await fetchData();
```

## Output

```markdown
## Build Fix Report

| Phase  | Status | Details            |
|--------|--------|--------------------|
| Review | DONE   | X critical, Y warn |
| Fix    | DONE   | X fixed, Y flagged |
| Verify | PASS/FAIL | Loops: N        |

### Fixed
- list of fixes

### Flagged for Manual Review
- list (if any)

### Quality Gate
| Check     | Status |
|-----------|--------|
| typecheck | OK/FAIL |
| lint      | OK/FAIL |
| format    | OK/FAIL |
| tests     | OK/FAIL |
```

Do not ask questions. Make reasonable decisions and document assumptions.
