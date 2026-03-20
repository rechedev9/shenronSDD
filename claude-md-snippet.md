<!-- SDD Workflow — Paste this into your project's CLAUDE.md -->
<!-- Source: https://github.com/rechedev9/shenronSDD -->

## Spec-Driven Development (SDD)

> Source & install: https://github.com/rechedev9/shenronSDD

SDD uses a Go CLI (`sdd`) for deterministic orchestration and Claude for reasoning. The CLI handles state, context assembly, caching, and quality gating at zero token cost.

### How it works

```
sdd context  → Go assembles SKILL + artifacts + cascade summary (0 tokens)
             → Claude reasons, writes to .pending/phase.md
sdd write    → Go promotes artifact, advances state machine (0 tokens)
```

### Pipeline

```
init → explore → propose → spec + design (parallel) → tasks → apply → review → verify → clean → archive
```

### SDD Commands

Use these slash commands (they use the `sdd` CLI under the hood):

| Command | Description |
|---------|-------------|
| `/sdd:init` | Detect stack, create openspec/ (zero tokens) |
| `/sdd:new <name> [desc]` | Start new change (explore + propose) |
| `/sdd:continue [name]` | Run next dependency-ready phase |
| `/sdd:ff <name>` | Fast-forward all planning phases |
| `/sdd:apply [name]` | Implement code (`--tdd`, `--phase N`, `--fix-only`) |
| `/sdd:review [name]` | Semantic code review against specs |
| `/sdd:verify [name]` | Run build/lint/test (zero tokens if green) |
| `/sdd:clean [name]` | Dead code removal + simplification |
| `/sdd:archive [name]` | Archive completed change (zero tokens) |

### CLI Commands (direct use)

```bash
sdd init                    # Detect stack, create openspec/
sdd new <name> <desc>       # Create change + explore context
sdd context <name> [phase]  # Assemble context (cached)
sdd write <name> <phase>    # Promote .pending artifact
sdd status <name>           # Phase progress
sdd list                    # Active changes
sdd verify <name>           # Build/lint/test (zero tokens)
sdd archive <name>          # Move to archive (zero tokens)
sdd diff <name>             # Files changed since sdd new
sdd health <name>           # Pipeline health + cache stats
```

### Sub-Agent Pattern

The slash commands launch sub-agents following this pattern:

```
Agent(
  description: 'sdd-{phase} for {change-name}',
  model: 'sonnet',  # Opus for design + apply
  prompt: '{context from sdd context output}

  Write output to: openspec/changes/{change-name}/.pending/{phase}.md
  Follow the SKILL instructions exactly.'
)
```

After the sub-agent returns, promote with `sdd write <name> <phase>`.

### Sub-Agent Model Selection

| Agent | Model | Reason |
|---|---|---|
| explore, tasks, clean | `sonnet` | Tool use + mechanical decomposition |
| **propose** | **Opus** | Proposal quality shapes the entire pipeline |
| **spec** | **Opus** | Precise requirements, not superficial |
| **design** | **Opus** | Architecture decisions shape everything |
| **apply** | **Opus** | Production code quality |
| **review** | **Opus** | Adversarial review finds subtle bugs |
| verify, archive | **Go (0 tokens)** | Deterministic — no LLM needed |

### Trigger Detection

Recognize intent and suggest the appropriate command:
- "Add a feature..." / "I want to..." → `/sdd:new <name>`
- "Continue" / "Next step" → `/sdd:continue`
- "Fast forward" / "Plan everything" → `/sdd:ff`
- "Implement" → `/sdd:apply`
- "Review" → `/sdd:review`
- "Verify" / "Test" → `/sdd:verify`
- "Archive" / "Close" → `/sdd:archive`
- "Explore" / "Investigate" → `/sdd:explore <topic>`

### Utility Commands (standalone)

- `/commit-push-pr` — Commit, push, open PR
- `/verify [mode]` — Quick verification outside SDD (quick|full|pre-commit)
- `/build-fix [mode]` — Emergency build fix (types|lint|all)
- `/code-review [files]` — Standalone code review

## Framework Skills — Lazy Loading

Load framework skills ONLY when working in that domain. The `sdd context` assembler loads the phase SKILL.md automatically; framework skills are loaded manually when needed.

<!-- Add your project-specific framework skills below -->
| Domain | Trigger | Skill Path |
|---|---|---|
| React 19 | Writing `.tsx` components | `~/.claude/skills/frameworks/react-19/SKILL.md` |
| Tailwind 4 | Styling with Tailwind | `~/.claude/skills/frameworks/tailwind-4/SKILL.md` |
| TypeScript | Strict TS patterns | `~/.claude/skills/frameworks/typescript/SKILL.md` |
| Go | Go CLIs, error handling, testing | `~/.claude/skills/frameworks/go-shenron/SKILL.md` |
| Next.js 15 | App Router, Server Components | `~/.claude/skills/frameworks/nextjs-15/SKILL.md` |
| Zod 4 | Schema validation | `~/.claude/skills/frameworks/zod-4/SKILL.md` |
| Zustand 5 | State management | `~/.claude/skills/frameworks/zustand-5/SKILL.md` |
| Playwright | E2E testing | `~/.claude/skills/frameworks/playwright/SKILL.md` |
| AI SDK 5 | Vercel AI integration | `~/.claude/skills/frameworks/ai-sdk-5/SKILL.md` |
| Django DRF | Python REST APIs | `~/.claude/skills/frameworks/django-drf/SKILL.md` |
| pytest | Python testing | `~/.claude/skills/frameworks/pytest/SKILL.md` |
| GitHub PR | Creating pull requests | `~/.claude/skills/frameworks/github-pr/SKILL.md` |
| Jira Epic | Epic creation | `~/.claude/skills/frameworks/jira-epic/SKILL.md` |
| Jira Task | Task creation from proposals | `~/.claude/skills/frameworks/jira-task/SKILL.md` |
| Skill Creator | Creating new SKILL.md | `~/.claude/skills/frameworks/skill-creator/SKILL.md` |
