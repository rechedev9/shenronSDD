# SDD CLI — Build Roadmap

## Overview

Go binary (`sdd`) that acts as a context engine inside Claude Code. Handles state management, file I/O, artifact assembly, and context compression so Claude Code's token budget goes entirely to reasoning.

**Source:** Design doc + eng review (2026-03-20)

---

## Phase 1: Project Scaffold

- [ ] Initialize Go module (`sdd-cli/`, `go.mod`)
- [ ] Create directory structure:
  ```
  sdd-cli/
    cmd/sdd/main.go
    internal/state/
    internal/context/
    internal/artifacts/
    internal/config/
    internal/verify/
  ```
- [ ] Set up command routing in `main.go` (subcommands: init, new, context, write, status, list, verify, archive)
- [ ] Define shared types: `Phase`, `PhaseStatus`, `State`, `Config`, `Error`
- [ ] Add Makefile with `build`, `install`, `test` targets
- [ ] Structured JSON error output on stderr (all commands)

---

## Phase 2: State Machine

- [ ] `state/state.go` — `State` struct with phase map, `NewState()`, `Load()`, `Save()`
- [ ] Phase transition graph (explore → propose → spec+design → tasks → apply → review → verify → clean → archive)
- [ ] Validation: reject invalid transitions (e.g., explore → apply)
- [ ] Atomic writes: write to `.tmp`, rename
- [ ] Recovery: rebuild state from existing artifacts when `state.json` is missing/corrupt
- [ ] ASCII diagram comment in source documenting valid transitions
- [ ] Tests: valid transitions, invalid transitions, atomic write, recovery, corrupt JSON

---

## Phase 3: Config & Init

- [ ] `config/config.go` — `Config` struct, `Load()`, `Detect()`
- [ ] Stack detection: scan for `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, etc.
- [ ] Infer build/test/lint commands from manifest and ecosystem conventions
- [ ] Auto-detect `skills_path` (default `~/.claude/skills/sdd/`)
- [ ] `sdd init` command: create `openspec/` directory structure, write `config.yaml`
- [ ] PARCER contract assembly: scan SKILL.md files, extract `## PARCER Contract` sections
- [ ] Handle edge cases: no manifest (error), existing openspec/ (warn/skip), `--force` flag
- [ ] Tests: Go project, Node project, Python project, Rust project, no manifest, monorepo

---

## Phase 4: Artifacts

- [ ] `artifacts/reader.go` — `Read(changePath, artifactName)` reads artifact files
- [ ] `artifacts/writer.go` — `Write()` writes to `.pending/{phase}.md`
- [ ] `artifacts/promote.go` — `Promote()` moves `.pending/{phase}.md` → final location, called by `sdd write`
- [ ] `artifacts/list.go` — `List(changePath)` returns existing artifacts
- [ ] `sdd write <name> <phase>` command: promote pending artifact, advance state
- [ ] Handle missing `.pending` file with structured JSON error
- [ ] Tests: write to pending, promote, read, list, missing pending error

---

## Phase 5: Context Assemblers (Planning Phases)

Each assembler: load SKILL.md + relevant artifacts + source context → print to stdout.

- [ ] `context/context.go` — dispatcher: resolve phase → call assembler
- [ ] `context/explore.go` — `git ls-files` for file tree, load config, load sdd-explore SKILL.md
- [ ] `context/propose.go` — load exploration.md, strip to key findings, load sdd-propose SKILL.md
- [ ] `context/spec.go` — load proposal.md, extract scope/requirements, load sdd-spec SKILL.md
- [ ] `context/design.go` — load spec files, extract MUST/SHOULD requirements, load sdd-design SKILL.md
- [ ] `context/tasks.go` — load spec + design, load sdd-tasks SKILL.md
- [ ] `sdd context <name> [phase]` command: resolve current phase (or use specified), run assembler
- [ ] `sdd new <name> "<desc>"` command: create change dir, write initial state, run explore assembler
- [ ] Tests: each assembler with sample artifact fixtures

---

## Phase 6: Context Assemblers (Implementation & Review Phases)

- [ ] `context/apply.go` — load tasks.md (current incomplete task only) + target file contents, load sdd-apply SKILL.md
- [ ] `context/review.go` — load spec + design + `git diff` of changed files, load sdd-review SKILL.md
- [ ] `context/clean.go` — load review report issues + affected file contents, load sdd-clean SKILL.md
- [ ] Tests: each assembler with sample fixtures

