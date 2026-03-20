# Exploration: 5 steipete patterns for sdd-cli

**Change:** steipete-patterns
**Phase:** explore
**Date:** 2026-03-20

---

## Overview

Five patterns to implement across 4 files and 1 new location. Patterns are:

1. Atomic writes — `verify.WriteReport` + `archive.writeManifest`
2. Error classification — `errs.WriteError` + `errs.WriteJSON`
3. Per-dimension TTL cache freshness — `context/cache.go`
4. Progress logging — `verify.Run` / `verify.runOne`
5. Progress surface in commands — `runVerify` / `runArchive`

---

## Pattern 1: Atomic writes

### What exists

`verify.WriteReport` [verify.go:195]:
```go
if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
```
Direct `os.WriteFile` — no temp+rename. A crash mid-write corrupts `verify-report.md`.

`archive.writeManifest` [archive.go:97]:
```go
if err := os.WriteFile(manifestPath, []byte(b.String()), 0o644); err != nil {
```
Same issue. Manifest written after `os.Rename` already moved the directory, so a crash here leaves the archive with no manifest.

### Precedent (already done correctly)

`state.Save` [state/state.go:120–141] uses the canonical pattern:
```go
tmp := path + ".tmp"
os.WriteFile(tmp, data, 0o644)
os.Rename(tmp, path)
os.Remove(tmp) // cleanup on rename failure
```
This is the reference implementation. Both `WriteReport` and `writeManifest` must match it.

### What needs to change

**File:** `internal/verify/verify.go`, line 195
- Replace `os.WriteFile(path, buf.Bytes(), 0o644)` with temp+rename.
- Add `os.Remove(tmp)` on rename failure.
- No new imports needed (`os`, `path/filepath` already imported).

**File:** `internal/verify/archive.go`, line 97
- Same replacement for `writeManifest`.
- `strings.Builder` → convert to `[]byte` via `[]byte(b.String())` (unchanged).
- No new imports needed.

### Risk: LOW
- `os.Rename` is atomic on Linux for same-filesystem paths. Both tmp and final are in the same directory, so this holds.
- Only behavioral change: partial writes no longer corrupt the file. No API changes.
- Existing tests in `verify_test.go` exercise the full path and will still pass.

---

## Pattern 2: Error classification

### What exists

`errs.WriteJSON` [errs/errs.go:36–46]:
```go
func WriteJSON(w io.Writer, command, message string) error {
    code := "not_implemented"  // hardcoded — always "not_implemented"
    ...
}
```
`code` is always `"not_implemented"` regardless of what `message` says. This is incorrect for unimplemented-feature stubs vs actual internal errors.

`errs.WriteError` [errs/errs.go:49–62]:
```go
func WriteError(w io.Writer, command string, err error) error {
    code := "internal"
    if IsUsage(err) {
        code = "usage"
    }
    ...
}
```
Only two classifications: `"usage"` or `"internal"`. No way to distinguish transient/retriable errors (e.g., network, I/O) from logic bugs.

Defined error type: only `usageError`. No transport/network error type.

### What needs to change

**File:** `internal/cli/errs/errs.go`

Three additions:

1. New sentinel error type `transportError` for retriable failures (file I/O, external process):
```go
type transportError struct{ msg string }
func (e *transportError) Error() string { return e.msg }
func Transport(msg string) error { return &transportError{msg: msg} }
func IsTransport(err error) bool { var te *transportError; return errors.As(err, &te) }
```

2. Update `WriteError` to classify transport errors:
```go
code := "internal"
if IsUsage(err) {
    code = "usage"
} else if IsTransport(err) {
    code = "transport"
}
```

3. `WriteJSON` keeps `"not_implemented"` — that's the correct code for the stub path. No change needed there.

### Call sites to update (optional, incremental)

`commands.go` currently wraps I/O errors with plain `fmt.Errorf`. After `Transport()` exists, callers can use it for file-system and exec errors. This is additive — existing `"internal"` classification degrades gracefully.

The `JSONError.Code` field docs should be updated to reflect the new `"transport"` value.

### Risk: LOW
- Purely additive: new exported functions, new unexported type.
- `WriteError` existing behavior unchanged for `usageError`; `internal` is the fallback.
- No callers break — `Transport()` is optional for callers to adopt.
- `errs_test.go` will need new table rows but existing rows pass unchanged.

---

## Pattern 3: Per-dimension TTL cache freshness

