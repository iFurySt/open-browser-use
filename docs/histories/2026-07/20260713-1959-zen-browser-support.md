## 2026-07-13 19:59 | Task: Add Zen Browser support

### 🤖 Execution Context

- **Agent ID**: `/root`
- **Base Model**: GPT-5
- **Runtime**: Codex

### 📥 User Query

> Add support for Firefox (Zen Browser).

### 🛠 Changes Overview

**Scope:** browser extensions, Go CLI/native messaging, profile routing, release packaging, docs

**Key Actions:**

- **[Zen extension]**: Added a Firefox MV3 manifest with a stable Gecko id and a
  compatibility adapter for navigation, reload/close, JavaScript evaluation,
  target listing, and browser version calls.
- **[Shared runtime]**: Made debugger and tab-group behavior capability-aware;
  Firefox sessions track tab membership in extension storage and unsupported
  Chrome-only operations return explicit errors.
- **[CLI]**: Added Firefox native launch detection, `allowed_extensions`
  manifests, Mozilla manifest paths/registry registration, Zen profile roots,
  `profiles.ini`/`extensions.json` discovery, and instance-to-profile lookup.
- **[Linux follow-up]**: Added the `~/.config/zen` profile root used by packaged
  Zen installations, alongside the existing tarball and Flatpak roots.
- **[Page interaction follow-up]**: Replaced CSP-blocked content-script `eval`
  with Firefox's isolated user-script world, using a registered bridge on
  Firefox 136–152 and direct `userScripts.execute` on Firefox 153+. Added a
  one-time global permission control and status to the popup, plus adapter tests
  for both execution paths, missing permission, and script errors.
- **[Release]**: Added Zen XPI packaging, CI validation, release evidence,
  provenance, and GitHub Release upload wiring.
- **[Docs]**: Documented installation, supported capabilities, limitations,
  architecture, frontend boundaries, quality status, and agent skill routing.

### 🧠 Design Intent (Why)

Zen is Firefox-based and Firefox does not implement Chrome's extension debugger
or tab-group APIs. A separate manifest plus a narrow compatibility adapter keeps
the existing protocol useful for core agent workflows while preserving Chrome's
full CDP path and making unsupported behavior honest and diagnosable. Firefox
MV3 also rejects dynamic `eval` in extension/content-script contexts, so dynamic
automation expressions run as user-script source in an isolated world after one
explicit global permission grant instead of weakening website CSP.

### 📁 Files Modified

- `apps/chrome-extension/background.js`
- `apps/zen-extension/manifest.json`
- `apps/zen-extension/firefox-compat.js`
- `apps/zen-extension/firefox-compat.test.mjs`
- `apps/chrome-extension/popup.html`
- `apps/chrome-extension/popup.js`
- `apps/chrome-extension/popup.css`
- `cmd/open-browser-use/main.go`
- `scripts/package-zen-extension.sh`
- `.github/workflows/release.yml`
- `docs/ARCHITECTURE.md`
- `skills/open-browser-use/references/installation.md`
