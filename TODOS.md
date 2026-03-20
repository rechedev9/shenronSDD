# TODOS

## Post-MVP

### SKILL.md Compression
Parse SKILL.md files and strip sections not relevant to the current project's tech stack (e.g., remove Python examples when working on a Go project). Could cut context size by 30-50% on top of structural savings. Requires machine-parseable markers or heuristic parsing. Risk: accidentally stripping useful content.
**Depends on:** Core CLI working end-to-end.

### Token Usage Tracking & Analytics
`sdd write --tokens N` logs token count per phase to state.json. `sdd analytics <name>` prints cost-per-phase, total cost, comparison to estimated baseline. Proves the CLI saves tokens and identifies expensive phases. Requires Claude Code to report token usage.
**Depends on:** Core CLI working end-to-end.

### Phase Caching (Explore)
Cache exploration context by content hash of source files. If files haven't changed, `sdd context explore` returns cached result instantly. Eliminates re-exploration after context compaction or session restart. Content-hash approach: any file change invalidates cache. Simple and sufficient for personal use.
**Depends on:** Core CLI + context assemblers working.
