# Design: `sdd diff <name>`

## Overview

Implements `sdd diff <name>` as a zero-token inspection command. Loads change state, shells out `git diff --name-only <BaseRef>`, and emits a structured JSON list of changed file paths. No new packages. All logic lives in the existing `internal/cli` package.

---

## 1. State Struct Change — `internal/state/types.go`

Add one field to `State` after `UpdatedAt`:

```go
BaseRef string `json:"base_ref,omitempty"`
```

**Exact insertion point** — after line 38 (`UpdatedAt time.Time`):

```go
type State struct {
    Name         string                `json:"name"`
    Description  string                `json:"description"`
    CurrentPhase Phase                 `json:"current_phase"`
    Phases       map[Phase]PhaseStatus `json:"phases"`
    CreatedAt    time.Time             `json:"created_at"`
    UpdatedAt    time.Time             `json:"updated_at"`
    BaseRef      string                `json:"base_ref,omitempty"`  // ← new
}
```

`omitempty` means existing `state.json` files without `base_ref` unmarshal cleanly — the field is simply left as `""`. `validate()` in `state.go` must not be modified; it does not check `BaseRef`.

---

## 2. BaseRef Capture — `runNew` in `internal/cli/commands.go`

After `state.Save(st, statePath)` succeeds and before the context assembly block, insert a git SHA capture:

```go
// Capture git HEAD for later diff support. Non-fatal: not all projects use git.
if sha, err := gitHeadSHA(cwd); err == nil {
    st.BaseRef = sha
    // Best-effort re-save; if this fails we still have a valid state without BaseRef.
    _ = state.Save(st, statePath)
} else {
    fmt.Fprintf(stderr, "warning: could not capture git HEAD (not a git repo?): %v\n", err)
}
```

Add the unexported helper at package scope in `commands.go` (not a separate file — single use):

```go
// gitHeadSHA returns the trimmed output of `git rev-parse HEAD` run in dir.
func gitHeadSHA(dir string) (string, error) {
    cmd := exec.Command("git", "rev-parse", "HEAD")
    cmd.Dir = dir
    out, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("git rev-parse HEAD: %w", err)
    }
    return strings.TrimSpace(string(out)), nil
}
```

Add `"os/exec"` to the import block (already has `"strings"`).

**Failure modes handled gracefully:**
- Not a git repo → `git rev-parse HEAD` exits non-zero → warning on stderr, `BaseRef` stays `""`
- Empty repo (no commits) → same path
- Detached HEAD with commits → `git rev-parse HEAD` succeeds, SHA captured

---

## 3. `runDiff` Implementation — `internal/cli/commands.go`

Add after `runArchive`. Full function signature matches the existing pattern:

```go
func runDiff(args []string, stdout io.Writer, stderr io.Writer) error {
    if len(args) < 1 {
        return errs.Usage("usage: sdd diff <name>")
    }

    name := args[0]

    changeDir, err := resolveChangeDir(name)
    if err != nil {
        return errs.WriteError(stderr, "diff", err)
    }

    statePath := filepath.Join(changeDir, "state.json")
    st, err := state.Load(statePath)
    if err != nil {
        return errs.WriteError(stderr, "diff", fmt.Errorf("load state: %w", err))
    }

    if st.BaseRef == "" {
        return errs.WriteError(stderr, "diff",
            fmt.Errorf("base_ref not recorded; change was created before diff support was added — "+
                "add base_ref manually to %s to enable diff", statePath))
    }

    cwd, err := os.Getwd()
    if err != nil {
        return errs.WriteError(stderr, "diff", fmt.Errorf("get working directory: %w", err))
    }

    files, err := gitDiff(cwd, st.BaseRef)
    if err != nil {
        return errs.WriteError(stderr, "diff", fmt.Errorf("git diff failed: %w", err))
    }

    out := struct {
        Command string   `json:"command"`
        Status  string   `json:"status"`
        Change  string   `json:"change"`
        BaseRef string   `json:"base_ref"`
        Files   []string `json:"files"`
        Count   int      `json:"count"`
    }{
        Command: "diff",
        Status:  "success",
        Change:  name,
        BaseRef: st.BaseRef,
        Files:   files,
        Count:   len(files),
    }
    data, _ := json.MarshalIndent(out, "", "  ")
    fmt.Fprintln(stdout, string(data))
    return nil
}
```

Add the unexported helper immediately below (same file, single consumer):

