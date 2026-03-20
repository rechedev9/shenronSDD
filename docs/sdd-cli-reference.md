---
summary: "Complete reference for sdd CLI commands, pipeline state, context assembly, and quality gates."
read_when:
  - "Looking up a sdd CLI command"
  - "Understanding cache or metrics system"
---

# sdd CLI Reference

`sdd` is the Spec-Driven Development context engine. It manages the SDD pipeline state, assembles per-phase context for Claude sub-agents, promotes artifacts through the pipeline, and runs zero-token quality gates.

---

## Table of Contents

1. [Global Usage](#1-global-usage)
2. [Exit Codes](#2-exit-codes)
3. [Error Format](#3-error-format)
4. [Pipeline Overview](#4-pipeline-overview)
5. [Commands — Pipeline](#5-commands--pipeline)
   - [init](#51-sdd-init)
   - [new](#52-sdd-new)
   - [context](#53-sdd-context)
   - [write](#54-sdd-write)
   - [verify](#55-sdd-verify)
   - [archive](#56-sdd-archive)
6. [Commands — Inspection](#6-commands--inspection)
   - [status](#61-sdd-status)
   - [list](#62-sdd-list)
   - [diff](#63-sdd-diff)
   - [health](#64-sdd-health)
7. [Commands — Utility](#7-commands--utility)
8. [Cache System](#8-cache-system)
9. [Pipeline Metrics](#9-pipeline-metrics)
10. [Context Cascade](#10-context-cascade)
11. [Configuration Reference](#11-configuration-reference)
12. [State Machine](#12-state-machine)
13. [Directory Layout](#13-directory-layout)

---

## 1. Global Usage

```
sdd <command> [arguments]
```

All commands read `openspec/config.yaml` from the current working directory (except `init`, which creates it). There is no global flag set; flags are per-command.

Per-command help:

```
sdd <command> --help
sdd <command> -h
```

---

## 2. Exit Codes

| Code | Meaning |
|------|---------|
| `0`  | Success |
| `1`  | General error (I/O failure, subprocess failure, state violation) |
| `2`  | Usage error (wrong argument count, unknown flag, unknown command) |

---

## 3. Error Format

All errors are written to **stderr** as a single-line JSON object before the process exits:

```json
{"command":"verify","error":"change directory not found: openspec/changes/my-change","code":"internal"}
```

Error codes:

| Code | Condition |
|------|-----------|
| `usage` | Invalid CLI usage: wrong arguments, unknown flags |
| `transport` | External process failure: git, shell commands |
| `internal` | All other errors: I/O, state corruption, artifact missing |

Success output goes to **stdout** as pretty-printed JSON (most commands) or raw text (context assemblers).

---

## 4. Pipeline Overview

The SDD pipeline is a linear sequence of phases with one parallel window:

```
explore → propose → spec  ─┐
                   design ─┤  (spec and design run in parallel after propose)
                            ↓
                          tasks → apply → review → verify → clean → archive
```

Each phase produces a Markdown artifact in the change directory. Claude writes the artifact to `.pending/<phase>.md`; `sdd write` promotes it to its final location and advances the state machine.

| Phase | Artifact produced | Token operation |
|-------|------------------|-----------------|
| `explore` | `exploration.md` | Claude-assisted |
| `propose` | `proposal.md` | Claude-assisted |
| `spec` | `specs/*.md` | Claude-assisted |
| `design` | `design.md` | Claude-assisted |
| `tasks` | `tasks.md` | Claude-assisted |
| `apply` | source code edits | Claude-assisted |
| `review` | `review-report.md` | Claude-assisted |
| `verify` | `verify-report.md` | **Zero-token** (Go) |
| `clean` | `clean-report.md` | Claude-assisted |
| `archive` | `archive-manifest.md` | **Zero-token** (Go) |

---

## 5. Commands — Pipeline

### 5.1 `sdd init`

Bootstrap SDD in a project directory.

**Usage:**
```
sdd init [path] [--force]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `path` | No | Directory to initialize. Defaults to `.` (current directory). |
| `--force`, `-f` | No | Reinitialize even if `openspec/` already exists. Without this flag, `init` fails with `ErrAlreadyInitialized`. |

**Behavior:**

1. Resolves `path` to an absolute directory.
2. Scans for manifest files in priority order: `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, `build.gradle`, `pom.xml`. First match wins.
3. Creates the directory structure: `openspec/`, `openspec/changes/`, `openspec/changes/archive/`.
4. Writes `openspec/config.yaml` with detected stack, inferred build/test/lint commands, and the default `skills_path` (`~/.claude/skills/sdd`).

**Output (stdout):**
```json
{
  "command": "init",
  "status": "success",
  "config_path": "/path/to/project/openspec/config.yaml",
  "dirs": [
    "/path/to/project/openspec",
    "/path/to/project/openspec/changes",
    "/path/to/project/openspec/changes/archive"
  ],
  "config": {
    "project_name": "my-project",
    "stack": {
      "language": "go",
      "build_tool": "go",
      "manifests": ["go.mod"]
    },
    "commands": {
      "build": "go build ./...",
      "test": "go test ./...",
      "lint": "golangci-lint run ./...",
      "format": "gofumpt -w ."
    },
    "skills_path": "/home/user/.claude/skills/sdd"
  }
}
```

**Exit codes:** `0` success, `1` no manifest found or I/O error, `2` usage error.

**Example:**
```bash
# Initialize in current directory
sdd init

# Initialize a specific directory
sdd init /path/to/project

# Reinitialize (overwrites config.yaml)
sdd init --force
```

---

### 5.2 `sdd new`

Start a new SDD change. Creates the change directory, initializes `state.json`, captures the current git HEAD as `base_ref`, and runs the `explore` context assembler.

**Usage:**
```
sdd new <name> "<description>"
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Change identifier. Kebab-case recommended. Becomes the directory name under `openspec/changes/`. |
| `description` | Yes | Brief intent description. Quoted if it contains spaces. |

**Behavior:**

1. Loads `openspec/config.yaml` (fails with a helpful message if missing: "run 'sdd init' first").
2. Creates `openspec/changes/<name>/` with mode `0755`.
3. Writes `openspec/changes/<name>/state.json` with all phases set to `pending` and `current_phase` set to `explore`.
4. Runs `git rev-parse HEAD` to capture `base_ref` in `state.json`. Non-fatal: changes in non-git projects proceed without `base_ref`.
5. Runs the `explore` context assembler and writes assembled context to **stdout**.

**Context assembly failure is non-fatal.** If the explore assembler fails (e.g., missing SKILL.md), `sdd new` prints a warning to stderr and exits `0`. The change directory and `state.json` are still created.

**Output (stdout):** Assembled explore context (plain text with labeled sections). See [Context Cascade](#10-context-cascade) for section format.

**Exit codes:** `0` success (including assembler warning), `1` config/I/O error, `2` usage error.

**Example:**
```bash
sdd new add-rate-limiter "Add per-user rate limiting to the API gateway"
sdd new fix-auth-bug "Fix JWT expiry not being checked on refresh endpoint"
```

---

### 5.3 `sdd context`

Assemble context for a change phase and print it to stdout. Used to re-run context assembly after state has advanced or when resuming a session.

**Usage:**
```
sdd context <name> [phase]
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Change name. Must correspond to a directory under `openspec/changes/`. |
| `phase` | No | Explicit phase to assemble. If omitted, the current phase is read from `state.json`. |

Valid phase values: `explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `review`, `clean`.

**Behavior:**

- **Explicit phase:** Runs the assembler for the specified phase and exits.
- **Auto-resolve (no phase argument):** Calls `ReadyPhases()` on the state. If exactly one phase is ready, assembles it. If multiple phases are ready (the `spec`+`design` parallel window after `propose` completes), assembles both **concurrently** and writes their output to stdout in pipeline order (`spec` then `design`). If zero phases are ready, returns an error.

**Caching:** Assembly results are content-hash cached in `openspec/changes/<name>/.cache/`. A cache hit skips the assembler entirely and serves the cached bytes. See [Cache System](#8-cache-system).

**Size guard:** The assembled context must not exceed 100 KB (~25K tokens). If it does, `sdd context` returns an error listing the actual byte count and estimated token count.

**Stderr output (metrics):**
```
sdd: phase=propose ↑48KB Δ12K tokens 23ms (assembled)
sdd: phase=propose ↑48KB Δ12K tokens 1ms (cached)
```

**Output (stdout):** Assembled context (plain text). Format varies by phase; see [Context Cascade](#10-context-cascade).

**Exit codes:** `0` success, `1` error (missing artifact, size guard exceeded, no phases ready), `2` usage error.

**Example:**
```bash
# Assemble current phase
sdd context add-rate-limiter

# Assemble a specific phase
sdd context add-rate-limiter design

# Pipe into a Claude invocation
sdd context add-rate-limiter | claude --no-markdown
```

---

### 5.4 `sdd write`

Promote a `.pending` artifact to its final location and advance the state machine.

**Usage:**
```
sdd write <name> <phase>
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Change name. |
| `phase` | Yes | Phase whose artifact to promote. |

Valid phase values: `explore`, `propose`, `spec`, `design`, `tasks`, `apply`, `review`, `verify`, `clean`.

**Behavior:**

1. Loads `state.json`.
2. Calls `artifacts.Promote(changeDir, phase)`: moves `openspec/changes/<name>/.pending/<phase>.md` to its final path.
3. Calls `state.Advance(phase)`: validates prerequisites are met, marks the phase `completed`, recomputes `current_phase` as the next pending phase with all prerequisites satisfied.
4. Saves `state.json` atomically (write to `.tmp`, then `rename`).

**Output (stdout):**
```json
{
  "command": "write",
  "status": "success",
  "change": "add-rate-limiter",
  "phase": "explore",
  "promoted_to": "openspec/changes/add-rate-limiter/exploration.md",
  "current_phase": "propose"
}
```

**State errors:** `write` will fail if:
- The phase has already been completed (`ErrAlreadyCompleted`).
- The phase's prerequisites are not yet completed (`ErrPrerequisitesNotMet`). For example, `tasks` requires both `spec` and `design` completed.

**Exit codes:** `0` success, `1` missing pending artifact, state violation, I/O error, `2` usage error.

**Example:**
```bash
# Promote the exploration artifact after Claude writes it to .pending/explore.md
sdd write add-rate-limiter explore

# Promote design (requires propose already completed)
sdd write add-rate-limiter design
```

---

### 5.5 `sdd verify`

Run the build/lint/test quality gate. Zero-token operation — runs entirely in Go.

**Usage:**
```
sdd verify <name>
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Change name. |

**Behavior:**

1. Loads `openspec/config.yaml` for `commands.build`, `commands.lint`, `commands.test`.
2. Checks smart-skip: if `verify-report.md` exists with `**Status:** PASSED` and no source files outside `openspec/` have changed since HEAD, the previous result is reused.
3. If not skipping: runs `build`, `lint`, `test` sequentially via `sh -c` in the project root, stopping on the first failure. Each command has a 5-minute timeout per command. Commands with empty strings in config are skipped.
4. Writes `verify-report.md` to the change directory atomically.

**Smart-skip logic (discrawl pattern):**
- Reads `verify-report.md` for `**Status:** PASSED`.
- Runs `git diff --name-only HEAD` and filters out `openspec/` paths.
- If the report passed and no non-openspec files changed, sets `skipped: true` in output and returns.

**Process groups:** Each command runs as a new process group (`Setpgid: true`) so a timeout kills the entire process tree, not just the shell.

**Output (stdout) — skipped:**
```json
{
  "command": "verify",
  "status": "success",
  "change": "add-rate-limiter",
  "passed": true,
  "skipped": true,
  "report_path": "openspec/changes/add-rate-limiter/verify-report.md"
}
```

**Output (stdout) — executed:**
```json
{
  "command": "verify",
  "status": "success",
  "change": "add-rate-limiter",
  "passed": true,
  "report_path": "openspec/changes/add-rate-limiter/verify-report.md"
}
```

On failure, `status` is `"failed"` and `passed` is `false`.

**Stderr progress during execution:**
```
sdd: verify build...
sdd: verify build: ok (1.234s)
sdd: verify lint...
sdd: verify lint: FAILED (exit 1)
```

**verify-report.md format:**
```markdown
# Verify Report

**Timestamp:** 2026-03-20T14:30:00Z

**Status:** PASSED

All commands passed.

## build — PASS

- **Command:** `go build ./...`
- **Duration:** 1.234s
- **Exit code:** 0

## lint — FAIL

- **Command:** `golangci-lint run ./...`
- **Duration:** 0.456s
- **Exit code:** 1
- **Timed out:** yes

**Error output:**

```
  1: ./internal/foo/bar.go:12:5: error message
  2: ...
```
```

Up to 30 lines of error output are included per failed command.

**Exit codes:** `0` all commands pass (or skipped), `1` any command fails, `2` usage error.

**Example:**
```bash
sdd verify add-rate-limiter
echo $?  # 0 = pass, 1 = fail
```

---

### 5.6 `sdd archive`

Archive a completed change. Zero-token operation — runs entirely in Go.

**Usage:**
```
sdd archive <name>
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Change name. |

**Behavior:**

1. Loads `state.json` and calls `CanTransition(archive)`: the `clean` phase must be completed.
2. Moves `openspec/changes/<name>/` to `openspec/changes/archive/<timestamp>-<name>/` using `os.Rename`. Timestamp format: `2006-01-02-150405` (UTC).
3. Writes `archive-manifest.md` in the archive directory listing all preserved files, spec count, and completed phase count.

**Prerequisite:** The `clean` phase must be completed before `archive` is allowed.

**Output (stdout):**
```json
{
  "command": "archive",
  "status": "success",
  "change": "add-rate-limiter",
  "archive_path": "openspec/changes/archive/2026-03-20-143000-add-rate-limiter",
  "manifest_path": "openspec/changes/archive/2026-03-20-143000-add-rate-limiter/archive-manifest.md"
}
```

**archive-manifest.md format:**
```markdown
# Archive Manifest

**Change:** add-rate-limiter
**Archived:** 2026-03-20T14:30:00Z

## Artifacts

- `exploration.md`
- `proposal.md`
- `specs/` (3 files)
- `design.md`
- `tasks.md`
- `review-report.md`
- `verify-report.md`
- `clean-report.md`
- `state.json`

## Summary

- **Completed phases:** 8
- **Spec files:** 3
```

**Exit codes:** `0` success, `1` prerequisites not met or I/O error, `2` usage error.

**Example:**
```bash
sdd archive add-rate-limiter
```

---

## 6. Commands — Inspection

### 6.1 `sdd status`

Show the full phase progress for a change.

**Usage:**
```
sdd status <name>
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Change name. |

**Stale detection:** A change is considered stale if it has not been updated in more than 24 hours and is not yet complete.

**Output (stdout):**
```json
{
  "command": "status",
  "status": "success",
  "change": "add-rate-limiter",
  "description": "Add per-user rate limiting to the API gateway",
  "current_phase": "design",
  "completed": ["explore", "propose", "spec"],
  "phases": [
    {"phase": "explore",  "status": "completed"},
    {"phase": "propose",  "status": "completed"},
    {"phase": "spec",     "status": "completed"},
    {"phase": "design",   "status": "pending"},
    {"phase": "tasks",    "status": "pending"},
    {"phase": "apply",    "status": "pending"},
    {"phase": "review",   "status": "pending"},
    {"phase": "verify",   "status": "pending"},
    {"phase": "clean",    "status": "pending"},
    {"phase": "archive",  "status": "pending"}
  ],
  "is_complete": false,
  "updated_at": "2026-03-20T14:00:00Z",
  "stale": false,
  "stale_hours": 0
}
```

`stale` and `stale_hours` are omitted when `false`/`0`.

Phase status values: `pending`, `in_progress`, `completed`, `skipped`.

**Exit codes:** `0` success, `1` error, `2` usage error.

---

### 6.2 `sdd list`

List all active changes in the project.

**Usage:**
```
sdd list
```

**Behavior:** Scans `openspec/changes/` for subdirectories with valid `state.json`. Skips the `archive/` subdirectory and any entry that cannot be parsed. Accepts no arguments.

**Output (stdout):**
```json
{
  "command": "list",
  "status": "success",
  "count": 2,
  "changes": [
    {
      "name": "add-rate-limiter",
      "current_phase": "design",
      "description": "Add per-user rate limiting to the API gateway",
      "is_complete": false,
      "stale": false
    },
    {
      "name": "fix-auth-bug",
      "current_phase": "apply",
      "description": "Fix JWT expiry not being checked on refresh endpoint",
      "is_complete": false
    }
  ]
}
```

`stale` is omitted when `false`.

**Exit codes:** `0` success (empty list is also success), `1` error reading changes directory.

---

### 6.3 `sdd diff`

List files changed in the working tree since `sdd new` was run.

**Usage:**
```
sdd diff <name>
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Change name. |

**Behavior:** Reads `base_ref` from `state.json` (the git SHA captured at `sdd new` time), then runs `git diff --name-only <base_ref>` in the project root. Fails if `base_ref` is empty (change was created before diff support was added, or in a non-git project).

**Output (stdout):**
```json
{
  "command": "diff",
  "status": "success",
  "change": "add-rate-limiter",
  "base_ref": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
  "files": [
    "internal/middleware/ratelimit.go",
    "internal/middleware/ratelimit_test.go",
    "cmd/server/main.go"
  ],
  "count": 3
}
```

`files` is `null` when there are no changes.

**Exit codes:** `0` success, `1` git error or missing `base_ref`, `2` usage error.

**Example:**
```bash
sdd diff add-rate-limiter
# Check how many files changed
sdd diff add-rate-limiter | jq '.count'
```

---

### 6.4 `sdd health`

Pipeline health summary with progress, cache stats, token usage, and warnings.

**Usage:**
```
sdd health <name>
```

**Arguments:**

| Argument | Required | Description |
|----------|----------|-------------|
| `name` | Yes | Change name. |

**Behavior:** Loads `state.json` and `.cache/metrics.json`. Emits warnings for:
- Changes inactive for more than 24 hours (`"change inactive for N hours"`).
- A failed last verify run (`"last verify FAILED"`).

**Output (stdout):**
```json
{
  "command": "health",
  "status": "success",
  "change": "add-rate-limiter",
  "current_phase": "apply",
  "completed": 5,
  "total_phases": 10,
  "cache_hits": 3,
  "cache_misses": 2,
  "total_tokens": 42000,
  "stale": false,
  "stale_hours": 0,
  "warnings": ["last verify FAILED"]
}
```

`stale`, `stale_hours`, and `warnings` are omitted when empty/zero/false.

**Exit codes:** `0` success, `1` error, `2` usage error.

---

## 7. Commands — Utility

### `sdd version`

```
sdd version
sdd --version
```

Prints the binary version string to stdout (single line). The version is injected at build time via ldflags:

```
-X github.com/rechedev9/shenronSDD/sdd-cli/internal/cli.version=<version>
```

Defaults to `"dev"` when built without ldflags.

### `sdd help`

```
sdd help
sdd --help
```

Prints the top-level command listing to stdout.

---

## 8. Cache System

Context assembly results are cached per-phase in `openspec/changes/<name>/.cache/`.

### Cache files

| File | Purpose |
|------|---------|
| `<phase>.ctx` | Assembled context bytes |
| `<phase>.hash` | Content hash + timestamp: `<sha256hex>|<unix_seconds>` |
| `metrics.json` | Cumulative pipeline metrics |

### Cache version

`cacheVersion = 4`. The version is mixed into every hash computation. Bumping the constant auto-invalidates all existing cache entries across all changes.

### Content hash

The cache key is a SHA-256 hash computed from:

1. Version prefix: `v4:`.
2. The SKILL.md file for the phase (if `skills_path` is set). Editing a SKILL.md invalidates the cache for that phase.
3. All input artifact files for the phase (sorted, to ensure determinism).

**Phase input artifacts:**

| Phase | Inputs hashed |
|-------|--------------|
| `explore` | _(none — SKILL.md only)_ |
| `propose` | `exploration.md` |
| `spec` | `proposal.md`, `exploration.md` |
| `design` | `proposal.md`, `specs/` (all `.md` files) |
| `tasks` | `design.md`, `specs/` |
| `apply` | `tasks.md`, `design.md`, `specs/` |
| `review` | `tasks.md`, `design.md`, `specs/` |
| `clean` | `verify-report.md`, `tasks.md`, `design.md`, `specs/` |

`specs/` is hashed at the directory level: all `.md` files are included in sorted order, with each file contributing `specs/<name>:<len>:<content>` to the hash.

### TTL

Per-phase time-to-live enforced in addition to the content hash:

| Phase | TTL |
|-------|-----|
| `propose` | 4 hours |
| `spec` | 2 hours |
| `design` | 2 hours |
| `tasks` | 1 hour |
| `apply` | 30 minutes |
| `review` | 1 hour |
| `clean` | 1 hour |
| `explore` | _(no TTL — valid until content changes)_ |

A cache entry is valid only when **both** the content hash matches **and** the TTL has not expired. The hash file stores the Unix timestamp of when the entry was written; the TTL is checked against `time.Since(stored_timestamp)`.

**Legacy files** (without the `|<timestamp>` suffix) always produce a cache miss and trigger re-assembly, transparently upgrading to the new format on write.

### Size guard

Assembled context must not exceed **100 KB** (~25K tokens at ~4 bytes/token). Oversized contexts return an error rather than writing a truncated cache entry.

---

## 9. Pipeline Metrics

Each context assembly operation appends to `.cache/metrics.json` in the change directory.

### Schema

```json
{
  "version": 4,
  "phases": {
    "explore": {
      "bytes": 12800,
      "tokens": 3200,
      "cached": false,
      "duration_ms": 45
    },
    "propose": {
      "bytes": 48000,
      "tokens": 12000,
      "cached": true,
      "duration_ms": 2
    }
  },
  "total_bytes": 60800,
  "total_tokens": 15200,
  "cache_hits": 1,
  "cache_misses": 1
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | `int` | Cache version at time of write. Mismatched versions return a fresh empty metrics object. |
| `phases` | `object` | Map of phase name to `PhaseMetrics`. |
| `phases.<phase>.bytes` | `int` | Assembled context size in bytes. |
| `phases.<phase>.tokens` | `int` | Estimated token count (`bytes / 4`). |
| `phases.<phase>.cached` | `bool` | Whether this assembly was served from cache. |
| `phases.<phase>.duration_ms` | `int64` | Wall-clock time for assembly or cache lookup, in milliseconds. |
| `total_bytes` | `int` | Sum of bytes across all recorded phases. |
| `total_tokens` | `int` | Sum of tokens across all recorded phases. |
| `cache_hits` | `int` | Number of phases served from cache. |
| `cache_misses` | `int` | Number of phases requiring fresh assembly. |

If `metrics.json` does not exist or has a mismatched `version`, it is treated as empty and re-created on the next write. Token estimation is approximate: `bytes / 4` for English/code mixed content.

`sdd health` reads `metrics.json` to populate `cache_hits`, `cache_misses`, and `total_tokens` in its output.

---

## 10. Context Cascade

Each phase assembler writes sections in the format:

```
--- SECTION LABEL ---

<content>

```

All sections are concatenated to stdout. The labels and content vary by phase:

### explore

Sections in order: `SKILL`, `PROJECT`, `CHANGE`, `FILE TREE`, `MANIFESTS`.

- `SKILL`: Contents of `<skills_path>/sdd-explore/SKILL.md`.
- `PROJECT`: `project_name`, `language`, `build_tool`, `manifests` from config.
- `CHANGE`: Change name and description.
- `FILE TREE`: Output of `git ls-files` in the project root. Falls back to an error note if git is unavailable.
- `MANIFESTS`: Raw contents of each detected manifest file (capped at 2 KB each) for dependency/version context.

### propose

Sections: `SKILL`, `CHANGE`, `PROJECT`, `FILE TREE`, `EXPLORATION`.

- `EXPLORATION`: Full contents of `exploration.md` (required; fails if missing).

### spec

Sections: `SKILL`, `CHANGE`, `PIPELINE CONTEXT`, `PROPOSAL`.

- `PIPELINE CONTEXT`: Compact summary (~500–800 bytes) of prior phase artifacts. See below.
- `PROPOSAL`: Full contents of `proposal.md`.

### design

Sections: `SKILL`, `CHANGE`, `PIPELINE CONTEXT`, `PROPOSAL`, `SPECIFICATIONS`.

- `SPECIFICATIONS`: All `.md` files from `specs/`, concatenated.

### tasks

Sections: `SKILL`, `CHANGE`, `PIPELINE CONTEXT`, `DESIGN`, `SPECIFICATIONS`.

### apply

Sections: `SKILL`, `CHANGE`, `PIPELINE CONTEXT`, `COMPLETED TASKS`, `CURRENT TASK`, `DESIGN`, `SPECIFICATIONS`.

- `CURRENT TASK`: The first incomplete task section (identified by `- [ ]`) extracted from `tasks.md`, along with its parent section header. Minimizes context by omitting already-completed work.
- `COMPLETED TASKS`: A flat summary of all checked (`- [x]`) tasks.

### review

Sections: `SKILL`, `CHANGE`, `PIPELINE CONTEXT`, `DESIGN`, `SPECIFICATIONS`, `TASKS`.

### clean

Sections: `SKILL`, `CHANGE`, `PIPELINE CONTEXT`, `VERIFY REPORT`, `TASKS`, `DESIGN`.

### Pipeline Context section

Included in `spec`, `design`, `tasks`, `apply`, `review`, `clean`. A compact (~500–800 byte) summary containing:

- Change name and description.
- Stack language and build tool.
- First 3 content lines after the first `##` heading of `exploration.md` (if present).
- First 3 content lines after the first `##` heading of `proposal.md` (if present).
- First 3 content lines after the first `##` heading of `design.md` (if present).
- First 1 content line after `Verdict` in `review-report.md` (if present).

This carries key decisions forward without repeating entire prior artifacts.

---

## 11. Configuration Reference

`openspec/config.yaml` is created by `sdd init` and can be edited manually.

```yaml
project_name: my-project

stack:
  language: go
  framework: ""
  build_tool: go
  test_cmd: go test ./...
  lint_cmd: golangci-lint run ./...
  format_cmd: gofumpt -w .
  manifests:
    - go.mod

commands:
  build: go build ./...
  test: go test ./...
  lint: golangci-lint run ./...
  format: gofumpt -w .

skills_path: /home/user/.claude/skills/sdd

capabilities:
  memory_enabled: false
```

### Fields

#### `project_name` (string)

Human-readable project name. Used in explore context under the `PROJECT` section.

#### `stack` (object)

Describes the detected tech stack. Populated by `sdd init`. Can be edited if detection was incorrect.

| Field | Type | Description |
|-------|------|-------------|
| `language` | string | Primary language: `go`, `typescript`, `python`, `rust`, `java`. |
| `framework` | string | Framework name (empty if not detected). |
| `build_tool` | string | Build tool: `go`, `npm`, `pip`, `cargo`, `gradle`, `maven`. |
| `test_cmd` | string | Test command shown in context. |
| `lint_cmd` | string | Lint command shown in context. |
| `format_cmd` | string | Format command shown in context. |
| `manifests` | []string | List of manifest filenames found during detection. |

#### `commands` (object)

Commands executed by `sdd verify`. Empty strings are skipped.

| Field | Type | Description |
|-------|------|-------------|
| `build` | string | Build command. Run first. |
| `test` | string | Test command. Run second. |
| `lint` | string | Lint command. Run third. |
| `format` | string | Format command. Not run by verify; informational. |

Commands run via `sh -c <command>` in the project root directory with a 5-minute timeout per command.

#### `skills_path` (string)

Absolute path to the SDD skills directory. Default: `~/.claude/skills/sdd`. Each phase looks for `<skills_path>/sdd-<phase>/SKILL.md`.

#### `capabilities.memory_enabled` (bool)

Toggle for optional memory features. Defaults to `false`.

### Stack detection priority

`sdd init` scans for manifest files in this order and uses the first match:

| Manifest | Language | Build | Test | Lint | Format |
|----------|----------|-------|------|------|--------|
| `go.mod` | go | `go build ./...` | `go test ./...` | `golangci-lint run ./...` | `gofumpt -w .` |
| `package.json` | typescript | `npm run build` | `npm test` | `npm run lint` | `npm run format` |
| `pyproject.toml` | python | _(empty)_ | `pytest` | `ruff check .` | `ruff format .` |
| `Cargo.toml` | rust | `cargo build` | `cargo test` | `cargo clippy` | `cargo fmt` |
| `build.gradle` | java | `./gradlew build` | `./gradlew test` | _(empty)_ | _(empty)_ |
| `pom.xml` | java | `mvn compile` | `mvn test` | _(empty)_ | _(empty)_ |

In a monorepo with multiple manifests, all detected files are recorded in `stack.manifests`, but only the first match determines the stack and commands. Manually override `commands` in `config.yaml` after `sdd init` if needed.

---

## 12. State Machine

State is persisted in `openspec/changes/<name>/state.json`.

### state.json schema

```json
{
  "name": "add-rate-limiter",
  "description": "Add per-user rate limiting to the API gateway",
  "current_phase": "design",
  "phases": {
    "explore":  "completed",
    "propose":  "completed",
    "spec":     "completed",
    "design":   "pending",
    "tasks":    "pending",
    "apply":    "pending",
    "review":   "pending",
    "verify":   "pending",
    "clean":    "pending",
    "archive":  "pending"
  },
  "created_at": "2026-03-20T10:00:00Z",
  "updated_at": "2026-03-20T12:00:00Z",
  "base_ref": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
}
```

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Change identifier. Must be non-empty. |
| `description` | string | Change intent from `sdd new`. |
| `current_phase` | string | Next phase to work on. Set to `""` when pipeline is complete. |
| `phases` | object | Map of phase name → status. |
| `created_at` | RFC3339 | Timestamp when `sdd new` was run. |
| `updated_at` | RFC3339 | Timestamp of last `sdd write`. Used for stale detection. |
| `base_ref` | string | Git SHA at `sdd new` time. Omitted if git was unavailable. |

### Phase status values

| Value | Meaning |
|-------|---------|
| `pending` | Not yet started. |
| `in_progress` | Started but not completed. (Reserved; not currently set by any command.) |
| `completed` | Artifact promoted via `sdd write`. |
| `skipped` | Phase was intentionally bypassed. (`IsComplete()` treats skipped as done.) |

### Transition graph

```
explore  ──→  propose  ──→  spec   ─┐
                              design ─┤  ← parallel window
                                      ↓
                                    tasks ──→ apply ──→ review ──→ verify ──→ clean ──→ archive
```

### Prerequisites

| Phase | Requires |
|-------|---------|
| `explore` | _(none)_ |
| `propose` | `explore` completed |
| `spec` | `propose` completed |
| `design` | `propose` completed |
| `tasks` | `spec` **and** `design` completed |
| `apply` | `tasks` completed |
| `review` | `apply` completed |
| `verify` | `review` completed |
| `clean` | `verify` completed |
| `archive` | `clean` completed |

### Atomic persistence

`state.json` is written by first writing `state.json.tmp`, then calling `os.Rename`. This ensures the file is never partially written, supporting crash recovery.

### Crash recovery

If `state.json` is missing or corrupt, `state.Recover()` rebuilds state by scanning for known artifact files on disk:

| Phase | Artifact checked |
|-------|-----------------|
| `explore` | `exploration.md` |
| `propose` | `proposal.md` |
| `spec` | `specs/` (non-empty directory) |
| `design` | `design.md` |
| `tasks` | `tasks.md` |
| `review` | `review-report.md` |
| `verify` | `verify-report.md` |

`Recover` is not invoked automatically by any command — it is an available utility for manual repair.

---

## 13. Directory Layout

After `sdd init` and a full pipeline run for change `add-rate-limiter`:

```
openspec/
  config.yaml                          # Project configuration

  changes/
    add-rate-limiter/
      state.json                       # Phase state machine
      exploration.md                   # explore artifact
      proposal.md                      # propose artifact
      specs/
        api-spec.md                    # spec artifacts (one or more)
      design.md                        # design artifact
      tasks.md                         # tasks artifact
      review-report.md                 # review artifact
      verify-report.md                 # verify report (written by Go)
      clean-report.md                  # clean artifact

      .pending/                        # Claude writes here; sdd write promotes
        explore.md
        propose.md
        ...

      .cache/                          # Content-hash cache (gitignore recommended)
        explore.ctx
        explore.hash
        propose.ctx
        propose.hash
        ...
        metrics.json                   # Cumulative pipeline metrics

    archive/
      2026-03-20-143000-add-rate-limiter/
        ...                            # All of the above, preserved
        archive-manifest.md            # Written by sdd archive
```

The `.cache/` directory is auto-created by context assembly. It is safe to delete (cache misses will be assembled fresh). The `.pending/` directory is written by Claude sub-agents and consumed by `sdd write`.
