# Variant Prop Component Unification

**Extracted:** 2026-02-13
**Context:** React/TypeScript projects with near-duplicate components that differ only in styling/layout

## Problem
Two components share 80-90% of their logic and markup but differ in visual details
(padding, touch targets, hover effects, undo affordance style). Maintaining both
independently leads to drift — bug fixes applied to one but not the other, and
styling inconsistencies.

## Solution
Unify into a single component with a `variant` prop that controls only the
divergent CSS classes. Keep shared logic and markup as the single source of truth.

Decision criteria for when to use this vs. keeping separate:
- **Use variant prop** when: same props interface, same conditional logic, only
  CSS/layout differences (padding, font size, touch target size, hover behavior)
- **Keep separate** when: different prop interfaces, different conditional logic
  trees, or the component is tightly coupled to its parent's layout (e.g.,
  TierSection in card layout uses flex, table layout uses `<td>` — incompatible)

## Example

```typescript
// Before: two files with ~90% identical code
// ResultCell in workout-row.tsx (table: tooltip undo, hover:scale-110, gap-1)
// CardResultCell in workout-row-card.tsx (card: inline undo text, gap-2.5, min-w-[48px])

// After: single component with variant prop
interface ResultCellProps {
  readonly variant: 'table' | 'card';
  // ... shared props
}

export function ResultCell({ variant, ... }: ResultCellProps) {
  const isCard = variant === 'card';
  return (
    <div className={`flex ${isCard ? 'gap-2.5' : 'gap-1 justify-center'}`}>
      <button className={`${isCard ? 'min-w-[48px] text-base' : 'text-sm'} ...shared`}>
```

Key implementation details:
- Use `readonly variant: 'table' | 'card'` (string literal union, not boolean)
  — scales to 3+ variants without API changes
- Compute `isCard = variant === 'card'` once, use in ternaries throughout
- Shared CSS stays inline; variant-specific CSS goes in ternary expressions
- Default the variant when one case is far more common:
  `variant = 'table'` as default parameter

## When to Use
- When two components in the same project share >70% identical markup/logic
- When a code review identifies "duplicated component" as a splitting candidate
- When fixing a bug in one component and realizing the same fix is needed in its twin
