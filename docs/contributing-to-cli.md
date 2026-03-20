---
summary: "How to contribute to the sdd CLI: build, test, add commands, add patterns."
read_when:
  - Adding a new command to sdd
  - Modifying an assembler
  - Adding a new Go CLI pattern
---

# Contributing to SDD CLI

## Build

```bash
cd sdd-cli
CGO_ENABLED=0 go build -o bin/sdd ./cmd/sdd
```

## Test

```bash
go test ./... -count=1 -timeout=60s
go vet ./...
gofmt -l internal/
```

## Adding a new command

1. Add `runFoo` function in `internal/cli/commands.go` following existing pattern:
   - Parse args, `resolveChangeDir`, `state.Load`, business logic, JSON output
   - Errors via `errs.WriteError(stderr, "foo", err)`
   - Exit codes: 0 success, 1 error, 2 usage

2. Wire in `internal/cli/cli.go`:
   - Add `case "foo":` in `Run()` switch
   - Add line in `printHelp()`
   - Add entry in `commandHelp` map

3. Add tests in `internal/cli/cli_test.go`:
   - Error rows in `TestRunSubcommands` and `TestRunErrorsWriteJSON`
   - Dedicated `TestRunFoo` with real fixtures

## Adding a new assembler

1. Create `internal/context/{phase}.go` with `Assemble{Phase}(w io.Writer, p *Params) error`
2. Register in `dispatchers` map in `context.go`
3. Add `phaseInputs["{phase}"]` in `cache.go` listing input artifacts
4. Add tests in `context_test.go`

## Modifying an assembler

When changing what an assembler outputs (new sections, removed sections):
- **Bump `cacheVersion`** in `cache.go` — this auto-invalidates all cached contexts
- Update `phaseInputs` if the assembler now reads different artifacts

## Go patterns (from sdd-cli/CLAUDE.md)

- `fmt.Errorf("verb noun: %w", err)` at every call site
- Table-driven tests with `t.Run()` subtests
- `t.TempDir()` for file tests, `t.Parallel()` where safe
- No mocks — hand-written fakes only
- Interfaces only where consumed (consumer-defined)
- One file per concern within a package
- `CGO_ENABLED=0` always
- Rule of 3: no abstraction until 3+ real uses

## Using SDD to improve SDD (dogfooding)

```bash
sdd init --force        # if openspec/ already exists
sdd new my-improvement "description"
# ... run pipeline via /sdd:continue ...
sdd verify my-improvement
sdd archive my-improvement
```

The pipeline works on itself. We've completed 3 full SDD cycles on this project.
