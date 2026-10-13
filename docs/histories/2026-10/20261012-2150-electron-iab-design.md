## [2026-10-12 21:50] | Task: 整理 Electron IAB 集成设计草案

### 🤖 Execution Context

- **Agent ID**: `/root`
- **Base Model**: GPT-6
- **Runtime**: Codex desktop

### 📥 User Query

> 结合已有分析和 Browser Use 文章，讨论在仓库中支持可被 Electron app
> 引入的独立 IAB，提供架构图并探索设计。第一阶段选择 npm 模块加示例 app，
> 按命名 profile 共享登录态，任务隔离 tabs。

### 🛠 Changes Overview

**Scope:** 设计文档与架构导航，无运行时代码变更。

**Key Actions:**

- 添加讨论草案，包含总体架构图、身份模型、模块边界和候选集成 API。
- 区分已有 Codex 版本的逆向结论、当前 Chrome route 和拟议 IAB 能力。
- 记录 WebContentsView 布局、后台输入、弹窗、资源释放和客户端兼容验证项。
- 更新设计索引和架构导航，明确新 backend 尚未实现。
- 验证：`make check-repo` 和 `git diff --check` 通过。

### 🧠 Design Intent (Why)

把页面管理、自动化和展示绑定整理成可复用 Electron 组件，允许多任务共享
登录身份并拥有独立 tabs。未验证的输入与跨平台行为保留为 PoC 验收项。

### 📁 Files Modified

- `docs/design-docs/electron-iab.md`
- `docs/design-docs/index.md`
- `docs/ARCHITECTURE.md`
- `docs/histories/2026-10/20261012-2150-electron-iab-design.md`
