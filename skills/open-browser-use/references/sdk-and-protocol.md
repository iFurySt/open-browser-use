# Open Browser Use SDK And Protocol

Read this reference when the task requires multi-step automation, integration into another agent runtime, or direct Browser Use style JSON-RPC calls.

## Connection Model

The Chrome or Zen extension starts the native host through browser Native Messaging. The native host exposes a local socket and writes the active socket registry so the CLI and SDKs can discover it.

Default route:

```text
agent runtime
  -> open-browser-use CLI, MCP server, or SDK
  -> active Open Browser Use socket
  -> native messaging host
  -> Chrome or Zen extension
  -> browser tabs / debugger-compatible adapter / history / downloads
```

Pass an explicit socket only when the runtime provides one:

```sh
open-browser-use ping --socket /tmp/open-browser-use/example.sock
```

For SDKs, create a client with `socketPath` / `socket_path` / `SocketPath`.

## Browser Session Scope

Use a unique browser session id for each agent task or conversation. Prefer a
stable session/conversation id from the surrounding runtime when it exists;
otherwise create a short unique id such as `obu-<task-slug>-<timestamp>`.

Pass that same id through every CLI command, MCP server, or SDK client used for
the task. Do not rely on the CLI fallback `obu-cli` in agent workflows; it is a
manual convenience fallback and can reuse stale Chrome tab groups from unrelated
tasks.

Install the SDK package from the package registry for your runtime:

```sh
npm install open-browser-use-sdk
pip install open-browser-use-sdk
go get github.com/ifuryst/open-browser-use/packages/open-browser-use-go
```

The Python distribution is named `open-browser-use-sdk`, while the import module
is `open_browser_use`. Go code usually imports the package as `obu`.

## JavaScript SDK Pattern

Use the high-level browser helper for common multi-step flows:

```ts
import { connectOpenBrowserUse } from "open-browser-use-sdk";

const browser = await connectOpenBrowserUse({
  socketPath: "/tmp/open-browser-use/example.sock",
  sessionId: "obu-docs-scan-20260510",
});

try {
  await browser.client.nameSession("Task - OBU");
  const tab = await browser.newTab();
  await tab.goto("https://example.com", { waitUntil: "domcontentloaded" });
  const text = await tab.playwright.domSnapshot();
  console.log(text.slice(0, 4000));
} finally {
  await browser.client.finalizeTabs([]);
  browser.close();
}
```

Use the low-level client when you need direct Browser Use JSON-RPC/CDP calls:

```ts
import { OpenBrowserUseClient } from "open-browser-use-sdk";

const client = new OpenBrowserUseClient({
  socketPath: "/tmp/open-browser-use/example.sock",
  sessionId: "obu-docs-scan-20260510",
});

await client.connect();
await client.nameSession("Task - OBU");
const tab = await client.createTab() as { id: number };
await client.executeCdp(tab.id, "Page.navigate", { url: "https://example.com" });
await client.finalizeTabs([]);
client.close();
```

The JavaScript SDK supports notification handlers:

```ts
const unsubscribe = client.onNotification((event) => {
  if (event.method === "onDownloadChange") {
    console.log(event.params);
  }
});
```

## Python SDK Pattern

```py
import json
from pathlib import Path

from open_browser_use import connect_open_browser_use

registry = json.loads(Path("/tmp/open-browser-use/active.json").read_text())
browser = connect_open_browser_use(
    socket_path=registry["socketPath"],
    session_id="obu-issue-scan-20260510",
)

try:
    browser.client.name_session("Issue scan - OBU")
    tab = browser.new_tab()
    tab.goto("https://github.com/iFurySt/open-codex-computer-use/issues", wait_until="domcontentloaded")
    tab.playwright.wait_for_load_state(state="domcontentloaded", timeout=15)
    tab.playwright.wait_for_timeout(1500)

    text = tab.playwright.locator("body").inner_text(timeout_ms=10000)
    result = {
        "title": tab.title(),
        "url": tab.url(),
        "text": text[:4000],
    }
    print(result)
finally:
    browser.client.finalize_tabs([])
    browser.close()
```

Use the low-level client when you need raw JSON-RPC/CDP calls:

```py
from open_browser_use import OpenBrowserUseClient

client = OpenBrowserUseClient(
    socket_path="/tmp/open-browser-use/example.sock",
    session_id="obu-docs-scan-20260510",
)

client.name_session("Task - OBU")
tab = client.create_tab()
client.execute_cdp(tab["id"], "Page.navigate", {"url": "https://example.com"})
client.finalize_tabs([])
client.close()
```

## Go SDK Pattern

