# Proposal: 5 steipete patterns for sdd-cli

**Change:** steipete-patterns
**Phase:** propose
**Date:** 2026-03-20

---

## Summary

Implement 5 hardening patterns across 5 existing files. No new packages, no new dependencies, no breaking API changes visible outside `internal/`. Each pattern is independently revertable.

---

## Scope

**In:**
1. Atomic writes — `verify.WriteReport` + `archive.writeManifest`
2. Error classification — `errs.Transport` / `errs.IsTransport` + `WriteError` update
3. Progress logging — `verify.Run` gains `progress io.Writer` parameter
4. Per-dimension TTL cache — `context/cache.go` embeds timestamp in hash file
5. Incomplete-batch resume — already handled by the state machine; document only

**Out:**
- No new packages or modules
- No new external dependencies
- No changes to public-facing command output format or exit codes
- No config.yaml schema additions

---

## Priority order and rationale

### 1. Atomic writes (LOW risk)

**Why first:** Zero behavior change for callers; eliminates data loss on crash. Mirrors the pattern already used in `state.Save`. Quickest to implement and verify.

**Files:** `internal/verify/verify.go:195`, `internal/verify/archive.go:97`

**Change:** Replace `os.WriteFile(path, data, 0o644)` with temp+rename at both sites:

```go
tmp := path + ".tmp"
if err := os.WriteFile(tmp, data, 0o644); err != nil {
    return err
}
if err := os.Rename(tmp, path); err != nil {
    os.Remove(tmp)
    return err
}
```

Reference implementation: `state/state.go:120–141`.

**Tests:** `verify_test.go` — existing tests exercise the full write path; no new cases needed. Add one crash-simulation test (write to read-only dir) to confirm tmp cleanup.

**Rollback:** Revert two call sites. No state migration.

---

### 2. Error classification (LOW risk)

**Why second:** Additive only. New exported symbols, no existing behavior changed. Lays groundwork for callers to upgrade incrementally.

**File:** `internal/cli/errs/errs.go`

**Change — new type and constructors:**

```go
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

**Change — update `WriteError` classifier:**

```go
code := "internal"
if IsUsage(err) {
    code = "usage"
} else if IsTransport(err) {
    code = "transport"
}
```

`WriteJSON` stays unchanged (`"not_implemented"` is correct for stub paths).

**Call site adoption:** Optional and incremental. Callers wrapping file-system or exec errors may switch to `errs.Transport(...)`. Existing `"internal"` classification degrades gracefully until they do.

**Tests:** `errs_test.go` — add table rows for `Transport()`, `IsTransport()`, and `WriteError` with a transport error.

**Rollback:** Delete the new type and constructors; revert `WriteError` classifier. No callers depend on `"transport"` code until they opt in.

---

### 3. Progress logging for verify (LOW risk)

**Why third:** Pattern 4 (commands.go wiring) depends on this. Adding the parameter first, then wiring it, keeps the diff reviewable per-step.

**File:** `internal/verify/verify.go`

**Change — extend `Run` signature:**

```go
// Before
func Run(workDir string, commands []CommandSpec, timeout time.Duration) (*Report, error)

// After
func Run(workDir string, commands []CommandSpec, timeout time.Duration, progress io.Writer) (*Report, error)
```

**Change — emit progress lines inside the loop:**

```go
if progress != nil {
    fmt.Fprintf(progress, "sdd: verify %s...\n", spec.Name)
}
result := runOne(ctx, workDir, spec)
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

`runOne` is unchanged — it still captures all output into its internal buffer.

**Tests:** `verify_test.go` — update every `verify.Run(...)` call site to pass `nil`. Add one subtest passing a `bytes.Buffer` and asserting progress lines appear.

**Rollback:** Revert signature and loop additions; update `verify_test.go` call sites back.

---

### 4. Per-dimension TTL cache (MEDIUM risk)

**Why fourth:** Requires a hash-file format change. MEDIUM risk because existing caches become misses (not corrupt). Placed after the LOW-risk items so those are stable before this lands.

**File:** `internal/context/cache.go`

**Change — TTL map:**

```go
var phaseTTL = map[string]time.Duration{
    "propose": 4 * time.Hour,
    "spec":    4 * time.Hour,
    "design":  4 * time.Hour,
    "tasks":   2 * time.Hour,
    "apply":   1 * time.Hour,
    "review":  1 * time.Hour,
    "clean":   30 * time.Minute,
    // "explore" omitted — never cached (no phaseInputs entry)
}
```

