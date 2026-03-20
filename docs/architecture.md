---
summary: "sdd-cli internals: module structure, state persistence, context assembly, and quality gates."
read_when:
  - "Understanding sdd-cli internals"
  - "Adding a new package or command"
---

# sdd-cli Architecture

`sdd` is a Go binary that acts as a context engine for Spec-Driven Development (SDD) inside Claude Code. It handles state persistence, artifact lifecycle, context assembly, and quality-gate execution — all without invoking an LLM itself. The binary is a coordinator: it prepares structured context that Claude consumes and records artifacts that Claude produces.

Module: `github.com/rechedev9/shenronSDD/sdd-cli`

---

## System Overview

```
 User / Claude Code sub-agent
         │
         │  sdd <command> [args]
         ▼
┌─────────────────────────────────────────────────────────────┐
│  cmd/sdd/main.go  (≤15 lines — args + os.Exit only)        │
│         │                                                   │
│         └─► internal/cli.Run(args, stdout, stderr)         │
└─────────────────────────────────────────────────────────────┘
         │
         │ dispatches to one of 10 subcommand handlers
         │ all in internal/cli/commands.go
         │
    ┌────┴────────────────────────────────────────────┐
    │                                                  │
    ▼                                                  ▼
internal/state/          internal/context/
  state.go               context.go  (Assemble / AssembleConcurrent)
  types.go               cache.go    (hash + TTL + versioning)
  ─────────              summary.go  (Context Cascade)
  Phase state machine    explore.go, propose.go, spec.go,
  Atomic persistence     design.go, tasks.go, apply.go,
  Crash recovery         review.go, clean.go
    │
    │
    ▼
internal/artifacts/      internal/verify/
  artifacts.go           verify.go   (Run / WriteReport)
  writer.go              archive.go  (Archive / writeManifest)
  reader.go
  promote.go
  list.go
    │
    ▼
internal/config/
  config.go   (Detect / Load / Save)
  types.go    (Config, Stack, Commands, Capabilities)
  init.go

internal/cli/errs/
  errs.go     (structured JSON error envelope, typed errors)
```

### On-disk layout for a single change

```
openspec/
  config.yaml                     # project config (detected stack, skill path, commands)
  changes/
    <name>/
      state.json                  # authoritative phase state (atomic writes)
      .pending/                   # staging area — Claude writes here
        <phase>.md
      exploration.md              # promoted artifacts (one per completed phase)
      proposal.md
      specs/                      # spec is a directory, not a single file
        spec.md
      design.md
      tasks.md
      review-report.md
      verify-report.md
      clean-report.md
      .cache/                     # transparent cache (content-hash + TTL)
        <phase>.ctx               # cached assembled context bytes
        <phase>.hash              # "{sha256_hex}|{unix_seconds}"
        metrics.json              # cumulative token / cache-hit counters
    archive/
      <timestamp>-<name>/         # completed changes moved here by `sdd archive`
        archive-manifest.md
```

---

## Package Responsibilities

| Package | Responsibility |
|---|---|
| `cmd/sdd` | Entry point only. Parses `os.Args[1:]`, calls `cli.Run`, converts error to exit code via `cli.ExitCode`. |
| `internal/cli` | Subcommand dispatch (`Run`), all command handler functions, help strings. Owns the integration between all other packages. |
| `internal/cli/errs` | Structured JSON error envelope. Three typed error classes (`usageError`, `transportError`, internal). `WriteError` serializes to stderr; `ExitCode` maps error type to exit code (0/1/2). |
| `internal/state` | Phase type definitions, `State` struct, transition graph, prerequisite enforcement, `CanTransition`/`Advance`/`ReadyPhases`, atomic `Save`/`Load`, crash-recovery `Recover`. |
| `internal/context` | Per-phase assembler functions, `Assemble` (single phase), `AssembleConcurrent` (parallel phases), size guard, metrics emission. Delegates caching to `cache.go` and artifact extraction to `summary.go`. |
| `internal/artifacts` | `ArtifactFileName` mapping, `WritePending`/`Read`/`Promote`, `List`/`ListPending`. Owns the `.pending` staging pattern. |
| `internal/verify` | `Run` executes build/lint/test as child processes with timeout and process-group kill. `WriteReport` writes `verify-report.md`. `Archive` moves a change directory and generates `archive-manifest.md`. |
| `internal/config` | Stack auto-detection from manifest files (`go.mod`, `package.json`, etc.), YAML config `Load`/`Save`, `Init` (creates `openspec/`). |

---

## State Machine

### Phase constants

```
explore → propose → spec ──┐
                   design ─┴→ tasks → apply → review → verify → clean → archive
```

`spec` and `design` are independent parallelizable branches that both require `propose` to complete. `tasks` requires both `spec` AND `design`.

