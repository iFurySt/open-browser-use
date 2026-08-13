# Open Browser Use CLI

This npm package installs the `open-browser-use` native host and CLI binary.
It also exposes the short `obu` command.

The CLI is useful when your agent runtime is shell-first or when you want a
small action plan instead of a JavaScript/Python SDK script.

## Install

```sh
npm install -g open-browser-use
obu version
```

The package contains prebuilt Go binaries for macOS, Linux, and Windows on
`amd64` and `arm64`.

On Windows, run setup from PowerShell or cmd after the npm install:

```powershell
open-browser-use setup
open-browser-use info
```

## Setup

Run `open-browser-use` with no subcommand to print the CLI version, browser
extension status, extension version when available, and the next setup or
upgrade command.

After installation, run setup to register the Chrome native messaging host and
open the Chrome Web Store page for the matching extension:

```sh
open-browser-use setup
```

Install or enable Open Browser Use from that page, then restart Chrome if
Chrome asks or the extension does not appear immediately.

If the Chrome Web Store item is temporarily unavailable, use the latest GitHub
Release zip as an unpacked extension instead:

```sh
open-browser-use setup beta
```

That command opens `chrome://extensions/` and reveals a keyed release ZIP so
the user can drag it into Chrome with a stable fallback extension id.

Verify the browser connection:

```sh
open-browser-use ping
open-browser-use info
```

## One-Shot Commands

Use direct subcommands for simple inspection and single browser actions:

```sh
open-browser-use tabs
open-browser-use user-tabs
open-browser-use history --query "browser use" --limit 20

open-browser-use name-session --name "Docs scan - OBU"
open-browser-use open-tab --url https://docs.browser-use.com
open-browser-use navigate --tab-id <tab-id> --url https://github.com/iFurySt/open-browser-use
open-browser-use cdp --tab-id <tab-id> --method Runtime.evaluate --params '{"expression":"document.title","returnByValue":true}'
open-browser-use finalize-tabs --keep '[]'
```

Use `--socket <path>` when a runtime gives you a specific Open Browser Use
socket. Otherwise the CLI discovers `/tmp/open-browser-use/active.json`.
Direct subcommands and `run` use the same default browser session,
`obu-cli`, so a final `open-browser-use finalize-tabs --keep '[]'` cleans up
tabs opened by either style. Pass `--session-id <id>` only when you intentionally
want a separate tab group and cleanup scope.

## Action Plans

Use `run` when you want CLI-level orchestration without writing JS or Python.
The format is intentionally small: one action per line, optional `#` comments,
shell-like quotes, shared session/turn, and a default tab set by `open-tab` or
`claim-tab`.

```sh
open-browser-use run -c '
name-session "Docs scan - OBU"
open-tab https://docs.browser-use.com
wait-load domcontentloaded
page-info
finalize-tabs []
'
```

You can also load the same action plan from a file:

```sh
open-browser-use run --file ./docs-scan.obu
```

Supported actions:

- Session/info: `ping`, `info`, `tabs`, `user-tabs`, `active-tab`, `focus-state`, `turn-ended`
- Browser tabs: `open-tab`, `claim-tab`, `claim-status`, `close-tab`, `navigate`, `back`, `forward`, `wait-load`, `page-info`
- Browser methods: `history`, `scroll`, `extract`, `wait-for`, `click-element`, `type-into`, `select-option`, `screenshot`, `cdp`, `call`
- Input/files: `move-mouse`, `wait-file-chooser`, `set-file-chooser-files`
- Cleanup: `finalize-tabs`

Example with explicit CDP and default tab reuse:

```sh
open-browser-use run -c '
name-session "GitHub issue scan - OBU"
open-tab https://github.com/iFurySt/open-browser-use/issues
wait-load domcontentloaded
cdp Runtime.evaluate "{\"expression\":\"document.body.innerText.slice(0, 1000)\",\"returnByValue\":true}"
finalize-tabs []
'
```

Example claiming an existing user tab:

```sh
open-browser-use run -c '
claim-tab <tab-id>
page-info
finalize-tabs [{"tabId":<tab-id>,"status":"handoff"}]
'
```

## MCP Server

Use `mcp` when an agent runtime supports local MCP servers over stdio:

```sh
obu mcp
```

For Codex, add a server entry similar to this in `~/.codex/config.toml`:

```toml
[mcp_servers.open_browser_use]
command = "obu"
args = ["mcp"]
```

The MCP server exposes browser tools such as `user_tabs`, `open_tab`,
`claim_tab`, `claim_status`, `active_tab`, `navigate`, `back`, `forward`,
`wait_load`, `page_info`, `scroll`, `extract`, `wait_for`, `click_element`,
`type_into`, `select_option`, `screenshot`, `focus_state`, `close_tab`, `cdp`,
`history`, `reconcile_operation`, `run_action_plan`, `finalize_tabs`, and unrestricted `call`. It uses the same
socket discovery as the CLI; pass `--socket <path>` or `--socket-dir <dir>` in
`args` only when the runtime needs an explicit socket.

If a command times out and reports an operation id with `outcome=unknown`, do
not replay it. Run `obu reconcile-operation --operation-id <id>` first.

## Low-Level JSON-RPC

Use `call` when no convenience command or action exists:

```sh
open-browser-use call --method getInfo --params '{}'
open-browser-use call --method executeCdp --params '{"target":{"tabId":123},"method":"Runtime.evaluate","commandParams":{"expression":"document.title","returnByValue":true}}'
```

## Cleanup

End browser work by finalizing tabs:

```sh
open-browser-use finalize-tabs --keep '[]'
```

Keep a tab only when the live tab is the deliverable or the user should continue
from that state:

```sh
open-browser-use finalize-tabs --keep '[{"tabId":123,"status":"handoff"}]'
```
