# 架构总览

这个仓库当前目标是实现 Open Browser Use：一套开源 Browser Use 风格的
Chrome 浏览器自动化基础设施。

当前 `main` 分支包含 Chrome route 和 Zen/Firefox compatibility route。两者共享
Go native messaging host、SDK 与主要 extension runtime；Chrome route 使用
`chrome.debugger` 和 tab groups，Zen route 通过 Firefox WebExtension API 提供
tabs、history、session 和核心 navigation/evaluation 兼容能力。

已完成的 Chrome route 执行计划：

- `docs/exec-plans/completed/2026-05-08-open-browser-use-chrome-route.md`

已有 Browser Use / Codex extension 逆向知识沉淀在：

- `docs/wiki/browser-client/`
- `docs/references/`

## 预期的仓库结构

- `apps/chrome-extension/`：Open Browser Use MV3 Chrome extension，包含
  readable service worker、cursor content script 和 popup。
- `apps/zen-extension/`：Zen/Firefox MV3 manifest 和 Firefox compatibility
  adapter；打包时复用 Chrome extension 的 background、cursor、popup 与图标。
- `cmd/open-browser-use/`：Go native messaging host 和 CLI 主入口。二进制
  正式名是 `open-browser-use`，可通过同路径 symlink 或 shell alias 暴露为
  `obu`。
- `internal/host/`：Go native messaging stdio 到本地 Unix socket 的中继。
- `internal/wire/`：Open Browser Use native frame codec。
- `packages/browser-client-rewrite/`：当前 Browser Use client 的可读重写
  和协议行为参考。
- `packages/browser-use-protocol/`：Browser Use JSON-RPC native pipe frame、
  请求/响应类型和测试工具。
- `packages/open-browser-use-cli/`：npm 上发布的 `open-browser-use` 二进制
  CLI 包，暴露 `open-browser-use` 和 `obu` 两个命令，发布时只打包 Go CLI
  预构建二进制，不打包 Chrome extension。
- `packages/open-browser-use-js/`：JavaScript/TypeScript SDK，npm distribution
  名为 `open-browser-use-sdk`。
- `packages/open-browser-use-python/`：Python SDK，PyPI distribution 名为
  `open-browser-use-sdk`，import 模块名为 `open_browser_use`。
- `packages/open-browser-use-go/`：Go SDK，Go import path 为
  `github.com/ifuryst/open-browser-use/packages/open-browser-use-go`，package
  name 为 `obu`。
- `skills/open-browser-use/`：面向 AI Agent 的 Open Browser Use skill，说明
  如何安装、验证和使用浏览器插件、CLI、SDK 与 Browser Use 风格协议。
- `packages/`：其他跨应用复用的库、契约和共享能力。
- `infra/`：部署、基础设施和环境定义。
- `scripts/`：仓库级自动化脚本，供人和 Agent 直接调用。
- `docs/`：仓库知识库，也是本地规则和上下文的正式来源。
- `docs/wiki/`：长期演进的主题知识库，用于沉淀可持续追加的调研、
  逆向和架构理解。

## 边界建议

- 业务逻辑优先沉淀到可复用包里，不要一开始就散落在各个 app 中。
- 基础设施和运行编排要显式版本化，不要藏在手工操作里。
- 避免隐式跨包耦合；一旦仓库成形，就把允许的依赖方向写清楚。
- 只要架构有变化，就同步更新这份文档。

## Open Browser Use Chrome Route

Chrome route 的目标拓扑：

```text
upper-layer runtime / SDK / CLI
  -> /tmp/open-browser-use/<uuid>.sock
  -> open-browser-use Go native host
  -> Chrome Native Messaging stdio
  -> Open Browser Use MV3 service worker
  -> chrome.tabs / chrome.debugger / chrome.history / chrome.tabGroups
  -> user's real Chrome profile
```

Chrome native messaging host name 固定为：

```text
com.ifuryst.open_browser_use.extension
```

Chrome native messaging host name 只能使用小写字母、数字、underscore 和
dot；hyphen 版本 `com.ifuryst.open-computer-use.extension` 会被
`chrome.runtime.connectNative` 直接拒绝。

当前 M1 骨架已经提供：

- native-endian 4-byte length-prefixed JSON frames。
- `open-browser-use host`：启动 Chrome native messaging stdio 到 SDK Unix
  socket 的中继；默认 socket 路径是
  `/tmp/open-browser-use/<uuid>.sock`，并写入
  `/tmp/open-browser-use/active.json` 供 CLI/SDK 自动发现当前活跃 socket。
  Chrome native messaging 以 `chrome-extension://...` origin 参数启动二进制时，
  CLI 会自动进入 host mode。
