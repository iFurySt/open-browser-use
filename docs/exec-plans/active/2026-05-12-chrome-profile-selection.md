# Chrome Profile 选择能力

## 目标

让上层 runtime（CLI、SDK、MCP、agent skill）能够：

1. 发现本机有哪些 Chrome profile 装了 Open Browser Use 插件。
2. 在调用时显式指定使用哪个 profile，类似现在 `--session-id` 区分逻辑 session 的方式。
3. agent skill 在多 profile 场景下主动询问用户，并在整个任务里持续使用同一个 profile。

## 范围

- 包含：
  - CLI 新子命令：列出已安装插件的 profile（候选名 `obu profiles`）。
  - 所有用户面入口新增 `--profile` 参数：CLI 子命令、`obu mcp` 启动参数、JS/Python/Go SDK client options。
  - native host `active.json` 改造为按 profile 索引的 registry，多 profile 同时运行时能各自维护 socket。
  - MV3 extension 上报自身所在的 profile 标识（目录名 + 展示名），写入 socket registry，便于 host/CLI 路由。
  - `skills/open-browser-use/` 补充多 profile 处理指引。
  - 三个 SDK 同步暴露 `listProfiles()` 风格的 API 和 `profile` 构造参数。
- 不包含：
  - 第一版不"按需唤起"未启动的 profile；用户需要先在目标 profile 里手动启动 Chrome 并启用插件。
  - 不做跨任务的 profile 偏好记忆（不写入用户级配置）；每个任务由 skill 现场询问。
  - 不支持非 Google Chrome 的 Chromium 变体（Edge/Brave/Arc 等），这部分留到后续 plan。

## 背景

- 相关文档：
  - `docs/ARCHITECTURE.md`（Chrome route 拓扑、native messaging、active.json 行为）
  - `skills/open-browser-use/`（需要更新的 agent 指引）
- 相关代码路径：
  - `cmd/open-browser-use/main.go`：`chromeProfileDirs`、`detectInstalledChromeExtensionByID`、`active.json` 写入逻辑、Cobra 子命令注册。
  - `internal/host/`：native host stdio ↔ socket relay。
  - `apps/chrome-extension/`：MV3 service worker，需要补一个上报当前 profile 标识的入口。
  - `packages/open-browser-use-js`、`...-python`、`...-go`：SDK client options 和高层 API。
- 已知约束：
  - 一次 `chrome.runtime.connectNative` 只对应一个 profile，extension 没办法跨 profile 操作。因此 `--profile` 实际作用在 host 端"连哪条 socket"上，而不是 extension 内部的字段。
  - Chrome `Local State` 文件提供 profile 展示名（`profile.info_cache[<dir>].name`），路径在 macOS 为 `~/Library/Application Support/Google/Chrome/Local State`，Linux 为 `~/.config/google-chrome/Local State`。
  - `active.json` 当前是单 entry；改造时要兼顾"老 CLI/SDK 读到新 registry 不要炸"。

## 风险

- 风险：socket registry 结构变更可能破坏正在运行的旧版 host / SDK。
  - 缓解：新结构包一层 `{ "version": 2, "profiles": { ... }, "active": "<dir>" }`，保留顶层 `socket` 字段指向 active profile 的 socket，让旧客户端继续可用。
- 风险：profile 展示名包含特殊字符（emoji、空格、中文），在 CLI/MCP 里直接当 ID 不稳定。
  - 缓解：对外稳定 ID 用目录名（`Default`、`Profile 1`），展示名只作为辅助显示。
- 风险：用户同时启动多个 profile 的 Chrome 时，native host 进程会有多份，每份各自写 registry，存在竞争。
  - 缓解：写入 registry 时用文件锁（`flock`/`LockFileEx`），并以 profile 目录名为唯一键。
- 风险：扫描 profile 列表会读 `Local State`，可能因为 Chrome 正在写而读到不一致内容。
  - 缓解：失败时只返回目录名，不致命；展示名标记为 unknown。

## 里程碑

1. **M1 — Profile 发现原子能力**（已完成）
   - 抽出 `listInstalledProfiles()` 函数，复用 `chromeProfileDirs` + 插件检测，附加 `Local State` 展示名解析。
   - 新增 `obu profiles` CLI 子命令（输出 JSON + 人类可读两种格式）。
   - 单元测试覆盖：单 profile、多 profile、`Local State` 缺失、`Local State` 损坏、目标 profile 没装插件、unpacked 检测、CRX vs unpacked 取版本较高者。