```go
package main

import (
	"fmt"
	"log"
	"time"

	obu "github.com/ifuryst/open-browser-use/packages/open-browser-use-go"
)

func main() {
	browser, err := obu.ConnectActive(obu.Options{
		SessionID: "obu-issue-scan-20260510",
		Timeout:   20 * time.Second,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer browser.Close()
	defer browser.Client.FinalizeTabs(nil)

	if _, err := browser.Client.NameSession("Issue scan - OBU"); err != nil {
		log.Fatal(err)
	}
	tab, err := browser.NewTab()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := tab.Goto("https://example.com", obu.GotoOptions{
		WaitUntil: obu.LoadStateDOMContentLoaded,
		Timeout:   15 * time.Second,
	}); err != nil {
		log.Fatal(err)
	}
	title, err := tab.Title()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(title)
}
```

Use the low-level client when you need raw JSON-RPC/CDP calls:

```go
client := obu.NewClient(obu.Options{
	SocketPath: "/tmp/open-browser-use/example.sock",
	SessionID:  "obu-docs-scan-20260510",
})
defer client.Close()

tab, err := client.CreateTab()
if err != nil {
	log.Fatal(err)
}
tabID := int(tab.(map[string]any)["id"].(float64))
if _, err := client.ExecuteCDP(tabID, "Page.navigate", obu.Params{"url": "https://example.com"}); err != nil {
	log.Fatal(err)
}
_, _ = client.FinalizeTabs(nil)
```

## Core Methods

Common Browser Use JSON-RPC methods:

- `ping`
- `getInfo`
- `createTab`
- `getTabs`
- `getUserTabs`
- `getUserHistory`
- `claimUserTab`
- `finalizeTabs`
- `nameSession`
- `attach`
- `detach`
- `executeCdp`
- `webmcp_list_tools`
- `webmcp_invoke_tool`
- `moveMouse`
- `waitForFileChooser`
- `setFileChooserFiles`
- `waitForDownload`
- `downloadPath`
- `readClipboardText`
- `writeClipboardText`
- `readClipboard`
- `writeClipboard`
- `turnEnded`

CLI unrestricted call:

```sh
open-browser-use call --session-id "$OBU_SESSION_ID" --method getInfo --params '{}'
open-browser-use call --session-id "$OBU_SESSION_ID" --method executeCdp --params '{"target":{"tabId":123},"method":"Runtime.evaluate","commandParams":{"expression":"document.title"}}'
```

### History search

Use `queries` to cover several search terms in one `getUserHistory` request
instead of calling once per term. The existing `query` input still works
for a single term:

```sh
open-browser-use call \
  --session-id "$OBU_SESSION_ID" \
  --method getUserHistory \
  --params '{"queries":["design guide","api reference"],"limit":20}'
```

Results keep the `{ url, title?, dateVisited }` shape, are ordered by newest
visit with one entry per URL, and `limit` applies to the merged list.
Without `from`, the search covers all retained history. Invalid inputs fail
before Chrome is queried. The dedicated `history` CLI/MCP command takes a
single `query`; use `call` or an SDK history wrapper for multiple terms.

### WebMCP page tools

The Chrome route can discover and invoke tools registered by the current
top-level page through `document.modelContext`. If the browser does not expose
that API natively, the MAIN-world content script installs a small compatible
shim at `document_start` so page code can still call `registerTool`. Check
`getInfo` first. The tab capability list contains `{"id":"webmcp",...}` only
when the extension WebMCP gate and content-script registration are active.

The target tab must already belong to the current Open Browser Use session.
Create it with `open-tab`, or list user tabs and claim the chosen tab first.

List the current tool snapshot:

```sh
open-browser-use call \
  --session-id "$OBU_SESSION_ID" \
  --method webmcp_list_tools \
  --params '{"tabId":123}'
```

The result has this shape:

```json
{
  "tools": [
    {
      "name": "book_table",
      "registration_id": "opaque-snapshot-id",
      "title": "Book table",
      "description": "Book a table.",
      "input_schema": {
        "type": "object"
      },
      "annotations": {
        "readOnlyHint": false,
        "untrustedContentHint": true
      },
      "origin": "https://example.com",
      "pageUrl": "https://example.com/reservations"
    }
  ]
}
```

Invoke a tool from that exact snapshot:

```sh
open-browser-use call \
  --session-id "$OBU_SESSION_ID" \
  --method webmcp_invoke_tool \
  --params '{"tabId":123,"tool_name":"book_table","registration_id":"opaque-snapshot-id","input":{"partySize":4},"timeout_ms":10000}'
```

