# Replacing Type Assertions with Type-Safe Alternatives

**Extracted:** 2026-02-13
**Context:** TypeScript projects with `@typescript-eslint/consistent-type-assertions` set to `assertionStyle: 'never'`

## Problem
`as Type` assertions bypass the type checker. When ESLint bans them, each assertion needs a type-safe replacement. There's no single technique — the right fix depends on what's being asserted.

## Solution

### Technique 1: Type guard function (for string unions / branded types)
When asserting a `string` into a narrower union like `'a' | 'b' | 'c'`.

```typescript
// Before (banned)
if (VALID_VIEWS.has(param as View)) return param as View;

// After: type guard + ReadonlySet<string>
const VALID_VIEWS: ReadonlySet<string> = new Set(['dashboard', 'tracker', 'profile']);

function isView(value: string): value is View {
  return VALID_VIEWS.has(value);
}

if (isView(param)) return param; // narrowed to View
```

Key detail: `Set<View>.has()` requires a `View` argument — TypeScript won't accept `string`. Declaring the Set as `ReadonlySet<string>` widens the `.has()` parameter while the type guard provides the narrowing.

### Technique 2: `instanceof` (for DOM / class types)
When asserting an event target or similar runtime object.

```typescript
// Before (banned)
if (!ref.current.contains(e.target as Node)) { ... }

// After: runtime instanceof check
if (e.target instanceof Node && !ref.current.contains(e.target)) { ... }
```

`instanceof` both narrows the type AND adds a runtime safety check.

### Technique 3: Dynamic key access (to flatten assertion chains)
When an if/else chain asserts the same value repeatedly to index an object.

```typescript
// Before (banned, also causes max-depth violations)
if (tier === 't1') entry.t1 = result;
else if (tier === 't2') entry.t2 = result;
else if (tier === 't3') entry.t3 = result;

// After: type guard + dynamic key
if (isTier(tierStr)) {
  entry[tierStr] = result; // tierStr is 't1' | 't2' | 't3', valid index
}
```

## When to Use
- Every time `consistent-type-assertions` flags an `as Type` violation
- When refactoring code to remove type assertions during lint rule adoption
- Decision tree: string union → Technique 1, DOM/class → Technique 2, repeated branching on same value → Technique 3
