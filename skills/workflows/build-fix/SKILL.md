---
name: build-fix
description: Automatically diagnose and fix build errors (types, lint, all)
version: 2.1.0
model: sonnet
allowed-tools: Bash, Read, Edit, Write, Grep, Glob
tags: [build, fix, typescript, lint]
---

# Build Error Fixer

Automatically diagnose and fix build errors in the project.

## Arguments
$ARGUMENTS - Mode: `types` | `lint` | `all` (default: `all`)

## Modes

### `types` - Fix TypeScript Errors
1. Run `bun run typecheck` to identify errors
2. For each error, apply fixes in this priority:
   - Add missing type annotations
   - Replace `any` with proper types
   - Remove invalid type assertions (`as Type`). Note: `as const` is safe to keep.
   - Add null checks for possibly undefined values
   - Fix incorrect type assignments

### `lint` - Fix ESLint Errors
1. First, run `bun run lint:fix` for auto-fixable issues
2. Run `bun run lint` again to check for remaining errors
3. If `lint:fix` introduced new issues (e.g., formatting conflicts), run `bun run lint` to identify them and fix manually
4. For remaining errors, manually fix:
   - Unused variables (remove or use)
   - Missing return types
   - Floating promises (add await or void)
   - Unsafe any operations

### `all` - Fix Everything
1. Run TypeScript fixes first (types inform lint)
2. Run ESLint fixes
3. Run Prettier for formatting
4. Verify with full check

## Fix Patterns

### Common Type Fixes
```typescript
// Bad: implicit any
const fn = (x) => x;
// Good: explicit types
const fn = (x: string): string => x;

// Bad: type assertion
const value = data as User;
// Good: type guard
function isUser(data: unknown): data is User {
  return typeof data === 'object' && data !== null && 'id' in data;
}

// Bad: non-null assertion
const name = user!.name;
// Good: null check
const name = user?.name ?? 'default';
```

### Common Lint Fixes
```typescript
// Bad: floating promise
fetchData();
// Good: awaited or explicitly ignored
await fetchData();
void fetchData(); // when intentionally fire-and-forget

// Bad: unused variable
const unused = getValue();
// Good: use it or remove the assignment entirely
processResult(getValue());
```

## Output Verification

After applying fixes, capture and include FULL output:
```bash
bun run typecheck 2>&1
bun run lint 2>&1
bun run format:check 2>&1
```

This enables 2-3x quality improvement through comprehensive feedback loops.

## Output
Report:
1. Errors found (by category)
2. Errors fixed (by category)
3. Errors remaining (if any, with manual fix suggestions)
