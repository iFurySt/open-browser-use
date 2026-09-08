# Zen PR #14 compatibility hardening

## Goal and scope

Fix Windows Chrome/Zen manifest coexistence and session ownership without tab
groups. Clarify temporary installation, verify Zen/Firefox source, and retain
source evidence. No browser installation or merge is required.

## Risks and validation

Preserve the existing Chrome manifest path and Firefox launch basename. Test
both install orders, concurrent claims, repeated claims, cleanup, and restored
session state. Run repository CI and cross-compile Windows; add a Windows CI
job for the targeted path/manifest tests. Browser runtime smoke remains separate
from these tests; no Windows or Zen runtime is available for this task.

## Progress

- [x] Inspect PR head and reproduce session collision.
- [x] Inspect Zen patches and Firefox 155.0.1 native manifest implementation.
- [x] Implement fixes and regression tests.
- [x] Synchronize docs and run local CI, typecheck/lint, and Windows cross-builds.
- [ ] Deliver supplemental commit to PR and inspect hosted CI.

## Decisions

- 2026-09-08: Retain Chromium manifest location; Firefox uses a family subdirectory.
- 2026-09-08: Reserve ungrouped ownership synchronously before storage writes,
  retaining the original tab origin on same-session claims.
