# 前端协作说明

Open Browser Use 当前共享一套 Chrome/Zen extension 工具界面。

## 当前前端边界

- `apps/chrome-extension/popup.html`：extension popup 外壳。
- `apps/chrome-extension/popup.css`：popup 样式。
- `apps/chrome-extension/popup.js`：popup 行为。
- `apps/chrome-extension/content-cursor.js`：页面内 cursor overlay。
- `apps/zen-extension/manifest.json`：Zen/Firefox MV3 manifest。
- `apps/zen-extension/firefox-compat.js`：Firefox 缺失 Chrome debugger/tab
  group API 时的核心命令兼容层。

这个 popup 是工具面，不是 landing page。设计上优先保持状态清楚、权限边界
明确、操作少而直接。popup 会展示当前 extension 版本，并根据浏览器平台检测结果
显示 CLI 安装命令：macOS 展示 npm 和 Homebrew，Windows/Linux 展示 npm。
Zen popup 还会显示全局 page interaction 权限状态；未授权时提供一次性启用按钮，
Chrome manifest 不声明该 optional permission，因此 Chrome popup 不显示此面板。

## 验证方式

```sh
pnpm package:chrome-extension
pnpm package:zen-extension
node --check apps/chrome-extension/popup.js
node --check apps/chrome-extension/content-cursor.js
```

打包脚本会校验 manifest、icons、background/content/popup 脚本基础语法，并
输出 `dist/chrome-extension/open-browser-use-chrome-extension-<version>.zip` 和
`dist/chrome-extension/open-browser-use-chrome-extension-<version>.crx`。
Zen 打包输出 `dist/zen-extension/open-browser-use-zen-extension-<version>.xpi`。

## 设计约束

- popup 内避免大块说明文案，优先使用短状态、明确按钮和必要的错误信息。
- 与 Chrome Web Store / Firefox 安装说明、privacy policy 和权限说明保持一致。
- 任何新增权限都必须同步更新 `apps/chrome-extension/manifest.json`、发布文档
  和隐私说明。
