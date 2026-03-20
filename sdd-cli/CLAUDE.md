# CLAUDE.md — sdd-cli (Go)

## Project

Go binary (`sdd`) — context engine for SDD workflow inside Claude Code. Handles state management, file I/O, artifact assembly, and context compression.

Module: `github.com/rechedev9/shenronSDD/sdd-cli`

## Architecture

Thin shell, fat core. `cmd/sdd/main.go` is ≤15 lines — delegates to `internal/cli.Run()`.

```
cmd/sdd/main.go           # Entry point only
internal/
  cli/                     # Command routing, subcommand dispatch
  state/                   # Phase state machine, atomic persistence
  context/                 # Per-phase context assemblers
  artifacts/               # Read/write/promote .pending artifacts
  config/                  # Config loading, stack detection
  verify/                  # Build/test/lint runner, timeout handling
```

## Go Rules (REQUIRED)

### Structure
- `cmd/` + `internal/` layout — no `pkg/`, no flat layout
- One file per concern: `types.go`, `errors.go`, `client.go`, `db.go`
- Package-per-context: `internal/state/`, `internal/config/` — not `internal/utils/`

### Interfaces
- **Consumer-defined only.** Define interfaces where they're consumed, not where they're implemented
- Minimal surface — only the methods the consumer actually calls
- No interface unless there are 2+ implementations (real + fake) or a clear testing boundary

### Error Handling
- `fmt.Errorf("verb noun: %w", err)` at every call site — never naked `return err`
- Structured JSON errors on stderr for all commands (machine-readable)
- Custom error types only where callers branch on error kind
- Sentinel errors (`var ErrX = errors.New(...)`) only for package-level conditions

### Testing
- Table-driven tests with `t.Run()` subtests
- Hand-written fakes — no gomock, no mockgen
- `t.TempDir()` for file/DB tests
- `t.Parallel()` where safe
- `testify/require` permitted for assertions

### Dependencies
- Standard library first — no frameworks for things Go stdlib handles
- `cobra` for CLI routing (already chosen per roadmap)
- No ORM, no DI container, no mock framework
- `*http.Client` with explicit timeouts when needed
- Pure Go SQLite (`modernc.org/sqlite`) if storage is needed — CGO_ENABLED=0

### Output
- All commands: structured JSON on stderr for errors
- Human-readable stdout for success output (context, status, lists)
- Exit codes: 0 success, 1 general error, 2 usage error

### Build
- `CGO_ENABLED=0` for all builds
- Version via ldflags: `-X github.com/rechedev9/shenronSDD/sdd-cli/internal/cli.version`
- `gofumpt` for formatting, `golangci-lint` for linting
- Makefile targets: `build`, `install`, `test`, `lint`, `fmt`, `check`

## Framework Skill

Load `~/.claude/skills/frameworks/go-shenron/SKILL.md` when writing Go code in this project.

## Subcommands

`sdd <command>`:
- `init` — create openspec/, detect stack, write config.yaml
- `new <name> "<desc>"` — create change dir, initial state, run explore assembler
- `context <name> [phase]` — resolve phase, run assembler, print to stdout
- `write <name> <phase>` — promote .pending artifact, advance state
- `status [name]` — display current phase, completed/next phases
- `list` — scan openspec/changes/, show active changes
- `verify <name>` — run typecheck/lint/test, write report
- `archive <name>` — move to archive, write manifest
