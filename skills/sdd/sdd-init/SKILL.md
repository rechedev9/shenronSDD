---
name: sdd-init
description: >
  Bootstrap Spec-Driven Development for a project. Detects tech stack, creates openspec/ directory structure, and generates config.yaml.
  Trigger: When user runs /sdd:init or starts SDD for the first time in a project.
license: MIT
metadata:
  version: "1.0"
---

# SDD Init

You are executing the **init** phase inline. Your job is to detect the project's technology stack, architecture patterns, and conventions, then create the `openspec/` directory structure with a comprehensive `config.yaml`.

## Activation

User runs `/sdd:init`. The project root is the current working directory. Flags:
- `--force`: Overwrite existing `openspec/`
- `--dry-run`: Report what would be created without writing files

## Execution Steps

### Step 1: Check for Existing SDD Setup

1. Check if `openspec/` directory already exists at the project root.
2. If it exists and `force` is false:
   - Read `openspec/config.yaml`
   - Report the current SDD state (schema version, detected stack, number of specs, active changes)
   - Present a summary noting that SDD was already initialized, with the current config summary and next steps
3. If it exists and `force` is true:
   - Back up existing `config.yaml` as `config.yaml.bak`
   - Proceed with re-detection

### Step 2: Detect Technology Stack

Scan the project root for manifest files, lockfiles, and config files. If no manifest file is found (`package.json`, `go.mod`, `pyproject.toml`, `Cargo.toml`, or equivalent), report an error and suggest manual configuration. Detect:

1. **Language, runtime, and package manager**
2. **Frameworks** (frontend, backend, ORM)
3. **Test runner, linter, and formatter**
4. **Build/check commands** (from scripts or build system)

### Step 3: Detect Architecture Patterns

Determine:

1. **Monorepo vs single-package** (workspace configs, multiple build targets)
2. **Frontend/backend split** (separate entry points, directory structure)

### Step 4: Create Directory Structure

Create the following directories and files:

```
openspec/
  config.yaml           # Project configuration and SDD rules
  specs/                # Source of truth for current system specifications
    .gitkeep
  changes/              # Active change proposals and artifacts
    .gitkeep
    archive/            # Completed and archived changes
      .gitkeep
```

### Step 5: Generate config.yaml

Generate `openspec/config.yaml` with the following sections:

```yaml
schema: spec-driven
version: "1.0"
generated_at: <ISO 8601 timestamp>

project:
  name: <detected from manifest or directory name>
  path: <absolute project path>
  type: <monorepo | single-package>

stack:
  language: <detected language>
  runtime: <detected runtime>
  frameworks: <detected frameworks list>

commands:
  typecheck: <detected build/type check command>
  lint: <detected lint command>
  lint_fix: <detected lint fix command>
  test: <detected test command>
  format_check: <detected format check command>
  format_fix: <detected format fix command>

contracts:
  # Auto-assembled from PARCER Contract blocks in each phase's SKILL.md.
  # Scan ~/.claude/skills/sdd/sdd-*/SKILL.md for ## PARCER Contract sections
  # and merge here. Re-run /sdd:init to regenerate.
```

The `commands` block is the most critical output — all downstream phases read it. Detect commands from `CLAUDE.md`, manifest scripts, Makefile targets, or ecosystem conventions. Also map conventions from `CLAUDE.md` (if present) into the config. Never include secrets or environment variable values — only reference variable names.

### Step 5b: Assemble PARCER Contracts

Populate the `contracts` section by scanning `~/.claude/skills/sdd/sdd-*/SKILL.md` files. For each file with a `## PARCER Contract` section, extract the YAML block and merge it into `contracts:` keyed by phase name. Skip phases without contracts.

### Step 6: Present Summary

Present a markdown summary to the user, then STOP. Do not proceed automatically.

**On success, output:**

```markdown
## SDD Init Complete

**Project**: {project_name}
**Stack**: {stack_summary}
**Architecture**: {monorepo | single-package}

### Files Created
- `openspec/config.yaml` — full project configuration
- `openspec/specs/` — baseline spec directory
- `openspec/changes/` — change tracking directory

### Conventions Captured
- Source: {CLAUDE.md | inferred | none}
- {N} coding rules and {N} verification commands registered

{If warnings: ### ⚠ Warnings\n- {warning}\n}

**Next step**: Run `/sdd:explore <topic>` to investigate an area, or `/sdd:new <change-name> "<intent>"` to start a change.
```