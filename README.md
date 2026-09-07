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
> Interested in Computer Use? Check out [open-computer-use](https://github.com/iFurySt/open-codex-computer-use).

`open-browser-use` is a browser automation layer that stays neutral across
agent runtimes. It is also an open-source alternative to the Chrome Browser Use
capability recently shipped in Codex.app. For the story behind it, see the
[Browser Use Deep Dive](https://www.ifuryst.com/en/blog/2026/open-browser-use/).

Under the hood, it pairs a browser extension with the `open-browser-use` CLI.
You can integrate it through the JavaScript SDK, Python SDK, Go SDK, or the CLI.

https://github.com/user-attachments/assets/bcfba878-f6a8-44b9-b84b-29c7e0285687

## Quick Start

```bash
npm i -g open-browser-use
open-browser-use setup
```

### Install the CLI

```bash
# npm (macOS, Linux, Windows)
npm i -g open-browser-use

# Homebrew (macOS, Linux)
brew tap iFurySt/open-browser-use && brew install open-browser-use

# Upgrade
npm update -g open-browser-use
```

### Set Up Chrome

Register the native host for the extension. The setup command also opens the
Chrome Web Store page so you can install or enable the matching Chrome
extension, then restart Chrome if needed.

```bash
open-browser-use setup
```

If the Chrome Web Store item is temporarily unavailable, use
`open-browser-use setup beta` to prepare a keyed release ZIP for manual
installation from `chrome://extensions/`.

### Use It

#### SDK

```bash
# JavaScript / TypeScript
npm install open-browser-use-sdk

# Python
pip install open-browser-use-sdk

# Go
go get github.com/ifuryst/open-browser-use/packages/open-browser-use-go
```

The SDK package name is `open-browser-use-sdk` on both npm and PyPI. Python
code imports it as `open_browser_use`; Go code imports the SDK package as
`obu`.

#### Skill

Install the skill directly:

```bash
# Install for Codex
npx skills add iFurySt/open-browser-use -g -a codex --skill open-browser-use -y
npx skills ls -g -a codex | rg 'open-browser-use'
codex exec --skip-git-repo-check "Use open-browser-use to check today’s Hacker News and summarize the most worth-reading posts."

# Install for Claude Code
npx skills add iFurySt/open-browser-use -g -a claude-code --skill open-browser-use -y
```

Update an existing global install, including the Codex install created above:

```bash
npx skills update open-browser-use -g -y

# `upgrade` is an alias for `update`
npx skills upgrade open-browser-use -g -y
```

You can also manually download and install the
[`open-browser-use` skill](./skills/open-browser-use), then start using it from
your agent.

Downloadable `.skill` and `.zip` packages are available in
[GitHub Releases](https://github.com/iFurySt/open-browser-use/releases).

#### MCP

Install the MCP server into all supported global agent configs:

```bash
npx add-mcp "obu mcp" --name open_browser_use --all -g -y
npx add-mcp list -g
```

You can also configure an agent runtime with local MCP stdio support manually:

```toml
[mcp_servers.open_browser_use]
command = "obu"
args = ["mcp"]
```

The server exposes browser tools for tab listing, opening, claiming,
navigation, CDP, action plans, and cleanup.

## License

[MIT](./LICENSE)
