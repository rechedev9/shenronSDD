# Design: 5 steipete patterns for sdd-cli

**Change:** steipete-patterns
**Phase:** design
**Date:** 2026-03-20

---

## Overview

Five independent hardening patterns. Each section specifies exact code changes: file path, line reference into current source, before/after diff, imports affected, and test requirements. Patterns are ordered by implementation dependency.

No new packages. No new external imports beyond `strconv` (pattern 4) and `io` (pattern 3). No public API changes outside `internal/verify`.

---

## Pattern 1 — Atomic writes

### Motivation

`verify.WriteReport` (verify.go:195) and `writeManifest` (archive.go:97) both use bare `os.WriteFile`. A crash mid-write leaves a truncated file. `state.Save` already uses temp+rename (state.go:132–138); apply the same idiom to these two sites.

### No shared helper

Two sites do not meet the rule-of-3 threshold. Inline the pattern at both sites. If a third site appears, extract `atomicWrite` then.

### Site A — `verify.go:WriteReport`

**File:** `internal/verify/verify.go`

**Current (line 195):**
```go
if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
    return fmt.Errorf("write verify report: %w", err)
}
```

**Replace with:**
```go
tmp := path + ".tmp"
if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
    return fmt.Errorf("write verify report: %w", err)
}
if err := os.Rename(tmp, path); err != nil {
    os.Remove(tmp) // best-effort cleanup
    return fmt.Errorf("rename verify report: %w", err)
}
```

No import changes — `os` already imported.

### Site B — `archive.go:writeManifest`

**File:** `internal/verify/archive.go`

**Current (line 97):**
```go
if err := os.WriteFile(manifestPath, []byte(b.String()), 0o644); err != nil {
    return fmt.Errorf("write archive manifest: %w", err)
}
```

**Replace with:**
```go
tmp := manifestPath + ".tmp"
if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
    return fmt.Errorf("write archive manifest: %w", err)
}
if err := os.Rename(tmp, manifestPath); err != nil {
    os.Remove(tmp) // best-effort cleanup
    return fmt.Errorf("rename archive manifest: %w", err)
}
```

No import changes — `os` already imported.

### Tests

`internal/verify/verify_test.go`: existing `WriteReport` tests exercise the full write path and pass unchanged. Add one subtest:

```go
t.Run("WriteReport write error leaves no tmp", func(t *testing.T) {
    // Point path at a read-only directory so WriteFile fails.
    ro := t.TempDir()
    require.NoError(t, os.Chmod(ro, 0o555))
    t.Cleanup(func() { os.Chmod(ro, 0o755) })

    err := WriteReport(&Report{Timestamp: time.Now().UTC(), Passed: true}, ro)
    require.Error(t, err)
    // Confirm no .tmp file left behind.
    entries, _ := os.ReadDir(ro)
    for _, e := range entries {
        if strings.HasSuffix(e.Name(), ".tmp") {
            t.Fatalf("stale tmp file: %s", e.Name())
        }
    }
})
```

Note: this test must run as non-root; skip on root with `t.Skip("root bypasses permissions")`.

---

## Pattern 2 — Error classification

### Motivation

`errs.WriteError` emits `"internal"` or `"usage"`. Callers wrapping filesystem or exec failures have no machine-readable way to tell retriable transport errors from logic bugs. Add `Transport` / `IsTransport` mirroring the existing `Usage` / `IsUsage` pattern.

### File: `internal/cli/errs/errs.go`

**Add after the `IsUsage` block (after line 33):**

```go
// transportError marks retriable I/O or external-process errors.
type transportError struct{ msg string }

func (e *transportError) Error() string { return e.msg }

// Transport returns a transport error (exit code 1, code "transport").
// Use for filesystem I/O failures and external process errors that are
// safe to retry.
func Transport(msg string) error { return &transportError{msg: msg} }

// IsTransport reports whether err or any error in its chain is a transport error.
func IsTransport(err error) bool {
	var te *transportError
	return errors.As(err, &te)
}
```

**Update `WriteError` classifier (current lines 50–53):**

```go
// Current:
code := "internal"
if IsUsage(err) {
    code = "usage"
}

// Replace with:
code := "internal"
if IsUsage(err) {
    code = "usage"
} else if IsTransport(err) {
    code = "transport"
}
```

