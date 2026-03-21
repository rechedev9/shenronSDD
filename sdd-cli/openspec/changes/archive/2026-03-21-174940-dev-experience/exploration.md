# Exploration: Developer Experience (Phase 3)

**Date**: 2026-03-21
**Detail Level**: standard
**Change Name**: dev-experience

## Current State

The CLI dispatches commands via a manual `switch` in `cli.go:29-59`. Phase arguments are raw strings cast to `state.Phase` with no validation or abbreviation support. All commands output JSON to stdout and metrics/errors to stderr, but there's no verbosity control — every command always emits the same level of detail. No `doctor` command exists for health/integrity checks.

## Relevant Files

| File Path | Purpose | Lines | Complexity | Test Coverage |
|-----------|---------|-------|------------|---------------|
| `internal/cli/cli.go` | Command dispatch, help text, `commandHelp` map | 261 | low | yes |
| `internal/cli/commands.go` | All command handlers (`runInit`, `runNew`, etc.) | 919 | medium | yes |
| `internal/state/types.go` | `Phase` type, `AllPhases()`, phase constants | 88 | low | yes |
| `internal/config/config.go` | `Load()`, `Detect()`, `ConfigVersion` | 145 | low | yes |
| `internal/context/cache.go` | `cacheVersion`, cache paths, hash validation | 335 | medium | yes |
| `internal/context/context.go` | `Assemble()`, `emitMetrics()`, `writeMetrics()` | 196 | medium | yes |
| `internal/cli/errs/errs.go` | Error types, JSON error output | 84 | low | yes |

## Risk Assessment

| Dimension | Level | Notes |
|-----------|-------|-------|
| Blast radius | medium | 3.2 touches every command that accepts a phase arg; 3.3 touches stderr output in context.go |
| Type safety | low | Phase is a string type with constants |
| Test coverage | medium | cli_test.go and integration_test.go exist |
| Coupling | low | Each feature is additive |
| Complexity | low | String matching, exec.LookPath, fprintf |
| Data integrity | low | doctor is read-only; phase resolution is stateless |
| Breaking changes | low | All features are additive; existing behavior preserved |
| Security surface | low | No user input beyond CLI args |

## Approach

### 3.1 `sdd doctor`
New `runDoctor` command in `commands.go`. Runs 5 checks:
1. Config: load + version check
2. Cache: verify `.hash` files match recomputed hashes
3. Artifacts: scan for orphaned `.pending/` files across all changes
4. Skills: check `skillsPath` exists and has expected SKILL.md files
5. Build tools: `exec.LookPath` for build/test/lint commands from config

Output: JSON with `checks[]` array, each with `name`, `status` (pass/fail/warn), `message`. Supports `--json` flag (default behavior is human-friendly table).

### 3.2 Flexible Phase References
Add `resolvePhase(input string) (state.Phase, error)` to `state` package. Accepts:
- Full name: `propose` → `propose`
- Unique prefix: `p` → `propose`, `sp` → `spec`, `d` → `design`, `t` → `tasks`, `a` → `apply`, `r` → `review`, `v` → `verify`, `cl` → `clean`, `ar` → `archive`
- Index: `0` → `explore`, `1` → `propose`, ..., `9` → `archive`
- Ambiguous prefix → error listing matches

Wire into `runContext` and `runWrite` where `state.Phase(args[N])` is currently used.

### 3.3 Multi-Verbosity Output
Add a `Verbosity` type to `cli` package with levels: `Quiet`, `Default`, `Verbose`, `Debug`. Parse from flags: `-q`, `-v`, `-d` (or `--quiet`, `--verbose`, `--debug`). Pass via `Params.Verbosity` to assemblers. `writeMetrics()` respects verbosity — quiet suppresses all stderr, verbose adds cache hit/miss detail, debug adds full assembly trace.

## Recommendation

All three items are independent. Implement in parallel:
- 3.1: New file `internal/cli/doctor.go` + wire in `cli.go`
- 3.2: New function in `internal/state/resolve.go` + update `commands.go`
- 3.3: New type in `internal/cli/verbosity.go` + update `context.go` and `commands.go`

No blocking questions.
