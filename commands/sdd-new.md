# /sdd:new — Start a New SDD Change

Creates a new change and runs exploration + proposal with user approval between phases.

## Arguments
$ARGUMENTS — Change name (kebab-case, required), followed by description.
Example: `/sdd:new add-csv-export Export workout data as CSV files`

## Execution

You are the SDD Orchestrator. You manage the flow and delegate to sub-agents.

### Step 1: Parse arguments

- First word: change name (kebab-case)
- Remaining text: intent description
- If no description provided, ask the user for a brief intent

### Step 2: Create change + get explore context

```bash
sdd new <name> "<description>"
```

This creates the change directory, initial state, and prints explore context to stdout. The context includes the SKILL.md instructions, project info, and file tree.

### Step 3: Run exploration

Launch a sub-agent with the explore context from Step 2:

```
Agent(
  description: 'sdd-explore for {change-name}',
  model: 'sonnet',
  prompt: '{explore context from sdd new output}

  Write your exploration findings to the pending artifact:
  File: openspec/changes/{change-name}/.pending/explore.md

  Follow the SKILL instructions exactly.'
)
```

### Step 4: Promote exploration + present results

```bash
sdd write <name> explore
```

Show the user the exploration summary. Ask: "Proceed to proposal?"

### Step 5: Get propose context + run proposal

```bash
sdd context <name> propose
```

Launch a sub-agent with the propose context:

```
Agent(
  description: 'sdd-propose for {change-name}',
  model: 'sonnet',
  prompt: '{propose context from sdd context output}

  Write your proposal to the pending artifact:
  File: openspec/changes/{change-name}/.pending/propose.md

  Follow the SKILL instructions exactly.'
)
```

### Step 6: Promote proposal + present results

```bash
sdd write <name> propose
```

Show the user the proposal summary with: Intent, Scope, Approach, Risks, Rollback plan.

Ask: "Approve proposal? Next step: `/sdd:continue {change-name}` to generate specs + design."
