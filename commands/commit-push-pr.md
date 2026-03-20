# Commit, Push, and Open a PR

## Pre-computed Context

```bash
git status --short
git diff --cached --stat
git diff --stat
git branch --show-current
git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null || echo "No upstream set"
git log --oneline -5
git log --oneline @{u}..HEAD 2>/dev/null || echo "No upstream to compare"
```

## Step 1: Stage (if needed)

```bash
git add <specific-files>
```

## Step 2: Quality Checks

```bash
bun run typecheck && bun run lint && bun test
```

Fix failures before continuing.

## Step 3: Commit

```bash
git commit -m "commit message here"
```

## Step 4: Push

```bash
BRANCH=$(git branch --show-current)
if [ "$BRANCH" = "main" ] || [ "$BRANCH" = "master" ]; then
  echo "ERROR: Cannot push directly to $BRANCH. Create a feature branch first."
  exit 1
fi
git push -u origin "$BRANCH"
```

If rejected (diverged):
```bash
git pull --rebase origin "$BRANCH"
git push -u origin "$BRANCH"
```

## Step 5: Create PR

```bash
gh pr create --title "PR title" --body "$(cat <<'EOF'
## Summary
- Change 1

## Breaking Changes
- None | List breaking changes

## Test plan
- [ ] Test item
EOF
)"
```

If PR already exists:
```bash
gh pr view --web
```

Output: PR URL.
