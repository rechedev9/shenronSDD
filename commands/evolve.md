# /evolve - Cluster Learned Patterns into Skills, Commands, or Agents

Analyze learned instincts/patterns and cluster related ones into higher-level structures.

## Usage

```
/evolve                    # Analyze all learned patterns and suggest evolutions
/evolve --dry-run          # Show what would be created without creating
/evolve --threshold 5      # Require 5+ related patterns to cluster
```

## What to Do

1. Read all learned skill files from `~/.claude/skills/learned/`
2. Group patterns by:
   - Domain similarity (testing, debugging, code-style, git, workflow)
   - Trigger pattern overlap
   - Action sequence relationship
3. For each cluster of 3+ related patterns, determine evolution type:
   - **Command**: When patterns describe user-invoked actions
   - **Skill**: When patterns describe auto-triggered behaviors
   - **Agent**: When patterns describe complex, multi-step processes
4. Generate the appropriate file

## Evolution Rules

### Command (User-Invoked)
When patterns describe actions a user would explicitly request:
- Multiple patterns about "when user asks to..."
- Patterns that follow a repeatable sequence

### Skill (Auto-Triggered)
When patterns describe behaviors that should happen automatically:
- Pattern-matching triggers
- Error handling responses
- Code style enforcement

### Agent (Needs Depth/Isolation)
When patterns describe complex, multi-step processes:
- Debugging workflows
- Refactoring sequences
- Research tasks

## Output Format

```
Evolve Analysis
==================

Found X clusters ready for evolution:

## Cluster 1: [Name]
Patterns: [list of source patterns]
Type: Command / Skill / Agent
Confidence: X% (based on N observations)

Would create: [target file]

---
Run `/evolve --execute` to create these files.
```

## Flags

- `--execute` — Actually create the evolved structures (default is preview)
- `--dry-run` — Preview without creating
- `--threshold <n>` — Minimum patterns required to form cluster (default: 3)