### What exists

`cache.go` uses content-hash invalidation only — no time-based expiry.

`tryCachedContext` [cache.go:102–124]:
```go
func tryCachedContext(changeDir, phase string) ([]byte, bool) {
    inputs, ok := phaseInputs[phase]
    if !ok || len(inputs) == 0 {
        return nil, false
    }
    storedHash, err := os.ReadFile(hashCachePath(changeDir, phase))
    // ...
    currentHash := inputHash(changeDir, inputs)
    if strings.TrimSpace(string(storedHash)) != currentHash {
        return nil, false
    }
    cached, err := os.ReadFile(contextCachePath(changeDir, phase))
    // ...
    return cached, true
}
```

`saveContextCache` [cache.go:127–148]: writes hash + context bytes. No timestamp.

`phaseInputs` [cache.go:39–48]: maps phase → input files. No TTL configuration.

The cache `.hash` file stores only the hex hash string — no wall-clock timestamp embedded.

### What needs to change

**File:** `internal/context/cache.go`

Approach: embed a write timestamp in the hash file alongside the hash, or store a separate `.ts` file. Simpler: encode `{hash}|{unix_epoch}` in the hash file — single file, single parse.

Per-dimension TTLs: different phases have different staleness profiles. Explore context (no inputs) is never cached anyway. Propose/spec/design contexts depend on files that rarely change mid-session — longer TTL makes sense. Apply/clean contexts change frequently.

Concrete plan:

1. `phaseTTL` map:
```go
var phaseTTL = map[string]time.Duration{
    "explore": 0,               // never cached (phaseInputs has no entries)
    "propose": 4 * time.Hour,
    "spec":    4 * time.Hour,
    "design":  4 * time.Hour,
    "tasks":   2 * time.Hour,
    "apply":   1 * time.Hour,
    "review":  1 * time.Hour,
    "clean":   30 * time.Minute,
}
```

2. Extend hash file format: `{hash}|{unix_timestamp_sec}` instead of bare hash. Backward compat: if `|` not found, treat as legacy → cache miss (forces one-time re-assembly).

3. `tryCachedContext` additions after hash match:
```go
if ttl, ok := phaseTTL[phase]; ok && ttl > 0 {
    if time.Since(writtenAt) > ttl {
        return nil, false  // TTL expired
    }
}
```

4. `saveContextCache`: embed `fmt.Sprintf("%s|%d", hash, time.Now().Unix())` instead of bare hash.

5. `phaseMetrics` and `pipelineMetrics` structs: no change needed (TTL logic is transparent to metrics).

### What does NOT need to change

`cacheVersion` bump: format change (adding `|timestamp`) IS a breaking change for existing caches. But: backward-compat fallback (legacy = cache miss) makes a version bump unnecessary. Users just re-assemble once after the update.

### Risk: MEDIUM
- Format change in `.hash` files. Old caches become misses (not corrupt) — safe.
- TTL values are guesses; they should be tunable via config or env var later (out of scope here).
- `context_test.go` tests cache hit/miss behavior and will need updating to account for TTL checks.
- The `|` separator is fragile if hashes ever contain `|` — they won't (SHA256 hex is `[0-9a-f]` only).

---

## Pattern 4: Progress logging in verify runner

### What exists

`verify.Run` [verify.go:69–94]: executes each `CommandSpec` sequentially with no output until done. Long test suites run silently for minutes.

`verify.runOne` [verify.go:98–148]: captures stdout+stderr into `bytes.Buffer`, only returning after completion.

The `Run` signature:
```go
func Run(workDir string, commands []CommandSpec, timeout time.Duration) (*Report, error)
```

No `io.Writer` for progress. No stderr parameter.

### What needs to change

**File:** `internal/verify/verify.go`

1. Extend `Run` signature to accept a progress writer:
```go
func Run(workDir string, commands []CommandSpec, timeout time.Duration, progress io.Writer) (*Report, error)
```

2. In `Run`, before calling `runOne`, log start:
```go
if progress != nil {
    fmt.Fprintf(progress, "sdd: verify %s...\n", spec.Name)
}
```

3. After `runOne` returns, log result:
```go
if progress != nil {
    status := "ok"
    if !result.Passed {
        status = fmt.Sprintf("FAILED (exit %d, %s)", result.ExitCode, result.Duration.Round(time.Millisecond))
    } else {
        status = fmt.Sprintf("ok (%s)", result.Duration.Round(time.Millisecond))
    }
    fmt.Fprintf(progress, "sdd: verify %s: %s\n", spec.Name, status)
}
```

