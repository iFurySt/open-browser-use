# IAB Architecture

This page records the current working model for the Codex in-app browser
backend, referred to as `iab` by `browser-client.mjs`.

## Evidence Snapshot

Confirmed against:

- Browser client artifact:
  `~/.codex/plugins/cache/openai-bundled/browser/26.1007.30912/scripts/browser-client.mjs`
  plus `browser-service.mjs`
- ChatGPT app bundle:
  `/Applications/ChatGPT.app/Contents/Resources/app.asar`
- ChatGPT app version observed locally: `26.1007.30912`
  (`codexBuildFlavor = prod`, `codexBuildNumber = 20156`), Chromium `155.0.8059.40`
- Main-process bundle area: `.vite/build/main-CqEoQDOU.js`
- Renderer webview bundles: `webview/assets/*.js`
- Runtime sockets observed locally:
  `/tmp/codex-browser-use/<uuid>.sock`, owned by the ChatGPT app main process.
- Runtime storage: `~/Library/Application Support/Codex/Partitions/codex-browser-app/`

The bundle is minified, so names such as `BrowserSessionRegistry` describe the
observed class role rather than source-authored symbol names unless otherwise
noted.

### Renames Since The 2026-04 Snapshot

| 2026-04 (`Codex.app`) | 2026-10 (`ChatGPT.app`) |
| --- | --- |
| app named `Codex.app` | app named `ChatGPT.app`, bundle id still `com.openai.codex` |
| plugin path `.../browser-use/0.1.0-alpha1/` | plugin path `.../browser/26.1007.30912/` |
| logic in `browser-client.mjs` | logic split into `browser-client.mjs` + `browser-service.mjs` |
| `turnRoutes` | `sessionRoutes` |
| `browser-use-turn-route-capture` | `browser-use-session-route-capture` |
| `pageKey = ownerWebContentsId:conversationId` | `pageKey = conversationId\0browserTabId` |
| one page per route | many tabs per route |

## Key Concepts

### BrowserWindow

`BrowserWindow` is an Electron desktop window. In this app it is a top-level
Codex window. Its numeric ID is exposed as `BrowserWindow.id`.

The IAB route includes this ID because the same Codex conversation can be
opened or rebound in more than one desktop window.

### webContents

`webContents` is Electron's controller object for one loaded page. It can:

- load URLs
- read URL/title/loading state
- receive page lifecycle events
- execute JavaScript
- attach the Electron debugger and send CDP commands

There are two important `webContents` roles in IAB:

- owner `webContents`: the Codex React UI inside a `BrowserWindow`
- guest `webContents`: the actual page embedded in a browser sidebar webview

### webview

`<webview>` is an Electron renderer-side DOM element that embeds another page
inside the Codex UI. It is similar to an iframe at the UI level, but Electron
backs it with a separate guest `webContents`.

The current backend distinguishes browser sidebar tabs by `browserTabId` within
each `conversationId`. The renderer lifecycle snippets later on this page are
explicitly labeled as the older 2026-04 snapshot where applicable.

## Route Model

IAB has three route keys:

```text
routeKey    = `${windowId}:${conversationId}`
routeTabKey = `${routeKey}\0${browserTabId}`
pageKey     = `${conversationId}\0${browserTabId}`
```

The `windowId` is the Electron `BrowserWindow.id`. The `conversationId` is the
ChatGPT conversation/session ID used by browser-use calls as `session_id`.

When the browser-use client connects to an IAB backend, it filters discovered
backends by `metadata.codexSessionId`, which is the `conversationId`. Discovery
is now scoped explicitly:

```text
info.discoveryScope = "session"   // iab, mcpapps
info.discoveryScope = "global"    // extension, cdp
metadata = { codexAppBuildFlavor, codexAppSessionId, codexSessionId }
```

Session-scoped backends are only retained when `metadata.codexSessionId` matches
the current Codex session and (when set) `codexAppBuildFlavor` matches the
running build flavor. Non-matching candidate sockets are closed rather than
exposed. Connection failures are attributed to concrete reasons such as
`missing-session-metadata`, `no-iab-backends`, or `no-session-match`.

## Backend Lifecycle

The IAB backend is created lazily by the app main process. The relevant
responsibilities are split between two observed classes:

```text
BrowserSessionRegistry
  browserUseNativePipeEnabled: boolean
  sessionRoutes: Map<sessionId, SessionRoute>
  backendStatesBySessionId: Map<sessionId, BackendState>
  hostsByRouteKey: Map<routeKey, BrowserUseHost>
  windows: Map<windowId, WindowRecord>
  pendingLocalWorkSessionIds: Set<sessionId>
  localWorkCodexSessionIds: Map<browserConversationId, conversationId>

BrowserUseHost
  routeKey
  conversationId
  windowId
  page/webContents cache
```

`BrowserSessionRegistry.setBrowserUseNativePipeEnabled(true)` ensures a backend
for every registered `(windowId, conversationId)` route. Disabling it disposes
all backend states.

### Session Route Capture (replaces turn-route capture)

The old build bound command serving to a per-turn route:

```text
browser-use-turn-route-capture
browser-use-turn-route-release
turnRouteKey = `${conversationId}:${turnId}`
```

Those messages no longer exist. The 2026-10 build captures a **session** route
instead, and drains it on inactivity rather than on turn release:

```text
browser-use-session-route-capture   -> captureSessionRoute({ conversationId, browserConversationId, disposeAfterSessionActivity, owner })
browser-use-session-activity-ended  -> endSessionActivity({ conversationId })
```

`turn_id` is still present on individual calls and is still used for
turn-scoped bookkeeping (`usedTurnIdsByConversationId`, `markTab`, `turnEnded`
tab cleanup), but it no longer gates whether a socket can serve a request.

`captureSessionRoute` resolves the owner through
`getBrowserOwnerWebContentsForOrigin` and the window through
`BrowserWindow.fromWebContents(owner)`, then records:

```text
sessionRoutes.set(sessionId, {
  sessionId,
  browserConversationId,
  ownerWebContentsId: owner.id,
  windowId,
  disposeAfterSessionActivity,
})
```

Notable behaviors layered on top:

- **Owner rebind**: if the same session is captured from a different owner
  `webContents`, the route owner is rebound in place and a
  `session-route-owner-rebound` debug event is recorded, instead of dropping the
  capture.
- **Duplicate capture rejection**: a capture whose `pageKey` already has a live
  page is ignored with `session-route-capture-ignored`.
- **Cross-window rebind guard**: `bindSessionRouteOwner` refuses to move a route
  to a different window while a live page exists
  (`session-route-live-rebind-ignored`).
- **Child sessions**: `captureChildSessionRoute` attaches a subagent session to
  its parent's owner/window with `disposeAfterSessionActivity: true`.
- **Local Work sessions**: `prepareLocalWorkSessionRoute` /
  `cancelPendingLocalWorkSessionRoute` stage a route under a placeholder session
  id (`Oq` prefix) before the real Codex session id is known, then
  `bindSessionRouteOwner` moves the backend across.

`endSessionActivity` releases session control, clears per-tab browser-use
active state, and disposes the route only when
`disposeAfterSessionActivity` was set. It is driven by the app-server
`thread/status/changed` notification reaching `idle`, and
`thread/archived` maps to `disposeConversation`.

### Can-Serve Gate

`canServeSession` decides whether a given backend state may answer a request:

```text
backendStatesBySessionId.get(backendSessionId) === state
  AND ( conversationId === backendSessionId AND ensureSessionRoute(...)
        OR pendingLocalWorkSessionIds.size === 1
           AND pendingLocalWorkSessionIds.has(backendSessionId)
           AND !sessionRoutes.has(conversationId) )
```

This is the modern replacement for the old turn-route constraint.

## Page Key

`pageKey` is a Codex app internal page identifier. It is not an Electron or
Chrome primitive.

Observed shape (2026-10):

```text
pageKey = `${conversationId}\0${browserTabId}`
```

The `\0` separator is a literal NUL byte produced by the `QB` helper; the
composite `routeTabKey` uses the same convention via the `gq` helper.

Where:

- `conversationId` identifies the Codex conversation.
- `browserTabId` identifies one browser sidebar tab, generated as
  `browser-use:${randomUUID()}`.

Because `browserTabId` is part of the key, one conversation can hold many
concurrent sidebar pages. The page key identifies "the browser sidebar page for
this tab in this conversation".

The renderer-side debug/telemetry variant still uses the older three-part form:

```text
ZB(ownerWebContentsId, conversationId, browserTabId) = `${owner}:${conversationId}:${browserTabId}`
```

which is why debug events carry `ownerWebContentsId` alongside `pageKey`.

## Logical Tab Mapping

