# Specification: steipete-patterns

**Change:** steipete-patterns
**Phase:** spec
**Date:** 2026-03-20

---

## Overview

Five independent hardening patterns applied to six existing files in `internal/`. No new packages, no new external dependencies, no breaking API changes visible outside `internal/`. Each pattern is independently revertable.

---

## Pattern 1: Atomic Writes

### Problem

`verify.WriteReport` (`internal/verify/verify.go:195`) and `writeManifest` (`internal/verify/archive.go:97`) both use bare `os.WriteFile`. A process crash mid-write leaves a partial file at the final path. Readers see corrupt data on the next run.

### Solution

Replace each `os.WriteFile(path, data, perm)` with the temp-rename idiom already used by `state.Save` (`internal/state/state.go:132–140`):

```go
tmp := path + ".tmp"
if err := os.WriteFile(tmp, data, 0o644); err != nil {
    return fmt.Errorf("<context>: %w", err)
}
if err := os.Rename(tmp, path); err != nil {
    os.Remove(tmp) // best-effort cleanup
    return fmt.Errorf("<context>: %w", err)
}
```

The `os.Remove(tmp)` on rename failure is best-effort (same as `state.Save:137`); a rename failure is already an OS-level anomaly and the caller receives the error.

### Affected sites

**Site 1 — `verify.WriteReport`** (`internal/verify/verify.go:195–197`)

Current:
```go
if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
    return fmt.Errorf("write verify report: %w", err)
}
```

Replace with:
```go
tmp := path + ".tmp"
if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
    return fmt.Errorf("write verify report: %w", err)
}
if err := os.Rename(tmp, path); err != nil {
    os.Remove(tmp)
    return fmt.Errorf("write verify report: %w", err)
}
```

No import changes required; `os` is already imported.

**Site 2 — `writeManifest`** (`internal/verify/archive.go:97–99`)

Current:
```go
if err := os.WriteFile(manifestPath, []byte(b.String()), 0o644); err != nil {
    return fmt.Errorf("write archive manifest: %w", err)
}
```

Replace with:
```go
tmp := manifestPath + ".tmp"
if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
    return fmt.Errorf("write archive manifest: %w", err)
}
if err := os.Rename(tmp, manifestPath); err != nil {
    os.Remove(tmp)
    return fmt.Errorf("write archive manifest: %w", err)
}
```

No import changes required; `os` is already imported.

### Tests

**`internal/verify/verify_test.go`**

Existing `TestWriteReport_Pass`, `TestWriteReport_Fail`, and `TestArchive` tests cover the full write path; no structural change needed there.

Add one new subtest `TestWriteReport_AtomicOnReadOnlyDir`: create a read-only directory, call `WriteReport`, assert a non-nil error is returned and no `.tmp` file is left behind. This validates that the temp-cleanup path fires.

```go
func TestWriteReport_AtomicOnReadOnlyDir(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    if err := os.Chmod(dir, 0o555); err != nil {
        t.Skip("cannot set read-only dir:", err)
    }
    t.Cleanup(func() { os.Chmod(dir, 0o755) })

    report := &Report{Passed: true, Timestamp: time.Now().UTC()}
    err := WriteReport(report, dir)
    if err == nil {
        t.Fatal("expected error writing to read-only dir")
    }
    // No .tmp file should remain.
    matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
    if len(matches) != 0 {
        t.Errorf("unexpected .tmp files left: %v", matches)
    }
}
```

---

## Pattern 2: Error Classification

### Problem

`errs.WriteError` (`internal/cli/errs/errs.go:49–62`) emits one of two JSON codes: `"usage"` (for `usageError`) or `"internal"` (for everything else). Retriable I/O and external-process failures are indistinguishable from true internal errors at the machine-readable level.

### Solution

Add a `transportError` type alongside the existing `usageError` and update `WriteError` to detect it.

### Changes to `internal/cli/errs/errs.go`

**1. New type and constructors** — add after the closing brace of `IsUsage` (line 33):

```go
// transportError marks retriable I/O or external-process failures.
type transportError struct{ msg string }

func (e *transportError) Error() string { return e.msg }

// Transport wraps a retriable I/O or external-process error.
func Transport(msg string) error { return &transportError{msg: msg} }

// IsTransport reports whether err is a transport error.
func IsTransport(err error) bool {
    var te *transportError
    return errors.As(err, &te)
}
```

`errors` is already imported.

**2. Update `WriteError` classifier** — replace the current two-branch classifier:

