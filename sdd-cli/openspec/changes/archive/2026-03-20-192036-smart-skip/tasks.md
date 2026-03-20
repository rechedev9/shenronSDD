# Tasks: smart-skip

## Phase 1: Smart-Skip Verify
- [x] Add shouldSkipVerify function in commands.go
- [x] Update runVerify to call shouldSkipVerify before verify.Run
- [x] Add Skipped field to verify JSON output

## Phase 2: Concurrent Assembly
- [x] Add ReadyPhases to state.go
- [x] Add "sync" import to context.go
- [x] Add AssembleConcurrent function to context.go
- [x] Update runContext to use AssembleConcurrent when multiple phases ready

## Phase 3: Tests
- [x] Updated existing verify.Run calls (Phase 3 from previous change)
- [x] TestRun_ProgressOutput added (Phase 3 from previous change)
