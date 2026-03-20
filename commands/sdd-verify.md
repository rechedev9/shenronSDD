# /sdd:verify — Technical Quality Gate

Run typecheck, lint, and tests. This is a **zero-token** operation when all checks pass — runs entirely in Go.

## Arguments
$ARGUMENTS — Optional: change name. Flags:
- `--fix` — Auto-fix issues found (run build-fix loop, then re-verify)

## Execution

You are the SDD Orchestrator.

### Step 1: Run verify

```bash
sdd verify <name>
```

This runs build/lint/test commands from config.yaml in sequence, writes `verify-report.md` to the change directory, and returns JSON with pass/fail status.

**Zero tokens consumed if all checks pass.**

### Step 2: Handle results

**If passed:**
- Show verify-report.md summary (all green)
- Advance state: write a pending verify artifact and promote it:
  ```bash
  # Write a pending artifact so sdd write can promote it
  echo "# Verify Report\n\nAll checks passed." > openspec/changes/{change-name}/.pending/verify.md
  sdd write <name> verify
  ```
- Suggest next step: `/sdd:clean {change-name}`

**If failed:**
- Show verify-report.md with error details (command, exit code, first 30 error lines)
- If `--fix` flag:
  1. Launch sub-agent to fix the identified issues
  2. Re-run `sdd verify <name>`
  3. Max 3 fix-verify cycles
  4. Report final status
- Otherwise suggest: `/sdd:apply --fix-only {change-name}` then re-verify

### Step 3: If --fix mode

```
Agent(
  description: 'fix verify failures for {change-name}',
  prompt: 'The following verify checks failed:

  {error output from verify-report.md}

  Fix the issues in the source code. Run the failing commands to confirm fixes work.
  Do NOT modify any openspec/ files.'
)
```

Then re-run `sdd verify <name>`. Repeat up to 3 times.
