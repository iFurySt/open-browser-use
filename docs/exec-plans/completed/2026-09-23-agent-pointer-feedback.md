# Agent pointer feedback

## 目标

让用户在受控页面中看见 Agent 的光标位置、点击提示和目标元素边框，覆盖现有 CLI、MCP、SDK 与 browser-client 路径，不移动系统鼠标或切换前台标签页。

## 范围

- 包含：顶层 CDP 鼠标事件同步、页内点击提示和目标高亮、回归测试、浏览器验证与文档。
- 不包含：Pi 专属接入、系统鼠标控制、跨域子 frame 坐标转换、扩展安装流程变更。

## 背景

- `background.js` 的 `executeCdp` 直接转发鼠标事件；现有动画光标仅由独立 `moveMouse` 更新。
- `content-cursor.js` 已提供独立于系统鼠标的 overlay，但没有点击提示和目标边框。
- 反馈属于辅助 UI，不能改变 CDP 的返回值、失败行为或页面输入目标。

## 风险

- 页面导航、受限页面、慢 content script 可能使反馈不可用；反馈使用独立有界队列与超时，不延迟 CDP 返回。
- 页面布局变化可能留下错误高亮；滚动、resize 和会话结束清理反馈。
- 子 frame 坐标不属于顶层视口；不猜测转换，明确记录边界。

## 里程碑

1. 确认现有光标路径和缺口。
2. 增加 CDP 同步与页内反馈。
3. 回归、浏览器验证、CI、history、上游 PR。

## 验证方式

- Node Chrome API/DOM 回归测试：成功与失败、无效坐标、child session、清理、不激活窗口。
- 独立 Chromium 测试页面：可见反馈、点击穿透、reduced-motion、后台标签页。
- 仓库 `make ci`（记录平台限制与基线失败）。

## 进度记录

- [x] 确认范围和已有实现。
- [x] 完成同步与展示。
- [x] 完成验证、文档与上游 PR 准备。

## 决策记录

- 2026-09-23：在扩展统一同步原始 CDP 鼠标事件，避免各 SDK 重复实现；沿用页内 overlay，无新增权限。
- 2026-09-23：审查发现同步等待反馈会占用客户端总 deadline；改为独立队列，并增加低 deadline 回归。open shadow root 采用局部命中测试，closed root 和 iframe 保持不透明。
- 2026-09-23：扩展测试 41/41、真实 Chromium 检查 14 项通过，27 次输入后的活动标签页不变；Windows CI 的原有路径/socket 平台失败已与未修改上游基线核对。
- 2026-09-23：隔离 Linux 快照的完整 `make ci` 退出码 0；实现与测试文件 SHA 与工作树一致，Go、SDK、CLI 六平台构建和全部基础检查通过。