---

## Phase 7: Go-Native Phases (Zero Tokens)

- [ ] `verify/verify.go` — run typecheck/lint/test commands from config.yaml
- [ ] Parse command output: extract pass/fail, error count, first N error lines
- [ ] Command timeout handling (prevent hangs)
- [ ] Write pass report or failure report to `verify-report.md`
- [ ] `sdd verify <name>` command
- [ ] Archive logic: move `openspec/changes/{name}/` → `openspec/changes/archive/{date}-{name}/`
- [ ] Write `archive-manifest.md` listing all artifacts
- [ ] `sdd archive <name>` command
- [ ] Tests: all pass, one fails, timeout, archive flow

---

## Phase 8: Status & List Commands

- [ ] `sdd status [name]` — read state.json, display current phase, completed phases, next phase
- [ ] `sdd list` — scan `openspec/changes/`, show active changes with current phase
- [ ] Handle no active changes, multiple changes, archived changes
- [ ] Tests: status output, list with 0/1/multiple changes

---

## Phase 9: Integration Tests

- [ ] End-to-end flow in temp directory:
  1. `sdd init` → verify openspec/ + config.yaml
  2. `sdd new test-feature "desc"` → verify change dir + state + context output
  3. Write to `.pending/explore.md` → `sdd write test-feature explore` → verify promotion + state advance
  4. `sdd status test-feature` → verify phase display
  5. `sdd list` → verify listing
  6. `sdd verify test-feature` → verify report (with mock commands)
  7. `sdd archive test-feature` → verify archive
- [ ] Edge cases: special chars in name, wrong phase write, missing prior phase

---

## Phase 10: Rewrite Slash Commands

- [ ] Rewrite `commands/sdd-init.md` — thin wrapper: run `sdd init`, show output
- [ ] Rewrite `commands/sdd-new.md` — run `sdd new`, feed context to Claude, Claude writes to `.pending/`, run `sdd write`
- [ ] Rewrite `commands/sdd-continue.md` — run `sdd status` to get next phase, run `sdd context`, feed to Claude, run `sdd write`
- [ ] Rewrite `commands/sdd-apply.md` — run `sdd context apply`, Claude implements with tools, run `sdd write`
- [ ] Rewrite `commands/sdd-review.md` — run `sdd context review`, Claude reviews, writes to `.pending/`
- [ ] Rewrite `commands/sdd-verify.md` — run `sdd verify` (zero tokens if green)
- [ ] Rewrite `commands/sdd-clean.md` — run `sdd context clean`, Claude cleans, writes to `.pending/`
- [ ] Rewrite `commands/sdd-archive.md` — run `sdd archive` (zero tokens)
- [ ] Rewrite `commands/sdd-ff.md` — sequential: context+write for explore→propose→spec→design→tasks
- [ ] Rewrite `commands/sdd-explore.md` — standalone explore using `sdd context explore`
- [ ] Rewrite `commands/sdd-analytics.md` — run `sdd analytics` (zero tokens, post-MVP stub)

---

## Phase 11: Polish & Install

- [ ] Update `install.sh` to build and install the Go binary
- [ ] Add `sdd` to PATH (`~/.local/bin/sdd`)
- [ ] Verify end-to-end on a real project (not just test fixtures)
- [ ] Update README.md with new CLI usage
- [ ] Final pass: error messages, help text, `sdd --help` and `sdd <cmd> --help`

---

## Post-MVP (from TODOS.md)

- [ ] SKILL.md compression — strip irrelevant sections per project stack
- [ ] Token usage tracking — `sdd write --tokens N`, `sdd analytics`
- [ ] Phase caching — content-hash based, explore results cached

---

## Key Decisions (from Eng Review)

| Decision | Choice |
|---|---|
| Write pattern | `.pending/{phase}.md` — Claude writes, Go promotes |
| Skills path | `skills_path` field in `config.yaml` |
| Init detection | Deterministic Go code, zero tokens |
| Native phases | verify + archive fully in Go |
| State safety | Atomic writes + artifact-based recovery |
| Code organization | One assembler file per phase in `context/` |
| Error format | Structured JSON on stderr |
| File scanning | `git ls-files` for performance |