All other transitions are strictly sequential.

### Phase status values

| Value | Meaning |
|---|---|
| `pending` | Not yet started |
| `in_progress` | Started (reserved; not currently set by the engine) |
| `completed` | Artifact promoted and state advanced |
| `skipped` | Intentionally bypassed (treated as done for prerequisite checks) |

### Transition diagram

```
                     ┌──────────────────────────────────────────────────┐
                     │              prerequisites map                   │
                     │                                                  │
  explore            │  explore:  []                                    │
     │               │  propose:  [explore]                             │
     ▼               │  spec:     [propose]                             │
  propose            │  design:   [propose]                             │
   / \               │  tasks:    [spec, design]   ← both required      │
  ▼   ▼              │  apply:    [tasks]                               │
spec design          │  review:   [apply]                               │
  \   /              │  verify:   [review]                              │
   ▼ ▼               │  clean:    [verify]                              │
  tasks              │  archive:  [clean]                               │
    │                └──────────────────────────────────────────────────┘
    ▼
  apply → review → verify → clean → archive
```

### Key state methods

- `CanTransition(target)` — checks target is not already completed, and all prerequisites are `completed`.
- `Advance(completed)` — marks phase `completed`, calls `nextReady()` to set `CurrentPhase`.
- `ReadyPhases()` — returns all pending phases whose prerequisites are met. Used by `runContext` to detect the `spec+design` parallel window and trigger `AssembleConcurrent`.
- `IsStale(threshold)` — returns true if `UpdatedAt` is older than threshold and pipeline is not complete.

### Persistence

`Save` writes `state.json` atomically: marshals JSON to `state.json.tmp` then `os.Rename`. `os.Rename` is atomic on POSIX filesystems, so a crash between write and rename leaves the old state intact.

`Recover` rebuilds state from artifact presence on disk — if `exploration.md` exists, `explore` is marked completed, and so on. This is the "incomplete-batch resume" pattern: the next session calls `Load`; if the file is corrupt or missing, `Recover` reconstructs enough state to continue without manual inspection.

---

## Context Assembly Pipeline

```
sdd context <name> [phase]
         │
         ▼
  cli.runContext
    │
    ├─ Load config (openspec/config.yaml)
    ├─ Load state  (openspec/changes/<name>/state.json)
    ├─ Call state.ReadyPhases()
    │     ├─ 1 phase ready  → Assemble(w, phase, params)
    │     └─ N phases ready → AssembleConcurrent(w, phases, params)
    │
    ▼
  context.Assemble(w, phase, params)
    │
    ├─ 1. tryCachedContext(changeDir, phase, skillsPath)
    │       ├─ read <phase>.hash → parse "hash|timestamp"
    │       ├─ recompute inputHash (SHA256 of artifacts + SKILL.md + version prefix)
    │       ├─ compare hashes
    │       ├─ check TTL (phase-specific duration)
    │       └─ on hit → write cached bytes to w, emit metrics, return
    │
    ├─ 2. assembler fn(buf, params)       ← phase-specific function
    │       Each assembler follows the same structure:
    │       a. loadSkill(skillsPath, "sdd-<phase>")   → SKILL.md bytes
    │       b. writeSection(w, "SKILL", skill)
    │       c. writeSectionStr(w, "PROJECT", projectContext(p))
    │       d. loadManifestContents(projectDir, manifests)
    │       e. buildSummary(changeDir, p)             → Context Cascade
    │       f. load phase-specific prior artifacts
    │       g. write all sections to buf
    │
    ├─ 3. size guard: if buf > 100KB (~25K tokens) → error
    │
    ├─ 4. w.Write(buf.Bytes())
    │
    └─ 5. saveContextCache(changeDir, phase, skillsPath, content)
           emitMetrics(stderr, ...)
```

### AssembleConcurrent

Used when `ReadyPhases()` returns more than one phase (the `spec+design` window). Launches one goroutine per phase, collects results into an ordered slice, writes successes in input order (deterministic output), then aggregates errors. Partial output is intentional: better for Claude to receive a partial context than nothing.

### Context Cascade (summary.go)

`buildSummary` scans completed artifacts and extracts a compact (~500–800 byte) cumulative context forwarded into every downstream assembler. It pulls:

- Change name and description
- Detected stack (language, build tool)
- First heading content from `exploration.md`
- First heading content from `proposal.md`
- First heading content from `design.md`
- Review verdict line from `review-report.md`

Each assembler injects this summary as a section before its phase-specific content. This avoids re-loading entire prior artifacts into the context window while preserving the key decisions that affect the current phase.

`extractFirst` implements a line-scanner that collects up to N non-empty content lines after the first occurrence of a keyword, stopping at the next section heading.

---

## Cache Architecture

### Files per phase

