# /sdd:apply — Implement Code

Write actual code following the specs and design. Works one task batch at a time from tasks.md.

## Arguments
$ARGUMENTS — Optional: change name. Flags:
- `--phase N` — Implement only phase N from tasks.md
- `--tdd` — Write tests FIRST, then implementation
- `--all` — Implement all remaining phases sequentially
- `--fix-only` — Only run build-fix loop on existing code (no new implementation)

## Execution

You are the SDD Orchestrator.

### Step 1: Get apply context

```bash
sdd context <name> apply
```

This assembles: current incomplete task from tasks.md, target file contents, design constraints, and the sdd-apply SKILL.md instructions.

### Step 2: Launch sub-agent

```
Agent(
  description: 'sdd-apply for {change-name}',
  prompt: '{context from sdd context output}

  Implement the next incomplete task. Use Edit/Write tools to modify project files.
  Mode: {normal|tdd|fix-only}
  Batch: {phase N if specified, else next incomplete}

  After implementing, write updated tasks.md (with completed items marked [x]) to:
  File: openspec/changes/{change-name}/.pending/apply.md

  Run build-fix loop after each task:
  1. Typecheck -> fix errors (max 5 attempts)
  2. Lint -> fix errors
  3. Tests -> fix failures
  Report final build status.

  Follow the SKILL instructions exactly.'
)
```

### Step 3: Promote + advance state

```bash
sdd write <name> apply
```

### Step 4: Present results

1. Tasks completed count
2. Build status (typecheck, lint, tests)
3. Any deviations from design
4. Next step: `/sdd:apply` again if tasks remain, or `/sdd:review` if all done

### Step 5: If --all mode

Loop: get context -> sub-agent -> promote for each incomplete phase. Stop if any phase fails.
