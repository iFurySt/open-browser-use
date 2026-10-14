# Browser Use Client Reverse Engineering

## Analyzed Artifact

- Path: `~/.codex/plugins/cache/openai-bundled/browser-use/0.1.0-alpha1/scripts/browser-client.mjs`
- Size: 817168 bytes
- Lines: 177 minified bundle lines
- SHA-256: `20c5e21bc0c2f5532b6730a86a2556300fd8869b6335ff27ddac32ae33d48972`
- Export: `setupAtlasRuntime`
- Reproducible metadata: `docs/generated/browser-client-metadata.json`
- Incremental wiki: `docs/wiki/browser-client/`
- Extraction command:

```bash
node scripts/extract-browser-client-metadata.mjs
```

The bundle has no sourcemap. I used `npx terser --format beautify=true`
against a temporary copy to inspect control flow, then preserved only curated
facts and a readable rewrite in this repository.

## Top-Level Shape

Confirmed facts:

- The public module export is `setupAtlasRuntime`.
- The default backend selector is `chrome`.
- The backend transport is JSON-RPC over a native pipe exposed through
  `import.meta.__codexNativePipe.createConnection` in the initial artifact,
  replaced by `globalThis.nodeRepl.nativePipe.createConnection` in the
  2026-10 build.
- The native pipe base is `codex-browser-use`.
- The IAB route uses `codex-browser-use-iab`.
- Each backend request is augmented with `session_id` and `turn_id` from
  `globalThis.nodeRepl.requestMeta["x-codex-turn-metadata"]`.
- Successful command use sets response metadata key `codex/browserUse`.
- Remote site policy is checked through
  `https://chatgpt.com/backend-api/aura/site_status`.
- Per-origin permission prompts use `nodeRepl.createElicitation` with
  `connector_id: "browser-use"` and `persist: "always"`.
- Playwright selector support injects a page global named
  `__codexPlaywrightInjected`.

Re-verified on 2026-10-12 against ChatGPT `26.1007.30912`. Changes since the
2026-04 snapshot:

- The plugin moved from `plugins/cache/openai-bundled/browser-use/0.1.0-alpha1/`
  to `plugins/cache/openai-bundled/browser/26.1007.30912/`, and the bulk of the
  runtime now lives in `scripts/browser-service.mjs` rather than
  `scripts/browser-client.mjs`.
- The app installs as `ChatGPT.app` while keeping bundle id `com.openai.codex`,
  package name `openai-codex-electron`, and product name `Codex`.
- The transport framing is unchanged: 4-byte native-endian length prefix plus a
  UTF-8 JSON-RPC payload, with `8 MiB` as the default max frame and `5 s` as the
  default `getInfo` timeout.
- On macOS prod builds, `native/browser-use-peer-authorization.node` now gates
  the accepted connection before RPC parsing, so an unauthorized process cannot
  complete `getInfo` against the sockets. This does not imply its OS-level
  `connect()` call always fails.

Static peer-authorization inspection on 2026-10-13 additionally found audit
token access, code-signing identity imports and process-ancestry helper symbols.
The exact native allowlist and validity checks remain partially unverified; see
the evidence boundaries in `docs/wiki/browser-client/runtime/iab-architecture.md`.

Inferred structure:

```text
setupAtlasRuntime({ globals, backend })
  -> discover backend pipe
  -> create JSON-RPC browser API
  -> create browser context wrappers
  -> install Atlas agent/display objects on globals
  -> dispatch Atlas commands by command.type
       -> capability checks
       -> site policy and origin permission checks
       -> command handler
       -> CDP / native backend RPC / page JavaScript
```

## Backend Discovery

The bundle supports two backend families:

- Chrome extension backend:
  - Requested when `backend` is omitted or is `"chrome"`.
  - Discovers backend info type `"extension"`.
  - Falls back to a well-known pipe path:
    - macOS/Linux: `/tmp/codex-browser-use.sock`
    - Windows: `\\.\pipe\codex-browser-use`
  - The fallback info object is named `Chrome`, version `0.0.1`, type
    `extension`, with `fileUploads: false`.
- In-app browser backend:
  - Requested as `"iab"`.
  - Discovers backend info type `"iab"`.
  - Lists candidates by reading the whole `/tmp/codex-browser-use/` directory
    (or `\\.\pipe\codex-browser-use*` on Windows) rather than a single
    well-known path. `BROWSER_USE_BACKEND_PATHS` can override the list.
  - Filters discovered IAB backends by `metadata.codexSessionId` matching the
    current Codex session ID, and additionally by
    `metadata.codexAppBuildFlavor` when a build flavor is set.
  - Reports discovery failures with explicit reasons: `missing-session-metadata`,
    `no-iab-backends`, or `no-session-match`.

The discovery pass also scans platform-specific `codex-browser-use*` pipe
paths and races each connection attempt against a 3000 ms timeout.

## Transport Protocol

The native pipe transport frames each JSON-RPC message as:

```text
uint32 message_length_in_native_endianness
utf8 JSON payload
```

The JSON-RPC layer supports:

- `sendRequest(method, params)` with incrementing numeric IDs.
- Notifications and event handlers for backend-originated events.
- Backend request handler registration, currently including a local `ping`
  handler that returns `pong`.

Browser API methods observed over the session RPC wrapper:

- `executeCdp`
- `attach`
- `detach`
- `getTabs`
- `getUserTabs`
- `claimUserTab`
- `createTab`
- `finalizeTabs`
- `nameSession`
- `moveMouse`
- `getInfo`