The browser-use client sees only logical tab IDs. In IAB these are numeric
`cdpTabId` values assigned by the ChatGPT app.

IAB keeps explicit maps:

```text
tabIdsByRouteTabKey: Map<routeTabKey, cdpTabId>
tabIdsByPageKey: Map<pageKey, cdpTabId>
tabsById: Map<cdpTabId, { cdpTabId, browserTabId, pageKey, routeKey, lifetime, sessionControlled, mark, title, url, active }>
tabRuntimesById: Map<cdpTabId, { pageKey, webContents }>
selectedTabIdsByRouteKey: Map<routeKey, cdpTabId>
```

Effective lookup:

```text
browser-use tab.id
  -> cdpTabId
  -> tabsById.get(cdpTabId)
  -> { browserTabId, pageKey, routeKey }
  -> tabRuntimesById.get(cdpTabId)
  -> { pageKey, webContents }
  -> webContents.debugger.sendCommand(...) or webContents.executeJavaScript(...)
```

So `cdpTabId` is not a native Chrome tab ID. It is a compatibility ID that lets
the browser-use API look like a tabbed browser while the implementation targets
Electron `webContents`. `serializeTab` exposes both identities to the client:

```text
{ id: cdpTabId, providerTabId: browserTabId, sessionControlled, title, url, active }
```

`cdpTabId` allocation is keyed by `routeTabKey`, so the same conversation can
hold several stable `cdpTabId`s at once. `tabIdsByPageKey` remains a secondary
index used for page-scoped events such as `Page.navigationBlocked`.

## Native Pipe Transport

IAB serves the browser-use API over a local native pipe. On macOS/Linux the
socket root is:

```text
/tmp/codex-browser-use/
```

The server creates a randomized socket path:

```text
/tmp/codex-browser-use/<uuid>.sock
```

On Windows the base path is the named pipe namespace:

```text
\\.\pipe\codex-browser-use
```

The transport is JSON-RPC 2.0 framed with a 4-byte length prefix:

```text
uint32 message_length
utf8 JSON-RPC payload
```

The main-process pipe server registers public methods from the IAB API object,
so browser-use method names such as `getInfo`, `createTab`, `attach`,
`detach`, and `executeCdp` dispatch directly into IAB handlers.

On macOS the packaged prod build loads peer authorization from:

```text
/Applications/ChatGPT.app/Contents/Resources/native/browser-use-peer-authorization.node
```

The policy is decided by `shouldIncludeBrowserUsePeerAuthorization(bundle, platform)`;
the `CODEX_BROWSER_USE_PEER_AUTHORIZATION=1` env var can force it on. On
non-darwin platforms the authorizer is a no-op that returns
`{ authorized: true }`. In a macOS packaged build with no readable package metadata the
authorizer fails closed with `missing-package-build-flavor` and rejects peers.

The addon authorizes the connecting process from the socket file descriptor.
This is separate from browser profile isolation; it protects who can talk to the
local IAB pipe.

Practical consequence for tooling: on this macOS prod build, connecting to
`/tmp/codex-browser-use/<uuid>.sock` with a plain socket client and sending
`getInfo` does **not** produce a response. Observed results from an unprivileged
probe were `timeout` or `write EPIPE` on every candidate socket, because the
server drops unauthorized peers instead of replying with a JSON-RPC error. The
in-app `globalThis.nodeRepl.nativePipe` bridge is an observed authorized path;
the complete set of authorized client entry points has not been tested.

### Peer Authorization Static Evidence 2026-10-13

Re-read from the installed main bundle and native addon:

- `handleSocketConnection` calls `socketPeerAuthorizer` before it adds the
  socket to the client maps or registers its data handler. Exceptions and an
  unauthorized result call `socket.destroy()` immediately. This is a gate
  **after OS accept, before frame parsing**, not a guarantee that an unauthorized
  same-user process's `connect()` syscall itself fails.
- The main-process wrapper passes the accepted socket fd and the dev-mode
  boolean into `authorizeSocketPeer(fd, devMode)`.
- Native imports include `getsockopt`, `audit_token_to_pid`,
  `SecCodeCopyGuestWithAttributes`, `SecCodeCopySigningInformation`,
  `kSecGuestAttributeAudit`, `kSecGuestAttributePid` and `proc_pidinfo`.