2. **M2 — 通过 `extensionInstanceId` 做 profile ↔ socket 反查**
   - 复用 extension 已有的 `extensionInstanceId`（写在 `chrome.storage.local`，每个 profile 一份；`getInfo` 已经返回它）。
   - 新增 `resolveProfileForInstanceID(extensionID, instanceID)`：扫描每个已知 profile 的 `<profile>/Local Extension Settings/<extension-id>/*.log` + `*.ldb`，grep 这个 UUID 字符串，匹配上的 profile 就是该 host 对应的 profile。
   - 单元测试用伪造的 LevelDB log 文件覆盖：匹配命中、多 profile 唯一匹配、找不到。

   > 调整说明：原方案打算升级 `active.json` 为 per-profile registry，需要 host 启动时就知道自己是哪个 profile。但 Chrome extension API 不暴露 profile 目录名，外加 `active.json` 的并发写入需要文件锁，代价偏高。新方案利用 extension 已有的 instanceId 做反向解析，不动 host 也不动 `active.json` 格式，复杂度大幅下降；后续真出现多 host 并发瓶颈时再升级 registry。

3. **M3 — Extension 上报 profile 身份**（绝大部分已自由）
   - `getInfo` 已经返回 `metadata.extensionInstanceId`（来源：`chrome.storage.local.extensionInstanceId`，首次启动时由 `crypto.randomUUID()` 生成）。
   - 本里程碑只需要补充：documentation + 必要的话在 `getInfo` 中附带 `chrome.runtime.id`（已有 `extensionId`）和 manifest version。
   - 不需要新增 `identity` 权限，不需要扩展 handshake 协议。

4. **M4 — CLI / SDK / MCP 接入 `--profile`**
   - CLI：在 root persistent flag 加 `--profile`；socketOptions 携带 selector。
   - 连接策略：扫描 `socket-dir` 下所有 `.sock`，对能连通的逐个 `getInfo`，把 `metadata.extensionInstanceId` 反查到 profile directory + displayName；按 selector（接受 directory 名 或 displayName，case-insensitive）挑出匹配的 socket。
   - 找不到匹配时给清晰错误：列出当前连得通的 profile 列表 + 安装但没连接的 profile 列表，提示用户先打开对应 Chrome profile。
   - `obu mcp --profile`：server 启动时锁定 selector，每次工具调用复用同一个解析结果（带 1 分钟缓存）。
   - SDK：构造函数接 `profile` 选项。Python/JS/Go 复用同一查找逻辑。
   - `obu profiles --connected` / `obu info`：附加显示当前已连通的 profile（directory + displayName + instanceId）。

5. **M5 — Skill 指引**
   - 在 `skills/open-browser-use/SKILL.md` 增加 "Multi-profile handling" 章节：先 `obu profiles --connected` → 多于 1 个 connected target 才问用户 → 任务内持续使用。
   - 给出"用户已在 prompt 里指定 profile 名称"时的快路径，不必再问。

6. **M6 — 文档与发布**
   - 更新 `docs/ARCHITECTURE.md` 中 profile 选择行为描述。
   - `docs/releases/` 加一条 user-facing changelog。
   - history 记录到 `docs/histories/`。

## 验证方式

### 命令（自动化）

- `go test ./cmd/open-browser-use/... ./internal/host/...` 必须包括新加的 profile registry / 列表测试。
- `pnpm -F open-browser-use-sdk test`（JS SDK）覆盖 `profile` option 和 `listProfiles()`。
- Python SDK：`pytest packages/open-browser-use-python` 覆盖同上。
- Go SDK：`go test ./packages/open-browser-use-go/...` 覆盖同上。
- 仓库级 lint / typecheck 流水线照常通过。

### 手工 test case

下列用例需要本机至少装好 2 个 Chrome profile（`Default` + `Profile 1`），并且**在两个 profile 里都启用了 Open Browser Use 插件**。如果只有一个 profile，先在 Chrome 里"添加用户"造一个，然后在新 profile 中安装 / 启用插件。

记号：`OBU=./cmd/open-browser-use/open-browser-use` 假定已编译。

1. **TC-1 单 profile：行为不回退**
   - 关闭 `Profile 1` 的 Chrome 窗口，只留 `Default` 运行。
   - 运行 `obu profiles --connected` → 期望仍列出两个已安装 profile，但只有 `Default` 标记为 connected。
   - Agent 直接选择并固定使用 `Default`，不因已关闭的 `Profile 1` 打断用户确认。
   - 运行 `obu tabs --profile Default` → 期望能列 tabs。

