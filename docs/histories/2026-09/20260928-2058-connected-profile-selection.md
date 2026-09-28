## [2026-09-28 20:58] | Task: 按 connected 状态选择浏览器 Profile

### 🤖 Execution Context

- **Agent ID**: `TraeCode`
- **Base Model**: `GPT-5`
- **Runtime**: `Trae CLI`

### 📥 User Query

> OBU 检测到多个 Chrome Profile，但现场只有一个 Profile 连接；定位并修复不必要的目标确认。

### 🛠 Changes Overview

**Scope:** `skills/open-browser-use/` 与 Profile 选择设计文档

**Key Actions:**

- **[Selection policy]**: 未显式指定目标时，以 connected target 数量决定是否询问；只有一个 connected target 时直接选中并固定 selector。
- **[Disconnected targets]**: 保留完整 installed target 列表用于诊断，但 disconnected target 不再单独触发确认。
- **[Documentation]**: 同步原 Profile execution plan 的 Skill 指引和单 connected target 验收场景。

### 🧠 Design Intent (Why)

`profiles --connected` 会列出磁盘上所有安装过扩展的 Profile，再叠加实时连接状态。关闭或尚未启用扩展的 Profile 仍可能出现在 installed 列表中，因此用 installed 数量触发确认会在唯一可用目标已经明确时打断用户。新的决策树仍然保留显式目标和多 connected target 的安全边界，同时避免无意义确认。

### 📁 Files Modified

- `skills/open-browser-use/SKILL.md`
- `docs/exec-plans/active/2026-05-12-chrome-profile-selection.md`
- `docs/releases/feature-release-notes.md`
- `docs/histories/2026-09/20260928-2058-connected-profile-selection.md`

### ✅ Verification

- `make ci`
- 现场 `obu profiles --connected --json`：3 个 installed target 中只有 1 个 connected，符合新规则的无询问直连分支。