`WriteJSON` is unchanged — `"not_implemented"` is correct for stub paths.

**No import changes** — `errors` already imported.

**Update `JSONError.Code` doc comment (line 14):**

```go
// Current:
Code    string `json:"code"` // "usage", "internal", "not_implemented"

// Replace with:
Code    string `json:"code"` // "usage", "internal", "transport", "not_implemented"
```

### Tests

`internal/cli/errs/errs_test.go` — add table rows to existing table-driven test:

```go
// In the IsUsage/IsTransport table:
{name: "transport direct",   err: Transport("disk full"),        wantTransport: true,  wantUsage: false},
{name: "transport wrapped",  err: fmt.Errorf("op: %w", Transport("disk full")), wantTransport: true, wantUsage: false},
{name: "usage not transport",err: Usage("bad arg"),              wantTransport: false, wantUsage: true},
{name: "plain not transport",err: errors.New("boom"),            wantTransport: false, wantUsage: false},

// In the WriteError output table:
{name: "transport code", err: Transport("net timeout"), wantCode: "transport"},
```

---

## Pattern 3 — Progress logging for verify

### Motivation

`verify.Run` runs commands silently. A long test suite gives no feedback. Add an optional `io.Writer` parameter; pass `nil` to suppress output (all existing callers get `nil`).

### Signature change

**File:** `internal/verify/verify.go`

**Current (line 69):**
```go
func Run(workDir string, commands []CommandSpec, timeout time.Duration) (*Report, error) {
```

**Replace with:**
```go
// Run executes each command sequentially in workDir, stopping on first failure.
// Empty command strings are skipped. If progress is non-nil, one line is written
// before each command starts and one line after it completes.
func Run(workDir string, commands []CommandSpec, timeout time.Duration, progress io.Writer) (*Report, error) {
```

**Add `"io"` to imports** (already present in cache.go for reference; not yet in verify.go):
```go
import (
    "bytes"
    "context"
    "fmt"
    "io"        // add
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "syscall"
    "time"
)
```

### Loop changes

**Current loop body (lines 80–91):**
```go
for _, spec := range commands {
    if spec.Command == "" {
        continue
    }

    result := runOne(workDir, spec, timeout)
    report.Results = append(report.Results, result)

    if !result.Passed {
        report.Passed = false
        break // stop on first failure
    }
}
```

**Replace with:**
```go
for _, spec := range commands {
    if spec.Command == "" {
        continue
    }

    if progress != nil {
        fmt.Fprintf(progress, "sdd: verify %s...\n", spec.Name)
    }
    result := runOne(workDir, spec, timeout)
    report.Results = append(report.Results, result)
    if progress != nil {
        if result.Passed {
            fmt.Fprintf(progress, "sdd: verify %s: ok (%s)\n",
                spec.Name, result.Duration.Round(time.Millisecond))
        } else {
            fmt.Fprintf(progress, "sdd: verify %s: FAILED (exit %d, %s)\n",
                spec.Name, result.ExitCode, result.Duration.Round(time.Millisecond))
        }
    }

    if !result.Passed {
        report.Passed = false
        break // stop on first failure
    }
}
```

`runOne` signature is unchanged.

### Wiring in commands.go

**File:** `internal/cli/commands.go`

**Current (line 419):**
```go
report, err := verify.Run(cwd, commands, verify.DefaultTimeout)
```

**Replace with:**
```go
report, err := verify.Run(cwd, commands, verify.DefaultTimeout, stderr)
```

`stderr` is already the third parameter of `runVerify(args []string, stdout io.Writer, stderr io.Writer)`. This wires progress lines to the same stderr stream as JSON errors.

### Tests

`internal/verify/verify_test.go`:

1. Every existing `verify.Run(...)` call: add `nil` as the fourth argument.
2. Add subtest:

```go
t.Run("Run emits progress lines", func(t *testing.T) {
    t.Parallel()
    var buf bytes.Buffer
    cmds := []CommandSpec{
        {Name: "echo", Command: "echo hello"},
    }
    _, err := Run(t.TempDir(), cmds, 10*time.Second, &buf)
    require.NoError(t, err)
    out := buf.String()
    require.Contains(t, out, "sdd: verify echo...")
    require.Contains(t, out, "sdd: verify echo: ok (")
})
```

---

## Pattern 4 — Per-dimension TTL cache

