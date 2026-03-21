# Tasks: dev-experience

Phase 3 developer experience: sdd doctor, flexible phase refs, multi-verbosity.

---

## Phase 1: Core Types

- [ ] Create `internal/state/resolve.go` — define `ResolvePhase(input string) (Phase, error)` with exact-match → index → prefix resolution logic; return error on ambiguous or unknown input
- [ ] Create `internal/cli/verbosity.go` — define `Verbosity` type with constants `VerbosityQuiet(-1)`, `VerbosityDefault(0)`, `VerbosityVerbose(1)`, `VerbosityDebug(2)`; implement `ParseVerbosityFlags(args []string) ([]string, Verbosity)` stripping `-q`/`--quiet`, `-v`/`--verbose`, `-d`/`--debug`
- [ ] Create `internal/cli/doctor.go` — define `CheckResult` struct with `Name string`, `Status string`, `Message string` fields

---

## Phase 2: Doctor Command Implementation

- [ ] Implement `checkConfig(cwd string) CheckResult` in `internal/cli/doctor.go` — load `openspec/config.yaml` via `config.Load`, verify `cfg.Version == config.ConfigVersion`, set Status `"ok"` or `"error"`
- [ ] Implement `checkCache(cwd string, cfg *config.Config) CheckResult` in `internal/cli/doctor.go` — scan active changes under `openspec/changes/`, for each `.cache/*.hash` file recompute hash via `context.inputHash` equivalent and compare; report mismatches
- [ ] Implement `checkPending(cwd string) CheckResult` in `internal/cli/doctor.go` — scan each active change's `.pending/` dir; flag any `<phase>.md` files where that phase is already `StatusCompleted` in `state.json`
- [ ] Implement `checkSkills(cfg *config.Config) CheckResult` in `internal/cli/doctor.go` — stat `cfg.SkillsPath`, then check for `sdd-{phase}/SKILL.md` for each phase in `state.AllPhases()` (excluding `archive`); report missing files
- [ ] Implement `checkTools(cfg *config.Config) CheckResult` in `internal/cli/doctor.go` — call `exec.LookPath` for the first token of each non-empty command in `cfg.Commands.Build`, `.Test`, `.Lint`, `.Format`; report any not found
- [ ] Implement `runDoctor(args []string, stdout, stderr io.Writer) error` in `internal/cli/doctor.go` — parse `--json` flag, run all 5 checks, output aligned table to stdout (columns: name, status, message) or JSON array; return non-nil error if any check status is `"error"`

---

## Phase 3: Integration

- [ ] Wire `doctor` into `internal/cli/cli.go` — add `case "doctor": return runDoctor(rest, stdout, stderr)` to the switch; add help text entry to `commandHelp` map; add `doctor` line to `printHelp` under "Inspection commands"
- [ ] Update `runContext` in `internal/cli/commands.go` — replace `state.Phase(positional[1])` with a `state.ResolvePhase(positional[1])` call; propagate the error via `errs.WriteError`
- [ ] Update `runWrite` in `internal/cli/commands.go` — replace `state.Phase(phaseStr)` with `state.ResolvePhase(phaseStr)`; propagate the error via `errs.WriteError`
- [ ] Update `runContext` and `runNew` in `internal/cli/commands.go` — call `ParseVerbosityFlags` at the top of each function to strip verbosity flags before existing flag parsing; store the returned `Verbosity`
- [ ] Add `Verbosity int` field to `context.Params` struct in `internal/context/context.go`
- [ ] Update `writeMetrics` in `internal/context/cache.go` to respect `Verbosity`: suppress output when `VerbosityQuiet`, emit extra detail (bytes, duration, cache status) when `VerbosityVerbose` or `VerbosityDebug`; thread `Verbosity` through `emitMetrics` → `writeMetrics` call chain

---

## Phase 4: Tests

- [ ] Create `internal/state/resolve_test.go` — table-driven tests covering: exact phase name match, numeric index (e.g. `"0"` → `PhaseExplore`), unambiguous prefix (e.g. `"exp"` → `PhaseExplore`), ambiguous prefix returns error, unknown string returns error
- [ ] Create `internal/cli/verbosity_test.go` — table-driven tests for `ParseVerbosityFlags`: `-q` strips flag and returns `VerbosityQuiet`, `-v` returns `VerbosityVerbose`, `-d` returns `VerbosityDebug`, no flags returns `VerbosityDefault`, mixed positional+flag args strips only flags, unknown flags pass through untouched
- [ ] Create `internal/cli/doctor_test.go` — unit tests for each check function: `checkConfig` with valid and missing/corrupt config, `checkPending` with orphaned and clean pending dirs, `checkSkills` with missing and present skills dirs, `checkTools` with a known-present binary and a nonexistent command; `checkCache` can be tested with a temp change dir containing pre-written `.hash` files

---

## Phase 5: Verify

- [ ] Run `go build ./...` — confirm zero errors
- [ ] Run `go vet ./...` — confirm zero warnings
- [ ] Run `go test ./internal/state/... ./internal/cli/...` — confirm all new tests pass
- [ ] Manual smoke: `sdd doctor`, `sdd context <name> exp` (prefix resolution), `sdd context <name> 0` (index resolution), `sdd context -q <name>` (quiet), `sdd context -v <name>` (verbose)
