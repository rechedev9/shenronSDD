# Spec: `sdd diff <name>`

## 1. Overview

`sdd diff <name>` lists project files modified since the change was created. It shells out to `git diff --name-only <BaseRef>` and emits a machine-readable JSON result on stdout. It is a zero-token, read-only operation.

---

## 2. State struct change

**File:** `internal/state/types.go`

Add one field to `State`:

```go
BaseRef string `json:"base_ref,omitempty"`
```

Position: after `UpdatedAt`, before any future fields. The `omitempty` tag ensures old state files that lack the field deserialize without error. `validate()` in `internal/state/state.go` must NOT require this field — existing validation logic is unchanged.

`NewState()` in `types.go` does not set `BaseRef`; it is populated by the caller (`runNew`) after `git rev-parse HEAD`.

---

## 3. Capture `BaseRef` at `sdd new` time

**File:** `internal/cli/commands.go`, function `runNew`

After `state.NewState(name, description)` and before `state.Save(st, statePath)`, run:

```
git rev-parse HEAD
```

via `exec.Command("git", "rev-parse", "HEAD")` with `cmd.Dir = cwd` (the project root, which is `cwd` already in scope). Trim trailing whitespace from stdout. On success, set `st.BaseRef` to the trimmed SHA.

On any failure (non-zero exit, not a git repo, empty repo, no commits yet), write a warning to `stderr`:

```
warning: could not capture git base ref: <err>
```

Leave `st.BaseRef` empty and continue — do not abort `sdd new`.

---

## 4. `runDiff` function

**File:** `internal/cli/commands.go`

### 4.1 Signature

```go
func runDiff(args []string, stdout io.Writer, stderr io.Writer) error
```

### 4.2 Argument validation

```go
if len(args) < 1 {
    return errs.Usage("usage: sdd diff <name>")
}
```

Returns `errs.Usage(...)` → exit code 2.

### 4.3 Resolve change directory

Use existing `resolveChangeDir(name)`. If the change directory is not found, propagate via `errs.WriteError(stderr, "diff", err)` → exit code 1.

### 4.4 Load state

```go
st, err := state.Load(statePath)
```

On failure: `errs.WriteError(stderr, "diff", fmt.Errorf("load state: %w", err))` → exit code 1.

### 4.5 Guard: missing `BaseRef`

```go
if st.BaseRef == "" {
    return errs.WriteError(stderr, "diff",
        fmt.Errorf("base_ref not recorded; change was created before diff support was added — re-run 'sdd new' or manually add \"base_ref\" to state.json"))
}
```

Exit code 1.

### 4.6 Run `git diff`

Use an unexported helper at package scope:

```go
func gitDiff(dir, ref string) ([]string, error)
```

Implementation:

1. `exec.Command("git", "diff", "--name-only", ref)` with `cmd.Dir = dir`.
2. Collect combined stderr into a separate buffer for error reporting.
3. On non-zero exit: return `fmt.Errorf("git diff failed: %s", strings.TrimSpace(stderrBuf.String()))`.
4. Split stdout on `"\n"`, filter empty strings. Return the slice.

In `runDiff`, call `gitDiff(cwd, st.BaseRef)`. On error: `errs.WriteError(stderr, "diff", err)` → exit code 1.

### 4.7 Success output

Write JSON to `stdout` (not stderr):

```json
{
  "command": "diff",
  "status": "success",
  "change": "<name>",
  "base_ref": "<sha>",
  "files": ["path/to/file.go"],
  "count": 1
}
```

When no files are modified, `"files"` is an empty JSON array `[]` and `"count"` is `0`.

Use `json.MarshalIndent(out, "", "  ")` consistent with all other commands. Write via `fmt.Fprintln(stdout, string(data))`.

---

## 5. CLI dispatch wiring

**File:** `internal/cli/cli.go`

### 5.1 Switch case

In `Run()`, add before the `default` case:

```go
case "diff":
    return runDiff(rest, stdout, stderr)
```

