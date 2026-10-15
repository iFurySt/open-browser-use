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
  初始空闲状态不显示状态文案或底部分隔线，有操作状态、结果或错误时再显示。
- `renderer.js`：横向 tabs、新建/关闭、地址栏、后退/前进/刷新、隐藏/恢复。
- `preload.cjs` 与 `main.mjs`：固定操作 IPC，验证 sender/main frame/本地 URL；
  网页 guest 没有宿主 preload。
- native surface 与 DOM viewport 对齐；ResizeObserver 测量位置，主进程校验
  范围，不能覆盖宿主工具栏或左侧区域。展示区不放会从原生网页边缘露出的
  实现说明文字。

运行 `pnpm example:iab` 查看示例；`pnpm smoke:iab` 验证真实网页与 IPC、
窗口尺寸变化、地址历史、tabs 生命周期和 viewport 拒绝路径。
宿主 renderer 截图不包含 native guest，因此同时保留 host 与 page 截图。

“操作提示”开关控制 guest 内的 Browser Use 鼠标和页面内侧渐变边缘。库默认
关闭，示例开启；在动作的 `finally` 清理活动状态，支持运行中关闭。
鼠标素材、尺寸、变换和弹簧动画以 Chrome 的 `content-cursor.js` 与
`images/cursor-chat.png` 为唯一来源；Electron 包脚本生成可独立发布的内嵌
代码并检查是否过期。严格 img CSP 下使用 canvas 呈现原图像素。
视觉层位于 isolated world/closed shadow，pointer-events:none；它是体验
提示而非可信认证标记。截图检查包含正常页和 strict CSP 页。

宿主和本地演示页采用黑白灰极简布局；不放品牌色块、宣传标题或说明段落。
`styles.css` 管理动作列表、tabs 和工具栏；`fixture.css` 管理本地页的计数与
消息表单，附加 popup 控件收在“更多”。宿主 CSS 外置并使用 self-only CSP。
第 5 步打开真实 GitHub profile 的 Tab 3，结束与停止后保留全部页面供检查；
只有一键重新播放才清理旧页。手动 1–5 在空闲时均可反复点击，不按 next 锁定；
1/2/5 每次新开页面，3/4 操作最近打开的 Tab 1/2；缺少目标时提示先打开对应
页面。已完成进度去重，结果保留每个动作最近一次记录。执行中禁用动作按钮，
main 同样防止并发。Tab 切换/隐藏在运行中可用；地址栏编辑/Enter、刷新和
前后导航会等待当前动作结束并暂停剩余编排，再串行操作明确的目标 tab。
前进/后退仅按历史是否存在禁用。四边光效使用同一配置颜色（默认蓝色）的模糊内阴影形成
连续柔和衰减，不叠四条线性渐变或硬边框；角落无条带接缝，呼吸明暗变化轻微。
光效、原鼠标素材与动画均由 `open-browser-use-electron` 提供，示例只配置开关
并标记目标 tab 的操作生命周期，不在 example 中另写渲染实现。
SDK 的 `visualFeedback.edgeGlow` 独立控制光效，`edgeGlowColor` 接受
`#RGB` / `#RRGGBB`，四边统一向透明淡出。初始化和运行时 configure 均支持；
部分更新保留未指定值，关闭后保留颜色，活动 tab 即时换色且不改变原鼠标。
自动 smoke 使用本地第三页，真实 GitHub 在交互 app 单独验收。

地址栏共享 `address.mjs` 的 URL 归一化，支持裸域名、完整 http/https URL 和
localhost/loopback 端口；renderer 预检查与 main 再检查保持一致。首次聚焦
全选，后续点击/拖选保持正常；蓝色 focus-within 边框和 selection 背景明确
显示编辑状态。Cmd/Ctrl+L 在宿主及原生 guest 中都回到地址栏，快捷键仅作用
于示例进程。guest 仍没有 host preload/IPC。

地址栏空闲为白底，hover 切为浅灰底、边框略加深且图标更清楚，120ms 轻量
过渡；聚焦时优先保留白底与蓝色边框。减弱动态效果偏好关闭过渡。

runtime 按 tab 发布 loading、pendingUrl 与主 frame 的 loadError；tab 与
地址栏各有轻量旋转指示。切换 tab 不混用加载状态，失败可改地址重试。
host 状态读取不等待网页命令，历史可用性在后台更新；慢响应不会拖延 loading
显示。本地慢页与失败重试包含在真实 Electron smoke 中。
