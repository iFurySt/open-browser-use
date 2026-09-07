# ChatGPT Chrome extension review: September 2026

## Evidence

Inspected on 2026-09-07 with `browser-use` through Bruksrom.

| Item | Observed value |
| --- | --- |
| Extension ID | `hehggadaopoacecdllhhajmbjkdcmajg` |
| Installed name | `ChatGPT` |
| Installed version | `1.26.901.11451` |
| Manifest version | `3` |
| Minimum Chrome version | `116` |
| Service worker | `background.js` |
| Service worker size | 355,753 UTF-8 bytes |
| Service worker SHA-256 | `83b7b7708ab82ecc9aa2ccad20a2167941a706b38a9467dee0bb124d13f0033e` |
| Repository baseline | [Codex 1.1.4 snapshot](../../../references/codex-chrome-extension-1.1.4/README.md) |

Chrome reported the extension as enabled with no manifest or runtime errors.
Two running service-worker targets had the same version and source hash.
This describes the installed build, not the latest release in every store.

Metadata was read from the live extension. The behavior below was traced in
its service-worker source, fetched from its own extension URL. Backend methods,
feature gates, side-panel flows, and native host internals were not exercised.
No browser history, cookies, credentials, or profile storage was used as test
data. The new implementation uses synthetic history records.

## What changed since 1.1.4

The Native Messaging route remains present. The bundle names
`com.openai.codexextension` plus development and internal host variants. It
also declares required native-host and app-server protocol versions of `2`.

The extension now has a side panel, toolbar actions, keyboard commands, and
context menus. Its manifest includes new surfaces such as `bookmarks`,
`notifications`, `sessions`, `topSites`, `webNavigation`, `sidePanel`, and
`declarativeNetRequestWithHostAccess`. Permission presence alone does not prove
that a feature is enabled for an account.

Session control has become more explicit. The source tracks tab leases and
active turn IDs, serializes lifecycle work, checks source leases before
claiming child tabs, and checks clients during heartbeat cleanup. The current
Open Browser Use backend mainly uses persisted tab-group membership.

## Capability comparison

The installed-source column describes static code paths. The Open Browser Use
column includes the history change made in this review.

| Capability | Installed source | Open Browser Use | Follow-up |
| --- | --- | --- | --- |
| History search | `getUserHistory` accepts `queries`, searches each term, sorts by visit time, removes duplicate URLs, and applies one limit. It sets `startTime` to zero unless `from` is given. | Implemented here. The existing `query` alias still works. | Ready for extension release validation. |
| Tab search | The tab mention path accepts `query`, `limit`, and `includeFavicons`. It limits results to 100, excludes incognito tabs, and filters URL protocols. | `getUserTabs` returns up to 1,000 tabs by recency. | Add explicit search options while preserving the existing listing contract. |
| Child tabs | `handleCreatedNavigationTarget` finds the opener's active lease. `claimChildTab` checks the source lease and turn before claiming and grouping the new tab. | No navigation-target listener or automatic child-tab claim path. | High value for flows that open another tab. Add lifecycle tests before adding `webNavigation`. |
| Debugger targets | `attachTarget` and `detachTarget` track target IDs against tabs. `executeCdp` sends a target-only debuggee when `targetId` is present. | Public attachment methods accept tab IDs. | Add target ownership, event routing, and target cleanup together. |
| CDP timeout recovery | A typed timeout identifies the failed command. The session detaches the tab unless `preserveDebuggerOnTimeout` is true. | The timeout helper reports an error but leaves the attachment state set. | Update SDK attachment caches with backend recovery; do not replay failed actions. |
| Viewport control | `setViewport` and clear operations reach CDP device-metrics commands and are tied to attachment state. | Raw CDP can set metrics, but no managed viewport command exists. | Use existing CDP support until a managed lifecycle is needed. |
| Browser management | Methods include `getBookmarks`, `getTopSites`, `getRecentlyClosedSessions`, and `createNotification`. A separate browser-management path has a feature gate and an audit trail. | These high-level methods and permissions are absent. | Choose specific product needs before expanding permissions. |
| WebMCP | Dynamic MAIN and ISOLATED scripts, with a runtime gate. | Both script layers and a local enable setting already exist. | Keep the existing implementation; compare further only for a concrete gap. |
| Side panel | A dedicated side-panel page and website integration are in the manifest and worker. | A connection-status popup. | Separate UI work from the browser automation protocol. |

## Why history was selected

