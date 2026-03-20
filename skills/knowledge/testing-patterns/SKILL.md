---
name: testing-patterns
description: Testing conventions, file organization, and best practices for bun:test
version: 2.0.0
tags: [testing, bun, file-organization]
---

# Testing Patterns & File Organization

## File Organization

### File Size Limits
- **Warning threshold**: 600 lines
- **Error threshold**: 800 lines
- Files exceeding limits should be split following single responsibility principle

### Module Structure
- **Single responsibility per file** - each file should have one clear purpose
- **Test files live alongside source** - `feature.ts` -> `feature.test.ts`
- **Use `.test.ts` extension** (not `.spec.ts`) for consistency

### Directory Structure
```
feature/
├── types.ts          # Type definitions
├── index.ts          # Public exports
├── feature.ts        # Core logic
├── feature.test.ts   # Tests
└── utils.ts          # Helper functions (if needed)
```

## Testing Conventions

### Test Framework
- **Test runner**: `bun:test`
- **Test structure**: `describe` / `it` (not `test`)
- **Assertion library**: Built-in `expect`

### Test Structure
```typescript
import { describe, it, expect, mock } from 'bun:test';

describe('featureName', () => {
  describe('when given valid input', () => {
    it('should return the expected result', () => {
      // Arrange
      const input = createTestInput();

      // Act
      const result = featureName(input);

      // Assert
      expect(result).toEqual(expected);
    });
  });

  describe('when given invalid input', () => {
    it('should return an error result', () => {
      // Arrange
      const input = null;

      // Act
      const result = featureName(input);

      // Assert
      expect(result.success).toBe(false);
    });
  });
});
```

### Testing Principles
- **One assertion per test where practical** - focused tests are easier to debug
- **Arrange / Act / Assert pattern** - clear test structure
- **Descriptive test names** - `it('should return error when input is empty')`
- **Test behavior, not implementation** - tests should survive refactoring

### Mocking Strategy
- **Prefer dependency injection over mocking** - easier to test, fewer mocks needed
- **Use `bun:test` built-in `mock()`** for function mocks
- **Create typed test doubles** for external dependencies (APIs, databases)
- **Avoid mocking internals** of the module under test

### Mocking Examples
```typescript
import { mock } from 'bun:test';

// Good: Mock external dependency
const mockFetch = mock(async (url: string) => ({
  ok: true,
  json: async () => ({ data: 'test' })
}));

// Good: Dependency injection
function processData(
  data: string,
  logger: Logger = console
): Result<ProcessedData, Error> {
  // Function is testable without mocking console
}

// Test with mock logger
const mockLogger = {
  log: mock(() => {}),
  error: mock(() => {})
};
const result = processData('test', mockLogger);
```

### Test Coverage Guidelines
- **Critical paths**: 100% coverage for core business logic
- **Error handling**: Test all error branches
- **Edge cases**: Empty arrays, null, undefined, boundary values
- **Type safety in tests**: Use strict typing, `as` assertions acceptable in test files

### Test File Type Safety
```typescript
// Allowed in test files: as assertions for fixtures
const mockUser = {
  id: '123',
  name: 'Test'
} as User;

// Better: Type-safe test builders
function createTestUser(overrides?: Partial<User>): User {
  return {
    id: '123',
    name: 'Test User',
    email: 'test@example.com',
    ...overrides
  };
}
```

## Running Tests

### Commands
```bash
# Run all tests
bun test

# Run specific test file
bun test feature.test.ts

# Run tests in watch mode
bun test --watch

# Run tests with coverage
bun test --coverage
```

### Test Verification
After making changes, always run:
```bash
bun run typecheck  # Ensure tests are type-safe
bun test           # Ensure tests pass
```

## Common Patterns

### Testing Async Code
```typescript
describe('async operations', () => {
  it('should handle async success', async () => {
    const result = await fetchData('valid-id');
    expect(result.success).toBe(true);
  });

  it('should handle async errors', async () => {
    const result = await fetchData('invalid-id');
    expect(result.success).toBe(false);
    expect(result.error).toBeDefined();
  });
});
```

### Testing Error Cases
```typescript
describe('error handling', () => {
  it('should return error for null input', () => {
    const result = process(null);
    expect(result.success).toBe(false);
    expect(result.error.message).toContain('input required');
  });

  it('should return error for invalid format', () => {
    const result = process('invalid');
    expect(result.success).toBe(false);
  });
});
```

### Testing Type Guards
```typescript
describe('isUser type guard', () => {
  it('should return true for valid user object', () => {
    const data = { id: '123', name: 'John' };
    expect(isUser(data)).toBe(true);
  });

  it('should return false for null', () => {
    expect(isUser(null)).toBe(false);
  });

  it('should return false for object missing required fields', () => {
    const data = { name: 'John' };
    expect(isUser(data)).toBe(false);
  });
});
```

## When to Use This Skill

Invoke `/testing-patterns` when:
- Writing new tests
- Setting up test files for new features
- Reviewing test quality
- Organizing modules and deciding file structure
- Investigating test failures