```go
// Before
code := "internal"
if IsUsage(err) {
    code = "usage"
}

// After
code := "internal"
if IsUsage(err) {
    code = "usage"
} else if IsTransport(err) {
    code = "transport"
}
```

**3. Update `JSONError` doc comment** — the `Code` field tag comment currently reads `"usage", "internal", "not_implemented"`. Update to `"usage", "transport", "internal", "not_implemented"`.

`WriteJSON` is not changed; `"not_implemented"` remains correct for stub paths.

### JSON codes (complete set after this change)

| Code | Source | Meaning |
|---|---|---|
| `"usage"` | `errs.Usage(...)` | Invalid CLI invocation; exit 2 |
| `"transport"` | `errs.Transport(...)` | Retriable I/O or exec failure; exit 1 |
| `"internal"` | any other error | Unexpected failure; exit 1 |
| `"not_implemented"` | `errs.WriteJSON(...)` directly | Stub path |

### Call-site adoption

Optional and incremental. Existing callers that wrap file-system or exec errors may migrate to `errs.Transport(...)` in separate changes. Until they do, those errors continue to emit `"internal"` — no regression.

### Tests

**`internal/cli/errs/errs_test.go`**

Add three new rows to `TestWriteError` and two standalone tests:

```go
// In TestWriteError table:
{"transport error", Transport("dial tcp: i/o timeout"), "transport"},

// New standalone tests:
func TestTransportError(t *testing.T) {
    err := Transport("dial tcp: i/o timeout")
    if err == nil {
        t.Fatal("expected non-nil error")
    }
    if err.Error() != "dial tcp: i/o timeout" {
        t.Errorf("error = %q", err.Error())
    }
    if !IsTransport(err) {
        t.Error("expected IsTransport to return true")
    }
}

func TestIsTransportNonTransport(t *testing.T) {
    if IsTransport(Usage("bad input")) {
        t.Error("expected IsTransport false for usage error")
    }
}
```

---

## Pattern 3: Progress Logging

### Problem

`verify.Run` is silent. When a slow build or test suite runs, the caller (and the user watching stderr) receives no feedback until all commands complete.

### Solution

Add an optional `progress io.Writer` parameter to `verify.Run`. When non-nil, emit one line before each command and one line after. A nil writer preserves the current silent behavior (backward compatible).

### Changes to `internal/verify/verify.go`

**1. Add `"io"` import** — `io` is not currently imported; add it.

**2. New `Run` signature** (`verify.go:69`):

```go
// Before
func Run(workDir string, commands []CommandSpec, timeout time.Duration) (*Report, error)

// After
func Run(workDir string, commands []CommandSpec, timeout time.Duration, progress io.Writer) (*Report, error)
```

**3. Progress lines inside the execution loop** — insert around the `runOne` call at `verify.go:84`:

```go
if progress != nil {
    fmt.Fprintf(progress, "sdd: verify %s...\n", spec.Name)
}
result := runOne(workDir, spec, timeout)
if progress != nil {
    if result.Passed {
        fmt.Fprintf(progress, "sdd: verify %s: ok (%s)\n",
            spec.Name, result.Duration.Round(time.Millisecond))
    } else {
        fmt.Fprintf(progress, "sdd: verify %s: FAILED (exit %d, %s)\n",
            spec.Name, result.ExitCode, result.Duration.Round(time.Millisecond))
    }
}
```

`fmt` is already imported. `runOne` signature and implementation are unchanged.

### Progress line format (normative)

| Event | Format |
|---|---|
| Before command | `sdd: verify {name}...\n` |
| After pass | `sdd: verify {name}: ok ({duration})\n` |
| After fail | `sdd: verify {name}: FAILED (exit {code}, {duration})\n` |

Duration is rounded to milliseconds via `time.Duration.Round(time.Millisecond)`. For a timeout, `ExitCode` is `-1` (set by `runOne`).

### Changes to `internal/cli/commands.go`

**`runVerify`** (`commands.go:419`): pass `stderr` as the progress writer so the user sees real-time feedback on stderr:

```go
// Before
report, err := verify.Run(cwd, commands, verify.DefaultTimeout)

// After
report, err := verify.Run(cwd, commands, verify.DefaultTimeout, stderr)
```

No other changes to `runVerify`.

**`runArchive`** does not call `verify.Run`; no change needed there.

### Tests

**`internal/verify/verify_test.go`**

1. Update every existing `verify.Run(...)` call to pass `nil` as the fourth argument. Affected functions: `TestRun_AllPass`, `TestRun_OneFails`, `TestRun_Timeout`, `TestRun_SkipsEmptyCommands`.

