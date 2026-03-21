# Spec: dev-experience (Phase 3)

Change: dev-experience
Phase: spec
Date: 2026-03-21
Domain: dev-experience

---

## Overview

Delta spec. Three additive improvements to CLI developer ergonomics:

1. **`sdd doctor`** — new read-only diagnostic command; 5 checks; human table by default, `--json` for machine consumption.
2. **Flexible Phase References** — new `ResolvePhase` exported from `internal/state`; accepts full names, unique prefixes, and 0-based integer indexes; consumed by `runContext` and `runWrite`.
3. **Multi-Verbosity Output** — new `Verbosity` type in `internal/cli`; four levels; parsed from `-q`/`-v`/`-d` flags; gates `writeMetrics` output in `internal/context/cache.go`.

No new external dependencies. No changes to existing command JSON output shapes. No state machine or state.json schema changes.

---

## 3.1 `sdd doctor`

### ADDED Requirements

**DR-01** `internal/cli/doctor.go` MUST define:

```go
type CheckResult struct {
    Name    string `json:"name"`
    Status  string `json:"status"`  // "pass", "warn", or "fail"
    Message string `json:"message,omitempty"`
}
```

**DR-02** `internal/cli/doctor.go` MUST define `runDoctor(args []string, stdout, stderr io.Writer) error` that executes the 5 checks below in order and collects their `CheckResult` values.

**DR-03** Check 1 — `config`: call `config.Load(configPath)` where `configPath` is `filepath.Join(cwd, "openspec", "config.yaml")`. On load failure, result MUST be `fail`. On success, if `cfg.Version != config.ConfigVersion`, result MUST be `warn` with message `"config version <N>, expected <M>"`. On success with matching version, result MUST be `pass` with message `"config.yaml v<N> loaded"`.

**DR-04** Check 2 — `cache`: for each active change directory under `openspec/changes/`, read every `.cache/<phase>.hash` file. For each hash file, parse the `"{hex}|{timestamp}"` format and recompute `inputHash` (using the same logic as `tryCachedContext`). If stored hex differs from recomputed hex, count the file as stale. Result MUST be `warn` with message `"<N> stale hash file(s)"` when N > 0, and `pass` with message `"all cache entries current"` when N == 0. This check MUST NOT fail — stale cache is not an error.

