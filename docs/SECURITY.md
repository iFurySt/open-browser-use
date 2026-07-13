# Security Policy

Open Browser Use connects automation clients to the user's real Chrome or Zen profile through a browser extension, Native Messaging, and a local native host. Treat it as local browser control infrastructure, not as a sandbox boundary.

## Supported Scope

Security fixes are expected to cover the current Chrome and Zen routes:

- MV3 Chrome extension under `apps/chrome-extension/`.
- Zen/Firefox manifest and compatibility adapter under `apps/zen-extension/`.
- Go native host and CLI under `cmd/open-browser-use/` and `internal/`.
- JavaScript and Python SDK packages under `packages/`.
- Installer, manifest, release, and skill documentation that affects how users enable browser control.

Experimental references, reverse-engineering notes, and archived planning material are useful context, but they are not independently supported interfaces unless they are wired into the current route.

## Reporting Vulnerabilities

If you find a security issue, do not open a public issue with exploit details. Report it privately to the maintainers first.

Please include:

- A short description of the impact.
- Affected component and version, commit, or release.
- Reproduction steps or proof of concept, with secrets and personal browsing data removed.
- Whether the issue requires local access, extension installation, a malicious web page, or a malicious automation client.

Do not include real credentials, private cookies, personal browsing history, or unredacted local paths in reports. Maintainers should coordinate disclosure timing before publishing technical details.

## Security Boundaries

The Chrome route intentionally exposes powerful browser automation primitives to local callers that can reach the Open Browser Use socket.

- The SDKs and CLI do not implement a Codex-style site policy, command allowlist, or user approval workflow.
- Upper-layer runtimes are responsible for deciding which sites, methods, downloads, clipboard operations, and destructive actions are allowed.
- The native host and extension are not a general-purpose sandbox for untrusted code.
- Any integration that accepts remote instructions must add its own authentication, authorization, audit logging, and confirmation policy before invoking Open Browser Use.

## Local Transport

The Go native host bridges Chrome Native Messaging stdio to a local Unix socket.

- The default socket registry is under `/tmp/open-browser-use/`.
- `active.json` is written with `0600` permissions.
- Socket files are created with the current user's local permissions.
- The CLI may scan the socket directory when the active registry is missing or stale, so callers should treat access to the user's local account as access to the browser automation channel.

Planned hardening includes client tokens, peer validation, safer stale-socket handling, and security-focused failure-path tests.

## Browser Extension Permissions

The MV3 extension uses high-privilege Chrome APIs because browser control requires them:

- `chrome.debugger` for CDP access.
- `tabs` and `tabGroups` for tab and session management.
- `history` for history lookup.
- `downloads` for download observation.
- Native Messaging to communicate with the local host.

Users must understand that installing and enabling an extension allows Open Browser Use to inspect and operate real tabs in its active profile. The Chrome native messaging host manifest must keep `allowed_origins` restricted to the expected Open Browser Use extension ID.

The Zen manifest omits unsupported `debugger` and `tabGroups` permissions. It
still has broad tab, history, download, scripting, storage, and Native Messaging
access. Its native manifest must keep `allowed_extensions` restricted to the
stable Open Browser Use Gecko id.

## Clipboard, Downloads, And Files

Clipboard, download, and file chooser features are sensitive.

- Clipboard helpers run through the controlled page context and should only be called for explicit user workflows.
- Download paths and file chooser paths can reveal private filesystem structure; do not log them unless needed, and redact them from shared reports.
- Integrations should confirm uploads, downloads, file selection, form submission, purchases, deletion, and external message sending before execution.

## Secrets And Data Handling

- Never commit credentials, cookies, API keys, browser profile data, native host manifests with private machine-specific paths, or captured user data.
- Keep examples synthetic and minimal.
- Redact local usernames, home directories, tokens, session IDs, and private browsing history from docs, history entries, release notes, and issue reports.
- Prefer environment variables or local config files ignored by Git for development secrets.

## Supply Chain

Repository-level dependency, SBOM, provenance, and release artifact expectations live in `docs/SUPPLY_CHAIN_SECURITY.md`. Security-sensitive dependency or release changes should update that document in the same change when expectations move.

## Maintainer Checklist

For security-affecting changes, verify:

- Manifest origins remain constrained to the intended extension.
- Native host install paths and generated manifests do not leak private local state.
- Socket files, registries, temp files, and release artifacts use the intended permissions.
- Tests cover authorization, stale transport state, and failure paths where practical.
- User-facing setup docs describe the real permissions and risks.