2. **TC-2 多 profile：列表正确**
   - 同时启动 `Default` 和 `Profile 1` 两个 Chrome 窗口。
   - 运行 `obu profiles --json` → 期望返回 2 条：每条包含 `directory`、`displayName`、`extensionId`、`version`、`source`。
   - 运行 `obu profiles --connected --json` → 同上，额外多出 `connected: true`、`socketPath`、`instanceId`。
   - 展示名应该和 Chrome 头像菜单里看到的一致。

3. **TC-3 显式指定 profile**
   - `obu --profile "Profile 1" tabs` 返回 `Profile 1` 的 tab 列表；和 `obu --profile Default tabs` 结果**不重叠**（前提是两个 profile 打开了不同的网站，准备验证时人为造一组差异）。

4. **TC-4 指定未安装插件的 profile**
   - 找一个没装插件的 profile（或临时禁用 `Profile 1` 的扩展）。
   - `obu --profile "Profile 1" tabs` → 期望以非零退出码返回，错误信息明确说"profile 'Profile 1' 没有运行 Open Browser Use 插件"。

5. **TC-5 指定不存在的 profile 目录**
   - `obu --profile "Profile 999" tabs` → 错误信息提示该 profile 不存在，并在 stderr 列出可用 profile。

6. **TC-6 registry 向后兼容**
   - 手工把 `active.json` 改回 v1 格式（只有 `socket` 字段），跑当前发布版的 `obu tabs` → 应当仍能工作。
   - 然后跑新版 `obu host`（任意一个 profile 启动会重写）→ registry 升级到 v2，老 socket 字段仍然指向 active profile。

7. **TC-7 多 host 并发**
   - 让两个 profile 的 Chrome 都连通 native host（各自有 host 进程）。
   - 反复 `obu profiles` 10 次 → 列表稳定，没有缺失或重复条目。
   - 关掉一个 profile 的 Chrome → 下次 `obu profiles` 该 profile 从列表消失，对应 socket 文件被清理。

8. **TC-8 MCP server 锁定 profile**
   - `obu mcp --profile "Profile 1"` 启动后，发任意 tool call（如 `tabs`）→ 永远命中 `Profile 1`，无论 `Default` 上发生什么。
   - 试图通过 tool call 参数覆盖 profile（如果旧 schema 允许）→ 应当被忽略或报错。

9. **TC-9 SDK profile 参数**
   - JS：`new Client({ profile: "Profile 1" })`，然后 `client.listTabs()` → 命中正确 profile。
   - Python / Go 等价用例。
   - 不传 profile 时：行为和现在一致。

10. **TC-10 Skill 行为（人工跑 agent）**
    - 在装了 skill 的 Claude Code / Codex 里发任务："帮我看一下当前打开的 tabs"。
    - 单 profile 机器上：agent 不询问，直接执行。
    - 多 profile 机器上：agent 先调 `listProfiles`，向用户列出选项，等用户选择，后续调用都带 `--profile`。
    - 同一个会话里第二次问 tab 相关任务时，agent 应当继续使用上次选定的 profile，不再重新问。

11. **TC-11 展示名包含特殊字符**
    - 把某个 profile 改名为 `工作 🧪 / test`。
    - `obu profiles` 输出能正确显示该名称（JSON 中是 escape 后的 UTF-8）；`--profile` 仍用目录名传入，无需处理转义。

### 观测检查

- 启动 native host 时日志要打印：当前 profile 目录名、展示名、socket 路径、extension id。
- 写 registry 失败要打 warning（且不阻塞 host 主流程）。

## 进度记录

- [x] M1 Profile 发现原子能力（2026-05-12 完成）
  - 新增 `listInstalledChromeProfiles()`，覆盖 CRX 安装和 `Secure Preferences` 里登记的 unpacked 安装两条路径。
  - 新增 `obu profiles` 子命令，支持 `--json` 输出。
  - 测试：单 profile / 多 profile 排序 / 跳过未装插件的 profile / 缺失 `Local State` / 损坏 `Local State` / unpacked 检测 / CRX vs unpacked 版本择优 / CLI JSON / CLI human / CLI 空表 / 排序 key。
  - 实机回归（5 profile，beta 装在 Default 和 Profile 3）：CLI 正确列出 2 条 `0.1.29` 记录。
