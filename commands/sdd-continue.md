# /sdd:continue — Run Next SDD Phase

Automatically detects which phase is next for a change and runs it.

## Arguments
$ARGUMENTS — Optional: change name. If omitted, auto-detects from `sdd list`.

## Phase Dependency Graph

```
explore -> propose -> spec + design (parallel) -> tasks -> apply -> review -> verify -> clean -> archive
```

## Execution

You are the SDD Orchestrator.

### Step 1: Detect active change + current phase

If change name provided, use it. Otherwise:

```bash
sdd list
```

If multiple active changes, list them and ask user to pick one. If zero, suggest `/sdd:new`.

Then get current phase:

```bash
sdd status <name>
```

The `current_phase` field tells you what's next.

### Step 2: Route by phase

Based on `current_phase`, follow the appropriate path below.

---

#### Planning phases (explore, propose, spec, design, tasks)

These phases need Claude to reason and write artifacts.

1. Get context:
```bash
sdd context <name> [phase]
```

2. Launch sub-agent with the assembled context:
```
Agent(
  description: 'sdd-{phase} for {change-name}',
  model: 'sonnet',  # Use Opus for design (architecture decisions)
  prompt: '{context from sdd context output}

  Write your output to the pending artifact:
  File: openspec/changes/{change-name}/.pending/{phase}.md

  Follow the SKILL instructions exactly.'
)
```

3. Promote artifact + advance state:
```bash
sdd write <name> <phase>
```

4. Present results and suggest next step.

**Special case — spec + design are parallel.** When `current_phase` is `spec`:
- Run spec and design sub-agents in parallel (both use propose context)
- Promote both: `sdd write <name> spec` then `sdd write <name> design`

---

#### apply

Implementation phase — Claude writes production code.

1. Get context:
```bash
sdd context <name> apply
```

2. Launch sub-agent (use **Opus** — writes production code):
```
Agent(
  description: 'sdd-apply for {change-name}',
  prompt: '{context from sdd context output}

  Implement the next incomplete task. Use Edit/Write tools to modify project files.
  After implementing, write updated tasks.md (with completed items marked [x]) to:
  File: openspec/changes/{change-name}/.pending/apply.md

  Run build-fix loop after: typecheck -> lint -> tests (max 5 attempts).
  Follow the SKILL instructions exactly.'
)
```

3. Promote: `sdd write <name> apply`
4. If more incomplete tasks remain, suggest `/sdd:continue` again. Otherwise suggest `/sdd:review`.

---

#### review

1. Get context: `sdd context <name> review`
2. Launch sub-agent (Sonnet — review is analytical):
```
Agent(
  description: 'sdd-review for {change-name}',
  model: 'sonnet',
  prompt: '{context from sdd context output}

  Review the implementation against specs and design. Write review-report.md to:
  File: openspec/changes/{change-name}/.pending/review.md

  Follow the SKILL instructions exactly.'
)
```
3. Promote: `sdd write <name> review`
4. Present verdict. If PASS: suggest `/sdd:verify`. If FAIL: suggest `/sdd:apply --fix-only`.

---

#### verify

**Zero-token operation** — runs entirely in Go.

```bash
sdd verify <name>
```

Parse JSON output. If passed, advance state:
```bash
sdd write <name> verify
```

Present verify-report.md summary. Suggest `/sdd:clean` next.

If failed, show errors and suggest `/sdd:apply --fix-only`.

---

#### clean

1. Get context: `sdd context <name> clean`
2. Launch sub-agent:
```
Agent(
  description: 'sdd-clean for {change-name}',
  model: 'sonnet',
  prompt: '{context from sdd context output}

  Clean up code in files modified by this change. Write clean-report.md to:
  File: openspec/changes/{change-name}/.pending/clean.md

  Follow the SKILL instructions exactly.'
)
```
3. Promote: `sdd write <name> clean`
4. Suggest `/sdd:archive`.

---

#### archive

**Zero-token operation** — runs entirely in Go.

```bash
sdd archive <name>
```

Parse JSON output. Show archive location and manifest. Suggest `/sdd:new` for next change or committing the work.

### Step 3: Present results and suggest next step

Always show what was completed and what comes next. Ask for approval before proceeding to the next phase (unless in fast-forward mode).
