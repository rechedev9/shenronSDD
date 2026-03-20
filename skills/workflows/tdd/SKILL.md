---
name: tdd
description: Test-Driven Development workflow with Red-Green-Refactor cycle
version: 2.1.0
model: sonnet
allowed-tools: Bash, Read, Edit, Write, Grep, Glob
tags: [testing, tdd, workflow]
---

# Test-Driven Development Workflow

You are now in TDD mode. Follow the Red-Green-Refactor cycle strictly.

## Arguments
$ARGUMENTS - Description of the feature/function to implement

## Conventions

### Test File Location & Naming
- Test files live alongside source files: `feature.ts` -> `feature.test.ts`
- Use the `.test.ts` extension (not `.spec.ts`) for consistency
- Mirror the source file's directory structure

### Test Structure
- Use `describe` blocks to group related tests by function or behavior
- Use `it` (not `test`) for individual test cases
- Use descriptive names: `it('should return error when input is empty')`
- One assertion per test where practical

### Mocking Strategy
- Prefer dependency injection over mocking where possible
- Use `bun:test` built-in `mock()` for function mocks
- For external dependencies (APIs, databases), create typed test doubles
- Avoid mocking internals of the module under test

### Test Patterns
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
      const result = featureName(null);
      expect(result.success).toBe(false);
    });
  });
});
```

## Workflow

### Phase 1: RED - Write Failing Tests
1. Create or open the test file (`.test.ts`)
2. Write comprehensive tests for the expected behavior:
   - Happy path cases
   - Edge cases
   - Error cases
3. Run `bun test` to confirm tests fail
4. Do NOT write implementation code yet

### Phase 2: GREEN - Minimum Implementation
1. Write the MINIMUM code needed to pass all tests
2. No premature optimization
3. No extra features beyond what tests require
4. Run `bun test` after each change to verify progress

### Phase 3: REFACTOR - Clean Up
1. Improve code quality while keeping tests green
2. Apply project patterns:
   - Explicit return types
   - `readonly` for immutable data
   - Proper error handling with Result pattern
3. Run `bun test` after each refactor step

### Phase 4: VERIFY - Quality Checks
Run all quality checks:
```bash
bun run typecheck
bun run lint
bun test
```

## Output Verification

After completing TDD cycle, capture and include FULL output:
```bash
bun run typecheck 2>&1
bun run lint 2>&1
bun test 2>&1
```

This ensures 2-3x quality improvement through comprehensive feedback loops.

## Rules
- NEVER write implementation before tests
- NEVER skip the refactor phase
- NEVER leave failing tests
- Each test should test ONE behavior
- Use descriptive test names: `it('should return error when input is empty')`

## Output
After completing TDD cycle, report:
1. Number of tests written
2. Test coverage of the new code
3. Any type safety improvements made