2. Add one new subtest `TestRun_ProgressOutput`:

```go
func TestRun_ProgressOutput(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()

    commands := []CommandSpec{
        {Name: "build", Command: "echo build-ok"},
        {Name: "lint", Command: "exit 1"},
    }

    var prog bytes.Buffer
    report, err := Run(dir, commands, 30*time.Second, &prog)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    _ = report

    out := prog.String()
    if !strings.Contains(out, "sdd: verify build...") {
        t.Error("expected pre-command line for build")
    }
    if !strings.Contains(out, "sdd: verify build: ok") {
        t.Error("expected post-command ok line for build")
    }
    if !strings.Contains(out, "sdd: verify lint...") {
        t.Error("expected pre-command line for lint")
    }
    if !strings.Contains(out, "sdd: verify lint: FAILED") {
        t.Error("expected post-command FAILED line for lint")
    }
}
```

`bytes` is already imported in the test file.

**`internal/cli/integration_test.go`** — passes `io.Discard` for stderr; no change required since progress goes to stderr which is already discarded.

---

## Pattern 4: Per-Dimension TTL Cache

### Problem

`tryCachedContext` (`internal/context/cache.go:102–124`) validates only the input hash. A cached context is valid indefinitely. For phases whose inputs do not change (e.g., `propose` after exploration is written), the cache never expires even if days pass and the LLM context has drifted.

### Solution

Embed a Unix timestamp in the hash file. Add a `phaseTTL` map. On cache read, check both hash match and TTL not exceeded.

### Hash file format change

**Current format** (a single line):
```
<sha256hex>
```

**New format** (same file, same path `{changeDir}/.cache/{phase}.hash`):
```
<sha256hex>|<unix_seconds>
```

The `|` character does not appear in hex strings, making splitting unambiguous.

Legacy files (no `|`) → cache miss, not an error. On the next successful context assembly the file is overwritten in the new format. No migration script needed.

### Changes to `internal/context/cache.go`

**1. Add `"strconv"` and `"time"` imports** — `time` is not currently imported; `strconv` is not currently imported. Both are needed.

**2. Add `phaseTTL` map** — declare as a package-level `var` after `phaseInputs`:

```go
// phaseTTL defines the maximum age of a cached context per phase.
// Phases absent from this map (explore) are never cached — they have no
// phaseInputs entry, so saveContextCache and tryCachedContext already
// return early for them.
var phaseTTL = map[string]time.Duration{
    "propose": 4 * time.Hour,
    "spec":    2 * time.Hour,
    "design":  2 * time.Hour,
    "tasks":   1 * time.Hour,
    "apply":   30 * time.Minute,
    "review":  1 * time.Hour,
    "clean":   1 * time.Hour,
}
```

Note: the proposal listed `spec`/`design` at 4h and `clean` at 30min; the spec corrects these to match the task description (`spec`/`design`: 2h, `apply`: 30min, `review`/`clean`: 1h).

**3. Add file-local helper `mustParseInt64`**:

```go
// mustParseInt64 parses s as a base-10 int64.
// Returns 0 on any parse error (treated as epoch → immediate TTL miss).
func mustParseInt64(s string) int64 {
    v, err := strconv.ParseInt(s, 10, 64)
    if err != nil {
        return 0
    }
    return v
}
```

**4. Update `saveContextCache`** (`cache.go:140`) — replace the plain hash write:

```go
// Before
if err := os.WriteFile(hashCachePath(changeDir, phase), []byte(hash), 0o644); err != nil {
    return fmt.Errorf("write hash cache: %w", err)
}

// After
line := fmt.Sprintf("%s|%d", hash, time.Now().Unix())
if err := os.WriteFile(hashCachePath(changeDir, phase), []byte(line), 0o644); err != nil {
    return fmt.Errorf("write hash cache: %w", err)
}
```

**5. Update `tryCachedContext`** (`cache.go:102–124`) — replace the hash comparison block:

```go
// Before
currentHash := inputHash(changeDir, inputs)
if strings.TrimSpace(string(storedHash)) != currentHash {
    return nil, false
}

// After
raw := strings.TrimSpace(string(storedHash))
hashPart, tsPart, found := strings.Cut(raw, "|")
if !found {
    // Legacy format — force miss; will be rewritten on next save.
    return nil, false
}
writtenAt := time.Unix(mustParseInt64(tsPart), 0)
if ttl, ok := phaseTTL[phase]; ok && time.Since(writtenAt) > ttl {
    return nil, false
}
currentHash := inputHash(changeDir, inputs)
if hashPart != currentHash {
    return nil, false
}
```