- Native diagnostics identify `LOCAL_PEERTOKEN`, `teamId`, `signingIdentifier`,
  peer/parent/grandparent identities, `untrusted-process-ancestry`,
  `missing-code-signing-identity` and `untrusted-code-signing-identity`.
- Exported helper symbols include
  `extension_host_copy_code_identity_for_audit_token` and
  `extension_host_copy_code_identity_for_audit_token_at_depth`.
- Embedded identifiers include the Codex app variants and runtime/CLI/agent
  identities. Their precise allowlist combinations and dev-mode behavior remain
  to be established; a binary string is not proof that an identifier alone is
  sufficient for authorization.

The evidence supports audit-token-based peer identity plus signing/ancestry
checks. It does not establish every branch of the native policy, or prove that
reading signing metadata performs a complete signature validity check.

Addon SHA-256 for this inspection:
`d3b29939c28cb1ec1fcef72c35e9c5a6a34d2ad837beb387a83a6554105ae693`.
Only static symbols and curated conclusions are retained; the binary is not
copied into the repository. The proposed open-source policy is tracked
separately in [IAB local transport authentication](../../../design-docs/iab-transport-auth.md).

## Multi-Tab Route Semantics (2026-10 revision)

> Superseded model: the pre-2026-10 build exposed one browser sidebar page per
> `(windowId, conversationId)` route. The ChatGPT-branded build
> (`26.1007.30912`) exposes a full multi-tab sidebar keyed by
> `(conversationId, browserTabId)`. See
> `notes/open-questions.md` for the retired single-page hypothesis.

One conversation now owns **many** browser sidebar tabs. `windowId` still
scopes the route, but tabs are no longer one-per-conversation.

Observed state shapes:

```text
BrowserSessionRegistry
  sessionRoutes: Map<sessionId, SessionRoute>
  windows: Map<windowId, WindowState>

SessionRoute
  sessionId            // == conversationId for top-level sessions
  browserConversationId
  ownerWebContentsId
  windowId
  disposeAfterSessionActivity: boolean

WindowState
  threads: Map<pageKey, ThreadState>      // NOT keyed by conversationId
  pages: Map<pageKey, PageRecord>

ThreadState
  conversationId
  browserTabId        // per-tab identity
  browserUseTabLifetime: "temporary" | "persistent"
  isBrowserUsePage, isBrowserUseActive
  page: PageState | null
```

Key identifier change — `pageKey` includes the tab:

```text
routeKey     = `${windowId}:${conversationId}`      // unchanged
routeTabKey  = `${routeKey}\0${browserTabId}`       // new per-tab key
pageKey      = `${conversationId}\0${browserTabId}` // was `${ownerWebContentsId}:${conversationId}`
```

`cdpTabId` is still the browser-use–facing numeric ID, still independent of
Chrome. The map set is now genuinely multi-tab:

```text
tabIdsByRouteTabKey: Map<routeTabKey, cdpTabId>   // replaces tabIdsByPageKey as the create path
tabIdsByPageKey:     Map<pageKey, cdpTabId>
selectedTabIdsByRouteKey: Map<routeKey, cdpTabId> // selected tab within the route
```

`createTabForBrowserUse` mints a fresh `browserTabId` via
`browser-use:${randomUUID()}` and calls
`openPageForBrowserUse({ browserTabId, createPanelTab: true })`, so each
`createTab` materializes a distinct sidebar panel tab. `getTabsForBrowserUse`
walks **every** `ThreadState` whose `conversationId` matches the route, so the
backend now reports a real tab list rather than a single synthetic page.

`window.open` from a sidebar page is also admitted as a new sidebar tab
(`admitBrowserTab()` + `close-browser-tab` messaging) when the opener is an Owl
browser-sidebar host.

## Tab Lifetime And Turn Cleanup

Tab persistence is explicit and turn-scoped:

```text
lifetime = "temporary"   // agent-created via createTab
lifetime = "persistent"  // pre-existing user tab, or released via markHandoff
mark = { status: "handoff" | "deliverable", turnId }
```

`markTab` requires an integer `tabId` and a `turn_id`, and accepts only
`handoff` or `deliverable`. It is advertised in `getInfo` via
`apiSupportOverrides: { "Tab.markDeliverable": true, "Tab.markHandoff": true }`.

At `turnEnded`:

- a `handoff` mark on a **temporary** tab deletes the mark and leaves the tab open;
- any other mark, or a `persistent` lifetime, calls `releaseTab` (tab survives
  but leaves browser-use control);
