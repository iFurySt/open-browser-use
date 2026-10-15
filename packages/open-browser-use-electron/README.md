# Open Browser Use Electron

Embed a multi-tab browser in an Electron app. A named profile supplies shared
login storage; each task scope owns independent tabs. The host controls where
tabs are displayed and which tool processes can operate them.

This package is available in this workspace; npm publication is a separate step.
Electron is a peer dependency supplied by the host. Node clients import the
`/client` entry point, which does not initialize Electron.

## Run the reference app

From the repository root:

```sh
pnpm install
pnpm example:iab
pnpm smoke:iab
```

The [Electron example](../../examples/electron-iab/README.md) visibly demonstrates
Electron main + CLI + private channel through five manual or automated actions,
retaining all three tabs for inspection afterward.
Node/CLI × private/socket reference integrations remain in code and smoke tests.

Requirements: Node 22+, pnpm 8.15.9, Go 1.25.10+ for the example CLI, and a C++
toolchain plus Python for the Node-API addon. The example pins Electron 44.7.0.
The declared Electron range is 30–44; the local smoke matrix currently covers
macOS arm64 with Electron 44.7.0 and Node 24.19.0.

## Main-process integration

```ts
import { app, BrowserWindow } from 'electron';
import { createIabRuntime } from 'open-browser-use-electron';

app.whenReady().then(async () => {
  const iab = createIabRuntime({ appId: 'my-app' });
  iab.profiles.open({ id: 'work', persistent: true });
  const task = iab.scopes.open({ id: 'task-a', profileId: 'work' });
  const first = await task.tabs.create({ url: 'https://example.com' });
  const second = await task.tabs.create({ url: 'https://example.org' });

  const window = new BrowserWindow({ width: 1200, height: 800 });
  const surface = iab.surfaces.register({
    id: 'browser-panel', window, owner: window.webContents,
  });
  surface.setBounds({ x: 400, y: 80, width: 780, height: 680 });
  surface.present(first.id);

  // The target remains explicit even when another tab is displayed.
  await task.call('executeCdp', {
    target: { tabId: second.id }, method: 'Runtime.evaluate',
    commandParams: { expression: 'document.title', returnByValue: true },
  });
  // Hidden/removed surfaces do not close pages.
  surface.hide();
  // During host shutdown: await iab.dispose();
});
```

Use a unique appId per host integration. Different scopes can use the same
profile; different accounts should use different profiles. Scope ownership
isolates API access, while shared profile cookies and storage remain shared.
Each scope can hold 32 tabs; one runtime can hold 128.

