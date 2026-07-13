# Open Browser Use

[![English](https://img.shields.io/badge/English-Click-yellow)](./README.md)
[![简体中文](https://img.shields.io/badge/简体中文-点击查看-orange)](./README.zh-CN.md)
[![Release](https://img.shields.io/github/v/release/iFurySt/open-browser-use)](https://github.com/iFurySt/open-browser-use/releases)
[![npm SDK](https://img.shields.io/npm/v/open-browser-use-sdk?label=npm%20SDK)](https://www.npmjs.com/package/open-browser-use-sdk)
[![PyPI SDK](https://img.shields.io/pypi/v/open-browser-use-sdk?label=PyPI%20SDK)](https://pypi.org/project/open-browser-use-sdk/)
[![Go SDK](https://pkg.go.dev/badge/github.com/ifuryst/open-browser-use/packages/open-browser-use-go.svg)](https://pkg.go.dev/github.com/ifuryst/open-browser-use/packages/open-browser-use-go)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/iFurySt/open-browser-use)

---

> [!TIP]
> 对 Computer Use 感兴趣的话，可以看看 [open-computer-use](https://github.com/iFurySt/open-codex-computer-use)。

`open-browser-use` 是平台中立的浏览器操作方案，也是Codex.app最新发布的Chrome Browser Use的开源替代方案。背后的故事可以看这篇[Browser Use详解](https://www.ifuryst.com/blog/2026/open-browser-use/)文章。 技术方案采用的是浏览器插件和open-browser-use cli结合，可通过 JS/Python/Go SDK 或 CLI 的方式接入。

https://github.com/user-attachments/assets/bcfba878-f6a8-44b9-b84b-29c7e0285687

## Quick Start
第一次安装可以无脑运行：
```bash
npm i -g open-browser-use
open-browser-use setup
```

### 安装 CLI
```bash
# npm安装（macOS、Linux、Windows）
npm i -g open-browser-use

# Homebrew安装（macOS、Linux）
brew tap iFurySt/open-browser-use && brew install open-browser-use

# 升级
npm update -g open-browser-use
```

### 配置 Chrome
注册绑定到该插件的 native host。setup 也会打开 Chrome Web Store 页面，你需要在
页面里安装或启用对应的浏览器插件，必要时再重启 Chrome。

```bash
open-browser-use setup
```

如果 Chrome Web Store 暂时不可用，可以运行 `open-browser-use setup beta`
准备带固定 key 的 release ZIP，再到 `chrome://extensions/` 手动安装。

### 配置 Zen Browser

从匹配版本的 [GitHub Release](https://github.com/iFurySt/open-browser-use/releases)
下载 `open-browser-use-zen-extension-<version>.xpi`。在 Zen 打开
`about:debugging#/runtime/this-firefox`，选择“临时载入附加组件”并选中 XPI，
然后注册 Firefox native messaging host：

```bash
open-browser-use install-manifest --browser zen
open-browser-use profiles --connected
open-browser-use info --browser zen
```

Zen 基于 Firefox，不提供 Chrome extension debugger API。当前支持 tab、history、
navigation、page info、JavaScript evaluation、download、clipboard、cursor 和
session cleanup；不支持任意 Chrome-only CDP method 与本地 file chooser path 注入。

### 使用

#### SDK

```bash
# JavaScript / TypeScript
npm install open-browser-use-sdk

# Python
pip install open-browser-use-sdk

# Go
go get github.com/ifuryst/open-browser-use/packages/open-browser-use-go
```

SDK 在 npm 和 PyPI 上统一叫 `open-browser-use-sdk`。Python 代码里的 import
模块名保持 `open_browser_use`；Go 代码通常把 SDK import 为 `obu`。

#### Skill

一键安装 skill：

```bash
# 安装到codex
npx skills add iFurySt/open-browser-use -g -a codex --skill open-browser-use -y
npx skills ls -g -a codex | rg 'open-browser-use'
codex exec --skip-git-repo-check "用open-browser-use查看下今天Hacker News有什么值得关注的"

# 安装到claude code
npx skills add iFurySt/open-browser-use -g -a claude-code --skill open-browser-use -y
```

更新已有的全局安装，包括上面安装到 Codex 的那份：

```bash
npx skills update open-browser-use -g -y

# `upgrade` 是 `update` 的别名
npx skills upgrade open-browser-use -g -y
```

也可以手动下载`open-browser-use`的[skill](./skills/open-browser-use)并安装，就可以愉快的开始使用了🚀

*[GitHub Releases](https://github.com/iFurySt/open-browser-use/releases)里有可下载的.skill/.zip包*

#### MCP

一键安装 MCP server 到所有支持的全局 Agent 配置：

```bash
npx add-mcp "obu mcp" --name open_browser_use --all -g -y
npx add-mcp list -g
```

也可以手动配置到支持本地 stdio MCP 的 Agent runtime：

```toml
[mcp_servers.open_browser_use]
command = "obu"
args = ["mcp"]
```

这个 MCP server 会暴露标签页列表、打开/接管标签页、导航、CDP、action plan
和清理等浏览器工具。

## License

[MIT](./LICENSE)
