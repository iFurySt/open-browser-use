# Codex extension capability review

## Goal

Inspect installed Codex extension `hehggadaopoacecdllhhajmbjkdcmajg`, compare
its behavior with Open Browser Use, and implement a small useful improvement.

## Scope

- Read extension metadata and code through the local browser.
- Compare findings with the existing 1.1.4 reference and current extension.
- Keep source evidence, capability gaps, and validation results in repository docs.
- Use existing helpers and dependencies. Preserve unrelated browser tabs and work.

## Risks

- Bundled code can show an available path without proving its runtime behavior.
  Record static findings and live checks separately.
- Debugger recovery affects SDK attachment caches. Keep it as a separate
  follow-up; implement history search with synthetic data in this change.

## Validation

- Run focused regression tests for changed behavior.
- Run the repository checks that cover the changed files.
- Use a local browser check if it can run without replacing installed extensions.

## Progress

- [x] Read repository instructions and locate existing Codex research.
- [x] Inspect the installed extension and select a concrete capability gap.
- [x] Implement and test multi-query history search.
- [x] Update the wiki and history, then review the combined diff.

## Findings and decisions

- The installed extension is ChatGPT `1.26.901.11451`; both observed workers
  returned the same source hash.
- History search can use the current permission and generic parameter maps.
- Keep the existing `query` alias and response shape. Use `queries` for multiple
  terms, with one merged result limit and the newest result for each URL.
- Set the default `startTime` to zero. Chrome otherwise searches only the last
  24 hours.
- Record target attachment, popup ownership, debugger recovery, tab search,
  browser management, and side-panel work as separate capability gaps.
- Eight history regressions and all 17 extension test entries pass.
- Syntax, documentation, repository hygiene, and diff whitespace checks pass.
- The changed handler is ready in source. Chrome release or reload validation
  remains separate from the source inspection and synthetic regression tests.
- Fork PR preparation: independent Standards and Spec reviews each reported
  zero findings. The full `PYTHON=python3 make ci` suite passed.
