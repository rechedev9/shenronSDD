# Proposal: Developer Experience (Phase 3)

## Intent

Three independent, additive improvements to the CLI's developer ergonomics: a `doctor` diagnostic command for self-checking the workspace, flexible phase references so users can type `p` instead of `propose`, and multi-verbosity output levels so automation can silence stderr while humans can request more detail.

---

## Scope

### In

- `sdd doctor` — new command; 5 read-only checks; human-friendly table by default, `--json` for machine consumption.
- `resolvePhase(input string) (state.Phase, error)` — new function in `internal/state`; accepts full names, unique prefixes, and 0-based integer indexes.
- `Verbosity` type in `internal/cli` — 4 levels (`Quiet`, `Default`, `Verbose`, `Debug`); parsed from `-q`/`-v`/`-d` flags; propagated via `Params`; respected by `writeMetrics`.
- Tests for all three features in `cli_test.go` and `internal/state/resolve_test.go`.

### Out

- No changes to existing command JSON output shapes.
- No changes to state machine logic or phase ordering.
- No auto-repair in `doctor` — diagnose only.
- No structured logging framework — plain `fmt.Fprintf` to stderr stays.
- No interactive UI for verbosity (no progress bars, spinners).

---

## Approach

### 3.1 `sdd doctor`

New file: `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/doctor.go`

`runDoctor(args []string, stdout, stderr io.Writer) error` executes 5 checks in order:

| # | Name | What it checks |
|---|------|---------------|
| 1 | `config` | `config.Load(configPath)` succeeds and `ConfigVersion` matches expected |
| 2 | `cache` | `.hash` files in each change's `.cache/` directory match recomputed SHA-256 of the cached artifact |
| 3 | `orphaned_pending` | `.pending/` directory exists and contains `.md` files with no corresponding promoted artifact; counts per change |
| 4 | `skills_path` | `cfg.SkillsPath` directory exists and each expected `<phase>/SKILL.md` is readable |
| 5 | `build_tools` | `exec.LookPath` for each command in `cfg.Commands.Build`, `cfg.Commands.Lint`, `cfg.Commands.Test` |

Each check yields a `CheckResult{Name, Status, Message}` where `Status` is `"pass"`, `"warn"`, or `"fail"`.

Default (human) output — aligned table:
```
sdd doctor
  config          pass   config.yaml v1 loaded
  cache           warn   2 stale hash files in dev-experience/.cache
  orphaned_pending pass
  skills_path     pass   8/8 SKILL.md files present
  build_tools     fail   lint command 'golangci-lint' not found in PATH
```
Exit 0 if no `fail`; exit 1 if any `fail`; exit 0 with warnings if only `warn`.

With `--json`:
```json
{
  "command": "doctor",
  "status": "fail",
  "checks": [
    {"name": "config", "status": "pass", "message": "config.yaml v1 loaded"},
    {"name": "build_tools", "status": "fail", "message": "lint command 'golangci-lint' not found in PATH"}
  ]
}
```

Wire into `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/cli.go`:
- `case "doctor":` in `Run()` switch calling `runDoctor`.
- `"doctor"` entry in `commandHelp` map.
- `doctor` line in `printHelp()` under Inspection commands.

### 3.2 Flexible Phase References

New file: `/home/reche/projects/SDDworkflow/sdd-cli/internal/state/resolve.go`

```go
// resolvePhase accepts a full phase name, a unique prefix, or a 0-based integer
// index into AllPhases(). Returns an error for unknown or ambiguous inputs.
func resolvePhase(input string) (Phase, error)
```

Resolution rules (applied in order):

1. Exact match against `AllPhases()` → return directly.
2. Integer string `"0"`–`"9"` → index into `AllPhases()` (0=explore … 9=archive). Out-of-range → error.
3. Prefix match (case-insensitive) against all phase names → if exactly one match, return it; if multiple, error listing all matches.

Documented abbreviations (all unambiguous):

| Input | Resolves to |
|-------|------------|
| `p` | `propose` |
| `sp` | `spec` |
| `d` | `design` |
| `t` | `tasks` |
| `a` | `apply` |
| `r` | `review` |
| `v` | `verify` |
| `cl` | `clean` |
| `ar` | `archive` |
| `e` | `explore` |

Note: `s` is ambiguous (`spec`/`spec` is only one — actually unambiguous; but documented in tests regardless).

Wire into `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go`:

