# /sdd:analytics — Quality Analytics (Post-MVP)

Analyze the quality timeline for a change. This command is a **post-MVP stub**.

## Arguments
$ARGUMENTS — Optional: change name.

## Execution

This feature is not yet implemented. When ready, it will:

1. Run `sdd analytics <name>` (zero-token Go command)
2. Parse `quality-timeline.jsonl` for the change
3. Report: build health progression, issue density by phase, completeness curve, scope summary, phase timing, regressions

For now, report to the user:

> Analytics is a post-MVP feature. Track progress with:
> - `sdd status <name>` — current phase and completed phases
> - `sdd list` — all active changes
> - Check `verify-report.md` and `review-report.md` in the change directory for quality metrics.