- everything else calls `closeTab`.

Lifetime flips to `persistent` on `releasePageForBrowserUse`, which is the path
used when a user takes a tab back over.

## Renderer Webview Lifecycle 2026-04 Snapshot

The following DOM maps and attach snippets describe the original single-page
build. They have not been re-verified as the current multi-tab renderer contract.
Use the current route/tab maps above for the 2026-10 backend model.

The renderer owns the DOM-side `<webview>` objects. It keeps maps by
`conversationId`:

```text
snapshots: Map<conversationId, BrowserSnapshot>
webviews: Map<conversationId, BrowserSidebarWebview>
browserUseActiveStates: Map<conversationId, { turnId }>
browserUseCursorStates: Map<conversationId, CursorState>
```

When a `browser-sidebar-state` snapshot has `tabType = WEB`, the renderer
creates or reuses a hidden webview. When the tab type is no longer web, it
disposes the webview for that conversation.

The renderer creates the webview like this:

```text
container = document.createElement("div")
webview = document.createElement("webview")
webview.setAttribute("data-browser-sidebar-conversation-id", conversationId)
webview.setAttribute("partition", `persist:codex-browser-app-route:${encodeURIComponent(conversationId)}`)
webview.setAttribute("src", initialUrl || "about:blank")
document.body.append(container)
```

Visible browser panels and browser-use control are both projections of the same
webview. If the panel is visible, the container is positioned at the browser
panel bounds. If browser-use is active while the panel is hidden, the container
is kept as an almost invisible fixed element so the guest page stays alive and
automation can still target it.

## Webview Attach Handoff 2026-04 Snapshot

The main process listens to the owner `webContents`:

```text
ownerWebContents.on("will-attach-webview", ...)
ownerWebContents.on("did-attach-webview", ...)
```

During `will-attach-webview`, main process:

- reads the route-bearing partition to recover `conversationId`
- rejects the attachment if no matching thread/page exists
- rewrites `partition` to the shared `persist:codex-browser-app`
- sets the configured browser `session`
- injects `comment-preload.js`
- enforces `sandbox = true`, `nodeIntegration = false`,
  `contextIsolation = true`, and `devTools = true`

During `did-attach-webview`, main process receives the guest `webContents` and
attaches it to the `PageState`. That is the moment Codex can map:

```text
conversationId -> pageKey -> guest webContents -> cdpTabId
```

The page then gets event listeners for navigation, favicon updates, load
failures, destruction, render-process-gone, context menu, zoom, comments, and
runtime sync.

## Storage Partition

Electron uses `partition` as a browser storage boundary. A persistent partition
is a named profile-like storage bucket.

The renderer initially created webviews with the route-bearing partition below
in the 2026-04 snapshot. Its exact creation/handoff encoding has not been
re-verified for the newer multi-tab renderer:

```text
persist:codex-browser-app-route:<encoded conversationId>
```

The main process reads this value during webview attachment to recover the
`conversationId`, then rewrites the attached webview to use:

```text
persist:codex-browser-app
```

The main process also configures that partition through:

```text
session.fromPartition("persist:codex-browser-app")
```

Observed local storage path:

```text
~/Library/Application Support/Codex/Partitions/codex-browser-app/
```

This partition stores browser data such as cookies, local storage, IndexedDB,
and cache. It is shared by IAB webviews. Conversation/window separation is an
application-level routing abstraction, not a separate cookie profile.

Observed contents include `Cookies`, `Session Storage`, `WebStorage`,
`GPUCache`, `DawnWebGPUCache`, `TransportSecurity`, and Chromium preference
files.

## Command Execution

The browser-use command path is:

```text
browser-client.mjs
  -> native pipe JSON-RPC
  -> IAB API method
  -> cdpTabId lookup
  -> guest webContents
```

For ordinary CDP commands, IAB attaches Electron's debugger to the guest
`webContents` using protocol version `1.3` and calls:

```text
webContents.debugger.sendCommand(method, commandParams)
```

Debugger `message` events are re-emitted to browser-use clients as CDP events
with the logical `tabId`.

In the original 2026-04 input implementation, some `Input.*` CDP commands were
special. It did not forward all of them to Chromium directly and supported a
small translated set. The translation details below have not been re-verified
against the newer build:

```text
Input.dispatchKeyEvent
Input.dispatchMouseEvent
Input.insertText
Input.synthesizeScrollGesture
```

