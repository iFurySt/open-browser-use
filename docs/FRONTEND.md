# 前端协作说明

Open Browser Use 当前 `main` 分支只保留 Chrome extension 前端。

## 当前前端边界

- `apps/chrome-extension/popup.html`：extension popup 外壳。
- `apps/chrome-extension/popup.css`：popup 样式。
- `apps/chrome-extension/popup.js`：popup 行为。
- `apps/chrome-extension/content-cursor.js`：页面内 cursor overlay。

## 页内操作反馈

Agent 使用 `moveMouse`，或通过 CLI、MCP、SDK 发送顶层
`Input.dispatchMouseEvent` 时，页面内会显示 AI 光标和当前命中元素的边框。
成功的 `mousePressed` 还会显示短暂的点击圆环，帮助用户区分移动和按下。
原始 CDP 事件使用实际坐标立即同步光标，不等待动画到达；现有 `moveMouse`
保留动画行为。所有反馈均为 `pointer-events: none` 的页内覆盖层，不改变
页面元素样式、系统鼠标位置、当前标签页或窗口焦点。

目标边框优先选取命中位置的可交互祖先（例如按钮里的文字对应整个按钮），
支持 open shadow root 内的目标，closed shadow root 保留宿主边框。
滚动、resize、隐藏光标和结束 turn 会清理临时反馈；启用减少动态效果时，
点击提示使用静态短闪。视觉反馈使用独立有界队列，不延迟 CDP 返回；反馈失败
或超时不会改变底层 CDP 的结果，也不会重试输入。

范围与限制：

- 这是鼠标位置和命中目标的视觉提示，不是所有键盘、填表或 DOM 更新的操作记录。
- 仅同步顶层视口坐标。带 `target.sessionId` 的子 frame CDP 事件不猜测坐标转换。
- 顶层命中 iframe 时边框标识 iframe，不推断其中的跨域目标。
- 浏览器限制注入的页面可能无法显示覆盖层，浏览器自动化仍按原有 CDP 行为执行。

这个 popup 是工具面，不是 landing page。设计上优先保持状态清楚、权限边界
明确、操作少而直接。popup 会展示当前 extension 版本，并根据浏览器平台检测结果
显示 CLI 安装命令：macOS 展示 npm 和 Homebrew，Windows/Linux 展示 npm。

## 验证方式

```sh
pnpm package:chrome-extension
node --check apps/chrome-extension/popup.js
node --check apps/chrome-extension/content-cursor.js
node --test apps/chrome-extension/pointer-feedback.test.mjs apps/chrome-extension/content-cursor.test.mjs
```

打包脚本会校验 manifest、icons、background/content/popup 脚本基础语法，并
输出 `dist/chrome-extension/open-browser-use-chrome-extension-<version>.zip` 和
`dist/chrome-extension/open-browser-use-chrome-extension-<version>.crx`。

## 设计约束

- popup 内避免大块说明文案，优先使用短状态、明确按钮和必要的错误信息。
- 与 Chrome Web Store listing、privacy policy 和权限说明保持一致。
- 任何新增权限都必须同步更新 `apps/chrome-extension/manifest.json`、发布文档
  和隐私说明。