History search can reuse the current handler, the existing `history` permission,
and all three SDK parameter maps. It needs no new dependency, native-host
change, permission, or SDK wrapper.

The previous handler omitted `startTime` when `from` was absent. Chrome then
limited the search to the last 24 hours. The new handler uses zero as the
default start time, so older retained pages can match.
[Chrome history API reference](https://developer.chrome.com/docs/extensions/reference/api/history#method-search).

The debugger recovery path needs more work than a local detach call. The
JavaScript, Python, and Go high-level CDP clients, plus the browser-client
rewrite, each cache attached tab IDs. Recovery must invalidate those caches
or establish a safe reattach path. A failed input action must not be replayed
because it might already have changed the page.

## Implemented history contract

Call `getUserHistory` with the usual `session_id` and `turn_id` metadata.
CLI and SDK clients add that metadata.

| Field | Behavior |
| --- | --- |
| `queries` | Optional non-empty array of strings. It takes precedence over `query`. Duplicate terms run once. An empty string matches all pages. |
| `query` | Existing single-string alias. Used when `queries` is absent or null. |
| `limit` | Positive integer, default 100. Applied to the merged URL list. |
| `from` | Optional date string accepted by `Date.parse`. Default is the Unix epoch. |
| `to` | Optional date string. Must be on or after the start date. |

Results keep the existing `{ url, title?, dateVisited }` shape. They are ordered
by newest visit first. When searches return the same URL, its newest record
wins. Rows with missing or invalid timestamps are skipped. Invalid request
fields fail before a Chrome history search starts. A failed search rejects
the whole request, so a partial result cannot look complete.

This searches Chrome's retained URL summaries. It does not restore deleted
history or return every individual visit. Chrome receives `maxResults=limit`
for each distinct term. Search batches are held in memory; a queue or tighter
request limits may be needed if callers start sending large bulk query lists.

Use the existing unrestricted CLI call:

```sh
OBU_HISTORY_SESSION_ID="history-$(date +%Y%m%d%H%M%S)"
obu call --session-id "$OBU_HISTORY_SESSION_ID" \
  --method getUserHistory \
  --params '{"queries":["design guide","api reference"],"limit":20}'
obu finalize-tabs --session-id "$OBU_HISTORY_SESSION_ID" --keep '[]'
```

The SDK wrappers already accept this parameter shape:

```js
await client.getUserHistory({ queries: ["design guide", "api reference"], limit: 20 });
```

```python
client.get_user_history(queries=["design guide", "api reference"], limit=20)
```

```go
client.GetUserHistory(obu.Params{"queries": []string{"design guide", "api reference"}, "limit": 20})
```

The dedicated CLI and MCP `history` conveniences still accept a single
`query`. Use `call` or an SDK wrapper for multiple queries.

## Source anchors

These function names identify this exact minified bundle and can change on
the next build. Offsets are zero-based Python string positions, not byte
offsets.

| Anchor | Position | Finding |
| --- | --- | --- |
| First `async getUserTabs(` | 162166 | Tab mention query, result cap, favicon option, and profile identity. |
| `async getInfo(` | 178912 | Browser and tab capabilities, including runtime gates. |
| `async handleCreatedNavigationTarget(` | 180308 | Source-lease lookup before claiming a child tab. |
| `async function Os(` | 191610 | Serialized tab attachment and viewport setup. |
| `async function Zf(` | 191804 | Target attachment and target-to-tab tracking. |
| `async function Yf(` | 192728 | Removal of tab attachment state during timeout recovery. |
| `async function tg(` | 194354 | History query construction, merge, validation, and result conversion. |

To repeat the inspection, use `Target.getTargets` through `browser-use` and
select service workers whose URL contains the exact extension ID. Attach with
`Target.attachToTarget` and `flatten=True`. In that session, read
`chrome.runtime.getManifest()` and fetch
`chrome.runtime.getURL('background.js')`. Hash the returned UTF-8 source before
comparing anchors. Always detach the inspection session in a `finally` block.

## Validation

- The new regression file failed six of eight tests on the original handler.
- After the change, all eight history tests and all 17 extension test entries
  passed with `node --test apps/chrome-extension/*.test.mjs`.
- `node --check apps/chrome-extension/background.js` passed.
- Documentation, repository hygiene, and `git diff --check` passed.
- Fork PR preparation also passed `PYTHON=python3 make ci`, covering the full
  repository suite and package builds.
- The installed extension was inspected live. The changed handler was tested
  in the Node VM harness and has not been loaded into Chrome in this task.