- 无参数运行 `open-browser-use` 或 `obu` 输出 CLI 版本、浏览器插件检测状态、
  插件版本和下一步安装/升级命令，不再启动 native host；手工启动 host 使用
  `open-browser-use host`。
- `open-browser-use manifest`：输出 Chrome native messaging host manifest。
- `open-browser-use install-manifest`：把 native messaging host manifest
  写入 Chrome 默认位置，或通过 `--output` 写到指定路径。默认 extension id
  是 Chrome Web Store 版 `bgjoihaepiejlfjinojjfgokghnodnhd`。
  Windows 默认写入
  `%LOCALAPPDATA%\OpenBrowserUse\NativeMessagingHosts\com.ifuryst.open_browser_use.extension.json`，
  并注册
  `HKCU\Software\Google\Chrome\NativeMessagingHosts\com.ifuryst.open_browser_use.extension`
  的默认值指向该 manifest。
- `open-browser-use setup`：显式安装流程，先调用 native host manifest
  注册，再写入 Chrome External Extensions JSON。只有检测到浏览器插件未安装或
  版本低于当前 CLI 期望版本时，CLI 才打开 Chrome Web Store 正式扩展页，引导
  用户手动安装或启用扩展。macOS/Windows 仍可能需要用户在 Chrome 中确认启用
  扩展并重启；Linux 的 External Extensions 写入默认使用 Chrome 官方系统路径，
  可能需要更高权限。
- `open-browser-use setup beta`：Chrome Web Store 临时不可用时的备用路径，
  注册 native host 后从 GitHub Releases 下载最新
  `open-browser-use-chrome-extension-*.zip`。CLI 会在本地 unpacked 目录和待拖入
  Chrome 的 ZIP 中写入稳定 beta public key，用该 id 注册 native host allowed
  origin；只有检测到浏览器插件未安装或版本低于当前 CLI 期望版本时，才打开
  `chrome://extensions/` 和 Finder/文件管理器，引导用户把这个 ZIP 拖到扩展页面
  手动安装；GitHub Release 中的正式 zip 本身保持为 Chrome Web Store 上传包，
  不再预写 beta key。setup 过程中如果本机已有可用的 `npx skills`，会
  best-effort 执行 `npx skills update open-browser-use -g -y` 更新已有 agent
  skill；未检测到 skills 时只展示 Codex 和 Claude Code 的安装命令。
- manifest 的 `path` 默认统一写入稳定 native host link：
  macOS 为
  `~/Library/Application Support/OpenBrowserUse/native-host/open-browser-use`，
  Linux 为 `~/.local/share/open-browser-use/native-host/open-browser-use`。
  Windows 为
  `%LOCALAPPDATA%\OpenBrowserUse\native-host\open-browser-use.exe`。macOS/Linux 上
  `install-manifest --path` 表示这个稳定 link 指向的真实二进制 target；Windows
  上会把 target 复制到稳定 exe 路径，避免依赖开发者模式或管理员权限创建 symlink。
- npm 包 `open-browser-use` 是 CLI 二进制分发入口，安装后提供
  `open-browser-use` 和 `obu`；`postinstall` 只提示用户运行显式
  `open-browser-use setup`。
- SDK 发布包在 npm 和 PyPI 上统一命名为 `open-browser-use-sdk`，避免和 npm
  CLI 包 `open-browser-use` 混淆。JavaScript 使用
  `import { ... } from "open-browser-use-sdk"`；Python distribution 安装名是
  `open-browser-use-sdk`，代码 import 模块名仍是 `open_browser_use`。
- Homebrew formula 从 GitHub Release 下载平台匹配的预编译 CLI tarball，
  安装后提供 `open-browser-use` 和 `obu`，并在 caveats 中提示用户运行显式
  `open-browser-use setup`；安装阶段不依赖 Go，也不在用户机器上编译。
- CLI 命令层使用 Cobra 实现；Chrome native messaging
  `chrome-extension://...` origin 参数启动时会绕过 Cobra，直接进入 host mode。
  这依赖 Chrome Native Messaging 的标准启动形状：MV3 service worker 调用
  `chrome.runtime.connectNative(...)` 后，Chrome 以 host manifest 中的可执行文件
  路径启动进程，并把调用方 extension origin 作为第一个 argv 传入。manifest
  自身不承载额外自定义 argv，因此 Chrome extension 专用启动信号使用这个
  Chrome 提供的 origin argv。Windows Chrome 还可能只暴露 `--parent-window=<handle>`
  形状，因此 CLI 也把该参数识别为 native host mode。