All session methods include `session_id` and `turn_id`.

## Security Gates

There are three command gates before dispatch:

1. Capability checks:
   - Download commands require `browserInfo.capabilities.downloads` or
     `mediaDownloads` not to be `false`.
   - File chooser commands require `browserInfo.capabilities.fileUploads` not
     to be `false`.
2. Site policy check:
   - Non-local `http` and `https` URLs are normalized to origin/display URL.
   - The cache key is the hostname with leading `www.` removed.
   - Cache TTL is 1440 minutes.
   - `feature_status.agent === true` means the site is blocked.
3. User origin permission:
   - Localhost, `*.localhost`, `127.0.0.1`, and `::1` bypass origin prompts.
   - Other `http` and `https` origins call `createElicitation`.
   - Commands without a URL or positive `tab_id` are treated as no-origin
     commands.

Commands explicitly treated as no-origin:

- `browser_user_open_tabs`
- `close_tab`
- `create_tab`
- `list_tabs`
- `name_session`
- `playwright_wait_for_timeout`
- `selected_tab`

`navigate_tab_url` is checked against its target URL. Most other tab-scoped
commands check the current tab URL before running.

## Runtime Context Wrappers

The bundle constructs a context object with these wrappers:

- `cdp`: attaches/detaches tabs, sends CDP commands, tracks file choosers,
  watches page load events, and normalizes navigation-blocked errors.
- `browserUser`: lists user tabs and claims a user tab. Claiming only works
  with the Chrome extension backend.
- `ui`: delegates mouse movement to the backend.
- `cua`: high-level computer-use actions over CDP input events.
- `dev`: enables Runtime events and keeps the last 500 console/exception logs
  per tab.
- `tabs`: create/list/get/active/finalize helpers. Finalize only works with the
  Chrome extension backend.
- `playwright`: injects Playwright's injected selector engine, resolves
  locators, reads element state, and executes locator actions.
- `security`: command-level policy and permission gate.

## Command Surface

The actual runtime command registry is `Object.values(Om)`. Metadata extraction
found these handler-backed command types:

- `browser_user_claim_tab`
- `browser_user_open_tabs`
- `close_tab`
- `create_tab`
- `cua_click`
- `cua_double_click`
- `cua_drag`
- `cua_get_visible_screenshot`
- `cua_keypress`
- `cua_move`
- `cua_scroll`
- `cua_type`
- `dom_cua_keypress`
- `finalize_tabs`
- `list_tabs`
- `name_session`
- `navigate_tab_back`
- `navigate_tab_forward`
- `navigate_tab_reload`
- `navigate_tab_url`
- `playwright_dom_snapshot`
- `playwright_file_chooser_set_files`
- `playwright_locator_click`
- `playwright_locator_count`
- `playwright_locator_dblclick`
- `playwright_locator_download_media`
- `playwright_locator_fill`
- `playwright_locator_get_attribute`
- `playwright_locator_inner_text`
- `playwright_locator_is_enabled`
- `playwright_locator_is_visible`
- `playwright_locator_press`
- `playwright_locator_select_option`
- `playwright_locator_set_checked`
- `playwright_locator_text_content`
- `playwright_locator_wait_for`
- `playwright_screenshot`
- `playwright_wait_for_file_chooser`
- `playwright_wait_for_load_state`
- `playwright_wait_for_timeout`
- `playwright_wait_for_url`
- `selected_tab`
- `tab_dev_logs`

The bundle also contains schema or API strings for additional commands such as
clipboard, Google Workspace export, media download, and tab content commands.
Those strings are present in the bundle, but they were not included in the
observed `Om` runtime registry extraction for this version.

## Playwright Selector Integration

The client does not depend on an external Playwright process. Instead it embeds
Playwright's injected selector script as a string and evaluates it in the page:

- Global page object: `window.__codexPlaywrightInjected`
- Selector parsing: `injected.parseSelector(selector)`
- Querying: `injected.querySelectorAll(parsed, document)`
- Strictness rule:
  - zero matches returns no element after deprecated-selector checks
  - one match succeeds
  - multiple matches are narrowed to one visible match if possible
  - otherwise strict mode violation is thrown

Locator clicks compute a center point in page JavaScript, then send mouse input
through CUA/CDP rather than directly invoking DOM click.

## Navigation Behavior

Navigation handlers use `Page.enable`, `Page.getNavigationHistory`,
`Page.navigate`, `Page.reload`, and `Page.navigateToHistoryEntry`.

Load completion is inferred from CDP events:

- Start-like events:
  - `Page.frameStartedLoading`
  - `Page.frameNavigated`
  - `Page.navigatedWithinDocument`
- Complete-like events:
  - `Page.domContentEventFired`
  - `Page.loadEventFired`
- Policy block:
  - `Page.navigationBlocked`

`playwright_wait_for_load_state` supports `load` and `domcontentloaded`, but
explicitly rejects `networkidle`.

## Reimplementation Notes

The readable rewrite in `packages/browser-client-rewrite/browser-client.mjs`
models the same outer contract and command architecture:

- same exported name: `setupAtlasRuntime`
- same backend selection model
- same native pipe framing
- same session metadata requirements
- same command gate ordering
- same handler-backed command type names

It intentionally does not vendor the bundled Statsig, Sentry, Zod, escodegen,
or full Playwright injected source. Those are bundle dependencies, not the
maintainable Browser Use client boundary.
