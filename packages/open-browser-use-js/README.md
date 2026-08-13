# Open Browser Use JavaScript SDK

JavaScript/TypeScript client for controlling a real Chrome profile through
Open Browser Use. The SDK keeps the low-level JSON-RPC/CDP surface available and
also provides higher-level browser/tab helpers for common agent workflows.

## Prerequisites

- The `open-browser-use` CLI and Chrome extension are installed and connected.
- `open-browser-use ping` returns `"pong"`.
- The native host has written an active socket registry at
  `/tmp/open-browser-use/active.json`.

```sh
open-browser-use ping
open-browser-use info
```

## Installation

```sh
npm install open-browser-use-sdk
```

## High-Level Browser API

Use `connectOpenBrowserUse` when you want a Playwright-like flow in a normal
Node runtime.

```js
import { readFile } from "node:fs/promises";
import { connectOpenBrowserUse } from "open-browser-use-sdk";

const registry = JSON.parse(
  await readFile("/tmp/open-browser-use/active.json", "utf8")
);

const browser = await connectOpenBrowserUse({
  socketPath: registry.socketPath,
  sessionId: "github-issue-scan",
  turnId: `turn-${Date.now()}`,
  timeoutMs: 20000,
});

let tab;

try {
  await browser.client.nameSession("GitHub issue scan - OBU");

  tab = await browser.newTab();

  await tab.goto("https://github.com/iFurySt/open-codex-computer-use/issues", {
    waitUntil: "domcontentloaded",
    timeoutMs: 15000,
  });

  await tab.playwright.waitForLoadState({
    state: "domcontentloaded",
    timeoutMs: 15000,
  });

  const snapshot = await tab.playwright.domSnapshot();
  const relevant = snapshot
    .split("\n")
    .filter((line) =>
      /Open|Closed|Issues|issue|No results|open-codex-computer-use|Pull requests|Starred/.test(line)
    );

  console.log(relevant.slice(0, 160).join("\n"));
} finally {
  await browser.client.finalizeTabs([]);
  browser.close();
}
```

The same tab helpers are available without the `playwright` alias:

```js
await tab.goto(url, { waitUntil: "load", timeoutMs: 15000 });
await tab.waitForLoadState({ state: "domcontentloaded", timeoutMs: 15000 });
const text = await tab.domSnapshot();
const value = await tab.evaluate("document.title");
```

## Multi-Tab Workflows

For multiple pages, create tabs first, then run navigation and extraction in
parallel. This avoids backend session races while still parallelizing the slow
page work.

```js
const targets = [
  ["repo", "https://github.com/iFurySt/open-codex-computer-use"],
  ["issues", "https://github.com/iFurySt/open-codex-computer-use/issues"],
  ["pulls", "https://github.com/iFurySt/open-codex-computer-use/pulls"],
];

const tabs = [];
for (const [kind] of targets) {
  tabs.push([kind, await browser.newTab()]);
}

await Promise.all(
  targets.map(([_, url], index) =>
    tabs[index][1].goto(url, { waitUntil: "load", timeoutMs: 25000 })
  )
);

const results = await Promise.all(
  tabs.map(async ([kind, tab]) => ({
    kind,
    tabId: tab.id,
    text: await tab.playwright.domSnapshot(),
  }))
);
```

## Low-Level Client API

Use `OpenBrowserUseClient` when you need direct Browser Use JSON-RPC methods or
raw CDP commands.

```js
import { readFile } from "node:fs/promises";
import { OpenBrowserUseClient } from "open-browser-use-sdk";

const { socketPath } = JSON.parse(
  await readFile("/tmp/open-browser-use/active.json", "utf8")
);

const client = new OpenBrowserUseClient({
  socketPath,
  sessionId: "raw-cdp-example",
});

try {
  await client.connect();
  await client.nameSession("Raw CDP example - OBU");

  const tab = await client.createTab();
  await client.attach(tab.id);
  await client.executeCdp(tab.id, "Page.navigate", {
    url: "https://example.com",
  });

  const title = await client.executeCdp(tab.id, "Runtime.evaluate", {
    expression: "document.title",
    returnByValue: true,
  });

  console.log(title.result.value);
} finally {
  await client.finalizeTabs([]);
  client.close();
}
```

The unrestricted escape hatch is `request(method, params)`:

```js
await client.request("executeCdp", {
  target: { tabId: 123 },
  method: "Runtime.evaluate",
  commandParams: {
    expression: "document.body.innerText",
    returnByValue: true,
  },
});
```

## Notifications

The JS SDK can subscribe to JSON-RPC notifications from the native socket. This
is useful for downloads and CDP events.

```js
const unsubscribe = client.onNotification((event) => {
  if (event.method === "onDownloadChange") {
    console.log("download", event.params);
  }
  if (event.method === "onCDPEvent") {
    console.log("cdp", event.params);
  }
});

// Later:
unsubscribe();
```

## Common Methods

- Browser/session: `getInfo`, `nameSession`, `focusState`, `turnEnded`, `reconcileOperation`
- Tabs: `createTab`, `getTabs`, `getUserTabs`, `claimUserTab`, `claimStatus`, `closeTab`, `finalizeTabs`
- CDP: `attach`, `detach`, `executeCdp`, `request`
- Input: `moveMouse`
- File chooser: `waitForFileChooser`, `setFileChooserFiles`
- Downloads: `waitForDownload`, `downloadPath`
- Clipboard: `readClipboardText`, `writeClipboardText`, `readClipboard`,
  `writeClipboard`

## Cleanup

Always finish browser work by finalizing tabs and closing the socket:

```js
try {
  // Browser work here.
} finally {
  await browser.client.finalizeTabs([]);
  browser.close();
}
```

Use `finalizeTabs([])` unless the user explicitly needs a tab left open. To keep
a tab, pass a keep item such as:

```js
await browser.client.finalizeTabs([{ tabId: tab.id, status: "handoff" }]);
```

## Notes

- The browser is the user's real Chrome profile. Do not inspect unrelated data.
- The SDK does not include Codex-specific site policy or approval prompts; add
  policy in your own runtime if needed.
- If connection fails, run `open-browser-use ping` and `open-browser-use info`.
- If there is no active socket, open Chrome with the extension enabled or rerun
  setup according to the Open Browser Use installation guide.
