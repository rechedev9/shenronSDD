# Review Report: sdd diff

**Verdict: PASS**

## Spec Compliance
- BaseRef field added with omitempty — backward compatible
- runNew captures SHA non-fatally — correct
- runDiff outputs JSON matching spec format — correct
- Error cases handled per spec (no BaseRef, no change, no args)

## Design Compliance
- No new package (Rule of 3) — correct
- gitHeadSHA and gitDiffFiles are unexported helpers — correct
- CLI wired with help text and commandHelp — correct

## Issues
None blocking.

## Notes
- git diff --name-only only shows tracked files. Untracked new files require git add first. This is expected git behavior.
