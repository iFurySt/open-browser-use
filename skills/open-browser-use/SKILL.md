---
name: open-browser-use
description: Operate and troubleshoot Open Browser Use in real Chrome or Zen Browser profiles through its local CLI, MCP server, or SDKs. Use for user tab claiming, DOM snapshots, guarded form drafts, JavaScript evaluation, navigation, file inputs, downloads, clipboard, session cleanup, and the custom Zen/Firefox compatibility fork.
---

# Open Browser Use

Use the real browser profile chosen for this task. Chrome has full CDP; Zen uses a Firefox compatibility adapter. Inspect capabilities before choosing an interaction method.

## Start with a route and a session

1. Read workspace instructions for the executable path. Prefer a project-local wrapper when specified; do not assume `obu` is on PATH or replace a stable native-host binary with `go run`.
2. Run `profiles --connected --json` before `info`, `ping`, or tab commands. An unselected command can connect to a different host through `active.json`. Choose the user's requested browser/profile, or the single suitable connected target. Ask only when the target is ambiguous. A missing `connected` flag is not definitive: retry the listing or verify the chosen route with `info` or `user-tabs`.
3. Pass the same explicit `--browser` and `--profile` to every CLI command. Profile selectors accept directory, display name, or extension instance id. An unresolved host is listed with `connected`, `instanceId`, and `socketPath`; it can be selected by `--profile <instanceId>`. Never fall back to a different browser when the selected route fails.
4. Choose a task-unique `--session-id`, reused throughout this task. Do not use the fallback `obu-cli` session for agent work.
5. Run `info` and `capabilities` on that route. Capabilities describe the backend; they do not prove Zen's page-interaction permission has been granted. If setup is missing, read [installation.md](references/installation.md).
6. Name the session `<short task> - OBU`, then inspect `user-tabs`. Claim an existing matching user, deliverable, handoff, or dev-server tab; open a new tab only when no matching tab exists. Never guess tab ids or claim unrelated tabs.

For MCP, supply `--browser`, `--profile`, and `--session-id` at server startup. An unselected server can instead call `connected_profiles`, then `select_browser` **once before tab work**. The selected route/session stays pinned. Start another server for another task or profile. Tools do not accept route flags individually. Restart an already running MCP server after updating the binary to refresh its tool catalog.

## Inspect and act

- After navigation, use `wait-for` with the intended URL and a visible selector. `wait-load` alone may observe the previous document; readiness does not prove an application has rendered. Wait for `document.body`/a relevant selector before DOM access.
- Use `snapshot` for bounded visible text and CSS selectors. It omits input values. Inspect the relevant field explicitly with `evaluate` when preserving a draft requires its current value.
- Use `fill` with `expectedValue` for text you expect to replace. It validates all targets before any changes and uses native setters plus `input`/`change` events. It handles text inputs, textareas, selects, and checkbox/radio states. It does not click submit or press Enter; website handlers may autosave.
- Use `click` only for an authorized action. A selector must match exactly one visible, enabled element. Reinspect after a rerender; selectors can become stale.
- Prefer `--text-file`, `--fields-file`, or `--expression-file` for multiline content. Keep prose out of shell command substitution. MCP accepts literal strings and arrays directly.
- Use `evaluate` for DOM checks or operations without a focused helper. It awaits promises, returns JSON data, and surfaces JavaScript exceptions. Zen evaluates in an isolated `USER_SCRIPT` world: DOM is accessible, page JavaScript globals are not.
- DOM helpers address the main document with CSS selectors. They do not provide cross-origin iframe traversal, trusted input events, arbitrary shadow-root locators, or native screenshots.

Read [agent-commands.md](references/agent-commands.md) for exact CLI/MCP parameters, result shapes, form guards, and upload limits. New DOM helpers are CLI/MCP conveniences, not new extension JSON-RPC methods or SDK methods.

## Authorization and existing work

Honor authorization already supplied by the user. Do not ask again for an action they explicitly requested. Read-only inspection, reversible draft preparation, and authorized implementation can proceed. Ask for a missing scope decision or an externally visible action that the user has not authorized.

If asked to fill without sending, leave submit buttons and Enter untouched. Keep those live draft tabs as deliverables for review. Do not replace unexpected text or existing file selections without authorization. Password fields are excluded from `fill`; use an explicitly authorized login flow. If login, CAPTCHA, hardware keys, or a browser permission prompt needs human interaction, preserve the tab and request that specific step.

Do not inspect unrelated cookies, credentials, session stores, or clipboard data. Upload only explicitly authorized files. A file input's change handler may upload immediately even without submitting the form.

## Zen file inputs and Chrome-only features

Zen does not support the intercepted native file chooser, network CDP methods, or CDP screenshots. Use `set-input-files` to assign authorized files to a visible input using `File`/`DataTransfer` events: at most 100 files and 512 KiB combined. Existing selections require explicit `--replace`. Larger files or sites requiring trusted file selection need user interaction in Zen. Chrome can use the native chooser flow documented in [sdk-and-protocol.md](references/sdk-and-protocol.md).

For unsupported operations, inspect `capabilities` and the error. Do not repeatedly try Chrome-only CDP calls in Zen or silently switch browsers.

## End the task cleanly

Finalization is the last browser action. Finalize the same explicit route/session:

- Keep no tabs by default: `finalize-tabs --keep '[]'`.
- Keep `status: "deliverable"` when the live page is the requested output, including unsent drafts the user wants to review.
- Keep `status: "handoff"` when work awaits login, input, approval, or another later step.
- Omit temporary fixtures, research, duplicate, blank, and error tabs. Claim and reuse matching kept tabs on a related follow-up.

Zen tracks session membership without Chrome tab groups; absence of a visual group does not imply missing ownership. A temporary unsigned XPI disappears on browser restart; reload it and check page-interaction permission if needed.

## Other interfaces and troubleshooting

`run -c/--file` and MCP `run_action_plan` execute the existing line-oriented action surface with a shared turn and current tab. Its `page-info` and `wait-load` actions also have direct CLI commands now. Use focused CLI/MCP calls for the new DOM helpers; do not assume every direct subcommand is a line-runner action.

For Chrome page-provided WebMCP tools, inspect `info` for the `webmcp` capability, list tools before invoking an authorized one, and use the returned registration id. The installed Zen backend does not offer this Chrome bridge. Exact wire parameters are in [sdk-and-protocol.md](references/sdk-and-protocol.md).

Use SDKs for event subscriptions, downloads, and larger workflows; raw `cdp`/`call` only when no focused helper fits. See [sdk-and-protocol.md](references/sdk-and-protocol.md). For socket or permission failures, read [troubleshooting.md](references/troubleshooting.md). Preserve stable native-host manifest paths and existing browser installs during repairs.
