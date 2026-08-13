## [2026-08-13 17:39] | Task: Harden agent runtime

### 🤖 Execution Context

- **Agent ID**: `Codex primary agent`
- **Base Model**: `GPT-5`
- **Runtime**: `Synara Ream Chat`

### 📥 User Query

> 根据真实 Ream Chat 验收结果，修复并优化 Open Browser Use，使它能作为 Ream、Cursor 和其他 agent 的独立真实 Chrome runtime；保持背景执行，并补齐 agent 常用浏览动作。

### 🛠 Changes Overview

**Scope:** Chrome extension、native relay、Go CLI/MCP、JavaScript/Python/Go SDK、agent skill 和协议文档。

**Key Actions:**

- **Protocol reliability**: CLI 按 exact request id 读取 response，native relay 丢弃 orphan response；timeout 附 operation id，extension 持久化有界 operation outcome 并支持 reconciliation。
- **Session handoff**: `handoff` 保留 tab 和视觉分组但释放 exclusive ownership；claim、finalize 和 turn-ended 返回结构化 receipt，并增加 claim status。
- **Background safety**: 拒绝会 raise/activate/resize Chrome 的 raw CDP method，不调用系统鼠标；增加只读 focus state。
- **Agent tool surface**: MCP 和 action plan 增加 active tab、back/forward、scroll、extract、wait、click/type/select、screenshot 和 close-tab 等高阶动作。
- **Verification**: 增加 relay、CLI correlation、MCP extraction、handoff reclaim、operation replay 和 focus fence 回归测试，并通过完整 `make ci`。

### 🧠 Design Intent (Why)

真实 agent runtime 不能把通知或迟到回应用成命令结果，也不能在 timeout 后盲目重放可能已完成的 mutation。Handoff 必须是可验证的 ownership transfer，而不是只 detach debugger。常见网页操作应由稳定的高阶契约承载，同时 raw escape hatch 必须守住 background-only 边界。

### 📁 Files Modified

- `apps/chrome-extension/background.js`
- `internal/host/relay.go`
- `cmd/open-browser-use/main.go`
- `cmd/open-browser-use/mcp.go`
- `packages/open-browser-use-js/src/index.ts`
- `packages/open-browser-use-python/open_browser_use/client.py`
- `packages/open-browser-use-go/client.go`
- `skills/open-browser-use/`
- `docs/ARCHITECTURE.md`
