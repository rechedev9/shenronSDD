# Tasks: steipete-patterns

## Phase 1: Atomic Writes
- [x] Replace os.WriteFile in WriteReport (verify.go) with temp+rename
- [x] Replace os.WriteFile in writeManifest (archive.go) with temp+rename

## Phase 2: Error Classification
- [x] Add transportError type to errs.go
- [x] Add Transport() constructor and IsTransport() checker
- [x] Update JSONError.Code doc comment to include "transport"
- [x] Update WriteError classifier to emit "transport" code

## Phase 3: Progress Logging
- [x] Add "io" import to verify.go
- [x] Add progress io.Writer parameter to verify.Run signature
- [x] Emit progress lines before/after each command
- [x] Wire stderr as progress in runVerify (commands.go)

## Phase 4: Per-Dimension TTL Cache
- [x] Add "strconv" and "time" imports to cache.go
- [x] Add phaseTTL map with per-phase durations
- [x] Update saveContextCache to write hash|timestamp format
- [x] Update tryCachedContext to parse new format + enforce TTL
- [x] Bump cacheVersion to 3

## Phase 5: Document Resume
- [x] Add comment block to state.go explaining resume invariant

## Phase 6: Tests
- [x] Update all existing verify.Run calls to pass nil progress
- [x] Add TestRun_ProgressOutput to verify_test.go
- [x] Add TestTransportError and TestIsTransportNonTransport to errs_test.go
- [x] Add transport row to TestWriteError table
