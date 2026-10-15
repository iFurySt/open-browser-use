# Electron IAB action demo

The app demonstrates one integration: **Electron main + Go CLI + private channel**.
Main launches `obu iab` child processes and supplies their authorization through
inherited FD 3. The CLI sends browser commands back to the main-owned IAB.

## Run

From the repository root, with Node 22+, pnpm 8.15.9, Go 1.25.10+ and native build
tools installed:

```sh
pnpm install
pnpm example:iab
```

The app starts with no browser tabs. Click any manual action repeatedly, or choose
“播放” to run the whole sequence:

| Step | Visible action | Implementation |
| --- | --- | --- |
| 1 | Open Tab 1 | CLI `iab open-tab` |
| 2 | Open Tab 2 | CLI `iab open-tab`, both pages remain open |
| 3 | Switch to Tab 1 and increment its counter | Main presents the tab; CLI reads coordinates and sends trusted CDP mouse input |
| 4 | Switch to Tab 2, click the field, type and submit | Main presents the tab; CLI mouse/text input submits `Hello from CLI` |
| 5 | Open Tab 3 at `https://github.com/ifuryst` | CLI opens a blank tab, main presents it, then CLI navigates and waits for document readiness |

Playback pauses briefly between steps so the changes are visible. “停止播放”
finishes the current step before stopping; manual execution can then continue.
All three tabs remain open after completion or stopping, so users can inspect
counts, submitted messages and GitHub. Only “重新播放” explicitly cleans this
example scope and starts fresh. Actions 1, 2 and 5 open a new page on every click,
preserving previous pages; actions 3 and 4 repeat the click or input/submit on
the most recently opened Tab 1 or Tab 2. Tab 2 and Tab 3 can be opened before
Tab 1. If the required tab is missing or closed, the app asks users to open it
with action 1 or 2. Manual actions are available whenever no operation is busy;
playback and concurrent manual calls remain guarded. Progress counts distinct
successful actions, and repeated runs replace that action's recorded result.
Raw results are
collapsed under “查看结果”. The right browser panel retains ordinary tab selection,
new/close, address navigation, history, reload and hide/restore controls. Tab
selection and hiding remain available during playback. Focusing the address
field requests a pause; Enter, reload, history navigation, create or close waits
for the current step and cancels remaining automatic steps before changing a
page. Manual requests are serialized, with the intended tab captured and checked
again in main. Other tabs retain their state.

The address field accepts full HTTP/HTTPS URLs, bare domains such as
`github.com/ifuryst`, and local addresses such as `localhost:3000/page`.
Bare public domains use HTTPS; localhost/loopback addresses use HTTP. Press
Enter to visit the entered site. First focus selects the entire address, with a
blue focus border and text selection; later clicks and drags place the caret or
select a range. Cmd/Ctrl+L focuses and selects the address, including from a guest
page. File/script URLs and embedded credentials are rejected.
The idle address bar has a white background; hovering adds a soft gray fill,
slightly stronger border and icon, with a brief transition. Editing retains the
blue focus border. Reduced-motion preferences disable the transition.

A spinner appears in each loading tab and in the selected tab's address bar.
Loading starts before the server responds, follows redirects, and stops when
Electron finishes loading. Switching tabs shows that tab's own state. Failed
navigation displays “加载失败”; editing the address and retrying clears it.
Main publishes state without waiting for a slow CLI navigation, and refreshes
back/forward history separately in the background.

All demo page operations use real CLI processes over private transport. Main
controls presentation and choreography, without replacing CLI actions with main
page calls or synthetic DOM clicks. No socket listener is needed for normal app
playback. The local fixtures require no account, API key or Chrome extension. Tab 3 opens
the public GitHub profile and requires network access; no GitHub actions are performed.
Persistent profile data uses the example app's normal userData directory.

