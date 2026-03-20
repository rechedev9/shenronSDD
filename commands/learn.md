# /learn — Extract Reusable Patterns

## Execution

1. Review session for extractable patterns
2. Identify most valuable/reusable insight
3. Draft skill file
4. Ask user to confirm before saving
5. Save to `~/.claude/skills/learned/`

## What to Extract

- Error resolution patterns (error → root cause → fix → reusable?)
- Non-obvious debugging techniques
- Library/API workarounds
- Project-specific conventions and architecture decisions

Skip: trivial fixes (typos, syntax), one-time issues (outages).

## Output

Save to `~/.claude/skills/learned/[pattern-name].md`:

```markdown
# [Descriptive Pattern Name]

**Extracted:** [Date]
**Context:** [When this applies]

## Problem
[Specific problem]

## Solution
[Pattern/technique/workaround]

## Example
[Code if applicable]

## When to Use
[Trigger conditions]
```

One pattern per skill. Focus on patterns that save time in future sessions.