- `open-browser-use call`：unrestricted JSON-RPC 入口，允许上层应用发送
  任意 method/params；未显式传入 `--socket` 时会优先读取 active socket
  registry。registry 缺失时，CLI 会按修改时间扫描 `--socket-dir` 下的
  `*.sock` 并连接最新可用 socket，连接成功后修复 active registry；如果
  registry 指向不可连接的旧 socket，CLI 会移除 stale registry entry 和对应
  stale socket file，再尝试目录扫描，避免后续命令持续命中同一个失效 socket。
- `open-browser-use run`：line-oriented action plan 入口，支持一次执行多条
  CLI action，保留同一个 session/turn，并让 `open-tab`/`claim-tab` 设置默认
  tab，供后续 `wait-load`、`page-info`、`navigate`、`cdp` 等 tab-scoped action
  复用；默认和普通 CLI 子命令使用同一个 `obu-cli` browser session，因此
  一次 `finalize-tabs` 可以清理两种调用方式打开的 tabs；需要隔离 tab group
  和清理范围时可以显式传 `--session-id`。它不是通用编程语言，适合 shell、
  CI 和轻量 agent 编排。
- `open-browser-use mcp` / `obu mcp`：stdio MCP server 入口，使用
  newline-delimited JSON-RPC，完成 `initialize` 生命周期握手，并暴露
  `tools/list` 与 `tools/call`。MCP tools 复用 CLI/runner 能力，包括
  `user_tabs`、`open_tab`、`navigate`、`page_info`、`cdp`、
  `run_action_plan`、`finalize_tabs` 和 unrestricted `call`，适合 Codex
  等支持本地 MCP server 的 agent runtime 直接接入。
- CLI 便捷子命令覆盖当前 SDK 核心能力：`ping`、`info`、`tabs`、
  `user-tabs`、`history`、`open-tab`、`navigate`、`claim-tab`、
  `finalize-tabs`、`name-session`、`cdp`、`move-mouse`、
  `wait-file-chooser`、`set-file-chooser-files`、`turn-ended`、`profiles`。
- Multi-browser / multi-profile 支持：`open-browser-use profiles` 扫描 Chrome
  Stable、Chrome Beta、Zen Browser 和 BitBrowser user-data roots。Chromium
  profile 通过 CRX/unpacked 路径检测，Zen profile 通过 Firefox
  `profiles.ini`/`extensions.json` 检测；结果附带 browser id、
  browser display name、profile directory 和展示名（来自
  `Local State.profile.info_cache`）。所有面向用户的命令支持
  `--browser <chrome|chrome-beta|zen|bitbrowser|displayName|instance>` 和
  `--profile <directory|displayName>`：CLI 枚举 `socket-dir` 下的 `.sock`
  文件，对每个连通的 socket 调用 `getInfo`，把
  `metadata.extensionInstanceId` 反查到 browser/profile（grep 每个支持 browser
  root 的 `Local Extension Settings/<extension-id>/*.{log,ldb}`），按 selector
  选出匹配 socket。`obu mcp --browser ... --profile ...` 在 MCP server 启动时锁定
  selector，每次工具调用复用同一个解析结果。selector 不匹配时 CLI 列出当前已连通
  target 列表，提示用户打开对应 browser/profile。Windows 当前覆盖 Chrome
  Stable、Chrome Beta 和 Zen 的 profile roots；BitBrowser root 仍只在 macOS
  路径中自动发现。Zen Linux 同时覆盖 system package 的 `~/.config/zen`、
  tarball 的 `~/.zen` 和 Flatpak profile root。未显式选择时仍保留
  `active.json` 快速路径；`active.json` 缺失或失效时，CLI 会扫描 socket 目录并
  修复 registry，因此不需要重装扩展。
- MV3 extension core handlers：`getInfo`、`createTab`、`getTabs`、
  `getUserTabs`、`getUserHistory`、`claimUserTab`、`finalizeTabs`、
  `nameSession`、`attach`、`detach`、`executeCdp`、`moveMouse`、
  `waitForFileChooser`、`setFileChooserFiles`、`waitForDownload`、
  `downloadPath`、`readClipboardText`、`writeClipboardText`、
  `readClipboard`、`writeClipboard`、`turnEnded`。
