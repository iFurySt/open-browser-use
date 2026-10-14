# 前端协作说明

Open Browser Use 的前端包含 Chrome extension 和独立 Electron IAB 示例。

## 当前前端边界

- `apps/chrome-extension/popup.html`：extension popup 外壳。
- `apps/chrome-extension/popup.css`：popup 样式。
- `apps/chrome-extension/popup.js`：popup 行为。
- `apps/chrome-extension/content-cursor.js`：页面内 cursor overlay。

这个 popup 是工具面，不是 landing page。设计上优先保持状态清楚、权限边界
明确、操作少而直接。popup 会展示当前 extension 版本，并根据浏览器平台检测结果
显示 CLI 安装命令：macOS 展示 npm 和 Homebrew，Windows/Linux 展示 npm。

## 验证方式

```sh
pnpm package:chrome-extension
node --check apps/chrome-extension/popup.js
node --check apps/chrome-extension/content-cursor.js
```

打包脚本会校验 manifest、icons、background/content/popup 脚本基础语法，并
输出 `dist/chrome-extension/open-browser-use-chrome-extension-<version>.zip` 和
`dist/chrome-extension/open-browser-use-chrome-extension-<version>.crx`。

## 设计约束

- popup 内避免大块说明文案，优先使用短状态、明确按钮和必要的错误信息。
- 与 Chrome Web Store listing、privacy policy 和权限说明保持一致。
- 任何新增权限都必须同步更新 `apps/chrome-extension/manifest.json`、发布文档
  和隐私说明。

## Electron IAB 示例

- `examples/electron-iab/index.html`：280 DIP 左侧栏展示五个动作步骤、
  播放/停止和简短状态，结果默认折叠；右侧浏览器面板。界面只表达
  Electron main + CLI + 私有通道，其他接法放 integrations.mjs 与 README。
- `renderer.js`：横向 tabs、新建/关闭、地址栏、后退/前进/刷新、隐藏/恢复。
- `preload.cjs` 与 `main.mjs`：固定操作 IPC，验证 sender/main frame/本地 URL；
  网页 guest 没有宿主 preload。
- native surface 与 DOM viewport 对齐；ResizeObserver 测量位置，主进程校验
  范围，不能覆盖宿主工具栏或左侧区域。展示区不放会从原生网页边缘露出的
  实现说明文字。

运行 `pnpm example:iab` 查看示例；`pnpm smoke:iab` 验证真实网页与 IPC、
窗口尺寸变化、地址历史、tabs 生命周期和 viewport 拒绝路径。
宿主 renderer 截图不包含 native guest，因此同时保留 host 与 page 截图。
