# Verify — Comprehensive Project Verification

One command for all verification needs: environment health, build checks, code quality, and security.

## Arguments
$ARGUMENTS — Mode:
- `quick` — Build + types only
- `full` — All checks (default)
- `pre-commit` — Checks relevant for commits
- `pre-pr` — Full checks plus security scan
- `healthcheck` — Environment diagnostics only
- `scan` — Full audit: healthcheck + fix loop + code review

## Mode: `quick`

1. `bun run build` — Stop on failure
2. `bun run typecheck` — Report errors with file:line

## Mode: `full` (default)

1. `bun run build` — Stop on failure
2. `bun run typecheck` — Report all errors
3. `bun run lint` — Report warnings and errors
4. `bun test` — Report pass/fail count
5. Grep for `console\.log\(` in `src/` — Report locations
6. Git status — Show uncommitted changes

## Mode: `pre-commit`

Same as `full`, minus git status.

## Mode: `pre-pr`

Same as `full`, plus:
- Grep for hardcoded secrets: `(password|secret|token|api_key)\s*[:=]\s*['"]`
- `bun pm audit` for dependency vulnerabilities
- Static analysis: Grep for `\bany\b`, `\bas\s+(?!const\b)`, `@ts-ignore`, `@ts-expect-error` in non-test `.ts` files

## Mode: `healthcheck`

Environment diagnostics — run ALL checks, do not stop on individual failures:

1. **Runtimes**: Check bun, git, tsc, gh, prettier, eslint availability and versions
2. **Git Health**: user.name, user.email, stale lock files, gh auth status
3. **Project Health**: node_modules, typecheck, lint, format, tests
4. **Hook & Config Health**: Validate `.claude/settings.json`, check command/agent files
5. **Process Cleanup**: Check for zombie node/bun processes, ports in use (3000, 3001, 5173, 8080)

Auto-fix what you can (install missing deps, remove stale locks). Report what needs manual attention.

## Mode: `scan`

Full autonomous audit — no questions, run everything:

1. **Healthcheck** — Verify environment (stop if critical issues)
2. **Fix Loop** — Review → Fix → Verify (max 3 loops):
   - Review: typecheck, lint, format:check, tests, static analysis
   - Fix: Priority order — types → lint → tests → static → format
   - Verify: Re-run all checks, loop if failures remain
3. **Code Review** — Type safety, immutability, security, code smells, file organization

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

For `scan` mode, include additional sections:
- Environment status table
- Fix loop results (fixed/flagged counts)
- Code review findings (critical/warning/suggestion counts)
- Commits created (if any)
- Verdict: CLEAN / NEEDS WORK / BLOCKED

Do not ask questions during execution. Run autonomously and report at the end.
