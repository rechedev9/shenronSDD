# Exploration: `sdd diff <name>`

## Current State

### Command Dispatch

All commands live in two files:

- `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/cli.go` — `Run()` switch, `printHelp()`, `commandHelp` map.
- `/home/reche/projects/SDDworkflow/sdd-cli/internal/cli/commands.go` — one `run*` function per command.

Adding `diff` requires: a new `case "diff":` in the switch, a `runDiff()` function in `commands.go`, a help string in `commandHelp`, and a line in `printHelp()`.

### State Machine

`/home/reche/projects/SDDworkflow/sdd-cli/internal/state/types.go` — `State` struct carries no list of modified files; it only tracks phase/status. There is no stored baseline (e.g., git SHA at `sdd new` time).

`/home/reche/projects/SDDworkflow/sdd-cli/internal/state/state.go` — `Save`/`Load`/`Advance`/`Recover`. Atomic write via `.tmp` + rename.

### Artifact Layout in a Change Directory

A fully-progressed change directory contains:

```
openspec/changes/<name>/
  state.json              # phase state machine
  exploration.md          # PhaseExplore promoted artifact
  proposal.md             # PhasePropose
  specs/                  # PhaseSpec (directory, not file)
    spec.md               # (filename matches pending: "spec.md")
  design.md               # PhaseDesign
  tasks.md                # PhaseApply (and PhaseTasks, same file)
  review-report.md        # PhaseReview
  verify-report.md        # PhaseVerify
  clean-report.md         # PhaseClean
  archive-manifest.md     # PhaseArchive
  .pending/               # in-flight artifacts (Claude writes here)
    <phase>.md
```

Source: `artifacts.ArtifactFileName` map in `/home/reche/projects/SDDworkflow/sdd-cli/internal/artifacts/artifacts.go`.

### Git Access

No git library or git-running utility exists in the codebase today. The project's only external dependency is `gopkg.in/yaml.v3`. The `verify` package runs shell commands via `os/exec` (`sh -c`), but there is no shared shell-execution helper exposed to other packages — `runOne` is unexported.

### Output Convention

All commands write structured JSON to stdout on success. Errors go to stderr as JSON via `errs.WriteError`. Exit codes: 0 success, 1 error, 2 usage. Pattern is consistent across all commands.

### Test Harness

Tests use `t.TempDir()` + `os.Chdir` to simulate a project root. The `setupChange` helper in `cli_test.go` creates a minimal `state.json`. Tests assert JSON fields and exit codes. A new `diff` command needs analogous table-driven tests.

---

## Affected Areas

| File | Change |
|---|---|
| `internal/cli/cli.go` | Add `case "diff":` to switch; add entry in `printHelp()`; add entry in `commandHelp` |
| `internal/cli/commands.go` | Add `runDiff()` function |
| `internal/cli/cli_test.go` | Add error-case rows for `diff` missing args / no change; add happy-path test |

Optionally, if git invocation is abstracted:

| File | Change |
|---|---|
| `internal/diff/diff.go` *(new package)* | `Run(projectDir, since string) ([]string, error)` — wraps `git diff --name-only` |

No changes needed to: `state/`, `artifacts/`, `context/`, `verify/`, `config/`.

---

## Approach

### What the command does

`sdd diff <name>` answers: "which project files have changed since this change began?"

The canonical signal is git: run `git diff --name-only <ref>` from the project root, where `<ref>` is determined by what we know about the change's start.

### Option A — diff against state creation time (simplest, no stored SHA)

Run `git log --since=<created_at> --name-only --pretty=format:` filtered to project files. Does not require storing anything in state.json. Works without modifying the state struct. Fragile if the clock drifts or commits span multiple days.

### Option B — store baseline SHA in state.json at `sdd new` time (recommended)

Extend `State` to carry `BaseRef string` (the git SHA captured when `sdd new` runs). `sdd diff` then runs `git diff --name-only <BaseRef>...HEAD`. Deterministic. Survives clock changes. Requires:

1. Adding `BaseRef string \`json:"base_ref,omitempty"\`` to `State` in `types.go`.
2. In `runNew()` in `commands.go`, capture `git rev-parse HEAD` and set `st.BaseRef` before saving.
3. `runDiff()` reads `st.BaseRef`, shells out `git diff --name-only <BaseRef>`, prints JSON.

Fallback: if `BaseRef` is empty (change created before this feature), fall back to Option A or return a clear error.

### Option C — no git, scan modification times (zero-dep, limited)

Walk the project tree, compare mtime against `state.CreatedAt`. No git required. Misses staged-but-not-written changes, unreliable for files touched by tools.

**Recommendation: Option B.** Deterministic, no new Go module dependencies (stdlib `os/exec` already used by `verify`), aligns with how git-native workflows think about "what changed in this branch."

### Output format

```json
{
  "command": "diff",
  "status": "success",
  "change": "<name>",
  "base_ref": "<sha>",
  "files": ["internal/foo/bar.go", "internal/baz/baz.go"],
  "count": 2
}
```

Mirrors the JSON-on-stdout convention used by all other commands.

### Git invocation

Shell out via `os/exec` (same pattern as `verify.runOne`). Use `exec.Command("git", "diff", "--name-only", baseRef)` with `cmd.Dir = projectDir`. No new packages required — stdlib only. Keep it in `commands.go` directly (or a small unexported `gitDiff` helper in that file) unless/until a second consumer exists (Rule of 3).

---

## Risks

**1. No git repository.** `git diff` fails if the project root is not a git repo. Must detect and return a clear error rather than letting the raw `git` error surface. Check: run `git rev-parse --git-dir` first, or interpret a non-zero exit from `git diff` as a specific error message.

**2. BaseRef not stored (old changes).** Changes created before Option B is implemented will have an empty `BaseRef`. Need a defined fallback behavior — either error with a message ("re-run sdd new or set base_ref manually"), or fall back to `git diff HEAD~N` heuristics. Simplest safe choice: return a clear error with instructions.

**3. Detached HEAD / no commits.** `git rev-parse HEAD` fails on an empty repo. `sdd new` must handle this gracefully (skip storing BaseRef, not crash). The diff command must handle missing BaseRef.

**4. Large working trees.** `git diff --name-only` output can be long; this is fine since we just collect lines into a slice. No performance concern at project scale.

**5. Paths.** `git diff --name-only` outputs paths relative to the git repo root, which may differ from `cwd`. If the sdd project is a subdirectory of a larger monorepo git root, paths will include the subdirectory prefix. The command should document this and not attempt to re-root paths (could silently mis-filter).

**6. State struct is serialized.** Adding `BaseRef` to `State` is a backward-compatible JSON change (`omitempty` handles old files). The `validate()` function in `state/state.go` must not be updated to require `BaseRef` — it remains optional.

**7. Test isolation.** Tests for `runDiff` will need a real git repo or a fake git binary. The existing test pattern uses `os.Chdir` to a `t.TempDir()`, which is not a git repo. Options: (a) inject a git command runner via a function var (testable), (b) init a bare git repo in `t.TempDir()`, (c) table-test only the error paths. Option (b) or (c) is simplest and keeps tests hermetic.
