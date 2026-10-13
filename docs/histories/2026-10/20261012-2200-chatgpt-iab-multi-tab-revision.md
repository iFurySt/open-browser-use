## [2026-10-12 22:00] | Task: 复核 ChatGPT IAB 多标签模型改版

### 🤖 Execution Context

- **Agent ID**: `/root`
- **Base Model**: GPT-6
- **Runtime**: Codex desktop

### 📥 User Query

> 我们之前分析过 codex 的 iab。你分析一下最新的 chatgpt，我感觉应该是改变了
> 或者改进了！因为我感觉先好像不是一个会话一个 tab 的感觉了，你看看

### 🛠 Changes Overview

**Scope:** 逆向复核与文档同步，无运行时代码变更。

**Key Actions:**

- 复核 `ChatGPT.app` `26.1007.30912`（包名仍为 `openai-codex-electron`，bundle id
  仍为 `com.openai.codex`）的 IAB 实现。
- 推翻旧的“一个 `(windowId, conversationId)` 一个侧边栏页面”结论，确认当前模型为
  「一个会话多个标签」。
- 更新 `docs/wiki/browser-client/runtime/iab-architecture.md`：改版证据快照、
  路由键（新增 `routeTabKey`）、`pageKey` 语义、会话级路由捕获、标签生命周期、
  多标签语义、peer authorization 现状。
- 更新 `docs/wiki/browser-client/notes/open-questions.md`：把多页面语义从待验证
  移到已解决，并补充 socket 生命周期与 peer authorization 的新问题。
- 更新 `docs/references/browser-client-reverse-engineering.md`：插件路径、客户端
  文件拆分、发现机制与 framing 复核结论。
- 验证：`make check-repo` 与 `git diff --check`。

### 🔍 Verified Findings

用户直觉正确。关键证据：

1. **`pageKey` 带上了标签维度**

   ```text
   旧: `${ownerWebContentsId}:${conversationId}`
   新: `${conversationId}\0${browserTabId}`
   ```

   `browserTabId` 由 `browser-use:${randomUUID()}` 生成，因此一个会话天然可以容纳
   多个页面。

2. **`createTab` 真的开面板标签**

   `createTabForBrowserUse` 调用
   `openPageForBrowserUse({ browserTabId, createPanelTab: true })`，并新增
   `tabIdsByRouteTabKey: Map<routeTabKey, cdpTabId>` 作为分配索引。

3. **`tabs.list()` 返回会话下所有标签**

   `getTabsForBrowserUse` 遍历该会话下**每一个** `ThreadState`，而 `WindowState.threads`
   现在按 `pageKey` 而不是按 `conversationId` 索引。

4. **回合路由被会话路由取代**

   `browser-use-turn-route-capture` / `-release` 已消失，替换为
   `browser-use-session-route-capture` / `browser-use-session-activity-ended`，
   状态从 `turnRoutes` 变为 `sessionRoutes`。`turn_id` 仍用于标签清理与 `markTab`
   记账，但不再决定 socket 能否服务请求。

5. **标签生命周期显式化**

   新增 `browserUseTabLifetime: "temporary" | "persistent"` 与
   `mark = { status: "handoff" | "deliverable", turnId }`；`getInfo` 通过
   `apiSupportOverrides` 广告 `Tab.markDeliverable` / `Tab.markHandoff`。回合结束时，
   临时标签上是 `handoff` 的会保留，其他情况按标记或 `persistent` 决定释放还是关闭。

6. **直连探测已不可行**

   macOS prod 包通过 `native/browser-use-peer-authorization.node` 在 `accept()`
   阶段校验对端。用普通 socket 客户端发 `getInfo` 只会得到 `timeout` 或
   `write EPIPE`，服务端静默丢弃未授权连接，而不是回 JSON-RPC 错误。

### 🧠 Design Intent (Why)

把 IAB 从“每个会话一个内置浏览页面”推进到“会话内多标签浏览器”，是把 Codex
内置浏览器当作真实浏览器表面的自然一步：多标签需要独立生命周期、选择态和显式
的保留语义，因此标签身份进入 `pageKey`，回合级路由让位给会话级路由加显式活动
结束信号。文档必须区分“已复核版本”与“历史版本”，避免把旧结论继续当现状使用。
