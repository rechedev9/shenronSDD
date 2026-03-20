# Tasks: resilience-patterns — ALL COMPLETE

## Phase 1: Skill-Hash Cache Fix
- [x] inputHash now includes SKILL.md content
- [x] tryCachedContext/saveContextCache pass skillsPath
- [x] explore phase now cacheable
- [x] cacheVersion bumped to 4

## Phase 2: Zombie Detection
- [x] IsStale + StaleHours on State
- [x] runStatus outputs updated_at, stale, stale_hours
- [x] runList outputs stale per change

## Phase 3: Partial-Failure Accumulator
- [x] AssembleConcurrent writes successes, collects errors

## Phase 4: Health Command
- [x] Exported PipelineMetrics, PhaseMetrics, LoadPipelineMetrics
- [x] Removed writePipelineSummary
- [x] runHealth with warnings
- [x] Wired in cli.go
