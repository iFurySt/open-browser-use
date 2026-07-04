## [2026-07-04 17:00] | Task: 修复验收后调试横幅残留与本地开发页不复用

### 🤖 Execution Context

- **Agent ID**: `Claude Code`
- **Base Model**: `Opus 4.8`
- **Runtime**: `Claude Code CLI`

### 📥 User Query

> GitHub issue #12：验收完成后没有清理浏览器现场。评论补充两点——(1) 验收完成后浏览器顶部的"已开始调试此浏览器"横幅仍然存在；(2) 有时候不复用已经打开的本地开发页，而是重新又打开一个页面。要求用简单方案修复（先做 A + C）。

### 🛠 Changes Overview

**Scope:** `apps/chrome-extension`, `skills/open-browser-use`

**Key Actions:**

- **[Fix A — 调试横幅]**: `finalizeTabs` 现在把保留的 `handoff` 标签页也纳入 `detachMany`。之前只 detach `agent`/`user`/`deliverable` 三类，handoff 被故意跳过，导致 debugger 一直挂在被保留的任务组标签页上，Chrome 顶部的调试横幅不消失。改动保留标签页与任务分组本身，只断开 debugger。
- **[Fix C — 本地页复用]**: 在 skill 的 Core Workflow、Operating Rules、Tab Lifecycle 三处补充指导——验证/验收本地改动前，先从 `user-tabs` 找用户已经打开的本地开发页（`localhost` / `127.0.0.1` / `*.local` / `*.test`），claim 复用而不是新开重复标签页；匹配 host/port/path，歧义时问用户。
- **[回归测试]**: 新增 `apps/chrome-extension/finalize-detach.test.mjs`，用 stub 过的 `chrome.debugger` 验证 handoff 标签页 finalize 后被 detach 且仍保持打开与分组；已并入 `node --test` CI（`scripts/ci.sh`）。
- **[Release]**: patch bump 到 `0.1.41`（runtime / manifest / 各 SDK package / pyproject），新增 `2026-07` release note。

### 🧠 Design Intent (Why)

那条横幅是 Chrome 原生的 debugger infobar，唯一开关是"是否有标签页处于 `chrome.debugger.attach` 状态"，跟 profile、跟"标签页有没有关"都只是间接相关。关标签页能消横幅只是因为关闭时 Chrome 会自动 detach。验收场景通常想把页面留给用户看结果，所以正确的做法是"留标签页、断 debugger"，而不是强行关标签页。detach handoff 是无损的：MV3 service worker 随时会被回收，`attachedTabs` 只是内存态，且 CLI runner / 后台 handler 每次 CDP 前都会按需重新 attach，下一轮自动重连。Fix C 补齐了此前只覆盖 `✅ Open Browser Use` / handoff 分组、没覆盖用户自开本地开发页的复用盲区。

### 📁 Files Modified

- `apps/chrome-extension/background.js`
- `apps/chrome-extension/finalize-detach.test.mjs`
- `skills/open-browser-use/SKILL.md`
- `cmd/open-browser-use/main.go`
- `apps/chrome-extension/manifest.json`
- `packages/*/package.json`
- `packages/open-browser-use-python/pyproject.toml`
- `docs/releases/feature-release-notes.md`
