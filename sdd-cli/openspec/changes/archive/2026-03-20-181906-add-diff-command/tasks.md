# Tasks: `sdd diff <name>`

## Phase 1: State struct + BaseRef capture

- [x] Add `BaseRef string` field to `State` struct in `internal/state/types.go`
- [x] Add `gitHeadSHA(dir string)` helper in `internal/cli/commands.go`
- [x] Update `runNew` to capture BaseRef after state creation

## Phase 2: diff command

- [x] Add `runDiff` function in `internal/cli/commands.go`
- [x] Add `gitDiffFiles` helper in `internal/cli/commands.go`
- [x] Wire `case "diff":` in `cli.go` switch
- [x] Add `"diff"` entry to `commandHelp` map
- [x] Add `diff <name>` line to `printHelp`

## Phase 3: Tests

- [x] Add `TestRunDiff` with happy path + no base_ref error case
- [x] Add diff error cases to `TestRunSubcommands`
- [x] Add diff to `TestRunErrorsWriteJSON`
