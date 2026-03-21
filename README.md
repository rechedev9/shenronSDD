<p align="center">
  <img src="assets/logo.png" alt="SDD Logo" width="200">
</p>

<h1 align="center">SDD — Spec-Driven Development</h1>

<p align="center">
  <strong>Go CLI that turns Claude Code into a structured engineering pipeline.<br>Deterministic orchestration. Zero-token state management. Cached context assembly.</strong>
</p>

<p align="center">
  <a href="https://github.com/rechedev9/shenronSDD/actions"><img src="https://img.shields.io/github/actions/workflow/status/rechedev9/shenronSDD/ci.yml?style=for-the-badge&label=CI" alt="CI"></a>
  <img src="https://img.shields.io/badge/Go-1.24-00ADD8?style=for-the-badge&logo=go" alt="Go 1.24">
  <img src="https://img.shields.io/badge/Phases-10-7C3AED?style=for-the-badge" alt="Phases: 10">
  <img src="https://img.shields.io/badge/License-MIT-blue?style=for-the-badge" alt="MIT License">
</p>

<p align="center">
  <a href="docs/01-why-sdd.md">Why SDD?</a> ·
  <a href="docs/02-pipeline.md">Pipeline</a> ·
  <a href="docs/architecture.md">Architecture</a> ·
  <a href="docs/04-commands-reference.md">Commands</a> ·
  <a href="docs/05-skills-catalog.md">Skills</a> ·
  <a href="docs/08-advanced.md">Advanced</a> ·
  <a href="docs/contributing-to-cli.md">Contributing</a>
</p>

---

AI coding agents waste ~65% of their tokens on mechanical bookkeeping — reading SKILL files, tracking state, loading artifacts, running build commands. None of that requires intelligence. `sdd` is a Go binary that handles all deterministic operations at zero token cost, so Claude only spends tokens on reasoning.

```
Claude = reasoning (expensive, probabilistic)
Go     = orchestration (free, deterministic)
```

---

## Install

```bash
curl -sSL https://raw.githubusercontent.com/rechedev9/shenronSDD/master/install.sh | bash
```

Or from source:

```bash
git clone https://github.com/rechedev9/shenronSDD
cd shenronSDD && ./install.sh
```

