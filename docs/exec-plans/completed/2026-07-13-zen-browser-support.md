# Zen Browser Support

## Goal

Add a supported Firefox-family route for Zen Browser without regressing the
existing Chromium route.

## Scope

- Package a Zen-compatible Manifest V3 WebExtension with a stable Gecko add-on
  id and Firefox background-script configuration.
- Register the native messaging host using Firefox manifest syntax and OS
  locations when `--browser zen` is selected.
- Detect installed Zen profiles and route `--browser zen --profile ...` to a
  connected host.
- Keep the existing Browser Use JSON-RPC surface. Emulate the core CDP calls
  needed for navigation and JavaScript evaluation; report unsupported
  Chrome-only CDP domains explicitly.
- Document installation, capabilities, and release/history impact.

## Constraints and decisions

- Zen is Firefox-based. Firefox does not implement Chrome's WebExtension
  `debugger` API, so full CDP parity is impossible inside the current extension
  architecture.
- The Zen package reuses shared extension assets and runtime code, with a small
  compatibility layer loaded before the common background script.
- Firefox native manifests use `allowed_extensions`, not `allowed_origins`, and
  Firefox passes the manifest path/add-on id to a launched native host.
- Linux Flatpak profiles are detected, but host native messaging from inside a
  sandbox may still require Flatpak filesystem/host integration outside this
  repository's control.

## Verification

- `go test ./...`
- `node --test apps/chrome-extension/*.test.mjs apps/zen-extension/*.test.mjs scripts/*.test.mjs`
- `pnpm -r --if-present typecheck`
- `pnpm -r --if-present lint`
- Shell syntax, docs/repository hygiene, and `git diff --check`
- Zen packaging validation is wired into CI; no local package/build command was
  run because repository instructions reserve builds for explicit requests.

## Status

Completed on 2026-07-13.