### Motivation

Cached contexts grow stale over wall-clock time even when input artifacts are unchanged (e.g., a design phase assembled 5 hours ago). Embed a Unix timestamp in the hash file and enforce per-phase TTLs.

### Hash file format change

**Current format** (plain text, one line): `{hex_hash}`

**New format** (two fields, pipe-separated): `{hex_hash}|{unix_seconds}`

The pipe character cannot appear in a hex SHA256 string, so `strings.Cut` splits cleanly.

**Backward compatibility:** Legacy files (no `|`) produce a cache miss, not a parse error. They are overwritten with the new format on the next successful `saveContextCache` call. No migration script required.

### TTL constants

**File:** `internal/context/cache.go`

**Add after the `phaseInputs` var block (after line 48):**

```go
// phaseTTL defines how long a cached context remains valid for each phase.
// Phases with faster-changing inputs get shorter TTLs.
// "explore" is omitted — it has no phaseInputs entry and is never cached.
var phaseTTL = map[string]time.Duration{
    "propose": 4 * time.Hour,
    "spec":    4 * time.Hour,
    "design":  4 * time.Hour,
    "tasks":   2 * time.Hour,
    "apply":   1 * time.Hour,
    "review":  1 * time.Hour,
    "clean":   30 * time.Minute,
}
```

### `mustParseInt64` helper

**Add as a file-local helper (bottom of cache.go):**

```go
// mustParseInt64 parses s as a base-10 int64.
// Returns 0 on parse failure, which causes the TTL check to see an ancient
// timestamp and force a cache miss — safe degradation.
func mustParseInt64(s string) int64 {
    n, err := strconv.ParseInt(s, 10, 64)
    if err != nil {
        return 0
    }
    return n
}
```

**Add `"strconv"` to imports.**

### `saveContextCache` — write new format

**File:** `internal/context/cache.go`

**Current (lines 138–141):**
```go
hash := inputHash(changeDir, inputs)

if err := os.WriteFile(hashCachePath(changeDir, phase), []byte(hash), 0o644); err != nil {
    return fmt.Errorf("write hash cache: %w", err)
}
```

**Replace with:**
```go
hash := inputHash(changeDir, inputs)
hashLine := fmt.Sprintf("%s|%d", hash, time.Now().Unix())

if err := os.WriteFile(hashCachePath(changeDir, phase), []byte(hashLine), 0o644); err != nil {
    return fmt.Errorf("write hash cache: %w", err)
}
```

No import changes beyond `strconv` above — `time` and `fmt` already imported.

### `tryCachedContext` — parse and TTL-check

**File:** `internal/context/cache.go`

**Current (lines 108–116):**
```go
storedHash, err := os.ReadFile(hashCachePath(changeDir, phase))
if err != nil {
    return nil, false
}

currentHash := inputHash(changeDir, inputs)
if strings.TrimSpace(string(storedHash)) != currentHash {
    return nil, false
}
```

**Replace with:**
```go
storedRaw, err := os.ReadFile(hashCachePath(changeDir, phase))
if err != nil {
    return nil, false
}

raw := strings.TrimSpace(string(storedRaw))
hash, tsStr, found := strings.Cut(raw, "|")
if !found {
    // Legacy format — force miss; file will be rewritten on next save.
    return nil, false
}

writtenAt := time.Unix(mustParseInt64(tsStr), 0)
if ttl, ok := phaseTTL[phase]; ok && time.Since(writtenAt) > ttl {
    return nil, false
}

currentHash := inputHash(changeDir, inputs)
if hash != currentHash {
    return nil, false
}
```

`strings.Cut` is available since Go 1.18 — already in use elsewhere in the stdlib surface this project targets.

### Tests

`internal/context/context_test.go`:

1. All tests that pre-write a `.hash` fixture must switch to the new format. Helper:

```go
func writeHashFixture(t *testing.T, changeDir, phase, hash string, age time.Duration) {
    t.Helper()
    ts := time.Now().Add(-age).Unix()
    line := fmt.Sprintf("%s|%d", hash, ts)
    path := filepath.Join(changeDir, ".cache", phase+".hash")
    require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
    require.NoError(t, os.WriteFile(path, []byte(line), 0o644))
}
```

2. Add TTL-expiry subtest:

