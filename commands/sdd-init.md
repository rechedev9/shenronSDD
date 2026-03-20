# /sdd:init — Bootstrap Spec-Driven Development

Initialize SDD in the current project. Detects tech stack and creates the `openspec/` directory structure.

## Arguments
$ARGUMENTS — Optional: `--force` to reinitialize an existing project.

## Execution

### Step 1: Run sdd init

```bash
sdd init $ARGUMENTS
```

This is a **zero-token** operation — Go detects the stack and writes config deterministically.

### Step 2: Present results

Parse the JSON output from stdout. Show the user:
1. Detected tech stack (language, build tool, manifests)
2. Created directory structure
3. Config.yaml location
4. Recommended next step: `/sdd:new <change-name> <description>`

If it fails, show the error from stderr JSON and suggest fixes (e.g., "no manifest found — create a go.mod/package.json first").
