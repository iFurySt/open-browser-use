# Open Questions

This page tracks questions that need more evidence before being treated as
confirmed behavior.

## IAB Multi-Page Semantics

Resolved on 2026-10-12 against ChatGPT `26.1007.30912`: the route model now
exposes **many** browser sidebar tabs per `(windowId, conversationId)`.
`pageKey` became `${conversationId}\0${browserTabId}`,
`createTabForBrowserUse` opens a real panel tab via `createPanelTab: true`, and
`getTabsForBrowserUse` walks every `ThreadState` for the conversation. The
earlier one-page-per-route reading described the pre-2026-10 build and no longer
applies. See `runtime/iab-architecture.md`.

Remaining sub-questions:

- Confirm the user-visible sidebar tab strip is driven by the same
  `browser-sidebar-tab-lifecycle` telemetry (`threadBrowserTabCount`,
  `appBrowserTabCount`) that the main process emits.
- Determine whether `tabIdsByPageKey` is still load-bearing beyond
  `Page.navigationBlocked`, or whether it is now a legacy secondary index.

## Dynamic Socket Lifecycle

`/tmp/codex-browser-use/<uuid>.sock` is confirmed as a discovery candidate, and
the observed IAB sockets are owned by the ChatGPT app main process. On
2026-10-12, 17 stale-looking socket files were present in
`/tmp/codex-browser-use/` under a single main-process PID, while the client-side
listing path re-reads the directory on every discovery. This suggests sockets
accumulate per backend session (including disposed routes and crashed sessions)
and are not aggressively unlinked.

Useful next checks:

- Map each socket file to a live backend by watching the directory during
  `createTab` / `endSessionActivity` / route disposal.
- Confirm whether stale socket files are ever reaped, and by what.
- Determine the correct cleanup contract for a client that finds many candidate
  sockets.

## Peer Authorization And Probeability

Direct JSON-RPC against the IAB sockets no longer works from an arbitrary
process on macOS prod builds. The `browser-use-peer-authorization.node` addon
gates `accept()`, and unauthorized peers are dropped silently (observed as
`timeout` / `write EPIPE` rather than a JSON-RPC error).

Useful next checks:

- Enumerate the exact peer criteria the addon checks (code signature, audit
  token, pid provenance).
- Confirm the behavior on non-darwin platforms and on dev/alpha build flavors
  where `shouldIncludeBrowserUsePeerAuthorization` may return false.

## Chrome Process Lifecycle

The client does not start Chrome. It connects to an existing backend. The
remaining question is which component starts or supervises the Chrome extension
native host in each installation mode.

Useful next checks:

- Inspect native messaging host manifests.
- Trace Chrome extension background service worker behavior.
- Compare process trees before and after first Browser Use command.

## Chrome Extension Backend Implementation Location

The client contains a complete Chrome-extension backend contract, but the
current packaged Browser plugin only includes client/service scripts. On the
observed 2026-10-12 installation, no OpenAI/Codex Chrome native messaging host
manifest or visible `chrome-internal` plugin payload was present.

Useful next checks:

- Compare a Dev/InternalAlpha/Nightly/Owl ChatGPT build against this packaged
  prod build.
- Inspect Chrome extension IDs and native messaging host manifests after
  installing any official Browser Use Chrome extension.
- Start the Chrome backend and capture which process owns
  `/tmp/codex-browser-use.sock`.
- Once the backend source or bundle is available, confirm whether Browser Use
  tab IDs are raw Chrome tab IDs or a backend-local mapping.

## Additional Hidden Command Areas

The bundle contains strings or schemas for clipboard, Google Workspace export,
media download, and tab content commands. More analysis is needed to decide
whether these are dead code, shared library residue, backend-only support, or
commands gated by a registry shape not captured by the first extraction.
