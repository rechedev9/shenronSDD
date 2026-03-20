# Build Fix — Diagnose, Fix, and Verify

## Arguments
$ARGUMENTS — Mode:
- `types` — TypeScript errors only
- `lint` — ESLint errors only
- `all` — Everything (default)
- Optionally append file scope: `all src/auth/`

## Step 1: Review

```bash
bun run typecheck 2>&1 | tail -50
bun run lint 2>&1 | tail -50
bun run format:check 2>&1 | tail -20
bun test 2>&1 | tail -50
```

Static analysis via Grep:
- `\bany\b` in `*.ts` (exclude test files)
- `\bas\s+(?!const\b)` in `*.ts` (exclude test files)
- `@ts-ignore|@ts-expect-error`
- `console\.log\(` in `src/`

Compile: `CRITICAL: [blocking]`, `WARNING: [should-fix]`, `COUNTS: X critical, Y warning`.

**0 critical, 0 warning → skip to Step 3.**

## Step 2: Fix (retry loop)

Priority order:
1. TypeScript errors
2. Lint errors — `bun run lint:fix` first, then manual
3. Test failures
4. Static analysis violations
5. Formatting — `bun run prettier --write <path>`

Per issue: 3 attempts → re-read file + broader context → different approach → flag `// TODO(#manual): <description>`, move on.

## Step 3: Verify

```bash
bun run typecheck && bun run lint && bun run format:check && bun test
```

Any fail → loop to Step 2. Max 3 loops.

## Output

```
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
