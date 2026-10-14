## [2026-10-12 21:50] | Task: 设计并实现 Electron IAB 集成

### 🤖 Execution Context

- **Agent ID**: `/root`
- **Base Model**: GPT-6
- **Runtime**: Codex desktop

### 📥 User Query

> 结合已有分析和 Browser Use 文章，讨论在仓库中支持可被 Electron app
> 引入的独立 IAB，提供架构图并探索设计。第一阶段选择 npm 模块加示例 app，
> 按命名 profile 共享登录态，任务隔离 tabs。
>
> 2026-10-13：复核新增未提交分析，把一会话多 tab 明确到状态与生命周期，
> 设计 socket 鉴权，避免任意本机客户端仅靠路径获得浏览器访问权。
>
> 随后授权实现 open-browser-use-electron、IAB 命令、私有通道/socket 和
> 对应鉴权；examples 中提供独立 Electron app，展示多种接入姿势并本地测试。

### 🛠 Changes Overview

**Scope:** Electron IAB 设计、独立 npm 模块源码、IAB CLI、通信鉴权、示例与测试；保留现有逆向证据整理。

**Key Actions:**

- 添加讨论草案，包含总体架构图、身份模型、模块边界和候选集成 API。
- 区分已有 Codex 版本的逆向结论、当前 Chrome route 和拟议 IAB 能力。
- 记录 WebContentsView 布局、后台输入、弹窗、资源释放和客户端兼容验证项。
- 更新设计索引和架构导航，明确新 backend 尚未实现。
- 验证：`make check-repo` 和 `git diff --check` 通过。

### 2026-10-13 设计修订

- 将设计依据更新为 ChatGPT `26.1007.30912` 的 session route、多 tab 和
  pageKey/browserTabId 模型，补充 tab 清理与 UI 选择态的边界。
- 增加本地连接鉴权设计：默认私有通道，显式 socket 需要原生 peer 校验、
  宿主发放的 scope lease、双向身份检查和逐请求/逐事件权限验证。
- 通过当前 app 的 main bundle、原生 addon 符号与诊断字符串复核鉴权入口；
  确认检查发生在 accept 后、frame parsing 前，并保留原生策略未知部分。
- 修正 wiki 中遗留的单 tab 结论和旧路由图；旧 Renderer、输入和 cursor
  细节标为历史快照，避免把尚未复核的内容当作新版事实。
- 同步架构、安全和设计索引；明确现有 Chrome socket 尚未增加鉴权。

### 🧠 Design Intent (Why)

把页面管理、自动化和展示绑定整理成可复用 Electron 组件，允许多任务共享
登录身份并拥有独立 tabs。隐藏页面的可信文本/鼠标输入已通过 fixture 实测；跨平台、IME、复杂站点和
最小化窗口行为继续保留为后续验收项。

### 📁 Files Modified

- `docs/design-docs/electron-iab.md`
- `docs/design-docs/iab-transport-auth.md`
- `docs/design-docs/index.md`
- `docs/ARCHITECTURE.md`
- `docs/SECURITY.md`
- `docs/wiki/browser-client/runtime/iab-architecture.md`
- `docs/wiki/browser-client/notes/open-questions.md`
- `docs/references/browser-client-reverse-engineering.md`
- `docs/histories/2026-10/20261012-2150-electron-iab-design.md`

### 2026-10-13 实现与验证

- 新增 `open-browser-use-electron`：命名 profile、scope 多 tab、native surface、
  page CDP allowlist、事件和资源生命周期；限制每 scope 32 tabs、总计 128。
- 新增 `/client`，复用现有 JS SDK browser/tab helper；私有继承 FD 3 为默认
  通道，不创建 listener。socket 需显式启用，目录/socket 为 0700/0600。
- 开源 Node-API adapter 在 macOS/Linux 读取内核 peer，绑定宿主登记的
  PID/UID/启动时间；两端验证进程实例，随后以 256-bit lease 握手。
  凭据只从私有通道交付，权限、scope、期限、撤销和事件都持续校验。
- 增加独立 `obu iab` 命令，不扫描 Chrome registry；Go 客户端实现同一
  framing、私有 bootstrap 和原生服务端身份检查。
- 新增 `examples/electron-iab`，展示主进程直调、JS private/socket、CLI
  private/socket 五种方式；含安全 preload、localhost fixture 和自动 smoke。
- 修复真实 Electron 的 ESM 启动等待、弹窗 WebContents 控制器销毁、退出后
  Chromium storage 写入的清理时序。隐藏 view 首次鼠标输入前渲染命中测试
  帧，并在该异步步骤后再次检查撤销状态，不显示页面或合成 DOM 点击。
- 更新根 README、包/示例接入文档、架构、安全、供应链、质量、feature note、
  design、tech debt 和 completed execution plan。

验证结果：

- `make ci` 通过，包含 16 项 Electron runtime/原生 socket/鉴权测试、Go
  测试和原有 Chrome/SDK 检查；`pnpm typecheck` 通过。
- `pnpm smoke:iab` 在 Electron 44.7.0 / macOS arm64 通过 12 项真实运行
  检查：多 tab、共享登录 cookie、隐藏 tab 可信文本/鼠标输入、ownership、
  四种工具 transport、匿名 socket 拒绝、popup opener 和截图/生命周期。
- Go CLI 交叉编译 Linux amd64 和 Windows amd64 通过。
- npm pack 通过；制品包含 dist、声明、LICENSE、原生源码和安装脚本，
  workspace SDK 依赖转换为 registry 版本；没有打入本机编译的 addon。
