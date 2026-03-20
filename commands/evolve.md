# /evolve — Cluster Learned Patterns into Skills, Commands, or Agents

## Arguments
$ARGUMENTS — Flags:
- `--dry-run` — Preview without creating
- `--execute` — Create evolved structures
- `--threshold <n>` — Min patterns to cluster (default: 3)

## Execution

1. Read `~/.claude/skills/learned/`
2. Group by: domain similarity, trigger overlap, action sequence relationship
3. For each cluster of 3+ patterns, determine type:
   - **Command** — user-invoked repeatable sequences
   - **Skill** — auto-triggered behaviors (pattern-matching, error handling, style enforcement)
   - **Agent** — multi-step processes (debugging, refactoring, research)
4. Generate file (or preview if `--dry-run`)

## Output

```
Evolve Analysis
==================

Found X clusters ready for evolution:

## Cluster 1: [Name]
Patterns: [list]
Type: Command / Skill / Agent
Confidence: X% (N observations)

Would create: [target file]

---
Run `/evolve --execute` to create these files.
```
