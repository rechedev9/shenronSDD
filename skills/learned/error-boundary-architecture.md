# Error Boundary Architecture

**Extracted:** 2026-02-13
**Context:** React apps with no SSR (static export Next.js) that need crash recovery

## Problem
Any uncaught render error in a React app without Error Boundaries causes a full white-screen crash with no recovery path.

## Solution
Two-tier Error Boundary strategy:

1. **Root boundary** — wraps the entire provider tree in `providers.tsx`. Uses a static fallback with `window.location.reload()` because if providers crash, there's no React state to reset.

2. **Granular boundaries** — wrap isolated, high-risk subtrees (e.g., `StatsPanel` with canvas-based charts). Use render-function fallback with `reset()` so the rest of the app keeps working.

Key architectural rules:
- Error Boundaries only catch **render errors**, not async/event handler errors. Hooks like `useCloudSync` need try-catch + Result pattern instead.
- React Error Boundaries **must** be class components — no hooks API exists.
- The reusable `ErrorBoundary` component lives at `src/components/error-boundary.tsx`.
- Its `fallback` prop accepts `ReactNode | ((props: { error, reset }) => ReactNode)`.

## When to Use
- When adding a new isolated feature panel or subtree that could fail independently
- When reviewing crash resilience of the app
- The component is already built — just import and wrap
