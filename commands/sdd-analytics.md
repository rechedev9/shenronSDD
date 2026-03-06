# /sdd:analytics — Quality Analytics from Phase Delta Tracking

Analyze the quality timeline for a change. Read quality-timeline.jsonl and produce trend reports.

## Arguments
$ARGUMENTS — Optional: change name. Auto-detected if only one active change exists.

## Execution

You are the SDD Orchestrator.

### Step 1: Locate timeline

- Find `openspec/changes/{change-name}/quality-timeline.jsonl`
- If not found, report "No quality timeline found for {change-name}" and exit

### Step 2: Launch analytics sub-agent

```
Task(
  description: 'sdd-analytics for {change-name}',
  subagent_type: 'general-purpose',
  model: 'sonnet',
  prompt: 'You are an SDD analytics sub-agent.

  Read the skill file at ~/.claude/skills/sdd/sdd-analytics/SKILL.md if it exists.

  CONTEXT:
  - Project: {cwd}
  - Change: {change-name}
  - Timeline: openspec/changes/{change-name}/quality-timeline.jsonl

  TASK: Parse the JSONL timeline file. For each snapshot, extract phase, timestamp,
  agentStatus, issues, buildHealth, completeness, and scope. Compute:
  1. Build health progression — did typecheck/lint/tests improve or regress between phases?
  2. Issue density by phase — which phases introduced the most critical issues?
  3. Completeness curve — how did task/spec coverage grow over phases?
  4. Scope summary — total files created, modified, reviewed across the pipeline
  5. Phase timing — duration between consecutive snapshots (if timestamps allow)
  6. Regressions — any metric that worsened between consecutive phases (flag these)

  Return JSON envelope with: status, executive_summary, trends, regressions, recommendations.'
)
```

### Step 3: Present results

```
QUALITY ANALYTICS: {change-name}
Phases tracked: {count}

Build Health Trend:
  typecheck: [progression across phases]
  lint:      [progression across phases]
  tests:     [progression across phases]

Completeness:
  Tasks:     [final X/Y] — [growth curve summary]
  Scenarios: [final A/B]

Scope:
  Files created:  {N}
  Files modified: {N}
  Files reviewed: {N}

Regressions: {count found}
  [list any metrics that worsened between phases]

Recommendations:
  [actionable suggestions based on trends]
```
