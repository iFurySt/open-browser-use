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

The app starts with no browser tabs. Click steps individually in order, or choose
“播放” to run the whole sequence:

| Step | Visible action | Implementation |
| --- | --- | --- |
| 1 | Open Tab 1 | CLI `iab open-tab` |
| 2 | Open Tab 2 | CLI `iab open-tab`, both pages remain open |
| 3 | Switch to Tab 1 and increment its counter | Main presents the tab; CLI reads coordinates and sends trusted CDP mouse input |
| 4 | Switch to Tab 2, click the field, type and submit | Main presents the tab; CLI mouse/text input submits `Hello from CLI` |
| 5 | Close all tabs | CLI lists and closes every page in this example scope |

Playback pauses briefly between steps so the changes are visible. “停止播放”
finishes the current step before stopping; manual execution can then continue.
“重新播放” cleans this example scope and starts fresh. Closing a demo tab manually
can invalidate a later step; replay restores the sequence. Raw results are
collapsed under “查看结果”. The right browser panel retains ordinary tab selection,
new/close, address navigation, history, reload and hide/restore controls.

All demo page operations use real CLI processes over private transport. Main
controls presentation and choreography, without replacing CLI actions with main
page calls or synthetic DOM clicks. No socket listener is needed for normal app
playback. The local fixture requires no account, API key or Chrome extension.
Persistent profile data uses the example app's normal userData directory.

“操作提示” shows the existing Browser Use mouse and inward gradient edges on the
operated page. It is enabled in this demo and can be switched off during playback.
Mouse positions follow real CLI/CDP input; main marks the operation lifecycle in
`demo.mjs` and clears it in `finally`. The library defaults both effects to off
and supports separate cursor/border options. See the package's
[feedback API](../../packages/open-browser-use-electron/README.md#optional-operation-feedback).

## Code to reference

- `main.mjs`: runtime, profile, scope, surface, sender-checked IPC and private CLI binding.
- `demo.mjs`: five-step controller, explicit tab IDs, trusted input, sequencing and replay/stop.
- `integrations.mjs`: two reference cases, each supporting both transports.
- `worker.mjs`: Node worker using the `/client` entry point.
- `fixture.mjs`: local counter, form, synthetic login cookie and popup opener fixture.

The other integrations remain in code and smoke tests, without separate app buttons:

| Case | Transport | Host call |
| --- | --- | --- |
| Electron main + Node worker | private | `clients.launchNode('private', url)` |
| Electron main + Node worker | socket | `clients.launchNode('socket', url)` |
| Electron main + CLI | private, the visible demo | `clients.launchCli('private', ['open-tab', url])` |
| Electron main + CLI | socket | `clients.launchCli('socket', ['open-tab', url])` |

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
```

The real Electron smoke exercises all four reference transports, shared login,
multi-tab ownership, hidden trusted input, socket rejection, popup lifecycle,
panel bounds/resize/navigation, optional feedback and the original cursor bitmap
under strict CSP, the five manual CLI-private steps, automated
replay and stop/cleanup. Reports and host/page/demo screenshots go to
`.cache/iab-smoke/`. The disposable smoke profile is removed after Electron exits.

Electron 44.7.0 / macOS arm64 is locally validated. Linux needs a desktop or Xvfb
for the smoke. Windows socket mode is unavailable, and Windows private channels
still require validation. Site, IME, cross-origin-frame and minimized-window
compatibility need further testing. The npm package has not been published.
