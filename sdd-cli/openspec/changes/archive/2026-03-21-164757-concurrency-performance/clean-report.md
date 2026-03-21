# Clean Report: concurrency-performance

## Dead Code Found

- `emitMetrics` — confirmed fully removed from all `.go` source files. No live references in Go code anywhere. Only appears in `.md` documentation/design files and cached context files under `openspec/`, which is expected. The refactor is complete.
- `LazySlice.MustGet(i int) T` in `internal/csync/lazyslice.go:113` — defined but never called from production code. Only exercised by `lazyslice_test.go`. Not harmful (tests cover it), but it is an exported method with no non-test callsites. Candidate for removal if the public API of `csync` is ever trimmed.
- `LazySlice.Len() int` in `internal/csync/lazyslice.go:49` — same situation: only called from `lazyslice_test.go`, not from any assembler or CLI code.

## Unused Imports

- None. All imports in the five files under review compile cleanly (`go build ./...` passes with zero warnings).

## Other Observations

- **Duplicate comment on `LoadPipelineMetrics`** (`internal/context/cache.go:315-316`): two consecutive `//` lines say almost the same thing ("reads the cumulative metrics file, or creates a new one" vs "reads the cumulative metrics file for a change"). One is a copy-paste artifact from a prior editing session. Should be collapsed to a single line before next release.
- **Duplicate inline comment in `AssembleConcurrent`** (`internal/context/context.go:169-170`): `// Partial output is intentional.` appears twice back-to-back. The second line is the more precise version; the first is a leftover. Minor noise, no correctness impact.
- **`sync` import in `context.go`** still needed — `AssembleConcurrent` uses `sync.WaitGroup` directly rather than delegating to `csync.LazySlice`. This is intentional (phases, not loaders), so the import is legitimate.
- Build: `go build ./...` — PASS (zero errors, zero warnings).
- Tests: `go test ./...` — all 9 packages PASS.

## Verdict

NEEDS_CLEANUP

Two cosmetic issues require a single follow-up commit (no logic changes, no file deletions):
1. Collapse duplicate doc comment on `LoadPipelineMetrics` (cache.go:315-316).
2. Remove duplicate inline comment in `AssembleConcurrent` (context.go:169).

The `MustGet`/`Len` test-only methods are low-priority; they are exported API surface but are covered by tests and cause no harm. Flag for future API trim if `csync` is ever stabilised as a public package.
