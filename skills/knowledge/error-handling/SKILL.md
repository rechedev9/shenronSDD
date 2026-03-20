---
name: error-handling
description: Error handling patterns using Result types and proper exception handling
version: 2.0.0
tags: [errors, result-pattern, exceptions]
---

# Error Handling Patterns

## Result Pattern

### Core Principle
Use `Result<T, E>` pattern for fallible operations. This makes errors explicit in the type system.

**Before using Result types, locate the project's Result type definition:**
```bash
# Search for Result type definition
grep -r "type Result" src/
grep -r "interface Result" src/
```

### Result Type Structure
```typescript
// Common Result pattern
type Result<T, E = Error> =
  | { success: true; data: T }
  | { success: false; error: E };

// Alternative: Ok/Err pattern
type Result<T, E = Error> = Ok<T> | Err<E>;
type Ok<T> = { ok: true; value: T };
type Err<E> = { ok: false; error: E };
```

### Using Result Types
```typescript
// Good: Result for fallible operations
function parseConfig(input: string): Result<Config, ParseError> {
  try {
    const parsed = JSON.parse(input);
    if (isValidConfig(parsed)) {
      return { success: true, data: parsed };
    }
    return {
      success: false,
      error: new ParseError('Invalid config structure')
    };
  } catch (error) {
    return {
      success: false,
      error: new ParseError('JSON parse failed')
    };
  }
}

// Good: Consuming Result types
const result = parseConfig(input);
if (result.success) {
  console.log(result.data.version);
} else {
  logger.error('Config parse failed', { error: result.error });
}
```

## Exception Handling

### Catch Clause Types
Use `unknown` in catch clauses with proper narrowing, never `any`:

```typescript
// Good: unknown + narrowing
try {
  await riskyOperation();
} catch (error: unknown) {
  if (error instanceof Error) {
    logger.error('Operation failed', { message: error.message });
  } else {
    logger.error('Unknown error', { error: String(error) });
  }
}

// Bad: any
try {
  await riskyOperation();
} catch (error: any) {
  console.log(error.message); // Unsafe
}
```

### Rules
- **No empty catch blocks** - always log with context
- **Throw only at system boundaries** - prefer Result returns internally
- **Always provide context** - include relevant data in error logs

### Error Boundaries
```typescript
// Throw at system boundaries (API routes, main)
app.post('/api/users', async (req, res) => {
  const result = await createUser(req.body);
  if (!result.success) {
    throw new ApiError(result.error); // OK to throw at boundary
  }
  res.json(result.data);
});

// Return Result internally
async function createUser(data: unknown): Promise<Result<User, ValidationError>> {
  const validated = validateUser(data);
  if (!validated.success) {
    return validated; // Return error, don't throw
  }
  // ... create user
}
```

## Error Types

### Define Specific Error Types
```typescript
class ValidationError extends Error {
  constructor(
    message: string,
    public readonly field: string
  ) {
    super(message);
    this.name = 'ValidationError';
  }
}

class NetworkError extends Error {
  constructor(
    message: string,
    public readonly statusCode: number
  ) {
    super(message);
    this.name = 'NetworkError';
  }
}
```

### Discriminated Error Unions
```typescript
type AppError =
  | { type: 'validation'; field: string; message: string }
  | { type: 'network'; statusCode: number; message: string }
  | { type: 'unknown'; error: unknown };

function handleError(error: AppError): void {
  switch (error.type) {
    case 'validation':
      logger.warn('Validation failed', { field: error.field });
      break;
    case 'network':
      logger.error('Network error', { status: error.statusCode });
      break;
    case 'unknown':
      logger.error('Unknown error', { error });
      break;
  }
}
```

## Logging Errors

```typescript
// Good: Structured logging with context
catch (error: unknown) {
  logger.error('Failed to process order', {
    orderId: order.id,
    error: error instanceof Error ? error.message : String(error),
    stack: error instanceof Error ? error.stack : undefined
  });
}

// Bad: console.log
catch (error) {
  console.log(error); // Not production-ready
}

// Bad: Empty catch
catch (error) {
  // Silent failure
}
```

## When to Use This Skill

Invoke `/error-handling` when:
- Implementing functions that can fail
- Refactoring throw-based code to Result pattern
- Reviewing error handling in code reviews
- Designing error propagation strategies
