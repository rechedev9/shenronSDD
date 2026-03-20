# Code Quality Review

Perform a comprehensive code review of the current codebase or specified files.

## Arguments
$ARGUMENTS - Optional: specific file paths or patterns to review (defaults to all TypeScript files)

## Review Checklist

### 1. Type Safety (Critical)
Check for violations of strict typing rules:
- [ ] No `any` type usage
- [ ] No unsafe `as Type` assertions (note: `as const` and `satisfies` are allowed)
- [ ] No `@ts-ignore` or `@ts-expect-error`
- [ ] No non-null assertions (`!`)
- [ ] All functions have explicit return types
- [ ] All function parameters have explicit types
- [ ] `as` assertions are acceptable in test files (`*.test.ts`, `*.spec.ts`)

### 2. Immutability
Check for mutation violations:
- [ ] Prefer immutable patterns where practical (spread operators, `.map()`, `.filter()`, `.reduce()`)
- [ ] Avoid object mutations on function parameters
- [ ] Proper use of `readonly` modifiers
- [ ] Array mutations (`push`, `pop`, `splice`) are acceptable for local variables; avoid mutating shared/passed-in arrays

### 3. File Organization
- [ ] Files under 600 lines (warning at 600, error at 800)
- [ ] Single responsibility per file
- [ ] Related code grouped together

### 4. Error Handling
- [ ] No empty catch blocks
- [ ] Errors logged with context
- [ ] Consistent error pattern (Result type or typed errors)
- [ ] Unknown type in catch clauses with proper narrowing

### 5. Code Smells
- [ ] No `console.log` statements (use proper logging)
- [ ] No commented-out code
- [ ] No TODO/FIXME without issue references
- [ ] No magic numbers/strings (use constants)
- [ ] No deeply nested code (max 3 levels)

### 6. Async Code
- [ ] All promises awaited or explicitly handled
- [ ] Prefer `async/await` over `.then()/.catch()` chains for multi-step flows. `.then()` is acceptable for simple single-step transforms.
- [ ] Proper error handling in async functions

### 7. Security (Critical for enterprise)
- [ ] No hardcoded secrets, API keys, tokens, or passwords (use Grep: `(password|secret|token|api_key|apiKey)\s*[:=]\s*['"]` in `src/`)
- [ ] No `eval()`, `new Function()`, or `Function()` calls with dynamic input
- [ ] No `innerHTML`, `outerHTML`, or `dangerouslySetInnerHTML` with unsanitized data
- [ ] No SQL string concatenation (use parameterized queries)
- [ ] All user input validated/sanitized at API boundaries
- [ ] No overly permissive CORS (`*` origin)
- [ ] No regex vulnerable to ReDoS (catastrophic backtracking) — flag nested quantifiers like `(a+)+`
- [ ] Sensitive data (PII, tokens) not logged or exposed in error messages
- [ ] `crypto.randomUUID()` or `crypto.getRandomValues()` for security-sensitive randomness (not `Math.random()`)

## Commands to Run
```bash
bun run typecheck    # Check for type errors
bun run lint         # Check for lint issues
```

## Output Format
Report findings in this format:

### Critical Issues (Must Fix)
- File:Line - Description

### Warnings (Should Fix)
- File:Line - Description

### Suggestions (Nice to Have)
- File:Line - Description

### Summary
- Total files reviewed: X
- Critical issues: X
- Warnings: X
- Suggestions: X
