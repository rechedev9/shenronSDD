# /sdd:review — Semantic Code Review

Compare implementation against specs, design, and project rules. Reports issues but does NOT fix them.

## Arguments
$ARGUMENTS — Optional: change name. Flags:
- `--strict` — Treat PREFER violations as blocking
- `--security` — Run deep security scan (OWASP Top 10)
- `--quick` — Check only REJECT/REQUIRE rules, skip PREFER

## Execution

You are the SDD Orchestrator.

### Step 1: Get review context

```bash
sdd context <name> review
```

This assembles: spec files, design.md, tasks.md, git diff of changed files, project rules (AGENTS.md/CLAUDE.md), and the sdd-review SKILL.md instructions.

### Step 2: Launch sub-agent

```
Agent(
  description: 'sdd-review for {change-name}',
  model: 'sonnet',
  prompt: '{context from sdd context output}

  Mode: {normal|strict|quick}
  Security: {true|false}

  Review all implemented code against specs, design, and project rules.
  Write review-report.md to:
  File: openspec/changes/{change-name}/.pending/review.md

  Follow the SKILL instructions exactly.'
)
```

### Step 3: Promote + advance state

```bash
sdd write <name> review
```

### Step 4: Present results

1. **Verdict**: PASS / FAIL
2. **Blocking issues** (REJECT + REQUIRE violations)
3. **Spec gaps** (scenarios not satisfied)
4. **Design deviations**
5. **Suggestions** (PREFER, non-blocking)
6. Next step:
   - If PASS: `/sdd:verify {change-name}`
   - If FAIL: `/sdd:apply --fix-only {change-name}` then re-review
