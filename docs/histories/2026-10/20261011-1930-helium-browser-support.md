## [2026-10-11 19:30] | Task: 支持 Helium 浏览器

### 📥 User Query

> 在 macOS 上使用 Helium 时，`open-browser-use profiles` 无法列出 Helium profile，并且因无法读取遗留的 Chrome 数据目录而报错。

### 🛠 Changes Overview

**Scope:** `cmd/open-browser-use/`、`skills/open-browser-use/`、架构与发布文档

**Key Actions:**

- **[Helium root]**: macOS browser roots 新增 `helium`（`~/Library/Application Support/net.imput.helium`），因此 `profiles`、`--browser helium` selector、extension instance 反查和 `install-manifest --browser helium` 都覆盖 Helium。
- **[Unreadable roots]**: `profiles` 扫描时跳过权限被拒的 browser root 并向 stderr 输出 warning，避免一个受保护目录让其他浏览器的 profile 全部不可见，同时不污染 `--json` 输出。
- **[Setup target]**: macOS 上 `setup` 打开 Chrome Web Store 或 `chrome://extensions/` 时使用所选 browser 的 app（如 Helium、Google Chrome Beta），不再固定打开 Google Chrome。
- **[Docs]**: 同步 Skill、安装参考、架构说明和发布记录。

### 🧠 Design Intent (Why)

Helium 是 Chromium 分支，user-data 布局与 Chrome 相同，只需新增 root 即可复用现有 CRX/unpacked 检测、`Local State` 展示名与 LevelDB instance 反查。Helium 会先读自己的 `NativeMessagingHosts`，再回退到 Chrome 的用户级目录，所以 `install-manifest --browser helium` 写入 Helium root 即可，不依赖 Chrome 目录存在。macOS 可能拒绝读取已卸载浏览器遗留的数据目录（`operation not permitted`），此时跳过该 root 与目录不存在的处理一致。

### 📁 Files Modified

- `cmd/open-browser-use/main.go`
- `cmd/open-browser-use/main_test.go`
- `skills/open-browser-use/SKILL.md`
- `skills/open-browser-use/references/installation.md`
- `docs/ARCHITECTURE.md`
- `docs/releases/feature-release-notes.md`

### ✅ Verification

- `make ci`
- 新增回归测试：`--browser` 选择对应的 macOS app；Helium profile 出现在多浏览器列表中；Chrome root 不可读时仍返回 Helium profile（移除权限跳过后该测试失败）；`install-manifest --browser helium` 写入 Helium 的 `NativeMessagingHosts`。
- 现场 macOS + Helium：`profiles --connected` 列出 `Helium Default (Personal)` 为 connected、`Profile 1 (Work)` 为 installed；`info --browser helium --profile Personal` 连通，`--profile Work` 正确报告未连接。
