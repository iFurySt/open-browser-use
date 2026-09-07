## [2026-09-07 00:14 UTC] | Task: Inspect Codex extension and improve history search

### Execution Context

- Agent: Codex. Exact model version and reasoning level were not exposed.
- Runtime: ChatGPT Work with Bruksrom workspace tools and local browser-use.

### User Request

Inspect Chrome extension `hehggadaopoacecdllhhajmbjkdcmajg` and use its behavior
to improve Open Browser Use. Use only Bruksrom tools for this task.

### Changes

- Inspected the installed ChatGPT extension, version `1.26.901.11451`.
- Recorded its worker hash, source anchors, architecture changes, capability
  gaps, and implementation priorities in the browser-client wiki.
- Added multi-query history search, URL deduplication, newest-first ordering,
  request validation, and a default range that includes older retained history.
- Kept the existing single-query input and result shape.
- Added eight regression tests using synthetic history data.

### Design Intent

The history capability fits the current handler and existing permission.
Generic CLI/MCP calls and all SDK history wrappers already accept its fields.
No new dependency or protocol wrapper is needed. Debugger recovery remains a
separate change because SDK clients cache their attachment state.

### Key Files

- `apps/chrome-extension/background.js`
- `apps/chrome-extension/history.test.mjs`
- `docs/wiki/browser-client/runtime/chatgpt-extension-2026-09.md`
- `docs/wiki/browser-client/README.md`
- `README.md`
- `docs/releases/feature-release-notes.md`

### Validation

- Baseline: six new regression tests failed before the implementation.
- All 17 extension test entries and the background script syntax check passed.
- Documentation, repository hygiene, and diff whitespace checks passed.
- No private history was used. The installed extension was not replaced or
  reloaded; the new handler was validated in the repository test harness.

### Pull Request Preparation

- Loaded the ask-matt router and its implementation, TDD, and code-review
  skills with `gh`.
- Use the existing repository docs and completed execution plan for the
  Standards and Spec reviews. Upstream base is
  `19af36b5c49df45e0933b0a7ab904247de51d765`.
- Added the user-facing release note required by `CONTRIBUTING.md`.
- Independent reviews found zero Standards findings and zero Spec findings.
- `PYTHON=python3 make ci` passed, including Go, extension, protocol, SDK,
  script, packaging, and CLI build checks. The checkout needed its frozen
  JavaScript dependencies installed and an explicit available Python command.