```
.cache/
  <phase>.hash    — "{sha256_hex}|{unix_timestamp}"
  <phase>.ctx     — raw assembled context bytes
  metrics.json    — PipelineMetrics (version-guarded JSON)
```

### Input hash

```
SHA256(
  "v{cacheVersion}:"          ← format version prefix
  + "skill:{len}:{bytes}"     ← SKILL.md content (invalidated on skill edits)
  + sorted(
      "{filename}:{len}:{bytes}"  ← for each input artifact
      | specs/ dir hash           ← all .md files in specs/ sorted by name
    )
)
```

`cacheVersion` is a package-level integer constant (`= 4` at time of writing). Bumping it invalidates all cached contexts without touching any artifact.

### Per-phase TTL

| Phase | TTL |
|---|---|
| propose | 4h |
| spec | 2h |
| design | 2h |
| tasks | 1h |
| apply | 30m |
| review | 1h |
| clean | 1h |

Phases with shorter TTLs are those that change most frequently during active development (apply, tasks). The `explore` phase has no TTL entry and its input set is empty — only the SKILL.md hash applies.

### Cache validation sequence

1. Read `<phase>.hash`. If missing → miss.
2. Split on `|`. If no `|` → legacy format → miss (silent upgrade).
3. Recompute `inputHash`. If mismatch → miss.
4. If phase has a TTL, parse timestamp and compare `time.Since`. If expired → miss.
5. Read `<phase>.ctx`. If read error → miss.
6. Return bytes → hit.

### Metrics

`contextMetrics` (phase, bytes, tokens, cached, duration_ms) is appended to `PipelineMetrics` on every assembly. `PipelineMetrics` is version-guarded: a version mismatch on load returns a fresh empty struct rather than stale counts. `sdd health` reads `metrics.json` directly and surfaces cache hit/miss ratio and total estimated tokens.

Token estimation: `bytes / 4` (English/code mixed content heuristic, matching ~4 bytes/token).

---

## Artifact Lifecycle

### Three zones

```
                 Claude writes              sdd write promotes
                     │                            │
                     ▼                            ▼
.pending/<phase>.md  ──────────────────►  <final-artifact>
                                                  │
                                                  │  sdd archive
                                                  ▼
                                   archive/<timestamp>-<name>/
                                     archive-manifest.md
```

### .pending zone

Claude Code sub-agents write their output to `.pending/<phase>.md`. This staging pattern separates Claude's write from `sdd`'s validation and state advance. The file is not visible to the pipeline or assemblers until promoted.

`WritePending(changeDir, phase, data)` — creates `.pending/` if absent, writes `{phase}.md`.
`PendingExists(changeDir, phase)` — used by commands to check readiness before promoting.

### Promotion

`Promote(changeDir, phase)` reads `.pending/{phase}.md`, writes to the final path (from `ArtifactFileName`), then removes the source. The copy-then-remove pattern is used rather than `os.Rename` to be cross-device safe (Docker, tmpfs mounts).

Special case: `PhaseSpec` promotes into `specs/{phase}.md` inside a `specs/` directory rather than directly in the change root.

`sdd write <name> <phase>` orchestrates: `Promote` → `state.Advance` → `state.Save`. All three must succeed; if `Advance` fails (prerequisites not met or already completed), the artifact has been promoted but the state is not advanced — the user must resolve the conflict manually.

### Phase → artifact filename mapping

| Phase | Artifact |
|---|---|
| explore | `exploration.md` |
| propose | `proposal.md` |
| spec | `specs/spec.md` (directory) |
| design | `design.md` |
| tasks | `tasks.md` |
| apply | `tasks.md` (apply updates tasks.md with completion markers) |
| review | `review-report.md` |
| verify | `verify-report.md` (written by `sdd verify`, not promoted from .pending) |
| clean | `clean-report.md` |
| archive | `archive-manifest.md` (written by `sdd archive`) |

### Archive

`verify.Archive(changeDir)` performs:

1. Computes `archive/<YYYY-MM-DD-HHmmss>-<name>` path under `openspec/changes/`.
2. `os.Rename(changeDir, archivePath)` — atomic directory move.
3. `writeManifest(archivePath, ...)` — scans directory, counts phase artifacts and spec files, writes `archive-manifest.md` atomically (tmp + rename).

`sdd archive` guards the operation with `st.CanTransition(PhaseArchive)` — the entire pipeline through `clean` must be completed first.

---

## Go CLI Patterns

14 patterns from production Go CLIs inform the architecture:

1. **Atomic file writes (tmp + rename)** — `state.Save`, `config.Save`, `verify.WriteReport`, `writeManifest`. Prevents partial writes on crash.

2. **Incomplete-batch resume** — `state.Recover` rebuilds state from artifact presence on disk. A crashed session continues from where it left off.

