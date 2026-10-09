## [2026-10-09 19:17] | Task: Support local extension ZIP in setup

### 🤖 Execution Context

- **Agent ID**: `TRAE`
- **Base Model**: `GPT-5`
- **Runtime**: `TRAE CLI`

### 📥 User Query

> Allow `obu setup --zip /path/to/x.zip` without requiring the `beta` subcommand.

### 🛠 Changes Overview

**Scope:** Go CLI setup flow and installation guidance.

**Key Actions:**

- **[CLI]**: Added `--zip` to the main `setup` command and routed it through the existing keyed local-package preparation and native host registration flow.
- **[Compatibility]**: Kept `setup beta` download behavior and `setup beta --zip <path>` working through the same shared implementation.
- **[Tests and docs]**: Covered both command forms and documented ZIP-only input, in-place repackaging, and the remaining manual Chrome installation step.

### 🧠 Design Intent (Why)

A local artifact is an input choice, not a release-channel choice. Accepting it
on the primary setup command makes offline installation discoverable while
preserving the existing beta fallback for users and automation that already
depend on it.

### 📁 Files Modified

- `cmd/open-browser-use/main.go`
- `cmd/open-browser-use/main_test.go`
- `README.md`
- `README.zh-CN.md`
- `packages/open-browser-use-cli/README.md`
- `skills/open-browser-use/references/installation.md`
- `skills/open-browser-use/references/troubleshooting.md`
- `docs/ARCHITECTURE.md`
- `docs/CHROME_WEB_STORE_RELEASE.md`
- `docs/releases/feature-release-notes.md`