```go
// gitDiff returns the list of files changed between ref and the current working tree.
// Runs `git diff --name-only <ref>` in dir. Trailing empty strings are filtered.
func gitDiff(dir, ref string) ([]string, error) {
    cmd := exec.Command("git", "diff", "--name-only", ref)
    cmd.Dir = dir
    out, err := cmd.Output()
    if err != nil {
        // Capture stderr from git for a useful error message.
        var exitErr *exec.ExitError
        if errors.As(err, &exitErr) {
            return nil, fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(exitErr.Stderr)))
        }
        return nil, fmt.Errorf("exec git: %w", err)
    }
    raw := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
    var files []string
    for _, f := range raw {
        if f != "" {
            files = append(files, f)
        }
    }
    return files, nil
}
```

Add `"errors"` to the import block (check — `state.go` already imports it but `commands.go` does not; it needs `errors.As` here).

**Note on `Files` when empty:** `json.MarshalIndent` encodes a nil `[]string` as `null`. Callers should treat both `null` and `[]` as zero files. If a non-null empty array is preferred, initialize `files` as `[]string{}` before the loop.

---

## 4. CLI Routing — `internal/cli/cli.go`

### 4a. Switch case

Add after `case "archive":` (line 45):

```go
case "diff":
    return runDiff(rest, stdout, stderr)
```

### 4b. `printHelp` — Inspection commands section

Add after the `list` line (line 83):

```go
fmt.Fprintln(w, "  diff <name>       List files changed since 'sdd new' was run")
```

### 4c. `commandHelp` map

Add an entry after `"list"`:

```go
"diff": `sdd diff — List files changed since change was created

Usage: sdd diff <name>

Shells out 'git diff --name-only <base_ref>' where base_ref is the git SHA
captured when 'sdd new' was run. Produces a machine-readable JSON list of
all modified paths relative to the git root.

Requires the change to have been created with a version of sdd that records
base_ref. Old changes can have base_ref added manually to state.json.

Arguments:
  name          Change name

Output: JSON with files list, count, and base_ref SHA.
Exit:   0 success, 1 error (including no base_ref, git failure), 2 usage`,
```

---

## 5. Tests — `internal/cli/cli_test.go`

### 5a. Error-case rows in `TestRunSubcommands`

Add to the `tests` slice:

```go
{"diff missing args",  []string{"diff"},             2},
{"diff no change",     []string{"diff", "nonexist"}, 1},
```

### 5b. Error-case row in `TestRunErrorsWriteJSON`

Add to the `tests` slice:

```go
{"diff no change", []string{"diff", "nope"}, `"command":"diff"`},
```

### 5c. `TestRunDiff` — table-driven, new top-level function

Add as a new function in `cli_test.go`:

