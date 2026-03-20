# Proposal: `sdd diff <name>`

## Intent

Add a `sdd diff <name>` subcommand that answers "which project files changed while this change was in progress?" It produces a machine-readable JSON list of modified paths by diffing the current git working tree against the commit that was HEAD when `sdd new` was run.

---

## Scope

### In

- `sdd diff <name>` command — reads state, shells out `git diff --name-only <BaseRef>`, emits JSON on stdout.
- `BaseRef string` field added to `State` struct — captured at `sdd new` time via `git rev-parse HEAD`.
- Backward-compatible handling of old state files that lack `BaseRef` — clear error, no crash.
- Error-case and happy-path tests in `cli_test.go`.

### Out

- Full git integration: no branch management, no staging, no commits.
- Filtering output by subdirectory or file pattern.
- Support for non-git version control systems.
- Interactive diff rendering (raw paths only, no hunks).
- A dedicated `internal/diff` package — single consumer, no abstraction until Rule of 3 applies.

---

## Approach

### 1. Extend `State` with `BaseRef`

In `internal/state/types.go`, add one field:

```go
BaseRef string `json:"base_ref,omitempty"`
```

`omitempty` ensures existing state files deserialize without error; `validate()` must not require the field.

### 2. Capture SHA at `sdd new` time

In `runNew()` (`internal/cli/commands.go`), after creating the state struct and before saving, run:

```
git rev-parse HEAD
```

via `exec.Command` with `cmd.Dir = projectDir`. On success, set `st.BaseRef` to the trimmed output. On any failure (not a git repo, empty repo, detached HEAD with no commits), log a warning to stderr and leave `BaseRef` empty — do not abort `sdd new`.

### 3. Add `runDiff()` command

In `internal/cli/commands.go`:

1. Parse and validate `args[1]` as change name (same pattern as other commands).
2. Load state via `state.Load`.
3. If `st.BaseRef == ""`, return exit-1 JSON error: `"base_ref not recorded; change was created before diff support was added"`.
4. Run `exec.Command("git", "diff", "--name-only", st.BaseRef)` with `cmd.Dir = projectDir`. Collect stdout, split on newlines, filter empty strings.
5. Write JSON to stdout:

```json
{
  "command": "diff",
  "status": "success",
  "change": "<name>",
  "base_ref": "<sha>",
  "files": ["internal/foo/bar.go"],
  "count": 1
}
```

No new package. A small unexported `gitDiff(dir, ref string) ([]string, error)` helper inside `commands.go` keeps the function testable via injection if needed.

### 4. Wire into CLI dispatch

In `internal/cli/cli.go`:
- Add `case "diff":` to the `Run()` switch calling `runDiff(projectDir, args)`.
- Add `"diff"` entry in `commandHelp` map with usage string.
- Add `diff` line to `printHelp()`.

### 5. Tests

Table-driven rows in `cli_test.go`:

| scenario | setup | expected |
|---|---|---|
| Missing argument | no change | exit 2, usage error JSON |
| Change does not exist | no state.json | exit 1, not-found error JSON |
| `BaseRef` empty | state without `base_ref` | exit 1, descriptive error JSON |
| Happy path | real `git init` + commit in `t.TempDir()`, state with `base_ref` = first commit SHA, one file modified after | exit 0, JSON with that file listed |

The happy-path test initialises a bare git repo in `t.TempDir()` so `git diff` can actually run — same technique available to `verify` tests.

---

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Project root is not a git repo | Medium | `git diff` exits non-zero, raw error surfaces | Check exit code; emit structured error: `"git diff failed: <stderr>"` |
| Old change missing `BaseRef` | High (all existing changes) | Command unusable for them | Detected on load; clear error message with instructions to manually add `base_ref` to state.json |
| Detached HEAD / empty repo at `sdd new` time | Low | `git rev-parse HEAD` fails | `runNew` swallows the error gracefully; leaves `BaseRef` empty; warns on stderr |
| SHA no longer reachable (force-push, history rewrite) | Low | `git diff <SHA>` fails | Propagate git error as structured JSON; user can resolve externally |
| Monorepo: git root above project root | Low | Paths include subdirectory prefix | Documented behaviour; no re-rooting attempted (avoids silent mis-filtering) |

---

## Rollback

Remove `case "diff":` from the switch, remove `runDiff()` from `commands.go`, remove the `commandHelp`/`printHelp` entries, and drop the `BaseRef` field from `types.go`. Because `base_ref` is `omitempty`, existing state files with the field present will still deserialize without error after rollback — the field is simply ignored. No migration required.
