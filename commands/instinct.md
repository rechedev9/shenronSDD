# /instinct — Manage Learned Patterns

## Arguments
$ARGUMENTS — Subcommand:
- `status` — Show all instincts (default)
- `import <file>` — Import from file
- `export` — Export to shareable file

## status

1. Read `~/.claude/skills/learned/`
2. Parse metadata (date, context, domain)
3. Display grouped by domain with confidence scores

Options: `--domain <name>`, `--low-confidence` (< 50%), `--high-confidence` (>= 70%)

Output:
```
Instinct Status
==================

## [Domain] (X instincts)

### [pattern-name]
Trigger: [when]
Action: [what]
Confidence: [bar] X%
Last updated: [date]

---
Total: X instincts
```

No instincts → suggest `/learn`. Confidence > 80% → candidates for `/evolve`.

## import <file>

1. Parse file (YAML, JSON, or markdown)
2. Validate: each instinct needs id, trigger, action, confidence, domain
3. Check duplicates against `~/.claude/skills/learned/`
4. Resolve: duplicate → higher confidence wins; conflict → skip, flag
5. Save to `~/.claude/skills/learned/imported/`

Options: `--dry-run`, `--force`, `--min-confidence <n>` (default: 0.3)

## export

1. Read `~/.claude/skills/learned/`
2. Strip sensitive info (absolute paths, session IDs)
3. Generate YAML export

Options: `--domain <name>`, `--min-confidence <n>` (default: 0.3), `--output <file>` (default: `instincts-export-YYYYMMDD.yaml`)

Includes: triggers, actions, confidence, domains, observation counts.
Excludes: code snippets, absolute paths, session transcripts.
