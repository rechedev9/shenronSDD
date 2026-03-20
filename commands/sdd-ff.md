# /sdd:ff — Fast-Forward All Planning Phases

Runs all planning phases sequentially without stopping for approval: explore -> propose -> spec + design (parallel) -> tasks.

## Arguments
$ARGUMENTS — Change name (required). Optionally prepend intent: `/sdd:ff add-dark-mode Add dark mode toggle to settings`

## Execution

You are the SDD Orchestrator. Fast-forward mode skips intermediate approvals.

### Step 1: Create change

```bash
sdd new <name> "<description>"
```

This creates the change directory and prints explore context to stdout.

### Step 2: Run explore -> write

Launch sub-agent with the explore context from Step 1:

```
Agent(
  description: 'sdd-explore for {change-name}',
  model: 'sonnet',
  prompt: '{explore context from sdd new output}

  Write exploration to: openspec/changes/{change-name}/.pending/explore.md
  Follow the SKILL instructions exactly.'
)
```

Promote:
```bash
sdd write <name> explore
```

Do NOT stop for approval — proceed immediately.

### Step 3: Run propose -> write

```bash
sdd context <name> propose
```

```
Agent(
  description: 'sdd-propose for {change-name}',
  model: 'sonnet',
  prompt: '{propose context}

  Write proposal to: openspec/changes/{change-name}/.pending/propose.md
  Follow the SKILL instructions exactly.'
)
```

```bash
sdd write <name> propose
```

### Step 4: Run spec + design in parallel -> write both

```bash
sdd context <name> spec
sdd context <name> design
```

Launch both sub-agents simultaneously:

```
Agent(
  description: 'sdd-spec for {change-name}',
  model: 'sonnet',
  run_in_background: true,
  prompt: '{spec context}

  Write spec to: openspec/changes/{change-name}/.pending/spec.md
  Follow the SKILL instructions exactly.'
)

Agent(
  description: 'sdd-design for {change-name}',
  prompt: '{design context}

  Write design to: openspec/changes/{change-name}/.pending/design.md
  Follow the SKILL instructions exactly.'
)
```

Wait for both, then promote:
```bash
sdd write <name> spec
sdd write <name> design
```

### Step 5: Run tasks -> write

```bash
sdd context <name> tasks
```

```
Agent(
  description: 'sdd-tasks for {change-name}',
  model: 'sonnet',
  prompt: '{tasks context}

  Write tasks to: openspec/changes/{change-name}/.pending/tasks.md
  Follow the SKILL instructions exactly.'
)
```

```bash
sdd write <name> tasks
```

### Step 6: Present complete planning summary

Show consolidated view:
1. **Exploration**: Key findings (2 lines)
2. **Proposal**: Intent + scope (2 lines)
3. **Specs**: Requirements count
4. **Design**: Key architecture decisions
5. **Tasks**: Phase count + task count
6. **Ready for**: `/sdd:apply {change-name}`

## Important

- Fast-forward is for experienced users who trust the planning pipeline
- All artifacts are still created — nothing is skipped, just approvals
- If any phase fails, STOP and report the error
- The user can review all artifacts before running `/sdd:apply`