The response is `{"result":<json-value>}`. Treat `registration_id` as opaque.
List tools again after a `toolchange`, navigation, or stale-registration error.
Running `webmcp_list_tools` creates a new snapshot and invalidates IDs from the
previous snapshot.

The command names and result field names follow the WebMCP page tool
specification. Open Browser Use keeps its existing transport envelope with
integer Chrome `tabId`, `session_id`, and `turn_id`. It does not add a second
`browser_id` routing layer.

CLI action plan:

```sh
export OBU_SESSION_ID="obu-docs-scan-$(date +%Y%m%d%H%M%S)"
open-browser-use run --session-id "$OBU_SESSION_ID" -c '
name-session "Docs scan - OBU"
open-tab https://docs.browser-use.com
wait-load domcontentloaded
page-info
finalize-tabs []
'
```

The action plan format is intentionally small: one action per line, comments
with `#`, shell-like quotes, shared session/turn, and a default tab set by
`open-tab` or `claim-tab`. Supported actions include `ping`, `info`, `tabs`,
`user-tabs`, `history`, `name-session`, `open-tab`, `claim-tab`, `navigate`,
`wait-load`, `page-info`, `cdp`, `move-mouse`, `wait-file-chooser`,
`set-file-chooser-files`, `finalize-tabs`, `turn-ended`, and `call`.

## MCP Server

Use the stdio MCP server when the surrounding runtime supports local MCP tools:

```toml
[mcp_servers.open_browser_use]
command = "obu"
args = ["mcp", "--session-id", "obu-<task-or-conversation-id>"]
```

`obu mcp` speaks newline-delimited JSON-RPC on stdin/stdout. It handles
`initialize`, `ping`, `tools/list`, and `tools/call`, and exposes tools that
mirror the CLI action surface:

- `ping`, `info`, `tabs`, `user_tabs`, `history`
- `open_tab`, `claim_tab`, `navigate`, `wait_load`, `page_info`
- `cdp`, `move_mouse`, `wait_file_chooser`, `set_file_chooser_files`
- `name_session`, `finalize_tabs`, `turn_ended`, `call`, `run_action_plan`

Pass `--socket` or `--socket-dir` in the MCP `args` only when the runtime needs
an explicit Open Browser Use socket. Otherwise the server uses the same socket
discovery as the CLI. Pass a fresh `--session-id` for each agent task or
conversation.

SDK request escape hatch:

```ts
await browser.client.request("executeCdp", {
  target: { tabId: 123 },
  method: "Runtime.evaluate",
  commandParams: { expression: "document.title" },
});
```

```py
browser.client.request("executeCdp", {
    "target": {"tabId": 123},
    "method": "Runtime.evaluate",
    "commandParams": {"expression": "document.title"},
})
```

```go
_, err := browser.Client.Request("executeCdp", obu.Params{
	"target":        obu.Params{"tabId": 123},
	"method":        "Runtime.evaluate",
	"commandParams": obu.Params{"expression": "document.title"},
})
```

## User Tab Claiming

1. List open user tabs with `open-browser-use user-tabs --session-id "$OBU_SESSION_ID"` or SDK `getUserTabs`.
2. Select the tab from returned data using visible evidence: title, URL, recency, and group.
3. Claim it with `open-browser-use claim-tab --session-id "$OBU_SESSION_ID" --tab-id <id>` or SDK `claimUserTab` / `claim_user_tab` / `ClaimUserTab`.
4. Use the returned controllable tab for later commands.

Never invent or reuse stale tab ids.

## Tab Cleanup

Before ending browser work, finalize exactly once for the active session:

```sh
open-browser-use finalize-tabs --session-id "$OBU_SESSION_ID" --keep '[]'
```

Omit tabs by default. Keep a tab only when the user needs that live page after
the turn. Use `status: "deliverable"` for a user-facing output or requested
open page. Use `status: "handoff"` only when the task is still in progress and
the user or a later turn should continue from the current task group, such as a
page waiting for login, approval, payment, CAPTCHA, or other user input.

Treat finalization as the last Open Browser Use browser action of the turn. If
more browser work is needed, do it before finalizing, then finalize once with
the final tab disposition.

## File Chooser Pattern

1. Start waiting with `wait-file-chooser --tab-id <id>` or SDK `waitForFileChooser` / `wait_for_file_chooser` / `WaitForFileChooser`.
2. Trigger the file picker in the page, usually through a click driven by CDP or a higher-level automation layer.
3. Set absolute file paths:

```sh
open-browser-use set-file-chooser-files --file-chooser-id <id> --file /absolute/path/file.txt
```

Use repeated `--file` values or comma-separated paths for multiple files.
