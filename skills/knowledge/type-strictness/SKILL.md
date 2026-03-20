---
name: type-strictness
description: Strict TypeScript type safety rules and immutability patterns for production code
version: 2.0.0
tags: [typescript, types, strictness, immutability]
---

# Type Strictness & Immutability

## Type Strictness Rules

### Banned in Production Code
- `any` - No implicit or explicit any types
- `as Type` assertions - Type assertions that override the type system
- `@ts-ignore` / `@ts-expect-error` - Type error suppressions
- Non-null assertions (`!`) - e.g., `obj!.prop`

### Allowed Everywhere
- `as const` - Const assertions for literal types
- `satisfies` - Type checking without widening
- Type guards - Runtime type checking functions
- `unknown` + narrowing - Safe handling of unknown types

### Allowed in Test Files Only
Test files (`*.test.ts`, `*.spec.ts`) may use:
- `as Type` assertions for test fixtures and mocks

### Required Practices
- Use `unknown` + type guards for all external data (API responses, user input, file reads)
- All functions must have explicit return types
- All function parameters must have explicit types
- Mark properties as `readonly` unless mutation is required

### Example: Type Guards vs Assertions
```typescript
// Bad: Type assertion
const user = data as User;

// Good: Type guard
function isUser(data: unknown): data is User {
  return (
    typeof data === 'object' &&
    data !== null &&
    'id' in data &&
    typeof data.id === 'string'
  );
}

if (isUser(data)) {
  // TypeScript knows data is User here
  console.log(data.id);
}
```

## Immutability Patterns

### Code Style
- Prefer `async/await` over `.then()` chains for multi-step flows
- `.then()` acceptable for single-step transforms
- Prefer immutable patterns; local array mutations (`push`, `pop`) acceptable
- No magic numbers/strings - use named constants
- Max nesting depth: 3 levels

### Immutable Operations
```typescript
// Preferred: Immutable patterns
const updated = [...items, newItem];
const filtered = items.filter(x => x.active);
const mapped = items.map(x => ({ ...x, updated: true }));

// Acceptable: Local mutations
function processItems(data: readonly Item[]): ProcessedItem[] {
  const results: ProcessedItem[] = [];
  for (const item of data) {
    results.push(process(item)); // Local mutation OK
  }
  return results;
}

// Avoid: Mutating parameters
function addItem(items: Item[], newItem: Item): void {
  items.push(newItem); // Don't mutate parameters
}
```

### Readonly Properties
```typescript
// Good: Readonly by default
type User = {
  readonly id: string;
  readonly name: string;
  preferences: UserPreferences; // Mutable only if needed
};

// Good: Readonly collections
function getUsers(): readonly User[] {
  return users;
}
```

## Enforcement

Use Grep tool to check for violations:
- **`any` type usage**: Pattern `\bany\b` in `*.ts` (excluding test files)
- **Unsafe assertions**: Pattern `\bas\s+(?!const\b)` in `*.ts` (excluding test files)
- **Type suppressions**: Pattern `@ts-(ignore|expect-error)`
- **Non-null assertions**: Pattern `\w+!\.\w+`

## When to Use This Skill

Invoke `/type-strictness` when:
- Writing new TypeScript code
- Reviewing code for type safety violations
- Refactoring to improve type safety
- Investigating type-related errors