Requires [Claude Code CLI](https://docs.anthropic.com/en/docs/claude-code) + [Go 1.24+](https://go.dev/dl/).

## Quick start

```bash
cd your-project
claude
/sdd-init                              # detect stack, create openspec/
/sdd-new my-feature "what to build"    # explore + propose
/sdd-continue                          # repeat until archive
```

---

## Highlights

- **[65% token reduction](docs/token-economics.md)** — from ~458K to ~161K tokens per pipeline. Go handles all mechanical work.
- **[10-phase pipeline](docs/02-pipeline.md)** — explore → propose → spec + design (parallel) → tasks → apply → review → verify → clean → archive.
- **[Zero-token phases](docs/02-pipeline.md)** — init, verify, and archive run entirely in Go. No LLM involved.
- **[Content-hash cache](docs/08-advanced.md)** — SHA256 of input artifacts + SKILL.md. Cache hits served in 0ms. Per-phase TTLs.
- **[Context Cascade](docs/08-advanced.md)** — cumulative decision summary carried forward across phases at zero cost.
- **[Phase registry](docs/architecture.md)** — pluggable `Phase` struct with prerequisites, artifacts, cache config, and assembler. Custom phases via `Register()`.
- **[Event broker](docs/architecture.md)** — pub/sub decouples state machine from metrics, caching, and logging. Non-blocking with panic recovery.
- **[Concurrent loading](docs/architecture.md)** — `LazySlice` loads artifacts in parallel goroutines. Blocks only on consumption.
- **[Flexible phase refs](docs/04-commands-reference.md)** — `sdd context foo p` (prefix), `sdd context foo 3` (index). Abbreviations just work.
- **[sdd doctor](docs/sdd-cli-reference.md)** — validates config, cache integrity, artifact completeness. `--json` for CI.

---

## How it works

```
┌─────────────────────────────────────────────────────────────────┐
│                        sdd context                              │
│                                                                 │
│  ┌─────────┐   ┌──────────┐   ┌──────────┐   ┌──────────────┐  │
│  │ SKILL.md│ + │Artifacts │ + │ Cascade  │ + │ Config/Tree  │  │
│  └────┬────┘   └────┬─────┘   └────┬─────┘   └──────┬───────┘  │
│       └─────────────┼──────────────┼────────────────┘           │
│                     ▼                                           │
│              Assembled Context                                  │
│              (cached, 0ms hit)                                  │
└─────────────────────┬───────────────────────────────────────────┘
                      ▼
               Claude reasons
               writes .pending/
                      ▼
┌─────────────────────┴───────────────────────────────────────────┐
│                        sdd write                                │
│                                                                 │
│  Promote artifact  →  Advance state machine  →  Emit events    │
└─────────────────────────────────────────────────────────────────┘
```

Each phase: Go assembles context (0 tokens) → Claude reasons → Go promotes + advances (0 tokens). Claude does all the thinking. Go does all the bookkeeping.

---

## Everything we built

### Pipeline phases

| Phase | Actor | Output |
|-------|-------|--------|
| **explore** | Claude (Sonnet) | `exploration.md` — codebase analysis, risk assessment |
| **propose** | Claude (Opus) | `proposal.md` — intent, scope, approach, risks, rollback |
| **spec** | Claude (Opus) | `specs/*.md` — RFC 2119 requirements with Given/When/Then |
| **design** | Claude (Opus) | `design.md` — type definitions, file changes, before/after |
| **tasks** | Claude (Sonnet) | `tasks.md` — ordered implementation checklist |
| **apply** | Claude (Opus) | source files — implements one task at a time |
| **review** | Claude (Opus) | `review-report.md` — semantic code review against spec |
| **verify** | **Go** | `verify-report.md` — runs build/lint/test (0 tokens) |
| **clean** | Claude (Sonnet) | `clean-report.md` — dead code removal |
| **archive** | **Go** | `archive/` — moves completed change (0 tokens) |

### CLI commands

| Command | What it does |
|---------|-------------|
| `sdd init` | Detect stack, create `openspec/`, write `config.yaml` |
| `sdd new <name> <desc>` | Create change, capture git HEAD, print explore context |
| `sdd context <name> [phase]` | Assemble SKILL + artifacts + cascade (cached) |
| `sdd write <name> <phase>` | Promote `.pending/` artifact, advance state machine |
| `sdd verify <name>` | Run build/lint/test with progress logging |
| `sdd archive <name>` | Move to `archive/`, write manifest |
| `sdd status <name>` | Phase progress + staleness detection |
| `sdd list` | Active changes with current phase |
| `sdd diff <name>` | Files changed since `sdd new` |
| `sdd health <name>` | Token usage, cache stats, pipeline warnings |
| `sdd doctor` | Validate config, cache, artifacts |
| `sdd dump` | Full workflow state as JSON |

Details: [CLI Reference](docs/sdd-cli-reference.md)

### Slash commands

| Command | What it does |
|---------|-------------|
| `/sdd-init` | Detect stack, create openspec/ |
| `/sdd-new <name> <desc>` | Explore + propose in one shot |
| `/sdd-continue [name]` | Run next phase automatically |
| `/sdd-ff <name>` | Fast-forward: explore → propose → spec+design → tasks |
| `/sdd-apply [name]` | Implement current task |
| `/sdd-review [name]` | Semantic code review |
| `/sdd-verify [name]` | Build/lint/test (zero tokens if green) |
| `/sdd-clean [name]` | Dead code removal |
| `/sdd-archive [name]` | Close and archive change |

Details: [Commands Reference](docs/04-commands-reference.md)

### Flags

`--json` on all commands for CI/scripting. `-q` quiet (exit code only). `-v` verbose (cache, timing). `-d` debug (full trace).

### Skills

| Category | Count | Examples |
|----------|-------|---------|
| SDD phases | 11 | `sdd-explore`, `sdd-propose`, `sdd-spec`, ... |
| Frameworks | 14+ | Go, React, TypeScript, Next.js, Python, ... |
| Analysis | 4 | Code review, security audit, ... |
| Knowledge | 3 | Domain modeling, ... |
| Workflows | 4 | CI/CD, deployment, ... |

Details: [Skills Catalog](docs/05-skills-catalog.md)

### Internal architecture

- **[Phase Registry](sdd-cli/internal/phase/)** — single `Phase` struct replaces 4 parallel maps. Pluggable custom phases via `Register()`.
- **[State Machine](sdd-cli/internal/state/)** — prerequisite-driven transitions, atomic persistence, crash recovery from artifacts.
- **[Context Assemblers](sdd-cli/internal/context/)** — per-phase assemblers with content-hash cache and TTL expiration.
- **[Event Broker](sdd-cli/internal/events/)** — pub/sub for metrics, cache persistence, stderr output. Panic-safe subscribers.
- **[LazySlice](sdd-cli/internal/csync/)** — concurrent artifact loading. Start goroutines eagerly, block on consumption.
- **[Artifact System](sdd-cli/internal/artifacts/)** — `.pending/` staging, promotion, listing, reading.
- **[Config Detection](sdd-cli/internal/config/)** — auto-detect language, build/test/lint commands, manifests.

Details: [Architecture](docs/architecture.md) · [Go CLI Patterns](docs/go-cli-patterns.md)

---

## Cache architecture

```
openspec/changes/{name}/.cache/
  spec.hash         # SHA256(v6:SKILL.md + proposal.md + exploration.md) | timestamp
  spec.ctx          # Pre-assembled context (0ms on cache hit)
  metrics.json      # Cumulative: bytes, tokens, cache hits/misses per phase
```

- **Content-hash** — SHA256 of input artifacts + SKILL.md + `cacheVersion`. Any input change = cache miss.
- **Per-phase TTL** — apply=30min, spec/design=2h, propose=4h. Defined in the phase registry.
- **Version guard** — bumping `cacheVersion` invalidates all caches automatically.
- **Smart-skip verify** — reuses last PASSED report if no source files changed.

---

## When to use SDD

```
One-line fix        →  just edit the file
Small change        →  /sdd-explore + manual edit
Medium change       →  /sdd-ff + /sdd-apply + /sdd-verify
Large change        →  full 10-phase pipeline
Architecture        →  full pipeline + extra review cycles
```

---

## Project structure

```
shenronSDD/
  sdd-cli/                    # Go CLI source
    cmd/sdd/main.go
    internal/
      phase/                  # Phase struct + Registry (single source of truth)
      cli/                    # 13 subcommands
      state/                  # State machine, atomic persistence
      context/                # Assemblers, cache, metrics, Context Cascade
      artifacts/              # .pending write/promote/read/list
      config/                 # Stack detection, config.yaml
      verify/                 # Build/lint/test runner, archive
      events/                 # Pub/sub event broker
      csync/                  # Concurrent artifact loading
  skills/                     # SKILL.md files (loaded by sdd context)
  commands/                   # Slash command definitions
  docs/                       # 13 documentation files
  install.sh                  # One-line installer
```

---

## Docs

**Getting started**
- [Why SDD?](docs/01-why-sdd.md) — the problem with AI coding and how SDD solves it
- [Pipeline](docs/02-pipeline.md) — deep dive into all 10 phases
- [Configuration](docs/07-configuration.md) — config.yaml, stack detection, setup

**Reference**
- [CLI Reference](docs/sdd-cli-reference.md) — all 13 sdd commands
- [Commands Reference](docs/04-commands-reference.md) — slash commands
- [Skills Catalog](docs/05-skills-catalog.md) — how skills work, how to create new ones

**Architecture**
- [Pillars](docs/03-pillars.md) — the architectural pillars including Harness Infrastructure
- [Architecture](docs/architecture.md) — internal architecture of the Go CLI
- [Go CLI Patterns](docs/go-cli-patterns.md) — 14 Go patterns from production CLIs
- [Token Economics](docs/token-economics.md) — measured token consumption before/after
- [Advanced](docs/08-advanced.md) — cache, Context Cascade, TTL, metrics

**Community**
- [Comparisons](docs/06-comparisons.md) — SDD vs alternatives
- [Contributing](docs/contributing-to-cli.md) — how to add commands, assemblers, patterns

---

## Contributing

- **CLI** — new commands, performance, caching improvements
- **Skills** — Vue, Svelte, FastAPI, Spring Boot, and more
- **Phases** — better prompts, output formats
- **Tests** — integration tests, edge case coverage
- **Docs** — tutorials, case studies

See [Contributing to CLI](docs/contributing-to-cli.md) for the build/test/patterns guide.

---

## License

MIT
