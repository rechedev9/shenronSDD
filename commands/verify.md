# Verify — Comprehensive Project Verification

## Arguments
$ARGUMENTS — Mode:
- `quick` — Build + types only
- `full` — All checks (default)
- `pre-commit` — Checks relevant for commits
- `pre-pr` — Full + security scan
- `healthcheck` — Environment diagnostics only
- `scan` — Healthcheck + fix loop + code review

## Mode: `quick`

1. `bun run build`
2. `bun run typecheck`

## Mode: `full` (default)

1. `bun run build`
2. `bun run typecheck`
3. `bun run lint`
4. `bun test`
5. Grep `console\.log\(` in `src/`
6. `git status`

## Mode: `pre-commit`

Same as `full`, minus git status.

## Mode: `pre-pr`

Same as `full`, plus:
- Grep: `(password|secret|token|api_key)\s*[:=]\s*['"]`
- `bun pm audit`
- Grep: `\bany\b`, `\bas\s+(?!const\b)`, `@ts-ignore`, `@ts-expect-error` in non-test `.ts` files

## Mode: `healthcheck`

Run ALL checks, do not stop on individual failures:

1. Runtimes: bun, git, tsc, gh, prettier, eslint — versions
2. Git: user.name, user.email, stale locks, `gh auth status`
3. Project: node_modules, typecheck, lint, format, tests
4. Hooks/Config: `.claude/settings.json`, command/agent files
5. Processes: zombie node/bun, ports 3000/3001/5173/8080

Auto-fix what you can. Report what needs manual attention.

## Mode: `scan`

Run autonomously, no questions:

1. Healthcheck — stop if critical
2. Fix loop (max 3): review → fix (types → lint → tests → static → format) → verify
3. Code review — type safety, immutability, security, code smells, file organization

## Output

```
VERIFICATION: [PASS/FAIL]

Build:    [OK/FAIL]
Types:    [OK/X errors]
Lint:     [OK/X issues]
Tests:    [X/Y passed]
Logs:     [OK/X console.logs]
Secrets:  [OK/X found]

Ready for PR: [YES/NO]
```

`scan` mode adds: environment table, fix loop results, code review findings, commits created, verdict (CLEAN / NEEDS WORK / BLOCKED).

Do not ask questions. Run autonomously, report at end.
