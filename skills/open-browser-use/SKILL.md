---
name: open-browser-use
description: Platform-neutral guidance for using Open Browser Use, the open-source Chrome automation stack for AI agents. Use when an agent needs to install, verify, troubleshoot, or operate Open Browser Use through its browser extension, native CLI, JavaScript SDK, Python SDK, Go SDK, or Browser Use style JSON-RPC methods; use for tasks involving real Chrome tabs, user tab claiming, CDP commands, downloads, file choosers, clipboard helpers, or session cleanup.
---

# Open Browser Use

## Overview

Open Browser Use connects an MV3 Chrome extension, a local native messaging host, a CLI, SDKs, and an optional stdio MCP server so agents can automate a real Chrome profile. It is not Codex.app-specific; adapt the commands, MCP config, and SDK examples to the agent runtime you are operating in.

## Core Workflow

1. Check setup with `open-browser-use ping` or `obu ping`. If it fails because setup is missing, read [references/installation.md](references/installation.md).
2. Pick the right browser/profile if multiple are installed. See "Browser and profile handling" below before issuing browser commands.
3. Choose a unique browser session id for the current agent task before opening or claiming tabs. Prefer the surrounding runtime's conversation/session id when available; otherwise create a short unique id such as `obu-<task-slug>-<timestamp>`. Reuse that same id for every Open Browser Use command in this task.
4. Name the current browser task group before opening or claiming tabs. Use a short task label followed by ` - OBU`; if no better task label is available, use `Task - OBU`.
5. Before opening a new tab, run `user-tabs` / `user_tabs` and check whether the task continues from an existing tab, including tabs in `✅ Open Browser Use` or an earlier `handoff` task group. If the URL/title/group clearly matches the current task, claim that tab and continue from it instead of opening a duplicate. When verifying or accepting a local change, this also covers a dev server tab the user already has open (for example `localhost`, `127.0.0.1`, or a `*.local` / `*.test` host): claim and reuse it rather than opening a second copy of the same app.
6. Use the CLI for simple inspection or one-shot actions: `info`, `tabs`, `user-tabs`, `history`, `open-tab`, `navigate`, `cdp`, and `call`.
7. Use `open-browser-use run` / `obu run` for CLI-level multi-step orchestration when a small line-oriented action plan is enough and writing SDK code would be unnecessary.
8. If the surrounding agent runtime supports local MCP servers, configure `obu mcp` and call the exposed browser tools directly. Use the `run_action_plan` MCP tool for the same line-oriented orchestration from MCP. Read [references/sdk-and-protocol.md](references/sdk-and-protocol.md).
9. Use the JavaScript, Python, or Go SDK for larger multi-step workflows, event subscriptions, richer control flow, or when the surrounding agent runtime already runs code. Read [references/sdk-and-protocol.md](references/sdk-and-protocol.md).
10. Before ending browser work, release or keep session tabs with `open-browser-use finalize-tabs --session-id "$OBU_SESSION_ID" --keep '<json-array>'`, the MCP `finalize_tabs` tool, or the SDK `finalizeTabs` / `finalize_tabs` / `FinalizeTabs` method.
11. If communication fails after setup, read [references/troubleshooting.md](references/troubleshooting.md).

## Operating Rules