- Zen/Firefox route 使用稳定 Gecko id `open-browser-use@ifuryst.com`，native
  manifest 使用 `allowed_extensions` 并安装到 Mozilla NativeMessagingHosts
  位置。Firefox 不实现 Chrome WebExtension debugger API，因此 adapter 只兼容
  `Page.navigate`、`Page.reload`、`Page.close`、`Runtime.evaluate`、
  `Target.getTargets` 和基础 enable/version 调用；其他 CDP method 返回明确的
  unsupported error，file chooser 本地路径注入也不可用。`Runtime.evaluate`
  使用 Firefox MV3 user-script API 在隔离的 `USER_SCRIPT` world 中运行：
  Firefox 136–152 通过已注册 bridge 执行，Firefox 153+ 可直接使用
  `userScripts.execute`。这允许 DOM 读取和表单交互而不依赖网站 CSP；
  `userScripts` 是一次性、全局的 optional-only permission，由用户在 popup 中
  明确授予。Zen 无 tab group API，
  session membership 改由 extension storage 的 tab origin map 维护。
- Session state persists the Chrome tab group id, tab origins, group title,
  deliverable group id, and logical active tab id in `chrome.storage.local` so
  MV3 service worker restarts can recover session tab listing semantics.
  新 session 的默认任务组名是 `Task - OBU`，上层 runtime 应通过
  `nameSession`/`name-session` 按 `<short task> - OBU` 设置更具体的任务名；
  `deliverable` tabs 统一移动到共享 `✅ Open Browser Use` tab group，`handoff`
  tabs 保留在当前任务组。每个 Chrome 窗口至多存在一个 `✅ Open Browser Use`
  分组：`moveToDeliverables` 按 `windowId` 调用 `chrome.tabGroups.query` 找出已有
  同名分组，合并任何重复（如 session restore 带回的旧分组）后再加入新 tab，
  仅在该窗口完全没有同名分组时才新建。
  `turnEnded` and `finalizeTabs` clear active session state when control ends
  so download notifications only fire while a browser session is active.
- MV3 extension event forwarding：`chrome.debugger.onEvent` 转发为
  `onCDPEvent`，`chrome.downloads` 创建/变更转发为 `onDownloadChange`，
  cursor content script 会回报 cursor arrival 以支持 `moveMouse`
  等待落点。file chooser 通过 CDP `Page.setInterceptFileChooserDialog` 和
  `Page.fileChooserOpened` 事件截获，再使用 `DOM.setFileInputFiles` 写入
  本地文件路径。download wait/path 和 clipboard helpers 由 extension backend
  暴露给 SDK；agent-facing Browser Use command rewrite 还覆盖 DOM CUA、media
  download、element info/screenshot、locator bulk reads、generic tab export 和
  clipboard commands。
- JS、Python 和 Go SDK：直接连接 Unix socket 发送 Browser Use JSON-RPC。
  JS SDK 支持订阅 native socket 上的 JSON-RPC notification；Python SDK 会在
  同步 request loop 中分发并跳过 notification，避免 CDP event 插队破坏响应读取；
  Go SDK 同样在同步 request loop 中分发 notification，并可自动读取 active
  socket registry。三个 SDK 都在原子 client 之外提供高层 browser/tab API，可直接使用
  `tab.goto`、`tab.waitForLoadState`/`tab.wait_for_load_state` 和
  `tab.playwright.domSnapshot`/`tab.playwright.dom_snapshot` 这类 Browser Use
  风格封装；Python SDK 还提供 `title`、`url`、`wait_for_timeout` 和
  `locator(...).inner_text(...)` 等适合 Jupyter/Python REPL 编排的薄封装，Go SDK
  提供对应的 `Title`、`URL`、`WaitForTimeout` 和 `Locator(...).InnerText(...)`
  helper。三个 SDK 同时提供核心 Browser Use method wrappers、download/clipboard
  convenience wrappers，也保留 unrestricted `request`。

当前 SDK 不内置 Codex 风格的站点限制、turn policy 或二次确认。上层应用
可以自由调用；生产集成如果需要安全策略，应在上层 runtime 或 host 前置网关
中实现。

## 新项目需要补齐的内容

- 核心产品面和运行拓扑。
- 包分层与依赖边界。
- 数据流与存储模型。
- 可观测性方案和本地开发模式。