Those commands are converted into JavaScript and executed inside the guest page
with:

```text
webContents.executeJavaScript(...)
```

The translation exists to preserve focus and DOM behavior in the embedded
Electron webview. Unsupported `Input.*` commands fail explicitly.

Navigation commands are also checked before execution. When a browser-use CDP
command implies navigation, IAB asks the browser sidebar manager to assert and
track the pending navigation for the target `pageKey`.

## Cursor Handshake 2026-04 Snapshot

The flow below records the original turn-route handshake. The current backend
captures session routes; the exact renderer arrival payload still needs a new
verification before treating this old handshake as current.

IAB also mirrors browser-use cursor state into the renderer. A cursor command
does not only call CDP. The backend sends:

```text
browser-sidebar-browser-use-cursor-state
```

to the owner renderer for the route. The renderer draws the cursor overlay and
then reports arrival with:

```text
browser-use-cursor-arrived
```

The main process checks that the arrival came from the owner `webContents` for
the captured `turnRouteKey`, then resolves the backend-side cursor wait.

This is why cursor movement can wait for visual arrival even though the actual
page is a guest webview.

## End-To-End Flow

```text
agent.browser.tabs.new()
  -> browser client/service JSON-RPC over an authorized native pipe
  -> ChatGPT.app IAB backend for the captured session route
  -> mint browserTabId = browser-use:<uuid>
  -> pageKey = conversationId + NUL + browserTabId
  -> openPageForBrowserUse({ browserTabId, createPanelTab: true })
  -> attach the new tab's guest webContents
  -> assign cdpTabId indexed by routeTabKey
  -> return logical tab info to browser-use client
```

For later commands:

```text
agent/browser-use command with tab.id and turn_id
  -> cdpTabId
  -> tabsById
  -> tabRuntimesById
  -> Electron guest webContents
  -> debugger CDP command, translated page JavaScript, or navigation call
```

## Architecture Sketch

```text
ChatGPT BrowserWindow #12
  owner webContents #100
    ChatGPT UI
      conversation A
        routeKey = "12:A"
        sidebar tab A1
          browserTabId = browser-use:<uuid-A1>
          pageKey = A + NUL + browserTabId
          guest webContents #201
          logical cdpTabId = 1
        sidebar tab A2
          browserTabId = browser-use:<uuid-A2>
          pageKey = A + NUL + browserTabId
          guest webContents #202
          logical cdpTabId = 2

      conversation B
        routeKey = "12:B"
        sidebar tab B1
          browserTabId = browser-use:<uuid-B1>
          pageKey = B + NUL + browserTabId
          guest webContents #203
          logical cdpTabId = 3

Shared browser storage:
  ~/Library/Application Support/Codex/Partitions/codex-browser-app/
```

## Implementation Sketch

```text
Renderer
  browser-sidebar-state
    -> render sidebar pages by conversation and browserTabId
    -> selected tab is a presentation choice, not the entire page collection

Main process
  captureSessionRoute
    -> resolve owner and window for the session
    -> canServeSession checks the session binding

  new sidebar tab
    -> pageKey = conversationId + NUL + browserTabId
    -> associate that page with its guest webContents

IAB API
  createTab(session_id, turn_id)
    -> resolve session route
    -> open/find BrowserUseHost
    -> create a fresh browserTabId and panel page
    -> record routeTabKey -> cdpTabId and tab runtime
    -> return { id: cdpTabId, providerTabId: browserTabId, ... }

  executeCdp(tabId, method, params)
    -> validate tab belongs to route
    -> attach webContents.debugger
    -> sendCommand(...) or executeJavaScript(input translation)
```

## Practical Interpretation

- Browser-use tab records include numeric `tab.id` (`cdpTabId`) and the provider's
  `providerTabId` (`browserTabId`).
- `pageKey` and `webContents` remain app internals.
- `windowId` keeps browser-use commands bound to the correct Codex desktop
  window.
- The current serving gate uses the captured session route. `turn_id` remains
  relevant to marks and cleanup, not peer identity authentication.
- One `(windowId, conversationId)` route owns multiple sidebar tabs in the
  2026-10 build; each page has its own browserTabId and guest webContents.
- On the inspected macOS prod build, native peer authorization precedes RPC
  parsing. Socket discovery metadata is not an authorization credential.
- Browser state such as cookies is shared across IAB pages through the
  `persist:codex-browser-app` partition.