- Treat the browser as the user's real Chrome profile. Do not inspect cookies, passwords, session stores, or unrelated browser data.
- Ask the user before installing the extension, opening Chrome for them, enabling extension permissions, uploading local files, reading/writing clipboard data, submitting forms, purchasing, deleting, sending, or making other externally visible changes.
- Do not assume Codex.app helpers, Node REPL globals, or a bundled plugin UI are available. Use the installed `open-browser-use` / `obu` CLI or the published SDKs.
- Do not guess tab ids. List tabs first, then use ids returned by `tabs`, `user-tabs`, `open-tab`, or SDK calls.
- Prefer `claim-tab` / `claimUserTab` for existing user tabs. Claiming should be based on the current `user-tabs` result and visible evidence such as URL, title, recency, or group.
- For follow-up tasks, inspect `user-tabs` before opening a tab and reuse a matching tab from `✅ Open Browser Use` or a previous handoff group. A deliverable tab can be claimed back into the new task session, worked on, and finalized as `deliverable` again when it remains the user-facing result. This keeps repeated work on the same page converged to one live tab.
- Do not claim unrelated deliverable tabs just because they are in `✅ Open Browser Use`. If several tabs plausibly match, prefer the most recent exact URL/title match; ask the user when the match is ambiguous.
- When verifying or accepting a local code change, prefer reusing a dev server tab the user already has open (matching host, port, and app, such as `localhost:<port>`) over opening a new one. Match on host, port, and path or title; if several plausibly match or the match is ambiguous, ask the user instead of opening a duplicate.
- Use `--socket` only when the user or runtime provides an explicit socket. Otherwise let the CLI and SDKs discover the active socket registry.
- Do not rely on the CLI fallback session `obu-cli` for agent tasks. Always pass a task-unique `--session-id` to CLI and MCP commands, or set `sessionId` / `session_id` / `SessionID` in SDK clients. The fallback exists for quick manual use and can reuse stale task groups across unrelated agent sessions.
- Direct CLI subcommands and `open-browser-use run` can share the same browser session only when they use the same explicit `--session-id`. Finalize that same session before ending browser work.
- Use `call --method <method> --params '<json>'` only when no safer convenience command or SDK wrapper exists.

## Browser and profile handling

Some users run several supported browsers (for example Google Chrome, Google
Chrome Beta, or BitBrowser) and may also have multiple profiles inside them. If
more than one browser/profile target has the Open Browser Use extension
installed, the agent must decide which target this task should operate on rather
than silently picking whatever window happens to be active.

1. Before any browser command, list installed browser/profile targets:

   ```sh
   open-browser-use profiles --connected
   ```

   Columns include `BROWSER`, `DIRECTORY` (stable profile id like `Default`,
   `Profile 1`), `DISPLAY NAME` (what the user sees in the browser avatar
   menu), `VERSION`, and `CONNECTED` (whether that target's host is currently
   reachable). JSON output is available via `--json` and includes a stable
   `target` such as `chrome:Default`, `chrome-beta:Default`, or
   `bitbrowser:<instance>:Default`.

2. If exactly one target is installed and connected, proceed without asking.
   If it is installed but not connected, ask the user to open Chrome on that
   browser/profile before running browser commands.

3. If multiple targets are installed and the user did not already specify
   which one to use, ask before the first browser command. List both directory
   name and display name plus the browser name so the user can recognize them,
   and include whether each target is connected.

4. If the chosen target is not connected, ask the user to open that browser and
   profile before retrying. Do not silently fall back to a different connected
   browser/profile.

5. After the user has chosen, pass `--browser <selector>` and, when needed,
   `--profile <selector>` to every CLI / MCP command for the rest of the task.
   Browser selectors accept ids such as `chrome`, `chrome-beta`, `bitbrowser`,
   browser display names, or a BitBrowser instance id. Profile selectors accept
   either the directory name (`Default`, `Profile 1`) or the display name
   (`Eva`, `cookiy.com`), case-insensitive. Do not switch browser/profile
   mid-task.

6. If `--browser` / `--profile` does not match any running host, the CLI prints
   which targets are currently connected. Ask the user to open the chosen
   browser/profile, then retry; do not silently fall back to a different target.

7. For MCP, lock the browser/profile at server start:

   ```toml
   [mcp_servers.open_browser_use]
   command = "obu"
   args = ["mcp", "--session-id", "obu-<task-id>", "--browser", "<browser>", "--profile", "<profile>"]
   ```

   Do not pass browser/profile as per-tool-call arguments — the MCP server
   applies the start-time selectors to every call.

8. Do not remember the user's browser/profile choice across unrelated tasks. A
   future task may belong to a different target; ask again rather than assuming.

## Common CLI Actions

```sh
export OBU_SESSION_ID="obu-docs-scan-$(date +%Y%m%d%H%M%S)"
open-browser-use ping --session-id "$OBU_SESSION_ID"
open-browser-use info --session-id "$OBU_SESSION_ID"
open-browser-use name-session --session-id "$OBU_SESSION_ID" --name "Task - OBU"
open-browser-use tabs --session-id "$OBU_SESSION_ID"
open-browser-use user-tabs --session-id "$OBU_SESSION_ID"
open-browser-use history --session-id "$OBU_SESSION_ID" --query "example" --limit 20
open-browser-use open-tab --session-id "$OBU_SESSION_ID" --url https://example.com
open-browser-use navigate --session-id "$OBU_SESSION_ID" --tab-id <tab-id> --url https://example.com
open-browser-use cdp --session-id "$OBU_SESSION_ID" --tab-id <tab-id> --method Runtime.evaluate --params '{"expression":"document.title"}'
open-browser-use finalize-tabs --session-id "$OBU_SESSION_ID" --keep '[]'
```

