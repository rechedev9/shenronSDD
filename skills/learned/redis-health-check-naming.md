---
name: redis-health-check-naming
source: sdd-archive
date: 2026-02-22
change: exploracion-de-mejoras
---

# Redis Health Check Status Naming Consistency

## Context

When adding a Redis health check to a `GET /health` endpoint in this project (ElysiaJS + ioredis), the status value for "Redis not configured" must match between:
1. The spec (REQ-DEBT-006)
2. The implementation (`apps/api/src/index.ts`)
3. The test (`apps/api/src/index.test.ts`)

## Pattern

- When `REDIS_URL` env var is absent, return `{ "status": "disabled" }` (not `"skipped"`, `"unconfigured"`, or any other variant).
- The spec explicitly states `"disabled"` as the canonical value.
- The test assertion must match exactly: `expect(body.redis.status).toBe('disabled')`.

## Gotcha

The initial implementation used `'skipped'` and the test was written to match the implementation rather than the spec. This created a silent spec deviation that persisted through review and verify. Always write tests against the spec, not against what the implementation happens to return.

## Anti-pattern

```typescript
// WRONG — returns 'skipped' but spec says 'disabled'
if (!process.env.REDIS_URL) {
  return { status: 'skipped' };
}

// CORRECT
if (!process.env.REDIS_URL) {
  return { status: 'disabled' };
}
```