**Change — hash file format** (stored as `{hash}|{unix_seconds}`):

In `saveContextCache`:
```go
line := fmt.Sprintf("%s|%d", hash, time.Now().Unix())
os.WriteFile(hashCachePath(changeDir, phase), []byte(line), 0o644)
```

In `tryCachedContext` — parse and check TTL after hash match:
```go
raw := strings.TrimSpace(string(storedHash))
hash, ts, found := strings.Cut(raw, "|")
if !found {
    // Legacy format — force miss, will be rewritten on next save
    return nil, false
}
writtenAt := time.Unix(mustParseInt64(ts), 0)
if ttl, ok := phaseTTL[phase]; ok && time.Since(writtenAt) > ttl {
    return nil, false
}
if hash != currentHash {
    return nil, false
}
```

`mustParseInt64` is a file-local helper (5 lines, no new import beyond `strconv`).

**Backward compatibility:** Legacy hash files (no `|`) produce a cache miss, not an error. On next successful context assembly the file is overwritten with the new format. No migration script needed.

**Tests:** `context_test.go` — existing cache-hit/miss tests need updating because `tryCachedContext` now requires a timestamp in the hash file. Write test helpers that pre-write the new format. For TTL expiry: write a hash file with `time.Now().Add(-5 * time.Hour).Unix()` and assert miss for a phase with a 4-hour TTL.

**Rollback:** Revert `phaseTTL`, `saveContextCache`, and `tryCachedContext` to their previous forms. Old-format caches are already gone (overwritten), but that is a cache miss only — no data loss.

---

### 5. Incomplete-batch resume — documentation only

**Why last / no code change:** The exploration confirmed that the state machine in `internal/state/` already handles interrupted runs correctly. `state.Save` is atomic (temp+rename), so phase state is never partially committed. Re-running a command after a crash picks up from the last successfully persisted state.

**Action:** Add a comment block in `internal/state/state.go` above `Save`:

```go
// Save persists s atomically (temp+rename). If the process is interrupted
// mid-write, the previous state file remains intact and the next run
// resumes from the last committed phase. No additional resume logic is
// required — the state machine handles incomplete batches by design.
```

No behavioral change. No test update needed.

**Rollback:** Delete the comment.

---

## File-by-file change summary

| File | Patterns | Risk |
|---|---|---|
| `internal/verify/verify.go` | 1 (WriteReport), 3 (Run sig + loop) | LOW |
| `internal/verify/archive.go` | 1 (writeManifest) | LOW |
| `internal/cli/errs/errs.go` | 2 (transportError + WriteError) | LOW |
| `internal/context/cache.go` | 4 (TTL map + format change) | MEDIUM |
| `internal/cli/commands.go` | 3 wiring (runVerify), progress lines (runArchive) | LOW |
| `internal/state/state.go` | 5 (doc comment only) | LOW |

---

## Dependency order for implementation

```
Pattern 1a  (verify.go WriteReport)    — independent
Pattern 1b  (archive.go writeManifest) — independent
Pattern 2   (errs.go)                  — independent
Pattern 3   (verify.go Run sig)        → must land before Pattern 3-wiring in commands.go
Pattern 3-wiring (commands.go)         → after Pattern 3
Pattern 4   (cache.go)                 — independent
Pattern 5   (state.go comment)         — independent
```

---

## Tests to update

| Test file | Required change |
|---|---|
| `internal/verify/verify_test.go` | Pass `nil` for new `progress` arg; add progress-output subtest |
| `internal/cli/errs/errs_test.go` | Add table rows for `Transport`, `IsTransport`, `WriteError` with transport error |
| `internal/context/context_test.go` | Update hash-file fixtures to new `hash\|ts` format; add TTL-expiry subtest |

`internal/cli/integration_test.go` passes `io.Discard` for stderr — no change required.

---

## Resolved open questions

1. **`phaseTTL` user-configurable?** No. Hardcode for now; add `config.yaml` support in a separate change if needed.
2. **`Transport` errors → different exit code?** No. Exit code 1; `code: "transport"` in the JSON body is the machine-readable discriminator.
3. **TTL wall-clock or process-age?** Wall-clock (`time.Now().Unix()`). Correct for cross-session staleness; simpler to implement and test.
