# Agent CLI and MCP commands

All CLI examples below assume an installed executable (`obu` may be replaced by the workspace-local wrapper), a chosen browser/profile, and a task session. Pass those route flags on **every** command. MCP gets them at startup or from its one-time `select_browser` tool.

```sh
obu profiles --connected --json
obu capabilities --browser zen --profile '<profile-directory>' --session-id '<task-id>'
obu wait-for --browser zen --profile '<profile-directory>' --session-id '<task-id>' --tab-id 123 --selector '#message' --url-includes '/application' --timeout 20s
obu snapshot --browser zen --profile '<profile-directory>' --session-id '<task-id>' --tab-id 123 --max-chars 12000 --max-elements 80
obu fill --browser zen --profile '<profile-directory>' --session-id '<task-id>' --tab-id 123 --selector '#message' --text-file /absolute/path/draft.txt --expected-value ''
```

Use a returned tab id, not the illustrative `123`. Direct CLI tab commands require `--tab-id`. MCP `tab_id` is optional after `open_tab`/`claim_tab` set its default; otherwise it is required and positive.

| CLI command | MCP tool | Parameters beyond route/session/tab |
| --- | --- | --- |
| `capabilities` | `capabilities` | None; backend-derived support, not a permission probe. |
| `profiles --connected --json` | `connected_profiles` | CLI returns installed rows plus unresolved connected hosts. MCP returns `{installed, connected}`. |
| MCP only | `select_browser` | `browser` and/or `profile`; optional `session_id`. Only before tab work and only on an unselected server. An unresolved host can use its instance id as `profile`. |
| `snapshot` | `snapshot` | CLI `--max-chars`, `--max-elements`; MCP `max_chars`, `max_elements`. Defaults 12000 and 80; ranges 1–50000 and 1–200. |
| `evaluate` | `evaluate` | CLI exactly one `--expression` or `--expression-file`; MCP `expression`. Return JSON-serializable data. |
| `click` | `click` | `--selector` / `selector`: unique visible, enabled CSS target. May submit or mutate the site. |
| `fill` | `fill` | CLI single-field flags or JSON fields array; MCP `fields` array. See below. |
| `wait-for` | `wait_for` | `--selector`, `--text`, `--url-includes`; MCP `selector`, `text`, `url_includes`. All supplied conditions must match. CLI `--timeout` (duration); MCP `timeout_ms` (integer). Positive, at most 60s; default 10s. |
| `set-input-files` | `set_input_files` | CLI `--selector`, repeatable `--file`, optional `--replace`; MCP `selector`, `files` absolute path array, `replace` boolean. |
| `page-info` | `page_info` | Legacy unbounded page info, nested CDP response. Prefer `snapshot`. |
| `wait-load` | `wait_load` | CLI `--state load\|domcontentloaded`, MCP `state`. `load` default. CLI `--timeout`, MCP uses server request timeout. Prefer URL/selector readiness for navigation. |

## Form fields

MCP example (literal JSON arguments, no shell quoting):

```json
{
  "tab_id": 123,
  "fields": [
    {"selector": "#message", "text": "Hallo,\n\nich kann ab Mitte Oktober starten.", "expectedValue": ""},
    {"selector": "#available", "checked": true},
    {"selector": "#location", "text": "remote"}
  ]
}
```

Fields require distinct, nonempty selectors and exactly one of `text` or `checked`; 1–100 fields. `expectedValue` is camelCase, including an empty string. `text` replaces the complete value; for selects it is the option value, not the visible label. `checked` sets a checkbox/radio state rather than blindly toggling it; unchecking a radio is unsupported. Disabled, inert, hidden, read-only, password, and unsupported fields fail before any field changes. Unknown JSON field keys are rejected.

CLI accepts `--fields '<JSON-array>'` or `--fields-file /absolute/path/fields.json`. Alternatively provide `--selector` plus exactly one of `--text`, `--text-file`, or `--checked=true|false`, and optional `--expected-value`. Do not mix the array and single-field modes.

All fields are validated first. This is not an atomic transaction: a site rerender, event handler, or failed post-write check can interrupt later writes. Inspect the page after failures. Successful filling returns each actual value/state with `submitInvoked: false`. This means the helper never invoked submit; it does not promise that site handlers did not autosave or submit.

## Uploads

`set-input-files` reads 1–100 explicitly authorized absolute regular-file paths on the host. The aggregate limit is 524288 bytes, leaving room for base64 and JSON under the browser's native-messaging limit. It constructs files in the browser, dispatches input/change, and returns names, sizes, MIME types, and `submitInvoked: false`; it does not print base64 or file contents.

The selector must identify one visible, enabled `input[type=file]`. A single-file input accepts one file. Existing files are preserved unless `replace` is true. The input's change handler may upload immediately. This synthetic selection is suitable for Zen sites that consume ordinary File objects; it does not open or control an OS dialog, bypass site validation, or produce trusted user events.

## Responses and backend limits

New DOM CLI commands return `{"result": <plain-data>}`. MCP wraps this same object in its `structuredContent` and text content. `snapshot` returns title, URL, readiness, text, element metadata/CSS selectors, and truncation flags. It omits field values and hidden elements. `wait-for` returns `{matched:true,url}` on success and errors on timeout or JavaScript failure. `evaluate` awaits promises, reports exceptions, and returns null for JavaScript undefined; other nonserializable values error.

`capabilities` returns backend name/version, selected route, evaluation world, upload limit, and support flags. Zen's `domInspection`, `domInteraction`, and `fileInputUpload` require the installed extension's page-interaction permission; the capability report cannot inspect whether it has been granted. Its `fullCDP`, `nativeFileChooser`, `cdpNetwork`, `cdpScreenshot`, and `tabGroups` are false. Isolated USER_SCRIPT evaluation sees the DOM but not website JavaScript globals.

Helpers use CSS selectors in the main document. Cross-origin frames, shadow-root traversal, trusted events, and screenshots need another supported interface or user cooperation. The new helpers run in the shared host runner; they are not new extension wire methods or SDK convenience methods. Raw SDK CDP evaluation remains available where needed.