“操作提示” shows the existing Browser Use mouse and a soft blue glow on all four sides of the
operated page. It is enabled in this demo and can be switched off during playback.
Mouse positions follow real CLI/CDP input; main marks the operation lifecycle in
`demo.mjs` and clears it in `finally`. The library defaults both effects to off
and supports separate cursor/border options. See the package's
[feedback API](../../packages/open-browser-use-electron/README.md#optional-operation-feedback).
Both visuals, including cursor assets and animation, are supplied by
`open-browser-use-electron`. The example only configures the SDK and marks the
active tab's operation lifetime. The edge glow fades smoothly inward, without a
hard outline or overlapping corner strips. `main.mjs` sets
`visualFeedback: { cursor: true, edgeGlow: true, edgeGlowColor: '#4f8fe8' }`.
An integrating host can independently toggle `edgeGlow` and change
`edgeGlowColor` (`#RGB` / `#RRGGBB`) with `runtime.visualFeedback.configure(...)`;
the example's existing checkbox changes the toggles while preserving that color.

The app and local pages use plain monochrome styling. The action list, browser
controls, counter and message form remain visible; extra fixture operations and
raw results are collapsed. Host styles are in `styles.css`, guest styles in
`fixture.css`; neither page uses external fonts or decorative image assets.

## Code to reference

- `main.mjs`: runtime, profile, scope, surface, sender-checked IPC and private CLI binding.
- `address.mjs`: shared URL normalization for Enter and main-process validation.
- `demo.mjs`: repeatable manual actions, explicit tab IDs, trusted input and ordered playback/stop.
- `integrations.mjs`: two reference cases, each supporting both transports.
- `worker.mjs`: Node worker using the `/client` entry point.
- `mcp.mjs`: dependency-free MCP client for already-launched IAB stdio pipes.
- `fixture.mjs` / `fixture.css`: minimal local counter, form, synthetic login cookie and popup opener fixture.

The other integrations remain in code and smoke tests, without separate app buttons:

| Case | Transport | Host call |
| --- | --- | --- |
| Electron main + Node worker | private | `clients.launchNode('private', url)` |
| Electron main + Node worker | socket | `clients.launchNode('socket', url)` |
| Electron main + CLI | private, the visible demo | `clients.launchCli('private', ['open-tab', url])` |
| Electron main + CLI | socket | `clients.launchCli('socket', ['open-tab', url])` |
| Electron main + MCP | private or socket | `clients.launchMcp('private')` / `clients.launchMcp('socket')` |

MCP remains a code reference without additional demo buttons:

```js
import { connectExampleMcp } from './mcp.mjs';

const mcp = await connectExampleMcp(await clients.launchMcp('private'));
try {
  const opened = await mcp.callTool('open_tab', { url: 'https://example.com' });
  if (opened.isError) throw new Error(opened.content[0].text);
  const tabId = opened.structuredContent.id;
  // Main chooses whether/where to display the new page.
  surface.present(tabId);
  const snapshot = await mcp.callTool('snapshot', { tab_id: tabId });
  const image = await mcp.callTool('screenshot', { tab_id: tabId });
} finally {
  await mcp.close(); // EOF + revoke; pages remain available in the app.
}
```

`launchMcp` starts `obu iab mcp` with `stdin: 'pipe'`. The MCP client handles
initialize/initialized and request correlation over stdin/stdout; FD 3 stays
private to the IAB transport. A production host can use its agent framework's
MCP client with the already-launched pipes instead of this small reference client.

`createExampleClients` launches managed tools through `runtime.launchClient`.
Private bootstrap fixes scope and grant; socket mode also verifies native peer
identities. Secrets do not appear in argv, UI or stdout. See the package
[integration API](../../packages/open-browser-use-electron/README.md) and
[authentication design](../../docs/design-docs/iab-transport-auth.md).

`preload.cjs` exposes fixed example operations. Main validates sender, main frame
and the exact local UI URL, and confines measured viewport bounds to the right
panel below its toolbar. Browser guests have no host preload or IPC access.

## Local smoke

```sh
pnpm smoke:iab
pnpm --filter @open-browser-use/electron-iab-example test
```

The real Electron smoke exercises all four reference transports, private/socket
MCP lifecycle and image results, trusted MCP mouse/text input, scope/grant
rejection, revocation and retained pages after EOF, shared login,
multi-tab ownership, hidden trusted input, socket rejection, popup lifecycle,
panel bounds/resize/navigation, address focus/selection, bare-host Enter, held
server responses, per-tab loading, navigation failure/retry, optional feedback,
live color updates and independent toggles, and the original cursor bitmap
under strict CSP, the five manual CLI-private steps, automated
replay, repeated clicks on all five manual action buttons, missing-tab guards,
preserved results, actual address-form/toolbar clicks and manual takeover during playback. Reports and host/page/demo screenshots go to
`.cache/iab-smoke/`. Automated smoke injects a local third-tab URL to verify
sequencing and retained state without depending on GitHub availability; validate
the actual GitHub URL separately in the interactive app. Smoke uses a hidden,
disposable window with host rendering enabled so manual app clicks cannot alter
the test sequence. Opening Tab 3 waits for
the document, rather than every remote image or other subresource. The disposable smoke profile is removed after Electron exits.

Electron 44.7.0 / macOS arm64 is locally validated. Linux needs a desktop or Xvfb
for the smoke. Windows socket mode is unavailable, and Windows private channels
still require validation. Site, IME, cross-origin-frame and minimized-window
compatibility need further testing. The npm package has not been published.
