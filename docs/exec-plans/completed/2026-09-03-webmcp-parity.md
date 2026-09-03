# WebMCP browser parity

## Goal

Add page-defined WebMCP discovery and invocation to the Chrome route. Align the
wire behavior with the WebMCP page tool specification while preserving
Open Browser Use session and tab ownership rules.

## Scope

- Include:
  - `webmcp_list_tools` and `webmcp_invoke_tool` JSON-RPC methods.
  - A WebMCP tab capability in `getInfo` while the feature gate is active.
  - MAIN-world and ISOLATED-world content scripts registered at
    `document_start` for top-level pages.
  - A page-facing `document.modelContext` fallback for browsers where the API
    is not native, so sites can register tools during application startup.
  - Snapshot registration IDs so invocation uses a tool returned by the latest
    discovery snapshot.
  - A pinned MCP-B polyfill fixture and deterministic bridge tests.
  - Protocol types and operator documentation.
- Exclude:
  - Turn-level tab leases or new handoff marks.
  - Per-site WebMCP allowlists, blocklists, or permission UI.
  - Fork, push, or pull request work.

## Background

- Related docs:
  - `docs/ARCHITECTURE.md`
  - `skills/open-browser-use/references/sdk-and-protocol.md`
  - Current Chrome WebMCP imperative API documentation.
  - Current MCP-B polyfill documentation.
- Related code:
  - `apps/chrome-extension/background.js`
  - `packages/browser-use-protocol/src/index.ts`
  - `scripts/package-chrome-extension.sh`
- Known constraints:
  - Open Browser Use uses integer Chrome tab IDs and selects the browser/profile
    outside the extension protocol. It does not require a separate
    `browser_id` transport layer.
  - WebMCP must run in the page MAIN world. Extension messaging must stay in an
    ISOLATED-world bridge.
  - The live browser smoke requires a connected Open Browser Use extension and
    native host.

## Risks

- Risk: a stale tool descriptor could invoke a different registration after a
  page update.
  - Mitigation: issue opaque registration IDs per discovery snapshot and require
    the exact ID plus tool name for invocation.
- Risk: native Chrome and polyfills can expose serialized JSON at the WebMCP
  boundary.
  - Mitigation: normalize schemas and results to JSON values before they cross
    the Open Browser Use wire.
- Risk: content scripts registered after a page loads are absent from that page.
  - Mitigation: register at `document_start` and lazily inject both bridge worlds
    when a WebMCP request targets an existing claimed tab.
- Risk: a site can gate all WebMCP registration on the existence of
  `document.modelContext`, while the current Chrome build does not expose that
  API natively.
  - Mitigation: install a small MAIN-world fallback before site JavaScript runs;
    preserve a native implementation when one already exists.
- Risk: WebMCP execution can outlive the requested timeout.
  - Mitigation: apply a bounded request timeout and pass an `AbortSignal` to
    `executeTool` when available.

## Milestones

1. Confirm the current WebMCP schema and compatibility behavior.
2. Add protocol types and extension bridge behavior.
3. Add fixture tests, package checks, docs, and history.
4. Run focused tests and repository CI.

## Validation

- Commands:
  - `pnpm --filter @open-browser-use/browser-use-protocol test`
  - `node --test apps/chrome-extension/*.test.mjs`
  - `./scripts/package-chrome-extension.sh`
  - `make ci`
- Manual check:
  - Serve the pinned fixture from localhost.
  - Claim the fixture tab with Open Browser Use.
  - List its WebMCP tools and invoke the fixture tool.
- Observation check:
  - `getInfo` advertises the `webmcp` tab capability only while the gate is
    active.
  - A registration ID from an older snapshot is rejected after a new list.

## Progress

- [x] Confirm scope and current public WebMCP behavior.
- [x] Add the implementation and deterministic tests.
- [x] Run validation and record the result.

## Decisions

- 2026-09-03: Align the command names and result field names with the standard
  WebMCP page tool specification. Use Open Browser Use's existing integer
  `tabId` plus `session_id` and `turn_id` instead of adding a second browser-ID
  routing layer.
- 2026-09-03: Pin the interoperability fixture to
  `@mcp-b/webmcp-polyfill@5.1.0`.
- 2026-09-03: `PYENV_VERSION=3.12.0 make ci` passed with WebMCP bridge,
  timeout, backend-routing, and gate tests included.
- 2026-09-03: Real-browser smoke passed in an isolated Playwright-managed
  Google Chrome for Testing profile with the local extension loaded unpacked.
  A temporary test-only global exposed the already-created `BrowserBackend`
  from the copied extension bundle so DevTools could call the real backend
  without changing repository source. The backend advertised the `webmcp`
  capability, discovered the pinned localhost MCP-B fixture as `fixture_echo`,
  returned a registration snapshot with schema/origin/page URL, invoked the
  tool with `message: "hello from obu"`, and received that message plus the
  fixture page URL. The test then finalized the session tab successfully.
- 2026-09-03: Real-world web verification exposed a compatibility requirement.
  Web applications frequently check for `document.modelContext` before tool
  registration. After adding the MAIN-world fallback shim, pages register their
  tools correctly and read-only tools execute against the live application.