### 5.2 `commandHelp` entry

Add to the `commandHelp` map:

```go
"diff": `sdd diff — List files modified since change creation

Usage: sdd diff <name>

Runs git diff --name-only against the commit SHA recorded when the change
was created (sdd new). Reports all files modified in the working tree since
that point.

Arguments:
  name          Change name

Output: JSON with base_ref, files list, and count.
Exit:   0 success, 1 error, 2 usage`,
```

### 5.3 `printHelp` entry

Add under "Inspection commands:":

```
  diff <name>       List files modified since change was created
```

---

## 6. Error output format

All errors follow the existing `errs.WriteError` / `errs.Usage` convention. Errors are written as JSON to `stderr`:

```json
{"command":"diff","error":"<message>","code":"internal"}
```

Usage errors use `"code":"usage"`. The function returns the error; `main.go` converts to exit code via `cli.ExitCode(err)`.

---

## 7. Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success — JSON result on stdout |
| 1 | Error — JSON error on stderr (git failure, missing BaseRef, change not found, state load failure) |
| 2 | Usage — missing argument |

---

## 8. Tests

**File:** `internal/cli/cli_test.go`

Table-driven rows using `t.Run()` subtests. All subtests call `t.Parallel()` where safe.

| Subtest name | Setup | Expected exit | Expected output |
|---|---|---|---|
| `diff missing arg` | none | 2 | stderr contains `"command":"diff"` |
| `diff change not found` | no state.json | 1 | stderr contains `"command":"diff"` |
| `diff base_ref empty` | state.json without `base_ref` field | 1 | stderr contains `base_ref not recorded` |
| `diff happy path` | git repo in `t.TempDir()`, initial commit, state with `base_ref` = that SHA, one file modified after | 0 | stdout is valid JSON; `files` contains the modified file; `count` == 1 |
| `diff no changes` | git repo, state with `base_ref`, no modifications after | 0 | stdout is valid JSON; `files` is empty array; `count` == 0 |

### Happy-path setup

```
tmpDir := t.TempDir()
// git init
exec.Command("git", "-C", tmpDir, "init").Run()
exec.Command("git", "-C", tmpDir, "config", "user.email", "test@test").Run()
exec.Command("git", "-C", tmpDir, "config", "user.name", "Test").Run()
// create initial commit
os.WriteFile(filepath.Join(tmpDir, "seed.txt"), []byte("seed"), 0o644)
exec.Command("git", "-C", tmpDir, "add", ".").Run()
exec.Command("git", "-C", tmpDir, "commit", "-m", "init").Run()
// capture SHA
out, _ := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD").Output()
baseRef := strings.TrimSpace(string(out))
// create change with base_ref
st := state.NewState("my-change", "desc")
st.BaseRef = baseRef
// ... save state, chdir to tmpDir, run diff ...
```

Modify a file after saving state to simulate work done during the change.

### `diff base_ref empty` setup

Use existing `setupChange` helper (which calls `state.NewState` without setting `BaseRef`), chdir, run `sdd diff <name>`, assert exit 1 and error message in stderr.

---

## 9. Out of scope

- Branch management, staging, or commits.
- Filtering by subdirectory or file pattern.
- Non-git VCS.
- Interactive/hunk rendering.
- A dedicated `internal/diff` package (single consumer; no abstraction until Rule of 3).
- Re-rooting paths relative to `openspec/` or project subdirectory (paths from `git diff` are returned verbatim).

---

## 10. Backward compatibility

Old state files without `base_ref` deserialize cleanly because the field is `omitempty`. `validate()` does not require it. The command detects the empty value and returns a clear error with instructions rather than crashing or producing a confusing result.

---

## 11. Rollback

Remove `case "diff":` from the switch in `cli.go`, remove `runDiff` and `gitDiff` from `commands.go`, remove the `commandHelp` entry and `printHelp` line, and remove the `BaseRef` field from `types.go`. No state file migration is needed — the `omitempty` field is simply ignored after removal.
