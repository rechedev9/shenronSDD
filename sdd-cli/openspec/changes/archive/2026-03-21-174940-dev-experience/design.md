# Design: dev-experience (Phase 3)

Change: dev-experience
Phase: design
Date: 2026-03-21

---

## Overview

Three additive features. No new external dependencies. No changes to existing JSON output shapes or state machine schema. All changes are backward-compatible at the binary interface.

---

## File Changes

| Absolute Path | Action | Summary |
|---|---|---|
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/doctor.go` | New | `CheckResult`, `runDoctor`, 5 check functions |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/verbosity.go` | New | `Verbosity` type, constants, `ParseVerbosityFlags` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/state/resolve.go` | New | `ResolvePhase` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/cli.go` | Edit | Add `case "doctor":`, `commandHelp["doctor"]`, `printHelp` line |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go` | Edit | Wire `ResolvePhase` in `runContext`/`runWrite`; call `ParseVerbosityFlags` in `runContext`/`runNew`; set `p.Verbosity` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/context.go` | Edit | Add `Verbosity int` to `Params`; pass to `emitMetrics` |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/context/cache.go` | Edit | Update `writeMetrics` to accept `verbosity int`; gate output per level |
| `/home/reche/projects/SDDworkflow/sdd-cli/internal/state/resolve_test.go` | New | Table-driven tests for `ResolvePhase` |

---

## 3.1 `sdd doctor`

### File: `internal/cli/doctor.go`

#### Types

```go
type CheckResult struct {
    Name    string `json:"name"`
    Status  string `json:"status"`  // "pass", "warn", "fail"
    Message string `json:"message,omitempty"`
}
```

#### `runDoctor` structure

```go
func runDoctor(args []string, stdout, stderr io.Writer) error {
    jsonOut := false
    for _, arg := range args {
        switch {
        case arg == "--json":
            jsonOut = true
        case !strings.HasPrefix(arg, "-"):
            return errs.Usage(fmt.Sprintf("unexpected argument: %s", arg))
        default:
            return errs.Usage(fmt.Sprintf("unknown flag: %s", arg))
        }
    }

    cwd, err := os.Getwd()
    if err != nil {
        return errs.WriteError(stderr, "doctor", fmt.Errorf("get working directory: %w", err))
    }

    configPath := filepath.Join(cwd, "openspec", "config.yaml")
    changesDir := filepath.Join(cwd, "openspec", "changes")

    checks := []CheckResult{
        checkConfig(configPath),
        checkCache(changesDir),
        checkOrphanedPending(changesDir),
        checkSkillsPath(cfg),       // see below — cfg threaded from checkConfig
        checkBuildTools(cfg),
    }
    // ... output
}
```

**Design choice**: `checkConfig` returns both a `CheckResult` and a `*config.Config`. All subsequent checks need the config. If `checkConfig` fails, subsequent checks that need config must handle `cfg == nil` gracefully (degrade to `warn` or `fail` with a clear message like `"skipped: config unavailable"`).

```go
func checkConfig(configPath string) (CheckResult, *config.Config) { ... }
```

The internal helper signature is not exported. `runDoctor` assembles the slice:

```go
configResult, cfg := checkConfig(configPath)
checks := []CheckResult{
    configResult,
    checkCache(changesDir),
    checkOrphanedPending(changesDir),
    checkSkillsPath(cfg),
    checkBuildTools(cfg),
}
```

#### Check 1 — config

```go
func checkConfig(configPath string) (CheckResult, *config.Config) {
    cfg, err := config.Load(configPath)
    if err != nil {
        return CheckResult{Name: "config", Status: "fail", Message: err.Error()}, nil
    }
    if cfg.Version != config.ConfigVersion {
        msg := fmt.Sprintf("config version %d, expected %d", cfg.Version, config.ConfigVersion)
        return CheckResult{Name: "config", Status: "warn", Message: msg}, cfg
    }
    msg := fmt.Sprintf("config.yaml v%d loaded", cfg.Version)
    return CheckResult{Name: "config", Status: "pass", Message: msg}, cfg
}
```

`config.ConfigVersion` is already defined as `const ConfigVersion = 1` in `internal/config/config.go`.

#### Check 2 — cache

Algorithm: scan `changesDir` for subdirs (excluding `archive`). For each change dir, glob `<changeDir>/.cache/*.hash`. For each hash file:
1. Read content, parse `"{hex}|{timestamp}"` with `strings.Cut(stored, "|")`.
2. Derive phase name from file base (strip `.hash`).
3. Look up `phaseInputs[phase]` from `internal/context/cache.go` — **problem**: `phaseInputs` is package-private.

**Decision**: `phaseInputs` and `inputHash` are unexported in `internal/context`. The doctor check cannot call them directly from `internal/cli` without either exporting them or duplicating logic.

**Option A**: Export `InputHash(changeDir string, inputs []string, skillsPath, phaseName string) string` and `PhaseInputs() map[string][]string` from `internal/context`.

**Option B**: Add a single exported function `CheckCacheIntegrity(changeDir, skillsPath string) (staleCount int, err error)` to `internal/context`, keeping internals private.

**Chosen: Option B.** Single export, narrower surface. The doctor check calls:

```go
// In internal/context/cache.go (new exported function)
func CheckCacheIntegrity(changeDir, skillsPath string) (int, error) {
    stale := 0
    hashFiles, err := filepath.Glob(filepath.Join(cacheDir(changeDir), "*.hash"))
    if err != nil || len(hashFiles) == 0 {
        return 0, nil
    }
    for _, hf := range hashFiles {
        phase := strings.TrimSuffix(filepath.Base(hf), ".hash")
        raw, err := os.ReadFile(hf)
        if err != nil {
            continue
        }
        stored := strings.TrimSpace(string(raw))
        storedHash, _, ok := strings.Cut(stored, "|")
        if !ok {
            stale++
            continue
        }
        inputs := phaseInputs[phase]
        current := inputHash(changeDir, inputs, skillsPath, phase)
        if storedHash != current {
            stale++
        }
    }
    return stale, nil
}
```

Doctor check:

```go
func checkCache(changesDir string) CheckResult {
    entries, err := os.ReadDir(changesDir)
    if err != nil {
        return CheckResult{Name: "cache", Status: "warn", Message: "cannot read changes dir"}
    }
    total := 0
    for _, e := range entries {
        if !e.IsDir() || e.Name() == "archive" {
            continue
        }
        changeDir := filepath.Join(changesDir, e.Name())
        // skillsPath: load from state's config if available, else best-effort empty string
        n, _ := sddctx.CheckCacheIntegrity(changeDir, "")
        total += n
    }
    if total > 0 {
        return CheckResult{Name: "cache", Status: "warn",
            Message: fmt.Sprintf("%d stale hash file(s)", total)}
    }
    return CheckResult{Name: "cache", Status: "pass", Message: "all cache entries current"}
}
```

Note: passing `skillsPath = ""` means the skill hash component is always empty; recomputed hash won't match any entry that included a skill — this counts those as stale, which is conservative but not incorrect for a diagnostic. Alternatively doctor loads each change's config to get `SkillsPath`. Given the complexity cost, passing `""` is acceptable for v1.

#### Check 3 — orphaned_pending

For each change dir, scan `.pending/*.md`. For each file, derive phase name (`strings.TrimSuffix(base, ".md")`). Check if promoted artifact exists at `<changeDir>/<phase>.md`. Count hits.

```go
func checkOrphanedPending(changesDir string) CheckResult {
    entries, _ := os.ReadDir(changesDir)
    count := 0
    for _, e := range entries {
        if !e.IsDir() || e.Name() == "archive" {
            continue
        }
        changeDir := filepath.Join(changesDir, e.Name())
        pendingDir := filepath.Join(changeDir, ".pending")
        pfiles, err := os.ReadDir(pendingDir)
        if err != nil {
            continue
        }
        for _, pf := range pfiles {
            if pf.IsDir() || !strings.HasSuffix(pf.Name(), ".md") {
                continue
            }
            phase := strings.TrimSuffix(pf.Name(), ".md")
            promoted := filepath.Join(changeDir, phase+".md")
            if _, err := os.Stat(promoted); err == nil {
                count++
            }
        }
    }
    if count > 0 {
        return CheckResult{Name: "orphaned_pending", Status: "warn",
            Message: fmt.Sprintf("%d orphaned .pending file(s)", count)}
    }
    return CheckResult{Name: "orphaned_pending", Status: "pass"}
}
```

#### Check 4 — skills_path

```go
func checkSkillsPath(cfg *config.Config) CheckResult {
    if cfg == nil {
        return CheckResult{Name: "skills_path", Status: "warn", Message: "skipped: config unavailable"}
    }
    if _, err := os.Stat(cfg.SkillsPath); err != nil {
        return CheckResult{Name: "skills_path", Status: "fail",
            Message: fmt.Sprintf("skills directory not found: %s", cfg.SkillsPath)}
    }
    phases := state.AllPhases()
    total := len(phases)
    present := 0
    for _, p := range phases {
        skillPath := filepath.Join(cfg.SkillsPath, "sdd-"+string(p), "SKILL.md")
        if _, err := os.ReadFile(skillPath); err == nil {
            present++
        }
    }
    msg := fmt.Sprintf("%d/%d SKILL.md files present", present, total)
    if present < total {
        return CheckResult{Name: "skills_path", Status: "warn", Message: msg}
    }
    return CheckResult{Name: "skills_path", Status: "pass", Message: msg}
}
```

The `dispatchers` map in `internal/context/context.go` only has 8 entries (archive and verify have no assemblers). The spec says "check for each phase in `AllPhases()` (10 total)". The check counts all 10 from `state.AllPhases()` — this matches the spec. Some will always be missing (`verify`, `archive`); that is acceptable because the warning will reflect reality.

#### Check 5 — build_tools

```go
func checkBuildTools(cfg *config.Config) CheckResult {
    if cfg == nil {
        return CheckResult{Name: "build_tools", Status: "warn", Message: "skipped: config unavailable"}
    }
    cmds := []string{cfg.Commands.Build, cfg.Commands.Test, cfg.Commands.Lint, cfg.Commands.Format}
    var missing []string
    seen := map[string]bool{}
    for _, cmd := range cmds {
        cmd = strings.TrimSpace(cmd)
        if cmd == "" {
            continue
        }
        bin := strings.Fields(cmd)[0]
        if seen[bin] {
            continue
        }
        seen[bin] = true
        if _, err := exec.LookPath(bin); err != nil {
            missing = append(missing, bin)
        }
    }
    if len(missing) > 0 {
        return CheckResult{Name: "build_tools", Status: "fail",
            Message: fmt.Sprintf("command(s) not in PATH: %s", strings.Join(missing, ", "))}
    }
    return CheckResult{Name: "build_tools", Status: "pass", Message: "all build commands found"}
}
```

Deduplication via `seen` prevents double-reporting the same binary (e.g., if build and test both start with `go`).

#### Output rendering

Human output (default):

```go
func printDoctorTable(w io.Writer, checks []CheckResult) {
    // Compute max name width.
    maxName := 0
    for _, c := range checks {
        if len(c.Name) > maxName {
            maxName = len(c.Name)
        }
    }
    fmt.Fprintln(w, "sdd doctor")
    for _, c := range checks {
        if c.Message != "" {
            fmt.Fprintf(w, "  %-*s  %-4s  %s\n", maxName, c.Name, c.Status, c.Message)
        } else {
            fmt.Fprintf(w, "  %-*s  %s\n", maxName, c.Name, c.Status)
        }
    }
}
```

Terminal color: the spec mentions "colored if terminal". Use `os.Stdout.Fd()` + `isatty` check — but we have no external deps. Decision: **skip ANSI color in v1**. The spec says "colored if terminal" in the prompt but the formal spec (DR-08) only requires aligned columns. Color is deferred.

JSON output: aggregate status computed as: if any `fail` → `"fail"`; else if any `warn` → `"warn"`; else `"pass"`.

```go
func aggregateStatus(checks []CheckResult) string {
    worst := "pass"
    for _, c := range checks {
        switch c.Status {
        case "fail":
            return "fail"
        case "warn":
            worst = "warn"
        }
    }
    return worst
}
```

Exit code: return a sentinel error when any check fails, so `ExitCode` returns 1:

```go
if !jsonOut {
    printDoctorTable(stdout, checks)
} else {
    // marshal and write JSON
}

for _, c := range checks {
    if c.Status == "fail" {
        return fmt.Errorf("doctor: %d check(s) failed", failCount)
    }
}
return nil
```

The returned error is a plain `fmt.Errorf`, not a `usageError`, so `ExitCode` returns 1 (not 2).

#### Wiring into `cli.go`

Three touch points (matching existing pattern):

1. `switch` case: `case "doctor": return runDoctor(rest, stdout, stderr)`
2. `printHelp`: under "Inspection commands:" add `"  doctor            Diagnose config, cache, skills, and tools"`
3. `commandHelp["doctor"]`: usage block with `--json` flag description

---

## 3.2 Flexible Phase References

### File: `internal/state/resolve.go`

```go
package state

import (
    "fmt"
    "strconv"
    "strings"
)

func ResolvePhase(input string) (Phase, error) {
    phases := AllPhases()

    // 1. Exact match.
    for _, p := range phases {
        if string(p) == input {
            return p, nil
        }
    }

    // 2. Integer index.
    if _, err := strconv.Atoi(input); err == nil {
        idx, _ := strconv.Atoi(input)
        if idx < 0 || idx >= len(phases) {
            return "", fmt.Errorf("phase index out of range: %s", input)
        }
        return phases[idx], nil
    }

    // 3. Case-insensitive prefix match.
    lower := strings.ToLower(input)
    var matches []string
    for _, p := range phases {
        if strings.HasPrefix(strings.ToLower(string(p)), lower) {
            matches = append(matches, string(p))
        }
    }
    switch len(matches) {
    case 1:
        return Phase(matches[0]), nil
    case 0:
        return "", fmt.Errorf("unknown phase: %q", input)
    default:
        return "", fmt.Errorf("ambiguous phase prefix %q: matches %s", input, strings.Join(matches, ", "))
    }
}
```

**Why `strconv.Atoi` twice**: readability. An alternative extracts the check as `isDigitString`. Since this is a hot path only in interactive CLI calls, not a loop, clarity wins.

**Prefix disambiguation**: the current 10 phases (`explore`, `propose`, `spec`, `spec`, `design`, `tasks`, `apply`, `review`, `verify`, `clean`, `archive`) have a near-collision: `spec` and `spec` — wait, no. The collision is `s` → matches `spec`. Only one phase starts with `s`, so `s` is unambiguous. `sp` is also unambiguous. The only actual collisions are `a` (→ `apply` only, since `archive` starts with `ar`) — actually `a` is unambiguous. After careful review: `ar` disambiguates `archive` from `apply`. See PR-03 table; all documented prefixes are unambiguous because of the specific phase names.

**Commands.go wiring**:

In `runContext`, replace:
```go
ph := state.Phase(positional[1])
```
with:
```go
ph, err := state.ResolvePhase(positional[1])
if err != nil {
    return errs.WriteError(stderr, "context", err)
}
```

In `runWrite`, replace:
```go
phase := state.Phase(phaseStr)
```
with:
```go
phase, err := state.ResolvePhase(phaseStr)
if err != nil {
    return errs.WriteError(stderr, "write", err)
}
```

---

## 3.3 Multi-Verbosity

### File: `internal/cli/verbosity.go`

```go
package cli

type Verbosity int

const (
    VerbosityQuiet   Verbosity = -1
    VerbosityDefault Verbosity = 0
    VerbosityVerbose Verbosity = 1
    VerbosityDebug   Verbosity = 2
)

func ParseVerbosityFlags(args []string) ([]string, Verbosity) {
    v := VerbosityDefault
    remaining := args[:0:0] // zero-length, same backing array avoidance
    remaining = make([]string, 0, len(args))
    for _, arg := range args {
        switch arg {
        case "-q", "--quiet":
            v = VerbosityQuiet
        case "-v", "--verbose":
            v = VerbosityVerbose
        case "-d", "--debug":
            v = VerbosityDebug
        default:
            remaining = append(remaining, arg)
        }
    }
    return remaining, v
}
```

**Last-wins**: the loop assigns directly; later flags overwrite earlier ones. No special handling needed.

**No error return**: unknown flags pass through; downstream flag parsers handle them. This avoids `ParseVerbosityFlags` needing to know all valid flags for every command.

### Changes to `internal/context/context.go`

Add to `Params`:
```go
Verbosity int // -1=quiet, 0=default, 1=verbose, 2=debug; uses int to avoid import cycle
```

Update `emitMetrics` to thread verbosity through:
```go
func emitMetrics(stderr io.Writer, changeDir, phase string, size int, cached bool, start time.Time, verbosity int) {
    m := &contextMetrics{ ... }
    recordMetrics(changeDir, m)
    if stderr == nil {
        return
    }
    writeMetrics(stderr, m, verbosity)
}
```

Both call sites in `Assemble` pass `p.Verbosity`:
```go
emitMetrics(p.Stderr, p.ChangeDir, phaseStr, size, true, start, p.Verbosity)
// and
emitMetrics(p.Stderr, p.ChangeDir, phaseStr, size, false, start, p.Verbosity)
```

### Changes to `internal/context/cache.go`

Updated `writeMetrics` signature:
```go
func writeMetrics(w io.Writer, m *contextMetrics, verbosity int) {
    if verbosity < 0 {
        return // quiet: suppress all output
    }

    source := "assembled"
    if m.Cached {
        source = "cached"
    }

    if verbosity >= 2 {
        // debug: add hash prefix and full duration
        // hash is not stored in contextMetrics — need to pass or recompute
        // Decision: add HashPrefix string field to contextMetrics (internal only)
        fmt.Fprintf(w, "sdd: phase=%s ↑%s Δ%dK tokens %dms (%s) hash=%s\n",
            m.Phase, formatBytes(m.Bytes), m.Tokens/1000, m.DurationMs, source, m.HashPrefix)
        return
    }

    // default (0) and verbose (1): current format (source already present)
    fmt.Fprintf(w, "sdd: phase=%s ↑%s Δ%dK tokens %dms (%s)\n",
        m.Phase, formatBytes(m.Bytes), m.Tokens/1000, m.DurationMs, source)
}
```

**HashPrefix availability**: `contextMetrics` does not currently carry the hash. To support `verbosity >= 2`, add `HashPrefix string` to `contextMetrics` (unexported field, internal only). Populate it in `emitMetrics` by passing the hash from `tryCachedContext` / `saveContextCache` sites, or re-derive it there.

**Simpler alternative**: compute the hash prefix inside `Assemble` at the point where we already have the phase and changeDir, then store it in the metrics struct before calling `emitMetrics`. This avoids touching `tryCachedContext` / `saveContextCache`:

```go
// In Assemble, before emitMetrics:
hashPrefix := ""
if p.Verbosity >= 2 {
    inputs := phaseInputs[string(phase)]
    h := inputHash(p.ChangeDir, inputs, p.SkillsPath, string(phase))
    if len(h) >= 8 {
        hashPrefix = h[:8]
    }
}
// pass hashPrefix to emitMetrics, which stores it in contextMetrics
```

This re-calls `inputHash` only when debug verbosity is requested, so no performance impact on normal paths.

**VB-04 level 1 (verbose)**: the spec says "append `[cache]` or `[assembled]` label" but acknowledges the existing `source` variable already shows this. The existing format `(%s)` at line end already contains `cached` or `assembled`. So verbose (1) produces identical output to default (0). This is intentional per the spec.

### Wiring in `commands.go`

`runContext`:
```go
func runContext(args []string, stdout io.Writer, stderr io.Writer) error {
    args, verbosity := ParseVerbosityFlags(args)  // strip -q/-v/-d first
    jsonOut := false
    var positional []string
    for _, arg := range args {
        // existing loop unchanged
    }
    // ...
    p := &sddctx.Params{
        // existing fields
        Verbosity: int(verbosity),
    }
    // ...
}
```

`runNew`:
```go
func runNew(args []string, stdout io.Writer, stderr io.Writer) error {
    args, verbosity := ParseVerbosityFlags(args)
    // existing flag loop unchanged
    // ...
    p := &sddctx.Params{
        // existing fields
        Verbosity: int(verbosity),
    }
    // ...
}
```

All other commands do not call `ParseVerbosityFlags`. If `-q` is passed to e.g. `sdd status`, the existing loop hits `default: return errs.Usage(...)`. This is the specified behavior (VB-08 scenario 6).

---

## Architecture Decisions

### Decision 1: `CheckCacheIntegrity` export vs. duplicating `inputHash`

**Rejected**: duplicate `inputHash` logic in `internal/cli/doctor.go`. The hash function includes `cacheVersion` prefix and SKILL.md content; duplicating it creates a maintenance hazard where the two implementations drift.

**Chosen**: export `CheckCacheIntegrity` from `internal/context/cache.go`. One function, localized.

### Decision 2: `Verbosity` as `int` in `Params` not `cli.Verbosity`

`internal/context` cannot import `internal/cli` without a cycle (`context` → `cli` → `context` via `Assemble`). Using bare `int` with documented constants avoids the cycle. The conversion `int(verbosity)` at the call site in `commands.go` is explicit and clear.

### Decision 3: `ParseVerbosityFlags` strips flags before existing loops

Existing loops use `default: return errs.Usage(...)` for unknown flags. If verbosity flags were not stripped first, `-q` would hit that default and return a usage error. Stripping first means existing flag parsers never see verbosity flags.

**Trade-off**: commands that do not call `ParseVerbosityFlags` will reject `-q`/`-v`/`-d` as unknown flags. This is specified behavior (spec VB-08 last scenario). No silent swallowing.

### Decision 4: doctor reads `config` once, threads `cfg` to subsequent checks

Alternative: each check independently calls `config.Load`. Rejected: 5 redundant filesystem reads for a diagnostic command. The config-threaded approach is explicit about dependency ordering.

### Decision 5: No ANSI color in v1

The formal spec requirements (DR-08) only mandate aligned columns; color is mentioned in the prompt's informal description. No `isatty` detection needed. Avoids a potential external dependency or CGO dependency.

---

## Testing Strategy

### `internal/state/resolve_test.go`

Table-driven, covering:
- All 10 exact phase name matches
- Indexes 0–9 (each returns correct phase)
- Out-of-range index (e.g., `"10"`) → error containing `"out of range"`
- All 12 prefix inputs from PR-03 table → correct phase
- Ambiguous prefix `"s"` — wait, only `spec` starts with `s`, so this is unambiguous. Find a genuinely ambiguous case: none exist in the current 10 phases for single chars. Test with a fabricated ambiguous string in a subtest that verifies the error message contains `"ambiguous"`.
- Unknown input `"xyz"` → error containing `"unknown phase"`
- Empty string `""` → error (no phase starts with empty prefix — all phases match, so ambiguous)

### `internal/cli/doctor_test.go` (new, or added to `cli_test.go`)

- Scaffold a temp `openspec/` with a valid `config.yaml` → all checks pass.
- Missing `config.yaml` → `config` check is `fail`, exit 1.
- `config.yaml` with `version: 0` → `config` check is `warn`, exit 0.
- Stale hash file (write a mismatched hash) → `cache` check is `warn`, exit 0.
- `.pending/apply.md` with `apply.md` also present → `orphaned_pending` is `warn`.
- Skills dir missing → `skills_path` is `fail`.
- `cfg.Commands.Lint = "notarealex lint"` → `build_tools` is `fail`, exit 1.
- `--json` flag → stdout is valid JSON, top-level `status` field correct.
- Table format: verify column alignment (max name width = `len("orphaned_pending")` = 16).

### `internal/cli/verbosity_test.go` (new, or added to `cli_test.go`)

- `ParseVerbosityFlags([]string{"-q", "foo"})` → `(["foo"], -1)`
- `ParseVerbosityFlags([]string{"-v"})` → `([], 1)`
- `ParseVerbosityFlags([]string{"-q", "-v"})` → `([], 1)` (last wins)
- `ParseVerbosityFlags([]string{"--debug", "--quiet"})` → `([], -1)` (last wins)
- `ParseVerbosityFlags([]string{"--unknown", "-v"})` → `(["--unknown"], 1)` (pass through)
- `ParseVerbosityFlags(nil)` → `([], 0)` (no panic)

### `internal/context` (cache.go changes)

- `writeMetrics` with `verbosity = -1`: assert 0 bytes written to output buffer.
- `writeMetrics` with `verbosity = 0`: assert one-line output matching current format.
- `writeMetrics` with `verbosity = 2` and non-empty `HashPrefix`: assert output contains `hash=`.

### Integration

- `sdd doctor` in a project with valid setup → exit 0.
- `sdd context mychange p` → resolves to propose phase (or correct error if phase not ready).
- `sdd context mychange -q explore` → no stderr output.
- `sdd context mychange -d explore` → stderr line contains `hash=`.

---

## Import Graph (after changes)

```
cmd/sdd/main.go
  └── internal/cli
        ├── internal/cli/errs
        ├── internal/config
        ├── internal/state          (+ resolve.go)
        ├── internal/context        (unchanged import)
        ├── internal/artifacts
        └── internal/verify

internal/context
  ├── internal/config
  └── internal/state
  (no new imports)
```

No new import cycles introduced. `internal/cli/verbosity.go` imports nothing from the project (only stdlib). `internal/state/resolve.go` imports only `fmt`, `strconv`, `strings` — all stdlib.
