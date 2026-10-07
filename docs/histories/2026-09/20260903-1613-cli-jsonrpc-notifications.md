## [2026-09-03 16:13] | Task: 修复 CLI JSON-RPC 通知插队

### 🤖 Execution Context

- **Agent ID**: `Codex`
- **Base Model**: `GPT-5`
- **Runtime**: `Codex desktop`

### 📥 User Query

> 修复 Open Browser Use CLI 偶发把 heartbeat 通知当作截图等命令响应的问题，并提交 PR。

### 🛠 Changes Overview

**Scope:** CLI JSON-RPC client

**Key Actions:**

- **响应匹配**: CLI 和 MCP 共用的请求路径按 JSON-RPC request id 等待对应响应，跳过插队通知。
- **回归覆盖**: 增加 heartbeat 先到、真实响应后到的 socket 协议测试。
- **文档同步**: 更新架构说明和功能发布记录。

### 🧠 Design Intent (Why)

Chrome extension 会周期性发送 heartbeat。native host relay 允许通知和请求响应在同一 socket 上交错，
因此客户端必须按 request id 完成响应匹配，不能把读取到的第一条消息直接作为结果返回。

### 📁 Files Modified

- `cmd/open-browser-use/main.go`
- `cmd/open-browser-use/main_test.go`
- `docs/ARCHITECTURE.md`
- `docs/releases/feature-release-notes.md`
