# Code Quality Review

## Arguments
$ARGUMENTS — Optional: file paths or patterns to review (defaults to all TypeScript files)

## Checklist

### 1. Type Safety (Critical)
- No `any`, no unsafe `as Type` (`as const` and `satisfies` allowed)
- No `@ts-ignore` or `@ts-expect-error`
- No non-null assertions (`!`)
- All functions: explicit return types + parameter types
- `as` assertions acceptable in test files

### 2. Immutability
- Prefer immutable patterns (spread, `.map()`, `.filter()`, `.reduce()`)
- No mutations on function parameters
- `readonly` where appropriate
- Array mutations (`push`, `pop`, `splice`) ok for local vars, not shared arrays

### 3. File Organization
- Files under 600 lines (warning 600, error 800)
- Single responsibility per file

### 4. Error Handling
- No empty catch blocks
- Errors logged with context
- `unknown` type in catch clauses with proper narrowing

### 5. Code Smells
- No `console.log` (use proper logging)
- No commented-out code
- No TODO/FIXME without issue references
- No magic numbers/strings
- Max 3 nesting levels

### 6. Async
- All promises awaited or explicitly handled
- Proper error handling in async functions

### 7. Security (Critical)
- Grep: `(password|secret|token|api_key|apiKey)\s*[:=]\s*['"]` in `src/`
- No `eval()`, `new Function()` with dynamic input
- No `innerHTML`/`dangerouslySetInnerHTML` with unsanitized data
- No SQL string concatenation
- All user input validated at API boundaries
- No `*` CORS origin
- No ReDoS-vulnerable regex (nested quantifiers)
- Sensitive data not logged or exposed in errors
- `crypto.randomUUID()` / `crypto.getRandomValues()` for security randomness

## Commands

```bash
bun run typecheck
bun run lint
```

## Output

### Critical Issues (Must Fix)
- File:Line - Description

### Warnings (Should Fix)
- File:Line - Description

### Suggestions (Nice to Have)
- File:Line - Description

### Summary
- Total files reviewed: X
- Critical: X / Warnings: X / Suggestions: X
