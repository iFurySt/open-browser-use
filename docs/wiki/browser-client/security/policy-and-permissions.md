# Policy And Permissions

## Capability Checks

Commands can be blocked before dispatch when the selected backend does not
advertise the required capability.

Observed checks:

- Download or media-download commands require download capability.
- File chooser commands require file-upload capability.

The fixed Chrome fallback info marks `fileUploads: false`.

## Site Policy

For non-local `http` and `https` URLs, the client checks:

```text
https://chatgpt.com/backend-api/aura/site_status
```

Observed behavior:

- The cache key is the hostname with leading `www.` removed.
- The cache TTL is 1440 minutes.
- `feature_status.agent === true` means the site is blocked for agent use.

## User Origin Permission

For most origin-bearing commands, the client prompts through:

```text
globalThis.nodeRepl.createElicitation
```

Observed prompt metadata:

```text
connector_id: browser-use
persist: always
```

Local origins bypass the prompt:

- `localhost`
- `*.localhost`
- `127.0.0.1`
- `::1`

## No-Origin Commands

Some commands are explicitly treated as not needing an origin:

- `browser_user_open_tabs`
- `close_tab`
- `create_tab`
- `list_tabs`
- `name_session`
- `playwright_wait_for_timeout`
- `selected_tab`

`navigate_tab_url` is checked against the target URL. Most other tab-scoped
commands check the current tab URL before dispatch.

## WebMCP Page Tools

The Open Browser Use Chrome extension can expose page-defined WebMCP tools when
its WebMCP gate is active. This path has these boundaries:

- `webmcp_list_tools` and `webmcp_invoke_tool` only accept a tab that belongs to
  the active Open Browser Use session.
- The MAIN-world script uses a native `document.modelContext` when one exists.
  When the browser does not provide it, the script installs a top-level
  page-facing shim before site JavaScript runs so sites can call
  `registerTool`. The shim keeps registrations in that document only and
  supports AbortSignal unregister plus `toolchange`. The ISOLATED-world script
  only relays WebMCP request and response data to the extension.
- Content scripts run in the top-level document only. This implementation does
  not discover WebMCP tools from iframes.
- Each list call creates opaque registration IDs. Invocation requires the exact
  registration ID and tool name from that snapshot. A `toolchange` invalidates
  the snapshot.
- WebMCP tools are site-defined code running in the live signed-in page. Treat
  their descriptions, schemas, and results as untrusted site content.

This WebMCP slice does not add a per-site allowlist or blocklist UI. That policy
surface remains separate from the page bridge.