4. `runOne` stays unchanged — still captures all output to buf. Progress is a separate channel from captured output.

### Call site impact

`commands.go:runVerify` [commands.go:419] calls:
```go
report, err := verify.Run(cwd, commands, verify.DefaultTimeout)
```
Must add `stderr` as progress writer. See Pattern 5.

### Risk: LOW
- API change: `Run` gains a parameter. Only one call site (`runVerify`). No external callers (internal package).
- `verify_test.go` calls `verify.Run` — must update all call sites to pass `nil` or a buffer.
- Nil-safe: `if progress != nil` guard means existing behavior is preserved when nil is passed.

---

## Pattern 5: Progress surface in commands (runVerify / runArchive)

### What exists

`runVerify` [commands.go:387–455]: silent until JSON output at end. No indication which step is running.

`runArchive` [commands.go:457–505]: single `verify.Archive(changeDir)` call. Archive moves directory + writes manifest — no output until done.

### What needs to change

**File:** `internal/cli/commands.go`

**runVerify** (line 419):
```go
// Before (line 419):
report, err := verify.Run(cwd, commands, verify.DefaultTimeout)

// After:
report, err := verify.Run(cwd, commands, verify.DefaultTimeout, stderr)
```
`stderr` is already in scope as a parameter of `runVerify`. This wires Pattern 4's progress writer.

**runArchive**: `verify.Archive` doesn't support progress currently, but the operation is fast (one `os.Rename` + one manifest write). Adding a single log line before the call is sufficient:
```go
// Add before line 482:
fmt.Fprintf(stderr, "sdd: archive %s...\n", name)
result, err := verify.Archive(changeDir)
if err == nil && stderr != nil {
    fmt.Fprintf(stderr, "sdd: archived to %s\n", filepath.Base(result.ArchivePath))
}
```
No change to `verify.Archive` signature needed.

### Risk: LOW
- `runVerify`: single argument addition. Behavior unchanged when `stderr` is `io.Discard` (integration tests use `io.Discard`).
- `runArchive`: two `fmt.Fprintf` lines added. Zero risk.
- `integration_test.go` passes `io.Discard` for stderr — progress output is discarded, tests unaffected.

---

## File-by-file change summary

| File | Lines affected | Patterns | Risk |
|---|---|---|---|
| `internal/verify/verify.go` | 195 (WriteReport), 69 (Run sig), 84 (loop body) | 1 + 4 | LOW |
| `internal/verify/archive.go` | 97 (writeManifest) | 1 | LOW |
| `internal/cli/errs/errs.go` | 17–63 (new type + WriteError update) | 2 | LOW |
| `internal/context/cache.go` | 39 (new phaseTTL map), 102–124 (tryCached), 127–148 (saveCache) | 3 | MEDIUM |
| `internal/cli/commands.go` | 419 (runVerify call), 481–484 (runArchive log) | 5 | LOW |

---

## New files required

None. All changes are in-place modifications of existing files.

## Dependencies

- Pattern 4 must land before Pattern 5 (Run signature change).
- Pattern 1 (WriteReport atomic) and Pattern 1 (writeManifest atomic) are independent.
- Pattern 2 (error classification) is fully independent.
- Pattern 3 (TTL cache) is fully independent.

## Tests to update

| Test file | Reason |
|---|---|
| `internal/verify/verify_test.go` | `verify.Run` signature gains `progress io.Writer` arg; pass `nil` |
| `internal/cli/errs/errs_test.go` | Add table rows for `Transport()` and `IsTransport()` |
| `internal/context/context_test.go` | TTL-aware cache: may need to mock time or set short TTL for tests |
| `internal/cli/integration_test.go` | `verify.Run` called indirectly via `runVerify`; no direct change needed |

## Open questions

1. Should `phaseTTL` values be user-configurable in `config.yaml`? (Recommend: no for now — hardcode, add later if needed.)
2. Should `Transport` errors cause a different exit code (e.g., exit 3)? (Recommend: no — keep exit 1 for simplicity; the JSON `code` field is the machine-readable discriminator.)
3. Should TTL be wall-clock or process-age? (Wall-clock is simpler and correct for the use case — cached contexts go stale across IDE sessions.)