`strings` is already imported. `strings.Cut` is available since Go 1.18.

### TTL table (normative)

| Phase | TTL | Rationale |
|---|---|---|
| explore | no cache | No phaseInputs; already skipped |
| propose | 4h | Exploration rarely changes; long window acceptable |
| spec | 2h | Proposal-derived; moderate churn |
| design | 2h | Proposal-derived; moderate churn |
| tasks | 1h | Design-derived; higher churn |
| apply | 30min | Actively changing code; short window |
| review | 1h | Tasks/design-derived; moderate churn |
| clean | 1h | Verify-report-derived; moderate churn |

### Tests

**`internal/context/context_test.go`**

All existing cache-hit/miss tests that write a hash file must be updated to use the new `hash|timestamp` format. Write a test helper:

```go
func writeHashFile(t *testing.T, changeDir, phase, hash string, writtenAt time.Time) {
    t.Helper()
    dir := filepath.Join(changeDir, ".cache")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        t.Fatal(err)
    }
    line := fmt.Sprintf("%s|%d", hash, writtenAt.Unix())
    path := filepath.Join(dir, phase+".hash")
    if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
        t.Fatal(err)
    }
}
```

Add subtests:

1. **`TestTryCachedContext_LegacyFormat`** — write a hash file with no `|`; assert cache miss, no error, no panic.

2. **`TestTryCachedContext_TTLExpired`** — write a valid hash file with timestamp `time.Now().Add(-5 * time.Hour)` for phase `"propose"` (TTL 4h); assert cache miss.

3. **`TestTryCachedContext_TTLValid`** — write a valid hash file with timestamp `time.Now()` for phase `"propose"`; assert cache hit.

4. **`TestSaveContextCache_NewFormat`** — call `saveContextCache`, then read the raw hash file and assert it contains `|` and a parseable timestamp.

---

## Pattern 5: Document Incomplete-Batch Resume

### Problem

It is not obvious from reading `state.go` that the state machine already handles interrupted runs. Developers may add redundant or conflicting resume logic.

### Solution

Add a comment block in `internal/state/state.go` above `Save` (currently line 119) explaining the invariant:

```go
// Save persists s atomically (write to .tmp, then rename). If the process
// is interrupted mid-write, the previous state file remains intact and the
// next run resumes from the last committed phase. No additional resume logic
// is required: Recover() rebuilds state from artifacts on disk, and
// nextReady() determines the correct next phase from the completed set.
// The state machine handles incomplete batches by design.
func Save(s *State, path string) error {
```

No behavioral change. No test update needed.

---

## File-by-file change summary

| File | Pattern(s) | Risk |
|---|---|---|
| `internal/verify/verify.go` | 1 (WriteReport atomic), 3 (Run signature + loop) | LOW |
| `internal/verify/archive.go` | 1 (writeManifest atomic) | LOW |
| `internal/cli/errs/errs.go` | 2 (transportError + WriteError) | LOW |
| `internal/cli/commands.go` | 3 (pass stderr to verify.Run) | LOW |
| `internal/context/cache.go` | 4 (phaseTTL + format change) | MEDIUM |
| `internal/state/state.go` | 5 (doc comment only) | LOW |

---

## Implementation order

```
Pattern 1a  verify.go:WriteReport        independent
Pattern 1b  archive.go:writeManifest     independent
Pattern 2   errs.go                      independent
Pattern 3a  verify.go:Run signature      independent
Pattern 3b  commands.go wiring           after Pattern 3a (compile dependency)
Pattern 4   cache.go                     independent
Pattern 5   state.go comment             independent
```

---

## Test file update summary

| Test file | Required changes |
|---|---|
| `internal/verify/verify_test.go` | Pass `nil` for new progress arg in 4 existing tests; add `TestRun_ProgressOutput`; add `TestWriteReport_AtomicOnReadOnlyDir` |
| `internal/cli/errs/errs_test.go` | Add `TestTransportError`, `TestIsTransportNonTransport`; add transport row to `TestWriteError` table |
| `internal/context/context_test.go` | Update hash-file fixtures to `hash\|ts` format; add 4 subtests (legacy, TTL expired, TTL valid, save format) |
| `internal/cli/integration_test.go` | No change (already passes `io.Discard` for stderr) |

---

## Invariants preserved

- Exit codes unchanged: 0 success, 1 general error, 2 usage error.
- Public command output format (stdout JSON) unchanged.
- `config.yaml` schema unchanged.
- No new packages or external dependencies.
- Each pattern is independently revertable to its prior form without affecting the others.