- In `runContext` (line 248): replace `ph := state.Phase(positional[1])` with `ph, err := state.ResolvePhase(positional[1])`.
- In `runWrite` (line 312): replace `phase := state.Phase(phaseStr)` with `phase, err := state.ResolvePhase(phaseStr)`.

`resolvePhase` is unexported within `state`; export as `ResolvePhase` since `commands.go` is in package `cli`.

### 3.3 Multi-Verbosity Output

New file: `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/verbosity.go`

```go
type Verbosity int

const (
    VerbosityQuiet   Verbosity = iota // -q: suppress all stderr output
    VerbosityDefault                  // (no flag): current behaviour
    VerbosityVerbose                  // -v: cache hit/miss detail
    VerbosityDebug                    // -d: full assembly trace
)

// ParseVerbosityFlags scans args for -q/-v/-d/--quiet/--verbose/--debug,
// removes them from the slice, and returns the effective Verbosity.
func ParseVerbosityFlags(args []string) ([]string, Verbosity)
```

Update `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go`:
- Add `Verbosity int` field to `Params` (using `int` to avoid an import cycle; caller casts from `cli.Verbosity`).
- In `emitMetrics`: pass verbosity down to `writeMetrics`.
- In `writeMetrics` (currently in `cache.go`): gate on verbosity level — `Quiet` skips the write entirely, `Verbose` appends cache source (`[cache]`/`[assembled]`), `Debug` appends duration + hash.

Update `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go`:
- In `runContext` and `runNew`, call `ParseVerbosityFlags` before flag parsing loop, inject `Verbosity` into `Params`.

---

## Affected Files

| File | Change |
|------|--------|
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/cli.go` | Add `doctor` case, help entry, printHelp line |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/doctor.go` | New file — `runDoctor`, `CheckResult`, 5 check functions |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go` | Wire `ResolvePhase` into `runContext`/`runWrite`; call `ParseVerbosityFlags` in `runContext`/`runNew` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/verbosity.go` | New file — `Verbosity` type, `ParseVerbosityFlags` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/state/resolve.go` | New file — `ResolvePhase` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go` | Add `Verbosity int` to `Params`; thread through `emitMetrics`/`writeMetrics` |

---

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| `resolvePhase` prefix collision on future phase additions | Low | Ambiguous resolution breaks existing scripts | Error message lists matches; scripts using full names are unaffected |
| `Verbosity int` in `Params` silently defaults to 0 (Quiet) if not set | Medium | Metrics suppressed unexpectedly | Define `VerbosityQuiet = 0` is intentional; callers that don't set it get `Quiet` — or reorder constants so `Default = 0`; use `Default = 0` in final impl |
| `doctor` cache check recomputes hashes on every invocation | Low | Slow on large caches | Cap at 50 hash files per change; warn if more exist |
| `exec.LookPath` result varies by shell PATH | Low | False `fail` for tools installed in non-standard locations | Message includes the looked-up command name so users can debug |

---

## Rollback

All three features are purely additive. Rollback per feature:

- **3.1 doctor**: Remove `case "doctor":` from `cli.go`, delete `internal/cli/doctor.go`, remove `commandHelp` and `printHelp` entries. No state changes; no data to migrate.
- **3.2 phase resolution**: Revert `runContext`/`runWrite` to `state.Phase(positional[N])` casts, delete `internal/state/resolve.go`. Existing calls with full phase names continue to work unchanged.
- **3.3 verbosity**: Remove `Verbosity int` from `Params`, revert `writeMetrics` to unconditional write, delete `internal/cli/verbosity.go`. Existing `-q`/`-v`/`-d` flags become unknown-flag errors again — acceptable since no one depends on them yet.

No database, no config-file format change, no state.json schema change.

---

## Success Criteria

- `sdd doctor` exits 0 on a clean workspace, 1 when any check fails; `--json` output parses without error.
- `sdd context <name> p` assembles the `propose` phase; `sdd write <name> 1` promotes `propose.md`.
- `sdd context <name> --quiet` produces context on stdout with no stderr output; `sdd context <name> --verbose` includes cache hit/miss detail on stderr.
- All existing `cli_test.go` and integration tests pass without modification.
- New tests cover: `ResolvePhase` with full names, prefixes, indexes, ambiguous input; `doctor` with a synthesised openspec dir; `ParseVerbosityFlags` with all flag forms.