```go
func TestRunDiff(t *testing.T) {
    t.Parallel()

    // ── Helper: create a real git repo in root ──────────────────────────────
    initGitRepo := func(t *testing.T, root string) string {
        t.Helper()
        mustRun := func(dir string, argv ...string) {
            t.Helper()
            cmd := exec.Command(argv[0], argv[1:]...)
            cmd.Dir = dir
            if out, err := cmd.CombinedOutput(); err != nil {
                t.Fatalf("%v failed: %v\n%s", argv, err, out)
            }
        }
        mustRun(root, "git", "init")
        mustRun(root, "git", "config", "user.email", "test@example.com")
        mustRun(root, "git", "config", "user.name", "Test")
        // Write and commit a file so HEAD exists.
        if err := os.WriteFile(filepath.Join(root, "init.go"), []byte("package main\n"), 0o644); err != nil {
            t.Fatal(err)
        }
        mustRun(root, "git", "add", "init.go")
        mustRun(root, "git", "commit", "-m", "initial")
        // Capture the SHA for state.
        cmd := exec.Command("git", "rev-parse", "HEAD")
        cmd.Dir = root
        sha, err := cmd.Output()
        if err != nil {
            t.Fatalf("rev-parse: %v", err)
        }
        return strings.TrimSpace(string(sha))
    }

    tests := []struct {
        name      string
        setup     func(t *testing.T, root string) string // returns change name
        wantExit  int
        wantFiles []string // nil means don't check
        wantErr   string   // substring in stderr JSON (empty = don't check)
    }{
        {
            name: "happy path - one file modified",
            setup: func(t *testing.T, root string) string {
                sha := initGitRepo(t, root)
                // Set up openspec.
                changeDir := filepath.Join(root, "openspec", "changes", "my-change")
                if err := os.MkdirAll(changeDir, 0o755); err != nil {
                    t.Fatal(err)
                }
                st := state.NewState("my-change", "desc")
                st.BaseRef = sha
                if err := state.Save(st, filepath.Join(changeDir, "state.json")); err != nil {
                    t.Fatal(err)
                }
                // Modify a file after the commit.
                if err := os.WriteFile(filepath.Join(root, "modified.go"), []byte("package main\n// changed\n"), 0o644); err != nil {
                    t.Fatal(err)
                }
                return "my-change"
            },
            wantExit:  0,
            wantFiles: []string{"modified.go"},
        },
        {
            name: "no base_ref in state",
            setup: func(t *testing.T, root string) string {
                changeDir := filepath.Join(root, "openspec", "changes", "old-change")
                if err := os.MkdirAll(changeDir, 0o755); err != nil {
                    t.Fatal(err)
                }
                st := state.NewState("old-change", "desc")
                // BaseRef deliberately left empty.
                if err := state.Save(st, filepath.Join(changeDir, "state.json")); err != nil {
                    t.Fatal(err)
                }
                return "old-change"
            },
            wantExit: 1,
            wantErr:  "base_ref not recorded",
        },
        {
            name: "change does not exist",
            setup: func(t *testing.T, root string) string {
                return "nonexistent"
            },
            wantExit: 1,
            wantErr:  `"command":"diff"`,
        },
    }

    for _, tt := range tests {
        tt := tt
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            root := t.TempDir()

            orig, _ := os.Getwd()
            t.Cleanup(func() { os.Chdir(orig) })
            os.Chdir(root)

            changeName := tt.setup(t, root)

            var stdout, stderr bytes.Buffer
            err := Run([]string{"diff", changeName}, &stdout, &stderr)
            gotExit := ExitCode(err)
            if gotExit != tt.wantExit {
                t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s",
                    gotExit, tt.wantExit, stdout.String(), stderr.String())
            }

            if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
                t.Errorf("stderr = %q, want to contain %q", stderr.String(), tt.wantErr)
            }

            if tt.wantFiles != nil {
                var out struct {
                    Files []string `json:"files"`
                    Count int      `json:"count"`
                }
                if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
                    t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout.String())
                }
                if len(out.Files) != len(tt.wantFiles) {
                    t.Errorf("files = %v, want %v", out.Files, tt.wantFiles)
                }
                // Check each expected file is present (git diff paths are repo-relative).
                for _, wf := range tt.wantFiles {
                    found := false
                    for _, f := range out.Files {
                        if filepath.Base(f) == filepath.Base(wf) {
                            found = true
                            break
                        }
                    }
                    if !found {
                        t.Errorf("expected file %q in diff output %v", wf, out.Files)
                    }
                }
                if out.Count != len(tt.wantFiles) {
                    t.Errorf("count = %d, want %d", out.Count, len(tt.wantFiles))
                }
            }
        })
    }
}
```

Add `"os/exec"` to the import block in `cli_test.go` (alongside the existing imports).

---

## 6. Import Changes Summary

| File | Additions |
|---|---|
| `internal/cli/commands.go` | `"errors"`, `"os/exec"` |
| `internal/cli/cli_test.go` | `"os/exec"` |
| `internal/state/types.go` | none |
| `internal/cli/cli.go` | none |

---

## 7. Invariants and Constraints

- **No new package.** `gitDiff` and `gitHeadSHA` are unexported helpers in `commands.go`. They gain a second consumer only when `runNew` + `runDiff` + one more use exists (Rule of 3).
- **Backward compatibility.** `omitempty` on `BaseRef` means old `state.json` files survive `state.Load` unchanged. `validate()` in `state.go` is not modified.
- **`runNew` is non-aborting.** SHA capture failure is a warning, not a fatal error. The change is created regardless.
- **git working directory.** Both `gitHeadSHA` and `gitDiff` use `cwd` (project root) as `cmd.Dir`, consistent with how `runVerify` passes `cwd` to `verify.Run`.
- **Exit codes.** `"diff"` follows existing conventions: 0 success, 1 general error, 2 usage error.
- **`Files` JSON field.** Initialized as `var files []string` then appended; encodes as `null` when nothing changed. Callers should treat `null` and `[]` identically.
- **Structured errors on stderr.** All error paths go through `errs.WriteError`, producing `{"command":"diff","error":"...","code":"internal"}` on stderr, consistent with all other commands.

---

## 8. Rollback

Remove in order:
1. `case "diff":` from `cli.go` switch
2. `runDiff` and `gitDiff` from `commands.go`
3. The `BaseRef` capture block and `gitHeadSHA` from `runNew` / `commands.go`
4. `"diff"` entry from `commandHelp` and `printHelp`
5. `BaseRef string` field from `types.go`
6. `TestRunDiff` and diff-related rows from `cli_test.go`

State files with `base_ref` already written survive rollback without error — JSON unmarshal ignores unknown fields by default.
