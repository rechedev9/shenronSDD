# /sdd:archive — Close Completed Change

Archive the change directory and generate a manifest. This is a **zero-token** operation — runs entirely in Go.

## Arguments
$ARGUMENTS — Change name (required).

## Execution

You are the SDD Orchestrator.

### Step 1: Run archive

```bash
sdd archive <name>
```

This moves `openspec/changes/{name}/` to `openspec/changes/archive/{timestamp}-{name}/` and writes `archive-manifest.md` listing all artifacts.

**Zero tokens consumed.**

### Step 2: Present results

Parse the JSON output. Show:
1. **Archive location**: full path
2. **Manifest**: artifact count, completed phases
3. **Change summary**: name, description, key artifacts preserved

### Step 3: Suggest next actions

- Commit and create PR for this change
- `/sdd:new <name>` — Start the next change

## Archive Contents

The archived folder contains the complete audit trail:
```
archive/{timestamp}-{change-name}/
  exploration.md
  proposal.md
  specs/
  design.md
  tasks.md
  review-report.md
  verify-report.md
  clean-report.md
  archive-manifest.md
```
