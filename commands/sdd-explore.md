# /sdd:explore — Investigate Codebase

Read-only exploration of a codebase area or idea. Produces analysis with risk assessment. Does not modify state.

## Arguments
$ARGUMENTS — Topic or question to explore (required). Flags:
- `--deep` — Deep analysis with more detail
- `--concise` — Shorter, focused analysis
- `--for <change-name>` — Associate with an existing change

## Execution

You are the SDD Orchestrator.

### Step 1: Get explore context

If `--for <change-name>` is provided:
```bash
sdd context <change-name> explore
```

Otherwise, this is a standalone exploration — run directly without `sdd context`.

### Step 2: Launch sub-agent

```
Agent(
  description: 'sdd-explore {topic}',
  model: 'sonnet',
  prompt: '{context from sdd context if available, otherwise:}

  You are an SDD exploration sub-agent.
  Project: {current working directory}
  Topic: {extracted topic}
  Detail level: {concise|standard|deep}

  Explore the codebase for the given topic. Produce:
  1. Current state analysis
  2. Affected areas with file paths
  3. Approach comparison (if multiple approaches exist)
  4. Recommendation
  5. Risks

  If associated with a change (--for), write exploration to:
  File: openspec/changes/{change-name}/.pending/explore.md'
)
```

### Step 3: If associated with a change, promote

```bash
sdd write <change-name> explore
```

### Step 4: Present results

1. Executive summary (2-3 sentences)
2. Affected areas table (file path, impact)
3. Recommendation
4. Risks
5. Suggested next step: `/sdd:new <name>` to start a change based on this exploration

Do not ask questions during execution. Run autonomously and report.
