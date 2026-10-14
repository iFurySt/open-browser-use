# Electron IAB 多 tab 与通信鉴权实施计划

## 目标

交付 open-browser-use-electron、obu iab 命令、宿主私有通道和带 peer/lease
鉴权的 socket，并在 examples/electron-iab 中展示完整接入和本地验证。

## 范围与边界

- 命名 profile、会话多 tab、展示绑定、CDP 和生命周期。
- 私有继承通道与明确 scope 授权，不创建默认监听 socket。
- macOS/Linux 原生内核 peer 身份、宿主登记进程、短期 lease、双向验证和撤销。
- Windows socket 缺少原生安全 adapter 时拒绝启动；私有 Node/Go 描述符通道仍待实机验证。
- IAB CLI 必须由宿主交付私有 bootstrap，不能依赖 argv/env/registry 中的秘密。
- 保留现有 Chrome route；不在本轮迁移其协议和鉴权。

## 实施方式

一个 Electron 发布包，包内拆 runtime、transport、client 和原生 peer adapter。
Node-API 读取内核连接身份；默认登记宿主启动的具体进程实例，不仅匹配 UID
或可执行路径。Go IAB 命令对同一 framing 和 bootstrap 做独立实现。
Electron 示例使用本地测试网站，不依赖真实登录、Chrome 或外部业务服务。

## 风险与验证

- 进程身份必须防 PID 复用；平台不能可靠读取时失败关闭。
- 未认证帧、scope/target 越权、过期撤销、事件泄漏和异常 frame 都需要测试。
- 页面、listener、socket、等待者必须在退出时清理。
- Electron UI 在本机真实启动；smoke 验证多 tab、共享 profile、输入、三个
  接入姿势和 native peer 拒绝。跨平台未实测部分明确记录。

## 进度

- [x] 读取当前设计、协议、CLI 与仓库约定。
- [x] 实现 runtime 与多 tab 管理。
- [x] 实现 native peer、private/socket transport 与鉴权。
- [x] 实现 JS client 和 obu iab 命令。
- [x] 实现 examples/electron-iab 和可重复 smoke。
- [x] 本地运行检查、安全测试、Electron smoke、make ci。
- [x] 同步架构、安全、发布、history 和安装说明，归档计划。

## 决策

- 2026-10-13：首轮 socket 支持 macOS/Linux，按宿主登记的进程实例授权。
  Windows 不以无鉴权 named pipe 兼容。示例目录采用用户指定的 examples/。

- 2026-10-13：真实 Electron 验证显示，never-presented view 首次鼠标点击需要
  先生成 compositor 命中测试数据；runtime 通过内部截图准备一帧，不展示或
  聚焦 tab。该步骤后重新检查授权，新增撤销回归测试。

## 完成结果

交付主进程模块与 Node client、Go IAB namespace、默认 FD 3 私有通道、
macOS/Linux 原生 peer + lease socket 和五种接入姿势的独立 Electron 示例。
`make ci`、typecheck、16 项 IAB package 测试、12 项真实 Electron smoke、
Go Linux/Windows 交叉编译和 npm pack 均通过。本地矩阵为 Electron 44.7.0 /
macOS arm64；Linux runtime、Windows private descriptor、IME/iframe/最小化
及签名发布宿主留待后续验证。npm 发布不属于本轮授权范围。

默认拒绝 global/child-target CDP、文件、下载、clipboard 与浏览器权限。
鉴权/事件/期限/撤销在实际运行与安全测试中覆盖，现有 Chrome route 不迁移。
初轮原生 UI 手动点按因锁屏暂未完成；用户随后解锁，补验结果见下文。
后续债务已记入 `docs/exec-plans/tech-debt-tracker.md`。

## 示例 UI 后续调整

用户反馈展示背后露出文字后，移除实现标签，改为右侧带横向 tabs 和地址栏
的浏览器面板。支持新建/关闭、历史/刷新、隐藏/恢复；native viewport 使用
实际 DOM 测量并由主进程验证界限。真实 Electron smoke 扩展为 15 项，全部
通过，包括两种窗口尺寸下的原生/DOM bounds 一致性与 UI IPC 拒绝路径。
包测试仍为 16 项并全部通过。

用户解锁后补齐原生窗口验收：实际截图确认面板/网页无错位和背景露字，
手动验证 private 按钮、tabs 新建/切换/关闭、隐藏/恢复、地址栏跳转、
后退/前进、页面点击和刷新（计数 1 → 0）。无需额外运行时代码修复。
