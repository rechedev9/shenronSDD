# /instinct — Manage Learned Patterns

View, import, and export learned instincts/patterns.

## Arguments
$ARGUMENTS — Subcommand:
- `status` — Show all instincts (default if no args)
- `import <file>` — Import instincts from a file
- `export` — Export instincts to a shareable file

## Subcommand: `status`

1. Read all learned skill files from `~/.claude/skills/learned/`
2. Parse each file for metadata (date, context, domain)
3. Display grouped by domain with confidence scores

Options:
- `--domain <name>` — Filter by domain (code-style, testing, git, debugging, workflow)
- `--low-confidence` — Show only confidence < 50%
- `--high-confidence` — Show only confidence >= 70%

Output:
```
Instinct Status
==================

## Code Style (X instincts)

### [pattern-name]
Trigger: [when this applies]
Action: [what to do]
Confidence: [visual bar] X%
Last updated: [date]

## Testing (X instincts)
...

---
Total: X instincts
```

- If no instincts exist, suggest running `/learn` after solving problems
- Instincts with confidence > 80% are candidates for `/evolve`

## Subcommand: `import <file>`

1. Parse the import file (YAML, JSON, or markdown)
2. Validate format: each instinct needs id, trigger, action, confidence, domain
3. Check for duplicates against `~/.claude/skills/learned/`
4. Resolve conflicts:
   - **Duplicate**: Higher confidence wins
   - **Conflict**: Skip, flag for manual review
5. Save new instincts to `~/.claude/skills/learned/imported/`

Options:
- `--dry-run` — Preview without importing
- `--force` — Import even if conflicts exist
- `--min-confidence <n>` — Only import above threshold (default: 0.3)

## Subcommand: `export`

1. Read instincts from `~/.claude/skills/learned/`
2. Strip sensitive info (absolute paths, session IDs)
3. Generate YAML export file

Options:
- `--domain <name>` — Export only specified domain
- `--min-confidence <n>` — Minimum threshold (default: 0.3)
- `--output <file>` — Output path (default: `instincts-export-YYYYMMDD.yaml`)

Export includes: triggers, actions, confidence, domains, observation counts.
Export excludes: code snippets, absolute paths, session transcripts.