For CLI-level orchestration without writing SDK code, use a line-oriented
action plan:

```sh
open-browser-use run --session-id "$OBU_SESSION_ID" -c '
name-session "Docs scan - OBU"
open-tab https://docs.browser-use.com
wait-load domcontentloaded
page-info
finalize-tabs []
'
```

Each action line shares one session/turn. `open-tab` and `claim-tab` set the
default tab for later tab-scoped actions such as `wait-load`, `page-info`,
`navigate`, `cdp`, `move-mouse`, and `wait-file-chooser`.

Use `obu` as the short alias when available.

## MCP Usage

For runtimes that can launch local MCP servers over stdio, use:

```toml
[mcp_servers.open_browser_use]
command = "obu"
args = ["mcp", "--session-id", "obu-<task-or-conversation-id>"]
```

Use a fresh `--session-id` value per agent task or conversation. If the runtime
has a stable conversation/session id, derive the MCP `--session-id` from it.

The MCP server exposes tools including `user_tabs`, `open_tab`, `claim_tab`,
`navigate`, `wait_load`, `page_info`, `cdp`, `history`, `run_action_plan`,
`finalize_tabs`, and unrestricted `call`.

Use `run_action_plan` when the runtime wants to execute the same compact action
plan format available through `open-browser-use run` without shelling out for
each individual browser operation.

## Tab Lifecycle

- Session tabs are tabs Open Browser Use has created or claimed for the current agent workflow.
- Use one unique session id per agent task or conversation. Do not share the fallback `obu-cli` session across unrelated tasks.
- Task session groups should be named from the task, using the pattern `<short task> - OBU`. Use `Task - OBU` as the fallback name.
- At the start of a related follow-up task, list all user tabs and check `tabGroup`, `title`, and `url` before creating anything new. Claim an existing matching deliverable or handoff tab — or a local dev server page the user already has open — into the current session; only open a new tab when no suitable tab exists.
- Keep no tabs by default: `open-browser-use finalize-tabs --session-id "$OBU_SESSION_ID" --keep '[]'`.
- Keep a tab only when the user needs that live page after the turn. Omit research, source, search, intermediate, duplicate, blank, error, and login/navigation tabs after extracting what you need.
- Keep a tab with `status: "deliverable"` when the tab itself is the user-facing output or requested open page, such as a created or edited document, dashboard, checkout/cart, submitted form result, or a page the user explicitly asked to inspect directly.
- Keep a tab with `status: "handoff"` only when the task is still in progress and the user or a later turn should continue from the current task group, such as a page waiting for user input, login, approval, payment, CAPTCHA, or an unfinished workflow.
- Handoff tabs stay in the task session group. Deliverable tabs move to the shared `✅ Open Browser Use` tab group.
- Run finalization as the last Open Browser Use browser action for the turn. Do not call Open Browser Use browser tools after finalizing; if more browser work is needed, do it first and finalize once with the final tab disposition.

## File Choosers, Downloads, And Clipboard

- File uploads use the intercepted file chooser flow: start waiting, trigger the chooser in the page, then set absolute local paths with `set-file-chooser-files` or the SDK equivalent.
- Downloads can be observed with SDK notification handlers or Browser Use methods such as `waitForDownload` and `downloadPath`.
- Clipboard helpers operate through the current controlled tab and should be treated as sensitive user actions.

## References

- [references/installation.md](references/installation.md): one-time CLI and browser extension setup, including cases where user cooperation is required.
- [references/sdk-and-protocol.md](references/sdk-and-protocol.md): JavaScript, Python, Go, socket, and JSON-RPC usage details.
- [references/troubleshooting.md](references/troubleshooting.md): connection failures, stale sockets, extension/native host checks, and permission issues.
