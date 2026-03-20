# Commit, Push, and Open a PR

Streamlined workflow to commit changes, push to remote, and create a pull request.

## Pre-computed Context

```bash
echo "=== Git Status ==="
git status --short

echo -e "\n=== Staged Changes ==="
git diff --cached --stat

echo -e "\n=== Unstaged Changes ==="
git diff --stat

echo -e "\n=== Current Branch ==="
git branch --show-current

echo -e "\n=== Remote Tracking ==="
git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null || echo "No upstream set"

echo -e "\n=== Recent Commits (for style reference) ==="
git log --oneline -5

echo -e "\n=== Unpushed Commits ==="
git log --oneline @{u}..HEAD 2>/dev/null || echo "No upstream to compare"
```

## Workflow

### Step 1: Review Changes
Analyze the pre-computed git status and diffs above. Identify:
- What files are staged vs unstaged
- Whether all intended changes are included
- If any sensitive files (.env, credentials) are accidentally staged

### Step 2: Stage Files (if needed)
If there are unstaged changes that should be included:
```bash
git add <specific-files>
```

### Step 3: Run Quality Checks
Before committing, ensure code quality:
```bash
bun run typecheck && bun run lint && bun test
```

If checks fail, fix the issues before continuing.

### Step 4: Create Commit
Write a commit message that:
- Follows the style of recent commits shown above
- Summarizes the "why" not just the "what"
- Is concise (subject line under 72 chars)

```bash
git commit -m "commit message here"
```

### Step 5: Push to Remote

**First, verify you are NOT on main/master:**
```bash
BRANCH=$(git branch --show-current)
if [ "$BRANCH" = "main" ] || [ "$BRANCH" = "master" ]; then
  echo "ERROR: Cannot push directly to $BRANCH. Create a feature branch first."
  exit 1
fi
git push -u origin "$BRANCH"
```

**If push is rejected** (diverged branch):
```bash
git pull --rebase origin "$BRANCH"
# Resolve any conflicts if needed, then:
git push -u origin "$BRANCH"
```

### Step 6: Create Pull Request
```bash
gh pr create --title "PR title" --body "$(cat <<'EOF'
## Summary
- Change 1
- Change 2

## Breaking Changes
- None | List any breaking changes to APIs, configs, or behavior

## Test plan
- [ ] Test item
EOF
)"
```

**If a PR already exists** for this branch:
```bash
echo "PR already exists:"
gh pr view --web
```

## Output
Return the PR URL when complete.