Tab snapshots from `scope.tabs.list()`, `getTabs`, and `onTabUpdated` include
`loading` (Electron's loading lifecycle), optional `pendingUrl` during main-frame
navigation/redirect, and optional `loadError: { code, description, url }`.
A new main-frame navigation clears the previous error; iframe failures and
aborted/replaced navigations do not mark the whole tab as failed. Hosts can
render loading immediately from these snapshots without awaiting a page command.
The example separately normalizes user-entered addresses; the runtime still
requires an explicit permitted URL.

## Private tool process

Host main:

```ts
const worker = await iab.launchClient({
  scopeId: task.id,
  command: '/absolute/path/to/node',
  args: ['/absolute/path/to/worker.mjs'],
  transport: 'private', // default; no listener is opened
  ttlMs: 60_000,
  operations: ['tabs.read', 'tabs.create', 'page.control'],
});
worker.child.stdout?.on('data', chunk => console.log(String(chunk)));
// worker.revoke() immediately closes its authorized connections.
```

Worker:

```ts
import { connectPrivateIab } from 'open-browser-use-electron/client';

const client = await connectPrivateIab(); // inherited duplex FD 3
try {
  const browser = client.browser(); // existing Browser Use SDK helpers
  const tab = await browser.newTab({ url: 'https://example.com' });
  console.log(await tab.domSnapshot());
} finally {
  client.close();
}
```

FD 3 carries a private bootstrap followed by framed JSON-RPC. The bootstrap
fixes instance, scope, grant and credential. Credentials are not passed in
argv, environment variables or discovery files. The tool must never log its
bootstrap. The host is responsible for choosing trustworthy commands/scripts;
possession or deliberate forwarding of the private channel delegates its grant.

## Authenticated socket

Change the host launch option to `transport: 'socket'`. The worker still calls
`connectPrivateIab()`: it receives private bootstrap, verifies the socket server
identity, then connects and authenticates. Standalone socket clients can use
`connectSocketIab(bootstrap)` only when a trusted host privately supplied that
bootstrap.

Socket mode requires the native adapter and currently supports macOS/Linux.
macOS uses the kernel audit token; Linux uses SO_PEERCRED. Both bind PID, UID
and process start time to a host-registered process, then require a 256-bit
expiring lease. This first version uses managed process registration, rather
than a signing-identifier allowlist. Unsupported platforms fail closed with
AUTH_UNAVAILABLE. Windows private-descriptor behavior has not been smoke tested.

The socket is created in a new 0700 directory with 0600 permissions, outside
Chrome discovery. Peer authorization happens before frame parsing. Same-UID
processes may establish an OS connection but cannot use browser methods without
the registered identity and grant. Clients also check server identity before
sending a secret. Secret comparisons use constant-time digests.

Grants require an explicit ttlMs up to 24 hours, default to one simultaneous
connection and allow at most 16. Expiry, revoke, scope disposal and managed
process exit terminate access. Each request/event checks scope and operation
permissions; queued CDP input rechecks authorization before execution. Previously
completed side effects cannot be rolled back by revocation.

## IAB CLI

The host launches the built CLI exactly like a worker:

```ts
await iab.launchClient({
  scopeId: task.id,
  command: '/absolute/path/to/obu',
  args: ['iab', 'tabs'],
  transport: 'socket', // or 'private'
  ttlMs: 30_000,
});
```

Commands: `info`, `ping`, `tabs`, `open-tab [url]`, `navigate <tab-id> <url>`,
`close-tab <tab-id>`, `cdp <tab-id> <method> [params-json]`,
`call <method> [params-json]`, and `turn-ended`.
`--channel-fd` selects the inherited bootstrap descriptor; `--turn-id` is
bookkeeping, not authorization. Without host bootstrap, commands fail rather
than scan or fall back to Chrome. Go IAB transport is tested on macOS and
cross-compiled for Linux/Windows; Windows IAB socket mode remains disabled.

## Optional operation feedback

Feedback is off by default. The host can enable the Browser Use cursor and a
soft blue glow on all four inward edges independently:

```ts
const iab = createIabRuntime({
  appId: 'my-app',
  visualFeedback: { cursor: true, edgeGlow: true, edgeGlowColor: '#4f8fe8' },
});
// Mark the full operation, including work between individual input commands.
await task.visualFeedback.setActive(first.id, true);
try {
  // Run the authorized CLI/worker actions targeting first.id.
} finally {
  await task.visualFeedback.setActive(first.id, false);
}
// Toggle the edge independently, or recolor active pages without reloading.
await iab.visualFeedback.configure({ edgeGlow: false });
await iab.visualFeedback.configure({ edgeGlow: true, edgeGlowColor: '#8b5cf6' });
await iab.visualFeedback.configure({ cursor: false, edgeGlow: false });
```

| Option | Default | Meaning |
| --- | --- | --- |
| `cursor` | `false` | Show the original Browser Use cursor and click ripple. |
| `edgeGlow` | `false` | Show the inward edge glow, independently of the cursor. |
| `edgeGlowColor` | `'#4f8fe8'` | One base color for all four edges, fading to transparent; accepts `#RGB` or `#RRGGBB`. |

The same options work in `createIabRuntime({ visualFeedback: ... })` and
`iab.visualFeedback.configure(...)`. Updates only replace supplied, defined
fields; setting a color does not enable the effect, and disabling/re-enabling
the edge preserves its color. Color updates apply to active tabs in this runtime
immediately, including pages with strict CSP. Hex values are normalized to
lowercase `#rrggbb`; invalid values reject the whole update without changing
existing options. `iab.visualFeedback.getOptions()` returns a copy of the
current options. Edge color does not change the original cursor or click ripple.

Accepted CDP `Input.*` calls also trigger a 1.2-second activity pulse. Mouse
coordinates come from the real input, with presses snapping to the actual point.
Activity belongs to the explicit target tab; selecting a surface does not move
that activity to another page. The host must finish explicit activity in a
`finally` block. Navigation resets pointer coordinates; deactivation, disable
and page disposal remove decorations and cancel animation. Reduced-motion
preferences disable travel, click-ring and border animation.

Both effects are implemented and shipped by `open-browser-use-electron`,
including the cursor bitmap, motion and edge rendering. The host only configures
them and marks the target tab's operation lifecycle; no example CSS or cursor
code needs to be copied. The edge glow uses blurred inset shadows of the same
configured color, with a wide transparent falloff and subtle breathing. There is no hard
outline or four overlapping gradient strips to form blocks at the corners.

The original extension's `cursor-chat.png`, 23×24 display size, geometry and
spring renderer are reused. `scripts/generate-cursor.mjs` creates the standalone
artifact from that source; build checks freshness. Run the generator after
changing the Chrome cursor. The only image-carrier adaptation draws the original
PNG pixels on canvas, allowing strict image CSP without weakening site policy.

Rendering uses an isolated guest world and closed shadow DOM, with no host IPC
or credentials. Pointer events pass through. This is a visual hint, not a secure
trust indicator: sites can remove or imitate it. The example's “操作提示” checkbox
enables/disables both effects during manual steps or automatic playback.

## Lifecycle and capability limits

- `tabs.create/list/close`, `scope.call/dispose`, `surfaces.register` with
  `present/setBounds/hide/dispose`, and `runtime.launchClient/dispose` are the
  current main API. `runtime.connections` also provides explicit socket/grant
  methods for advanced trusted-host integrations.
- `attach`, `detach`, `executeCdp`, `markTab`, `turnEnded`, `finalizeTabs` and
  session metadata methods are implemented. Temporary tabs are tracked by turn;
  handoff/deliverable marks preserve them. Preserving a tab does not revoke its
  scope's control; revoke the client grant explicitly when handing control back.
- Only page-scoped CDP methods are accepted. Browser/Target/Storage global
  operations, cookie-jar APIs, file uploads, downloads, clipboard, WebMCP and
  child-target sessions are not exposed in this first version. Upload/download
  and browser permission requests are denied by default.
- Navigation accepts http, https and about:blank. The host UI's preload is
  never loaded in remote guest pages; guests enforce sandbox, context isolation,
  disabled Node integration and enabled web security.
- Hidden mouse input prepares compositor hit-test data with an internal screenshot
  before the first event, and again after navigation, layout bounds or presentation
  changes. The screenshot is discarded. Text and mouse events are verified as
  trusted on the example fixture; IME, cross-origin frames and minimized windows
  require further validation.
- A surface is a native view, not a DOM element. Position it with integer DIP
  bounds and handle resizing in the host. The sample validates IPC sender,
  main frame, local UI URL and allowed actions before using main-only APIs.
- Profiles retain browser data until the host manages their storage. Closing
  a scope destroys pages, not its persistent login profile.

When packaging with ASAR, unpack the native `.node` addon and preserve its
`build/Release/iab_peer.node` path. The install script builds from source using
Node-API 8; no multi-platform prebuilt addon distribution or npm release is
included in this change. Host signing policy and more Electron/OS combinations
need their own validation.
