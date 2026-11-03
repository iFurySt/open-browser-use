## [2026-07-16 15:37] | Task: Restrict tab claiming

### 🤖 Execution Context

- **Agent ID**: `Codex`
- **Base Model**: `GPT-5`
- **Runtime**: `Codex CLI`

### 📥 User Query

> 用户希望先通过 skill 约束解决 OBU 多次使用后残留和误 claim 用户正常标签页的问题：OBU 只能 claim 自己 group 里的 tab，包括 `✅ Open Browser Use`，不能 claim 未分组的用户 tab。

### 🛠 Changes Overview

**Scope:** `skills/open-browser-use`, `packages/open-browser-use-cli`, `cmd/open-browser-use`

**Key Actions:**

- **[Skill guidance]**: Restricted agent tab claiming guidance to OBU-owned groups only.
- **[Protocol reference]**: Clarified that `claimUserTab` should only be used on tabs in `✅ Open Browser Use` or task groups ending with ` - OBU`.
- **[CLI docs]**: Replaced the generic "existing user tab" claim example with an OBU-owned tab claim rule.
- **[Tool descriptions]**: Updated CLI help and MCP tool metadata so they no longer describe claiming arbitrary Chrome tabs.

### 🧠 Design Intent (Why)

普通用户标签页可能正被用户使用。即使 URL、title 或 localhost 端口匹配当前任务，agent 直接 claim 也会移动标签页并改变用户的浏览器上下文。先在 skill 和使用文档里把 claim 收敛到 OBU 已拥有的分组，可以降低 Codex 等 agent 误用风险；如果仍有绕过路径，再把同样规则下沉到 MCP 或 extension backend。

### 📁 Files Modified

- `skills/open-browser-use/SKILL.md`
- `skills/open-browser-use/references/sdk-and-protocol.md`
- `packages/open-browser-use-cli/README.md`
- `cmd/open-browser-use/main.go`
- `cmd/open-browser-use/mcp.go`