```go
t.Run("tryCachedContext TTL expired", func(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    // Write proposal.md so inputHash is non-empty.
    require.NoError(t, os.WriteFile(filepath.Join(dir, "exploration.md"), []byte("x"), 0o644))

    // Pre-write hash file with correct hash but timestamp 5h ago (propose TTL = 4h).
    inputs := phaseInputs["propose"]
    hash := inputHash(dir, inputs)
    writeHashFixture(t, dir, "propose", hash, 5*time.Hour)

    // Also write a ctx file so we know the miss is from TTL, not missing ctx.
    ctxPath := filepath.Join(dir, ".cache", "propose.ctx")
    require.NoError(t, os.WriteFile(ctxPath, []byte("stale content"), 0o644))

    got, ok := tryCachedContext(dir, "propose")
    require.False(t, ok, "expected TTL miss")
    require.Nil(t, got)
})

t.Run("tryCachedContext TTL fresh", func(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    require.NoError(t, os.WriteFile(filepath.Join(dir, "exploration.md"), []byte("x"), 0o644))

    inputs := phaseInputs["propose"]
    hash := inputHash(dir, inputs)
    writeHashFixture(t, dir, "propose", hash, 1*time.Hour) // within 4h TTL

    ctxPath := filepath.Join(dir, ".cache", "propose.ctx")
    require.NoError(t, os.WriteFile(ctxPath, []byte("fresh content"), 0o644))

    got, ok := tryCachedContext(dir, "propose")
    require.True(t, ok)
    require.Equal(t, []byte("fresh content"), got)
})

t.Run("tryCachedContext legacy format is cache miss", func(t *testing.T) {
    t.Parallel()
    dir := t.TempDir()
    require.NoError(t, os.WriteFile(filepath.Join(dir, "exploration.md"), []byte("x"), 0o644))

    inputs := phaseInputs["propose"]
    hash := inputHash(dir, inputs)
    // Write legacy format (no pipe).
    hashPath := filepath.Join(dir, ".cache", "propose.hash")
    require.NoError(t, os.MkdirAll(filepath.Dir(hashPath), 0o755))
    require.NoError(t, os.WriteFile(hashPath, []byte(hash), 0o644))

    got, ok := tryCachedContext(dir, "propose")
    require.False(t, ok)
    require.Nil(t, got)
})
```

---

## Pattern 5 — Document resume (comment only)

### Motivation

The state machine already handles incomplete-batch resume via atomic `Save`. The behavior exists but is undocumented. Add a comment so future maintainers do not duplicate logic.

### File: `internal/state/state.go`

**Current (line 119):**
```go
// Save writes the state to path atomically (write .tmp, rename).
func Save(s *State, path string) error {
```

**Replace comment with:**
```go
// Save persists s atomically (temp+rename). If the process is interrupted
// mid-write, the previous state file remains intact and the next run
// resumes from the last committed phase. No additional resume logic is
// required — the state machine handles incomplete batches by design.
func Save(s *State, path string) error {
```

No code changes. No test updates.

---

## File-by-file change summary

| File | Pattern | Lines changed | Risk |
|---|---|---|---|
| `internal/verify/verify.go` | 1 (WriteReport), 3 (Run sig + loop) | ~20 | LOW |
| `internal/verify/archive.go` | 1 (writeManifest) | ~7 | LOW |
| `internal/cli/errs/errs.go` | 2 (transportError type + WriteError) | ~15 | LOW |
| `internal/context/cache.go` | 4 (TTL map + format + parsing) | ~25 | MEDIUM |
| `internal/cli/commands.go` | 3 (verify.Run wiring) | 1 | LOW |
| `internal/state/state.go` | 5 (comment only) | 3 | LOW |

---

## Implementation order

```
1a  verify.go WriteReport atomic       — independent
1b  archive.go writeManifest atomic    — independent
2   errs.go Transport type             — independent
3   verify.go Run signature + loop     — independent
3w  commands.go verify.Run wiring      — after 3
4   cache.go TTL                       — independent
5   state.go comment                   — independent
```

Patterns 1a, 1b, 2, 4, 5 can land in any order relative to each other. Pattern 3w must follow 3.

---

## Import delta per file

| File | Imports added |
|---|---|
| `internal/verify/verify.go` | `"io"` |
| `internal/context/cache.go` | `"strconv"` |
| All others | none |
