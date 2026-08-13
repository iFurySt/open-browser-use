# Agent-grade runtime hardening

## 目标

把真实 Ream Chat 验收暴露出的 OBU 协议、会话交接和 agent 工具短板收口成可测试的运行时契约，使 Ream、Cursor、Codex CLI 等上层 agent 可以在不抢前台焦点的前提下可靠地控制真实 Chrome。

## 范围

- 包含：JSON-RPC request/response 关联、迟到响应隔离、handoff ownership 释放、claim/finalize receipts、操作状态 reconciliation、背景焦点保护，以及常用高阶 MCP/CLI actions。
- 包含：extension、native relay、Go CLI/MCP、SDK 契约、回归测试和使用文档。
- 不包含：登录态网站测试、站点级授权策略、OCU 桌面控制、Ream 内部 workaround、发布到 npm/Chrome Web Store。

## 背景

- 相关文档：`docs/ARCHITECTURE.md`、`docs/RELIABILITY.md`、`docs/SECURITY.md`。
- 相关代码路径：`apps/chrome-extension/background.js`、`internal/host/relay.go`、`cmd/open-browser-use/`、`packages/open-browser-use-*`。
- 已知约束：真实 Chrome 操作必须保持背景执行；session tab 只能处理本 session 的 ownership；未知结果不得静默重试。

## 风险

- 变更 handoff 语义可能影响既有 session group 恢复。通过保留 tab/group 但删除 ownership record，并加入跨 session reclaim 回归测试缓解。
- 迟到响应可能在 client timeout 后到达。relay 只丢弃已失去 owner 的 `obu:*` response，并继续广播真正的 notification。
- 焦点状态可能同时被用户改变。保护逻辑只检测 OBU 操作自身造成的变化，绝不覆盖操作期间观察到的人类干预。
- 高阶动作可能扩大 mutation surface。默认提供确定性的 DOM/CDP primitive，不绕过上层审批或站点策略。

## 里程碑

1. 修复关联与迟到响应隔离，补协议回归测试。
2. 修复 handoff ownership 和结构化 receipts，补 session 回归测试。
3. 增加 operation reconciliation、背景焦点保护及高阶工具。
4. 完成 SDK/文档同步、聚焦测试与真实 Chrome 验收。

## 验证方式

- 命令：`go test ./...`、extension Node 回归测试、SDK package tests、repo CI。
- 手工检查：两个独立 session 的 handoff/reclaim；timeout 后 reconciliation；常见 browse/scroll/extract 流程。
- 观测检查：操作前后 focused window/active tab 不因 OBU 改变；receipt 能明确列出 claimed/released/closed/kept tabs。

## 进度记录

- [x] 2026-08-13：确认 0.1.41 干净基线与真实 Ream Chat 失败模式。
- [x] 2026-08-13：完成 request correlation 与 orphan response 隔离，并通过 Go race test。
- [x] 2026-08-13：完成 handoff release、claim status 与 lifecycle receipts；验证跨 session reclaim。
- [x] 2026-08-13：完成 durable reconciliation、background CDP fence 与高阶 MCP/action tools。
- [x] 2026-08-13：完成 `make ci`、`go vet ./...`、三种 SDK 和真实 MCP stdio surface 验证。
- [ ] 安装或发布 0.1.42 后，从新的 Ream provider session 对真实 Chrome 做最终 live acceptance。

## 决策记录

- 2026-08-13：OBU 与 OCU 保持独立 runtime，只共享 agent-grade 行为契约；本计划不修改 Ream 或 OCU。
- 2026-08-13：`handoff` 的含义是保留 tab 但释放 exclusive ownership；tab 可保留现有视觉分组，claim authority 不再由该 session 持有。
- 2026-08-13：timeout 一律报告 unknown outcome，并通过显式 operation id 查询结果，不自动重放 mutation。
- 2026-08-13：不直接覆盖用户当前 0.1.41 Chrome Web Store extension，也不未经授权发布 tag；0.1.42 release artifacts 已生成，live Ream acceptance 留在安装/发布之后执行。