- [x] M2 通过 `extensionInstanceId` 做 profile ↔ socket 反查（2026-05-12 完成）
  - 新增 `resolveProfileForInstanceID(extensionID, instanceID)`，对每个 profile 的 `Local Extension Settings/<ext-id>/*.{log,ldb}` byte-grep UUID。
  - 单测覆盖：命中、找不到、空参数、与 displayName 联合返回。
- [x] M3 Extension 上报 profile 身份（2026-05-12 自动满足）
  - 现有 `getInfo` 已经返回 `metadata.extensionInstanceId`（chrome.storage.local 中的 per-profile UUID）。
  - 不需要修改 extension 代码或申请新权限。
- [x] M4 CLI / MCP 接入 `--profile`（2026-05-12 完成）
  - `socketOptions` 增加 `profile` 字段，`addSocketFlags` 同步注册 `--profile`。
  - 新增 `dialBrowserSocketForProfile` / `pickSocketForProfile` / `verifySocketMatchesProfile`：枚举 socket 目录、对每个 socket `getInfo` 后通过 instanceID 反查 directory，按 selector（directory/displayName/instanceId，case-insensitive）挑出匹配 socket。
  - `obu profiles --connected` 与 `--json` 输出新增 `connected` / `socketPath` / `instanceId` 字段。
  - 错误信息列出当前连接的 profile，提示用户开对应 Chrome。
  - `obu mcp --profile` 通过共享 `socketOptions` 自动生效，每次工具调用复用 server 启动时的 selector。
  - SDK profile 选项延期；CLI/MCP 已覆盖 skill 主要用例。
  - 实机回归（Default/Eva 与 Profile 3/cookiy.com 同时在线）：
    - 无 `--profile`：连到 active socket（Profile 3）。
    - `--profile Default` / `--profile Eva` 路由到 Default 的 socket（instance c68f12f2…）。
    - `--profile "Profile 3"` / `--profile cookiy.com` 路由到 Profile 3 的 socket（instance ceb59c8d…）。
    - `--profile "Profile 999"`：非零退出，提示当前已连接的两个 profile。
    - TC-3 验证 user-tabs 在两个 profile 上返回不同的 tab 列表（Default=1, Profile 3=9）。
- [x] M5 Skill 指引（2026-05-12 完成）
  - `skills/open-browser-use/SKILL.md` 新增 "Multi-profile handling" 章节，覆盖 1/N profile 分支、selector 接受 directory/displayName、错误处理、MCP 启动锁定、不跨任务记忆 profile 选择。
- [ ] M6 文档与发布
  - 待补：`docs/ARCHITECTURE.md` profile 选择段落 + 用户面 changelog + history。

## 决策记录

- 2026-05-12：对外稳定 profile ID 用 Chrome profile 目录名（`Default`、`Profile 1`），展示名只用于辅助显示。理由：目录名稳定，展示名可被用户改名导致 ID 漂移。
- 2026-05-12：第一版不支持"按需唤起未启动的 profile"。理由：跨平台启动一个未运行的 Chrome profile 行为复杂且和 host detection 链路耦合，先把多 profile 的"被动发现 + 显式选择"做扎实。
- 2026-05-12：`obu mcp` 在启动参数锁定 profile，而不是每个 tool call 都接收 profile。理由：和 skill 的"一个任务一个 profile"语义一致，避免一次会话里 agent 跳来跳去。
- 2026-05-12：`active.json` 升级为 v2 结构，但保留 v1 顶层 `socket` 字段，确保旧 CLI/SDK 还能连。
- 2026-05-12：profile 检测必须同时覆盖 CRX 和 unpacked 两条路径。实机发现 beta 用户的扩展只出现在 `<Profile>/UnpackedExtensions/` 下，并由 `Secure Preferences.extensions.settings.<id>.path` 指向；只扫 `Extensions/<id>/<ver>` 会完全漏掉这些 profile。
- 2026-05-12：放弃原本计划的 `active.json` v2 多 profile registry。原因：Chrome extension API 不暴露 profile 目录名，host 启动时无法可靠自报身份，加上文件锁/并发写入的复杂度收益小。改用现有 `extensionInstanceId`（chrome.storage.local 里的 per-profile UUID，`getInfo` 已经返回）+ 在 CLI 侧 byte-grep 每个 profile 的 `Local Extension Settings/<ext-id>/*.{log,ldb}` 反查 directory。这条路径完全无需改 host 或 extension，靠扫 socket 目录 + getInfo 即可。后续真出现多 host 高频并发场景再回头补 registry。
