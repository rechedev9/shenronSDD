# CLAUDE.md — SDDworkflow

## Project

SDD (Spec-Driven Development) workflow for Claude Code. Two components:
- `sdd-cli/` — Go binary context engine (state, artifacts, context assembly, verify, archive)
- `skills/`, `commands/` — SKILL.md phase skills + slash command definitions

## Agent Protocol
- Docs: run `scripts/docs-list` before deep work; honor `read_when` hints.
- Keep files <500 LOC; split when exceeded.
- Load `sdd-cli/CLAUDE.md` when working on Go code.
- Load `~/.claude/skills/frameworks/go-shenron/SKILL.md` when writing Go code.

## Git
- Commit helper: `scripts/committer "type(scope): message" file1 file2`.
- Conventional Commits: `feat|fix|refactor|docs|test|chore|perf|build|ci|style(scope): summary`.
- Atomic commits: one concern per commit; list each path explicitly.
- No amend unless asked. Use `trash` for deletions, never `rm`.

## Build / Test (sdd-cli)
- Full gate: `cd sdd-cli && CGO_ENABLED=0 go build ./... && go test ./... && go vet ./...`
- Quick check: `cd sdd-cli && go build ./... && go test ./internal/cli/ -count=1`
- Single test: `cd sdd-cli && go test ./internal/verify/ -run TestRun_AllPass -v`
- Format: `gofmt -l internal/` (pre-existing files may have gofmt issues — only fix files you modify)

## Architecture
- `sdd-cli/cmd/sdd/` — entry point (13 lines → cli.Run)
- `sdd-cli/internal/cli/` — command routing + 11 subcommands
- `sdd-cli/internal/state/` — phase state machine, atomic persistence, recovery
- `sdd-cli/internal/context/` — per-phase assemblers + cache + metrics + Context Cascade
- `sdd-cli/internal/artifacts/` — .pending write/promote/read/list
- `sdd-cli/internal/config/` — stack detection + config.yaml
- `sdd-cli/internal/verify/` — build/lint/test runner + archive
- `skills/sdd/` — 11 SKILL.md phase skills (loaded by context assemblers)
- `commands/` — slash command definitions (use sdd CLI under the hood)
- `presentation/` — React slideshow (separate, not part of CLI)

## SDD Pipeline
```
explore → propose → spec + design (parallel) → tasks → apply → review → verify → clean → archive
```
- `sdd context` assembles (cached, 0 tokens). `sdd write` promotes + advances state.
- `sdd verify` and `sdd archive` are zero-token Go operations.
- Context Cascade carries cumulative summary through all phases.

## Session Continuity
- `/handoff`: read `docs/handoff.md` — dump state for next session.
- `/pickup`: read `docs/pickup.md` — rehydrate context.
- `sdd status <name>` and `sdd health <name>` provide pipeline state for handoff/pickup.

## Docs Convention
- Every `docs/*.md` needs YAML front-matter: `summary` + `read_when`.
- Run `scripts/docs-list` to verify compliance.
