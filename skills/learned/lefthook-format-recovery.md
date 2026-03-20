# Lefthook Pre-Commit Format Recovery

**Extracted:** 2026-02-13
**Context:** This project uses lefthook with pre-commit hooks (typecheck + lint + format:check)

## Problem
When `prettier --check` fails in the lefthook pre-commit hook, the entire `git commit` is aborted. The commit **did not happen** — there is nothing to amend.

## Solution
1. Run `bun run prettier --write <failing-file>`
2. Re-stage the fixed file: `git add <file>`
3. Create a **new** commit (same message) — never use `--amend`

## Example
```bash
# After failed commit due to format:check
bun run prettier --write src/components/gzclp-app.tsx
git add src/components/gzclp-app.tsx
git commit -m "same commit message"
```

## When to Use
- Every time a commit fails due to the `format:check` lefthook step
- Can be preempted by running `bun run format:check` before committing