**DR-05** Check 3 — `orphaned_pending`: for each change directory under `openspec/changes/`, stat `.pending/`. If `.pending/` exists, check whether each `.md` file within it corresponds to a phase whose artifact has already been promoted (i.e., the `.md` file's base name without extension matches a phase name and the promoted artifact exists at `<changeDir>/<phase>.md`). Count files where the artifact already exists as orphaned. Result MUST be `warn` with message `"<N> orphaned .pending file(s)"` when N > 0, and `pass` when N == 0.

**DR-06** Check 4 — `skills_path`: stat `cfg.SkillsPath`. If the directory does not exist, result MUST be `fail` with message `"skills directory not found: <path>"`. If it exists, check for `sdd-<phase>/SKILL.md` for each phase in `AllPhases()` (10 total). Count how many are readable. Result MUST be `warn` if any are missing, message `"<N>/<total> SKILL.md files present"`. Result MUST be `pass` with message `"<N>/<total> SKILL.md files present"` when all are found.

**DR-07** Check 5 — `build_tools`: for each non-empty command string in `cfg.Commands.Build`, `cfg.Commands.Test`, `cfg.Commands.Lint`, `cfg.Commands.Format`, extract the first whitespace-delimited token and call `exec.LookPath`. Collect each token that is not found. Result MUST be `fail` with message `"command(s) not in PATH: <list>"` when any are missing, and `pass` with message `"all build commands found"` when all are present or all command strings are empty.

**DR-08** `runDoctor` MUST emit a human-readable aligned table to `stdout` by default:

```
sdd doctor
  config            pass   config.yaml v1 loaded
  cache             warn   2 stale hash file(s)
  orphaned_pending  pass
  skills_path       pass   10/10 SKILL.md files present
  build_tools       fail   command(s) not in PATH: golangci-lint
```

Column widths MUST be determined by the longest check name. Each row MUST print `"  <name>  <status>  <message>"` with consistent spacing. When `Message` is empty, the message column is omitted.

**DR-09** With `--json`, `runDoctor` MUST write a single JSON object to `stdout`:

```json
{
  "command": "doctor",
  "status": "fail",
  "checks": [
    {"name": "config", "status": "pass", "message": "config.yaml v1 loaded"},
    {"name": "build_tools", "status": "fail", "message": "command(s) not in PATH: golangci-lint"}
  ]
}
```

The top-level `"status"` MUST be `"fail"` if any check is `fail`, `"warn"` if any check is `warn` and none are `fail`, and `"pass"` otherwise.

**DR-10** Exit code MUST be `0` when no check has status `fail`. Exit code MUST be `1` when any check has status `fail`. Exit code for usage error (unknown flag) MUST be `2`.

**DR-11** `internal/cli/cli.go` MUST add `case "doctor":` in `Run()` calling `runDoctor(rest, stdout, stderr)`.

**DR-12** `internal/cli/cli.go` MUST add `"doctor"` to `commandHelp` with usage text including the `--json` flag.

**DR-13** `internal/cli/cli.go` MUST list `doctor` under "Inspection commands:" in `printHelp()`.

### Scenarios

**WHEN** `config.yaml` does not exist at the expected path
**THEN** the `config` check yields `fail` and the overall exit code is 1

**WHEN** `cfg.Commands.Lint` is `"golangci-lint run ./..."` and `golangci-lint` is not in PATH
**THEN** the `build_tools` check yields `fail` with message `"command(s) not in PATH: golangci-lint"` and overall exit code is 1

**WHEN** all 5 checks pass and `--json` is provided
**THEN** stdout is valid JSON with `"status": "pass"` and exit code is 0

**WHEN** only the `cache` check yields `warn` and all others pass
**THEN** exit code is 0 and `--json` top-level `"status"` is `"warn"`

**WHEN** `cfg.SkillsPath` directory exists but 3 SKILL.md files are absent
**THEN** the `skills_path` check yields `warn` with message `"7/10 SKILL.md files present"` and exit code is 0

---

## 3.2 Flexible Phase References

### ADDED Requirements

**PR-01** `internal/state/resolve.go` MUST define:

```go
func ResolvePhase(input string) (Phase, error)
```

**PR-02** `ResolvePhase` MUST resolve in this order:
1. Exact match (case-sensitive) against `AllPhases()` → return directly.
2. If `input` is a decimal integer string `"0"`–`"9"`, index into `AllPhases()`. Out-of-range MUST return `fmt.Errorf("phase index out of range: %s", input)`.
3. Case-insensitive prefix match against all phase names. If exactly one phase name starts with `input`, return it. If multiple match, return `fmt.Errorf("ambiguous phase prefix %q: matches %s", input, strings.Join(matches, ", "))`. If none match, return `fmt.Errorf("unknown phase: %q", input)`.

**PR-03** The following prefix inputs MUST resolve unambiguously (enforced by tests):

| Input | Resolves to |
|-------|-------------|
| `e`   | `explore`   |
| `p`   | `propose`   |
| `sp`  | `spec`      |
| `d`   | `design`    |
| `t`   | `tasks`     |
| `a`   | `apply`     |
| `r`   | `review`    |
| `v`   | `verify`    |
| `cl`  | `clean`     |
| `ar`  | `archive`   |
| `0`   | `explore`   |
| `9`   | `archive`   |

**PR-04** `internal/cli/commands.go` in `runContext` MUST replace `ph := state.Phase(positional[1])` with:

```go
ph, err := state.ResolvePhase(positional[1])
if err != nil {
    return errs.WriteError(stderr, "context", err)
}
```

**PR-05** `internal/cli/commands.go` in `runWrite` MUST replace `phase := state.Phase(phaseStr)` with:

```go
phase, err := state.ResolvePhase(phaseStr)
if err != nil {
    return errs.WriteError(stderr, "write", err)
}
```

**PR-06** A dedicated test file `internal/state/resolve_test.go` MUST cover: exact match, integer index (in-range and out-of-range), each unambiguous prefix from PR-03, ambiguous prefix input, and unknown input.

### Scenarios

**WHEN** `sdd context my-change p` is called
**THEN** `ResolvePhase("p")` returns `PhasePropose` and context assembly proceeds for the `propose` phase

**WHEN** `sdd write my-change 5` is called
**THEN** `ResolvePhase("5")` returns `PhaseApply` and promotion proceeds for the `apply` phase

**WHEN** `sdd context my-change s` is called and only `spec` starts with `s`
**THEN** `ResolvePhase("s")` returns `PhaseSpec`

**WHEN** `sdd context my-change xyz` is called
**THEN** `ResolvePhase("xyz")` returns an error, stderr contains `"unknown phase"`, and exit code is 1

**WHEN** `sdd context my-change explore` is called (exact match, existing behaviour)
**THEN** `ResolvePhase("explore")` returns `PhaseExplore` — no regression

---

## 3.3 Multi-Verbosity Output

### ADDED Requirements

**VB-01** `internal/cli/verbosity.go` MUST define:

```go
type Verbosity int

const (
    VerbosityDefault Verbosity = iota // 0 — current behavior
    VerbosityVerbose                  // 1 — -v/--verbose
    VerbosityDebug                    // 2 — -d/--debug
)

const VerbosityQuiet Verbosity = -1   // -q/--quiet
```

**VB-02** `internal/cli/verbosity.go` MUST define:

```go
func ParseVerbosityFlags(args []string) (remaining []string, v Verbosity)
```

`ParseVerbosityFlags` MUST scan `args` for `-q`, `--quiet`, `-v`, `--verbose`, `-d`, `--debug`. It MUST remove matched tokens from the returned slice. When multiple conflicting flags appear, the last one wins. It MUST NOT error; unknown flags are passed through untouched.

**VB-03** `internal/context/context.go` `Params` struct MUST add:

```go
Verbosity int // cast from cli.Verbosity; 0=default, -1=quiet, 1=verbose, 2=debug
```

The field uses `int` to avoid an import cycle between `internal/context` and `internal/cli`.

**VB-04** `internal/context/cache.go` `writeMetrics` MUST gate output on verbosity:

- `v == -1` (quiet): MUST NOT write any output to `w`.
- `v == 0` (default): MUST write the current one-line format unchanged.
- `v == 1` (verbose): MUST append `[cache]` or `[assembled]` label to the existing line (it is already present via `source`; confirm the label is visible and not redundant — if the existing format already includes `source`, no change needed for this level).
- `v == 2` (debug): MUST append `hash=<first 8 chars of stored hex>` and full `DurationMs` to the line.

**VB-05** `writeMetrics` MUST accept verbosity. `emitMetrics` in `context.go` MUST pass `p.Verbosity` to `writeMetrics`. The signature change MUST be:

```go
func writeMetrics(w io.Writer, m *contextMetrics, verbosity int)
```

**VB-06** `internal/cli/commands.go` in `runContext` MUST call `ParseVerbosityFlags` before the existing flag-parsing loop and set `Params.Verbosity` accordingly.

**VB-07** `internal/cli/commands.go` in `runNew` MUST call `ParseVerbosityFlags` before the existing flag-parsing loop and propagate `Verbosity` into `Params`.

**VB-08** All callers of `Assemble` that do not set `Verbosity` explicitly receive `0` (default) via zero-value — existing behavior is preserved.

### Scenarios

**WHEN** `sdd context my-change explore --quiet` is called
**THEN** context is written to stdout and stderr receives no output

**WHEN** `sdd context my-change explore --verbose` is called
**THEN** stderr receives the metrics line with cache source label visible

**WHEN** `sdd context my-change explore --debug` is called
**THEN** stderr metrics line includes `hash=` prefix and full `DurationMs`

**WHEN** `sdd context my-change explore` is called (no verbosity flag)
**THEN** stderr output is identical to current behavior

**WHEN** both `-q` and `-v` appear in args (`-q -v`)
**THEN** `ParseVerbosityFlags` returns `VerbosityVerbose` (last flag wins) and the remaining args are unchanged

**WHEN** `-q` is passed to a command that does not call `ParseVerbosityFlags`
**THEN** the flag is treated as an unknown flag by the existing flag parser (no silent swallowing)

---

## Affected Files

| File | Change |
|------|--------|
| `internal/cli/doctor.go` | New — `CheckResult`, `runDoctor`, 5 check functions |
| `internal/cli/verbosity.go` | New — `Verbosity` type, constants, `ParseVerbosityFlags` |
| `internal/state/resolve.go` | New — `ResolvePhase` |
| `internal/cli/cli.go` | Add `case "doctor":`, `commandHelp` entry, `printHelp` line |
| `internal/cli/commands.go` | Wire `ResolvePhase` into `runContext`/`runWrite`; call `ParseVerbosityFlags` in `runContext`/`runNew`; set `Params.Verbosity` |
| `internal/context/context.go` | Add `Verbosity int` to `Params`; pass to `emitMetrics` |
| `internal/context/cache.go` | Update `writeMetrics` signature to accept `verbosity int`; gate output per level |
| `internal/state/resolve_test.go` | New — table-driven tests for `ResolvePhase` |
| `internal/cli/cli_test.go` | New rows for `doctor`, `ParseVerbosityFlags`, verbosity flag round-trips |

---

## Out of Scope

- Auto-repair in `doctor` — diagnose only, never write.
- Adding `doctor` checks for state.json schema validity or phase transition correctness.
- Structured logging framework — `fmt.Fprintf` to stderr remains.
- Progress bars or spinners.
- Verbosity flags on commands other than `context` and `new` in this change.
- Non-integer, non-name phase references (e.g., partial integers, Unicode).

---

## Eval Definitions

| ID | Condition | Pass |
|----|-----------|------|
| DR-01 | `CheckResult` struct defined with correct JSON tags | `grep -n 'CheckResult' internal/cli/doctor.go` returns struct definition |
| DR-02 | `runDoctor` executes all 5 checks | unit test: mock openspec dir, assert 5 results returned |
| DR-03 | Config check detects version mismatch as warn | unit test: config file with version 0 → `warn`; missing file → `fail` |
| DR-04 | Cache check reports stale entries as warn, not fail | unit test: tampered hash file → `warn`; exit code remains 0 |
| DR-05 | Orphaned pending check counts promoted artifacts correctly | unit test: `.pending/apply.md` with `apply.md` present → count 1 |
| DR-06 | Skills path check counts missing SKILL.md files | unit test: dir exists, 7 of 10 present → `warn` with `"7/10"` |
| DR-07 | Build tools check fails on missing binary | unit test: `cfg.Commands.Lint = "notarealex lint"` → `fail` |
| DR-08 | Human table output aligns columns | `sdd doctor` output matches aligned format; name column width = max name length |
| DR-09 | JSON output parses and top-level status reflects worst check | `sdd doctor --json \| jq .status` returns correct aggregated status |
| DR-10 | Exit 0 with only warns; exit 1 with any fail | integration: stale cache → exit 0; missing binary → exit 1 |
| DR-11 | `doctor` case wired in `cli.go` | `grep 'case "doctor"' internal/cli/cli.go` returns one match |
| PR-01 | `ResolvePhase` exported from `internal/state` | `grep 'func ResolvePhase' internal/state/resolve.go` returns one match |
| PR-02 | Resolution order: exact → index → prefix | unit tests: `"apply"` → exact; `"5"` → index; `"ap"` → prefix |
| PR-03 | All documented prefixes resolve unambiguously | `go test ./internal/state/...` passes including prefix table test |
| PR-04 | `runContext` uses `ResolvePhase` | `grep 'ResolvePhase' internal/cli/commands.go` returns ≥2 call sites |
| PR-05 | `runWrite` uses `ResolvePhase` | same grep as PR-04 |
| PR-06 | `resolve_test.go` covers all cases | `go test -run TestResolvePhase ./internal/state/...` passes |
| VB-01 | `Verbosity` type and constants defined correctly | `grep -n 'VerbosityQuiet\|VerbosityDefault\|VerbosityVerbose\|VerbosityDebug' internal/cli/verbosity.go` returns 4 lines |
| VB-02 | `ParseVerbosityFlags` strips matched flags from remaining args | unit test: `["-q", "foo"]` → remaining `["foo"]`, verbosity `-1` |
| VB-03 | `Params.Verbosity` field added | `grep 'Verbosity int' internal/context/context.go` returns one match |
| VB-04 | Quiet suppresses `writeMetrics` output | unit test: `writeMetrics` with verbosity `-1` writes 0 bytes |
| VB-05 | `writeMetrics` signature updated | `grep 'func writeMetrics' internal/context/cache.go` shows `verbosity int` param |
| VB-06 | `runContext` calls `ParseVerbosityFlags` | `grep 'ParseVerbosityFlags' internal/cli/commands.go` returns ≥1 match |
| VB-07 | `runNew` calls `ParseVerbosityFlags` | same grep as VB-06 |
| VB-08 | Zero-value Verbosity preserves existing output | existing `cli_test.go` tests pass without modification |
