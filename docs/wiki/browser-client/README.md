# Browser Client Wiki

This area tracks the reverse-engineered behavior of the bundled Browser Use
client runtime.

## Artifact

- Source artifact:
  `~/.codex/plugins/cache/openai-bundled/browser-use/0.1.0-alpha1/scripts/browser-client.mjs`
- SHA-256:
  `20c5e21bc0c2f5532b6730a86a2556300fd8869b6335ff27ddac32ae33d48972`
- Public export: `setupAtlasRuntime`
- Generated metadata: `docs/generated/browser-client-metadata.json`
- First full reference note:
  `docs/references/browser-client-reverse-engineering.md`

## Map

- `runtime/backend-discovery.md`: Chrome vs IAB selection and socket discovery.
- `runtime/chrome-extension-architecture.md`: non-IAB Chrome extension backend
  path, native host expectations, tab ownership, and local installation
  observations.
- `runtime/iab-architecture.md`: Codex in-app browser routing, Electron
  `webview`/`webContents`, logical tab mapping, and storage partitioning.
- `runtime/transport-rpc.md`: native pipe framing, JSON-RPC, and backend API.
- `automation/automation-stack.md`: CDP, Playwright injected selectors, and CUA.
- `automation/command-surface.md`: observed command handler registry by area.
- `security/policy-and-permissions.md`: capability checks, site policy, and
  user-origin permission prompts.
- `notes/open-questions.md`: unresolved or evidence-light questions for future
  analysis.

## Working Model

The client is a Codex runtime bridge. It discovers a local Browser Use backend,
connects over a native pipe, wraps the connection as session-scoped JSON-RPC,
checks command permissions, and dispatches Atlas browser commands through CDP,
page JavaScript, or backend-specific helpers.

The default backend is Chrome extension mode. IAB mode is selected only when the
runtime asks for `backend: "iab"`.