- 桌面锁屏期间未完成手动点按 UI；已启动真实 Electron smoke，检查独立
  host/page 截图并保存可复现报告。后续正常示例可通过 `pnpm example:iab` 查看。

首版边界：没有发布 npm 或提交代码；Windows socket 失败关闭，Windows private
通道和 Linux/Electron 多版本实机矩阵仍待验证。签名 allowlist、结构化诊断、
独立发布/provenance、global/child-target CDP、文件和 clipboard 尚未提供。
Chrome route 的既有 socket 不因本次 IAB 实现而获得新鉴权。

### 2026-10-13 浏览器面板调整

用户反馈原生网页背后露出实现标签，希望像参考 app 一样把 tabs 放在右侧
浏览器面板中。移除背景说明文字，将示例改为左侧五种接入方式和运行结果、
右侧横向 tabs + 地址栏 + 网页的布局，补充新建/关闭、后退/前进/刷新、
隐藏/恢复。原生 surface 与 DOM viewport 对齐，主进程验证布局参数范围。

页面导航和加载结束时同步 tab/URL 状态；异步历史导航固定初始 tabId，避免
用户切换标签后把历史操作发送给其他页面。同步示例 README 和 FRONTEND。
新增真实 Electron panel smoke，覆盖原生/DOM 几何、两种窗口尺寸、非法 bounds、
URL/历史/刷新/关闭以及隐藏/恢复。

本轮 `pnpm smoke:iab` 15 项通过，包含原生 view 与 DOM viewport 的实际
bounds 一致性；16 项包测试、JS 语法检查、`make check-repo` 和 diff hygiene
通过。检查更新后的 host/page 截图；桌面仍锁屏，未额外声称手动点按通过。

### 2026-10-13 解锁后的原生窗口验收

用户解锁后，通过原生窗口工具选择本次仓库的 Electron app，核对窗口标题
与宿主 UI URL。实际截图确认右侧 tabs、地址栏和网页区域对齐，背景实现标签
不再露出。手动点击私有通道按钮创建真实页面，并验证 tabs 切换、新建、关闭、
隐藏/恢复、地址栏访问本地 fixture、后退/前进和真实网页按钮响应。

刷新验收以页面计数先从 0 增加到 1，再通过工具栏刷新恢复到 0，确认操作
实际生效。原先“桌面锁屏，手动点按未验证”的限制现已解除；没有因本轮验收
额外修改运行时代码，也没有提交或发布。既有 15 项 Electron smoke 和
16 项 package 测试结果保持有效。

### 2026-10-13 示例文案精简

用户要求清理左侧冗余文字。删除宣传标题、引导段落、任务徽章、共享登录态
说明、按钮序号/箭头与逐项技术副标题；仅保留模块标题和五个明确区分
主进程/Node/CLI 的接入按钮。左侧从 360 DIP 缩为 280 DIP，profile 只在
右侧状态行展示一次。最近运行显示简短结果，完整 RPC JSON 默认折叠。
同步 native viewport 校验、现有 smoke 尺寸预期、README 和 FRONTEND。

本轮 15 项真实 Electron smoke、脚本语法、repo/docs 与 diff 检查通过，
包含缩窄侧栏后的原生 viewport 对齐和窗口尺寸变化。
原生窗口复核新版布局，点击主进程按钮后显示简短成功状态，JSON 详情默认折叠。

### 2026-10-13 CLI 私有通道动作演示

用户将接入归为 main+Node/main+CLI 两组、每组 private/socket 两种，并要求
app 只表达最重要的 main+CLI+private。原接入按钮改为五步动作：开 tab1，
开 tab2，回 tab1 点击计数，回 tab2 点击输入并提交，关闭全部。可逐次执行、
一键自动播放、停止、重新播放以及关闭后再次手动开始。

- `integrations.mjs` 保留两组四种真实工具接入，界面不暴露 transport 选择。
- `demo.mjs` 使用真实 `obu iab` 子进程进行页面创建/读取、坐标点击、可信
  CDP 文本输入、提交和关闭；main 只做 scope、私有 bootstrap 与展示编排。
- fixture 增加表单提交反馈；步骤状态、并发防护、停止和有界清理版本化。
- 关闭 opener 可能自动销毁 popup，因此关闭全部时每轮从 CLI 重读 live tabs，
  不继续使用失效的初始 tabId 列表。
- 界面启动为空；显示五步、播放/停止、状态和可展开结果。运行时保留浏览器
  tabs/address 控件。README、架构、FRONTEND、设计和 feature note 同步。

`make ci` 通过（包含 16 项 package 测试）；真实 Electron smoke 扩至 18 项，
覆盖四种参考 transport、五步手动/自动、可信点击/输入/提交、重复手动开始、
播放中并发拒绝与停止后清理。报告保留 demo-tab1/demo-tab2 截图。

### 2026-10-14 五步演示原生窗口验收

环境刷新后重新启动示例，原生窗口逐项点击五步。实际看到 Tab 1/Tab 2
独立存在，第三步选中 Tab 1 且计数变为 1，第四步选中 Tab 2 并显示
`Submitted: Hello from CLI`，第五步显示 0 个标签。随后一键自动播放，
最终五步全部标记完成、所有页面关闭，截图布局无错位。

动作演示计划归档到 completed；用户关注的 main+CLI+private 已作为 app
唯一展示路径，其他参考接入保留在代码。没有提交或发布。

### 2026-10-14 首版交付评审

用户授权先提交、推送并创建 MR，再继续可选鼠标和内侧渐变操作反馈。
首版提交前 `make ci` 通过，既有 18 项真实 Electron smoke 和原生五步验收
记录保持有效。源码与文档纳入评审，未包含本机二进制、浏览器数据或凭据。
