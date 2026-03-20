# SDD — Spec-Driven Development CLI

**A Go binary that turns Claude Code into a structured engineering pipeline. Deterministic orchestration, zero-token state management, cached context assembly.**

![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)
![Go: 1.24](https://img.shields.io/badge/Go-1.24-00ADD8.svg)
![Phases: 11](https://img.shields.io/badge/Phases-11-orange.svg)
![Version: 2.0](https://img.shields.io/badge/Version-2.0-purple.svg)

---

## The Problem

AI coding agents waste tokens on work that doesn't need intelligence: tracking which phase is next, reading the same SKILL.md file every session, loading artifacts, running build commands, parsing test output. In a typical 10-phase pipeline, ~65% of tokens go to mechanical bookkeeping, not reasoning.

## The Solution

`sdd` is a Go CLI that handles all deterministic operations at **zero token cost**:

```
Claude = reasoning (probabilistic, expensive)
Go     = orchestration (deterministic, free)
```

```
sdd context → Go assembles SKILL + artifacts + cascade summary (0 tokens)
              ↓
           Claude reasons, writes to .pending/phase.md
              ↓
sdd write  → Go promotes artifact, advances state machine (0 tokens)
```

**Result:** ~161K tokens per pipeline vs ~458K without the CLI. 65% reduction, with better context per phase.

---

## Install

```bash
curl -sSL https://raw.githubusercontent.com/rechedev9/shenronSDD/master/install.sh | bash
```

Or clone and run locally:

```bash
git clone https://github.com/rechedev9/shenronSDD
cd shenronSDD && ./install.sh
```

The installer sets up skills, slash commands, and (if Go is available) builds the `sdd` binary to `~/.local/bin/sdd`.

### Prerequisites

- [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) installed and authenticated
- [Go 1.24+](https://go.dev/dl/) for the `sdd` CLI binary
- Git initialized in your project

### First run

```bash
cd your-project
claude
/sdd:init            # Detect stack, create openspec/
/sdd:new my-feature "What I want to build"
/sdd:continue        # Repeat until archive
```

---

## How It Works

### The Pipeline

```
explore → propose → spec + design → tasks → apply → review → verify → clean → archive
                     (parallel)
```

Every phase has two actors: **Go orchestrates** (state, context, cache), **Claude reasons** (or not).

| # | Phase | Reasoning | Orchestration | Output | Claude tokens |
|---|-------|-----------|---------------|--------|---------------|
| 1 | **init** | — | Go detects stack, creates config | `config.yaml` | **0** |
| 2 | **explore** | Claude (Sonnet) | Go assembles SKILL + file tree + manifests | `exploration.md` | ~44K |
| 3 | **propose** | Claude (Opus) | Go assembles SKILL + exploration + project context | `proposal.md` | ~25K |
| 4 | **spec** | Claude (Opus) | Go assembles SKILL + proposal + cascade summary | `specs/*.md` | ~50K |
| 5 | **design** | Claude (Opus) | Go assembles SKILL + proposal + specs + cascade | `design.md` | ~42K |
| 6 | **tasks** | Claude (Sonnet) | Go assembles SKILL + design + specs | `tasks.md` | ~21K |
| 7 | **apply** | Claude (Opus) | Go assembles SKILL + current task + completed tasks + design | source files | varies |
| 8 | **review** | Claude (Opus) | Go assembles SKILL + specs + design + git diff | `review-report.md` | varies |
| 9 | **verify** | **None** | **Go runs build/lint/test directly** | `verify-report.md` | **0** |
| 10 | **clean** | Claude (Sonnet) | Go assembles SKILL + verify-report + design + specs | `clean-report.md` | varies |
| 11 | **archive** | **None** | **Go moves directory + writes manifest** | `archive/` | **0** |

### Where the 65% token savings come from

The savings are **not** just from verify + archive being Go. Three sources:

| Source | Tokens saved | How |
|--------|-------------|-----|
| **Go-native phases** (init, verify, archive) | ~75K | These phases used to require Claude to run commands and parse output |
| **Context assembly in Go** | ~160K | Before: Claude read SKILL.md + every artifact + scanned directories per phase. Now: Go assembles and delivers pre-built context |
| **Cache + Cascade** | ~60K | Content-hash cache serves repeated calls in 0ms. Context Cascade carries prior decisions forward without re-reading |
| **Total** | **~295K** | From ~458K down to ~161K per pipeline |

Claude still does **all the reasoning** — exploring, proposing, specifying, designing, implementing, reviewing, cleaning. Go only eliminates the mechanical work Claude used to waste tokens on: reading files, tracking state, discovering what phase is next, executing shell commands.

### The Flow (inside Claude Code)

```bash
# 1. Go assembles context (cached, with metrics)
$ sdd context my-feature propose
sdd: phase=propose ↑14KB Δ3K tokens 0ms (cached)

# 2. Slash command feeds context to Claude sub-agent
#    Claude writes to .pending/propose.md

# 3. Go promotes artifact + advances state
$ sdd write my-feature propose
{"phase": "propose", "status": "success", "current_phase": "spec"}

# 4. Repeat for each phase
```

### Context Cascade

Each phase receives a cumulative summary of all prior decisions — generated by Go at zero cost:

| Phase | Gets from prior phases |
|-------|----------------------|
| propose | exploration + project config + file tree |
| spec | proposal + stack info + pipeline summary |
| design | proposal + specs + stack info + pipeline summary |
| apply | current task + **completed tasks** + design + specs |
| clean | verify-report + tasks + **design + specs** + pipeline summary |

This solves the #1 problem of multi-agent pipelines: context loss between phases.

---

## The CLI

```
sdd <command> [arguments]
```

### Pipeline Commands

| Command | Description | Tokens |
|---------|-------------|--------|
| `sdd init` | Detect stack, create openspec/, write config.yaml | 0 |
| `sdd new <name> <desc>` | Create change, capture git HEAD, print explore context | 0 |
| `sdd context <name> [phase]` | Assemble SKILL + artifacts + cascade (cached) | 0 |
| `sdd write <name> <phase>` | Promote .pending artifact, advance state machine | 0 |
| `sdd verify <name>` | Run build/lint/test with progress logging | 0 |
| `sdd archive <name>` | Move to archive, write manifest | 0 |

### Inspection Commands

| Command | Description | Tokens |
|---------|-------------|--------|
| `sdd status <name>` | Phase progress + staleness detection | 0 |
| `sdd list` | Active changes with current phase | 0 |
| `sdd diff <name>` | Files changed since `sdd new` | 0 |
| `sdd health <name>` | Pipeline health: tokens, cache stats, warnings | 0 |

### Slash Commands (use CLI under the hood)

| Slash Command | What it does |
|---------------|-------------|
| `/sdd:init` | Runs `sdd init`, shows results |
| `/sdd:new <name> <desc>` | `sdd new` → Claude explores → `sdd write` → Claude proposes → `sdd write` |
| `/sdd:continue [name]` | `sdd status` → `sdd context` → Claude reasons → `sdd write` |
| `/sdd:ff <name>` | Fast-forward: explore → propose → spec+design (parallel) → tasks |
| `/sdd:apply [name]` | `sdd context apply` → Claude implements → `sdd write` |
| `/sdd:review [name]` | `sdd context review` → Claude reviews → `sdd write` |
| `/sdd:verify [name]` | `sdd verify` (zero tokens if green) |
| `/sdd:clean [name]` | `sdd context clean` → Claude cleans → `sdd write` |
| `/sdd:archive [name]` | `sdd archive` (zero tokens) |

---

## Why a Go CLI?

### The token tax

Without the CLI, an LLM orchestrator spends tokens on:
- Reading SKILL.md files (~40K across 8 phases)
- Loading artifacts from prior phases (~116K cumulative)
- Discovering state manually (~30K scanning directories)
- Running and parsing build/test output (~75K for verify + archive)

**Total tax: ~261K tokens per pipeline** — none of which requires intelligence.

### The harness pattern

The CLI is not a wrapper — it's **infrastructure that constrains the agent**:

- The state machine **prevents** skipping phases
- The artifact system **enforces** where files are written
- The verify runner **executes** build/lint/test without LLM involvement
- The cache system **prevents** redundant work
- The Context Cascade **carries** decisions forward without re-reading

Deterministic code orchestrates, LLM reasons.

### Cache architecture

```
openspec/changes/{name}/.cache/
  spec.hash         # SHA256(v4:SKILL.md + proposal.md + exploration.md) | unix_timestamp
  spec.ctx          # Pre-assembled context (served in 0ms on cache hit)
  metrics.json      # Cumulative: bytes, tokens, cache hits/misses per phase
```

- **Content-hash**: SHA256 of input artifacts + SKILL.md + cacheVersion
- **Per-dimension TTL**: apply=30min, spec/design=2h, propose=4h
- **Version guard**: bumping `cacheVersion` invalidates all caches
- **Smart-skip verify**: reuses last PASSED report if no source files changed

### Measured performance

| Metric | Without CLI | With CLI |
|--------|-----------|---------|
| Tokens per pipeline | ~458K | ~161K |
| State management | Manual (Claude scans) | Go state machine (0 tokens) |
| Context assembly | Claude reads each time | Cached, 0ms on hit |
| Verify + archive | ~75K tokens | 0 tokens |
| Errors (wrong path, skipped phase) | ~5% rate | 0% (mechanically enforced) |
| Recovery between sessions | Manual inspection | `sdd status` + `sdd health` |

---

## Project Structure

```
shenronSDD/
  sdd-cli/                    # Go CLI binary source
    cmd/sdd/main.go           # Entry point (13 lines)
    internal/
      cli/                    # Command routing + 11 subcommands
      state/                  # Phase state machine, atomic persistence
      context/                # Assemblers + cache + metrics + Context Cascade
      artifacts/              # .pending write/promote/read/list
      config/                 # Stack detection + config.yaml
      verify/                 # Build/lint/test runner + archive
  skills/                     # SKILL.md files (loaded by sdd context)
    sdd/                      # 11 phase skills
    frameworks/               # 14+ framework skills (Go, React, TS, ...)
    analysis/                 # 4 analysis skills
    knowledge/                # 3 knowledge skills
    workflows/                # 4 workflow skills
  commands/                   # Slash command definitions
  docs/                       # Documentation (15 files)
  presentation/               # React slideshow
  install.sh                  # Installer (skills + commands + Go binary)
```

### Per-project structure (created by `sdd init`)

```
your-project/
  openspec/
    config.yaml               # Auto-detected: language, build/test/lint commands
    changes/
      feature-name/
        state.json            # Phase state machine
        exploration.md        # Phase artifacts
        proposal.md
        specs/*.md
        design.md
        tasks.md
        review-report.md
        verify-report.md
        .pending/             # Claude writes here, Go promotes
        .cache/               # Content-hash cache + metrics
      archive/
        2026-03-20-feature/   # Completed changes (immutable)
```

---

## When to Use SDD

```
Trivial change     →  Just edit the file
Small change       →  /sdd:explore + manual edit
Medium change      →  /sdd:ff + /sdd:apply + /sdd:verify
Large change       →  Full 11-phase pipeline
Architecture       →  Full pipeline + extra review cycles
```

Skip SDD for: one-line fixes, trivial additions, prototyping, pure research.
Use `/sdd:ff` for: medium changes, clear requirements, 3-10 files.
Use full pipeline for: architecture changes, cross-cutting concerns, high-risk code.

---

## Documentation

| Document | Description |
|----------|-------------|
| [Why SDD?](docs/01-why-sdd.md) | The problem with standard AI coding and how SDD solves it |
| [Pipeline](docs/02-pipeline.md) | Deep dive into all 11 phases |
| [Pillars](docs/03-pillars.md) | The architectural pillars including Harness Infrastructure |
| [Commands Reference](docs/04-commands-reference.md) | Slash commands + CLI commands |
| [Skills Catalog](docs/05-skills-catalog.md) | How skills work and how to create new ones |
| [Comparisons](docs/06-comparisons.md) | SDD vs alternatives with token economics |
| [Configuration](docs/07-configuration.md) | config.yaml, stack detection, setup |
| [Advanced](docs/08-advanced.md) | Cache architecture, Context Cascade, TTL, metrics |
| [CLI Reference](docs/sdd-cli-reference.md) | Complete reference for all 11 sdd commands |
| [Architecture](docs/architecture.md) | Internal architecture of the Go CLI |
| [Token Economics](docs/token-economics.md) | Measured token consumption before/after |
| [Go CLI Patterns](docs/go-cli-patterns.md) | 14 Go patterns adopted from production CLIs |
| [Contributing](docs/contributing-to-cli.md) | How to add commands, assemblers, patterns |

---

## Contributing

Areas where help is needed:

- **CLI improvements**: new commands, better caching, performance
- **Framework skills**: Vue, Svelte, FastAPI, Spring Boot, etc.
- **Phase improvements**: better prompts, output formats
- **Testing**: more integration tests, edge case coverage
- **Documentation**: tutorials, case studies

See [Contributing to CLI](docs/contributing-to-cli.md) for build/test/patterns guide.

---

## License

MIT