3. **Content-hash cache invalidation** — SHA256 over sorted inputs + version prefix. Changed artifacts auto-invalidate cached contexts.

4. **Per-dimension TTL cache freshness** — `phaseTTL` map with different TTLs per pipeline phase (apply=30min, spec=2h, propose=4h).

5. **Staleness predicate** — `State.IsStale(threshold)` detects abandoned changes. Completed changes are never stale.

6. **Structured progress logging** — `verify.Run` writes `sdd: verify build: ok (281ms)` to a progress writer.

7. **Inline context metrics** — `writeMetrics` emits `sdd: phase=spec ↑14KB Δ3K tokens 0ms (cached)` to stderr.

8. **Error accumulator with partial output** — `AssembleConcurrent` writes successful results then returns a combined error. Partial output is intentional.

9. **Typed transport errors** — `errs.transportError` distinguishes retriable I/O failures from logic bugs at the JSON level.

10. **Concurrent assembly** — `AssembleConcurrent` runs spec+design in parallel with `sync.WaitGroup`.

11. **Smart-skip verify** — `shouldSkipVerify` checks for PASSED report + no source file changes. Skips entire verify run.

12. **Consumer-defined interfaces** — `Assembler` type defined in the consumer package, not a shared types package.

13. **Structured JSON stdout + stderr split** — All commands write JSON to stdout on success, errors to stderr. Fully scriptable.

14. **Version-guarded cache structs** — `LoadPipelineMetrics` returns empty struct on version mismatch instead of stale data.

---

## Design Decisions and Tradeoffs

### Thin CLI, fat core

`cmd/sdd/main.go` is ≤15 lines. All logic lives in `internal/`. This makes the core testable without process-level integration tests and keeps the entry point trivially auditable.

Tradeoff: the `internal/cli/commands.go` file is long (~670 lines). All command handlers live there rather than in separate files. This is intentional — each handler is a flat sequence of 4–8 steps with no internal branching complexity. Splitting into files would add navigation overhead without reducing cognitive load.

### No cobra

The CLI uses a hand-rolled `switch` dispatch in `cli.Run`. Cobra was listed as an approved dependency in CLAUDE.md but was not adopted. The command surface is simple (no nested subcommands, no flag parsing library needed), and the hand-rolled approach avoids the Cobra init overhead and keeps the binary smaller. Per-command help is stored in a `commandHelp` map.

### No ORM, no CGO, no SQLite

State is stored as JSON files on disk. This choice keeps the binary statically linkable (`CGO_ENABLED=0`), avoids a database dependency, and makes state human-readable and git-diff-friendly. The tradeoff is that concurrent writes from multiple processes could corrupt state — but the SDD workflow is inherently sequential (one active change at a time in a single session).

### Copy-then-remove promotion vs. os.Rename

`artifacts.Promote` reads the pending file, writes to the final destination, then removes the source. A plain `os.Rename` would be simpler and atomic, but fails across device boundaries (Docker volumes, tmpfs mounts). The tradeoff: if the process crashes between write and remove, the `.pending` file remains alongside the promoted file. This is safe — the next `sdd write` will re-read the pending file and overwrite the already-promoted file, which is idempotent.

### Assembler output to io.Writer, cached in memory buffer

Each assembler writes to an `io.Writer`. For the cache path, `Assemble` wraps a `bytes.Buffer` to capture output before checking the size guard and writing to the cache. This means the full context is held in memory twice briefly (buffer + write). For the expected context sizes (≤100KB), this is a non-issue. The alternative (streaming to stdout and caching simultaneously with a `io.MultiWriter`) would complicate the size guard.

### Size guard at 100KB (~25K tokens)

The 100KB limit is a hard error, not a soft warning. The intent is to keep sub-agent context windows manageable. If a phase's assembled context legitimately exceeds this, the assembler needs to be revised to load less (the `loadManifestContents` 2KB per manifest cap and `buildSummary` extraction are direct responses to this constraint).

### Verify is zero-token

`sdd verify` and `sdd archive` run entirely in Go without invoking Claude. They are "free" pipeline steps. This is a deliberate design point: quality gates should not consume token budget.

### Error classification at the boundary

Errors are classified into `usage` / `transport` / `internal` at the command handler level (in `commands.go`) before being serialized to JSON. Internal packages return plain `error` values wrapped with `fmt.Errorf`. Only `errs.go` knows about the JSON envelope. This keeps internal packages clean of CLI concerns.

### git operations via exec.Command, not a library

`gitHeadSHA` and `gitDiffFiles` shell out to `git` rather than using a Go git library. This avoids a heavyweight dependency and ensures behavior matches the user's installed git (including any git config, hooks, and worktree setup). Non-fatal: if git is not available, `sdd new` silently skips recording `BaseRef` and `sdd diff` returns an error with a clear message.
