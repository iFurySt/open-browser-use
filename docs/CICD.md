# CI/CD 说明

这个模板自带一套不依赖具体语言栈的 CI/CD 骨架。

## 默认包含的内容

- `ci.yml`：仓库级检查，覆盖 docs、repo hygiene、action pinning、extension
  打包、脚本语法、Go 测试、JS/TypeScript package 测试和 Python SDK smoke。
- `release.yml`：tag `v*` 推送或手动触发的 release 流水线，用来打包仓库级制品、
  CLI 预编译 tarball、Chrome extension zip、内部 CRX evidence、Zen extension XPI、Open Browser Use skill 包、
  生成 provenance，并创建 GitHub Release；Release 页面暴露 extension zip/CRX
  和 skill 下载包，普通安装入口使用 zip/unpacked，其他 manifest、SBOM 和 repo
  metadata 留在 workflow artifact 里。手动触发时按输入参数可把
  extension 上传并提交到 Chrome Web Store；tag 推送时也可以通过 repository
  variable `CWS_AUTO_PUBLISH=true` 自动上传并提交商店审核。商店提交前会检查
  active submission，默认遇到审核中或 staged 的旧提交时跳过 CWS 上传但不让
  release workflow 因此失败；需要替换旧审核版本时显式开启 cancel pending
  submission。新建 GitHub Release 时使用
  `gh release create --generate-notes`，交给 GitHub 自动生成 `What's Changed`、
  `New Contributors` 和 `Full Changelog`。
- `npm-publish.yml`：tag `v*` 推送触发的 npm 发布流水线，使用 npm
  trusted publishing/OIDC 发布 `packages/open-browser-use-cli` 和
  `packages/open-browser-use-js`。`open-browser-use` 是二进制 CLI 分发入口，
  `open-browser-use-sdk` 是 JavaScript/TypeScript SDK，不发布 Chrome
  extension。
- `pypi-publish.yml`：tag `v*` 推送或手动触发的 PyPI 发布流水线，使用 PyPI
  trusted publishing/OIDC 发布 `packages/open-browser-use-python`，distribution
  名为 `open-browser-use-sdk`，Python import 模块名仍是 `open_browser_use`。
- `homebrew-publish.yml`：tag `v*` 推送触发的 Homebrew tap 更新流水线，等待
  GitHub Release 中的 CLI 预编译 tarball 出现后，用真实 asset sha256 渲染
  `open-browser-use` binary formula 并推送到 `iFurySt/homebrew-open-browser-use`。
- `chrome-web-store-publish.yml`：手动触发的 Chrome Web Store 发布流水线，
  从已有 GitHub Release 下载 extension zip，再上传并可选提交审核；用于
  release 已经创建、只需要补跑商店发布的场景。

## 设计原则

这套默认流水线的目标，是在项目真正成形前先把交付链路搭起来，而不是假装已经知道未来项目该怎么 build 和 deploy。

当新项目的技术栈确定后，你应该把 `scripts/release-package.sh` 里的占位打包逻辑替换成真实构建产物，而不是另起一套平行流程。

所有 GitHub Actions 都已经 pin 到 commit SHA。后续升级 action 时，也要继续保持这个约束。

## 推荐接入顺序

1. 保留 `ci.yml`，作为唯一默认常驻的仓库基础门禁。
2. 在 `scripts/ci.sh` 里继续叠加项目自己的验证命令。
3. 用真实构建产物替换 `scripts/release-package.sh`。
4. CLI 和 JS SDK 的 npm 发布走 `npm-publish.yml`，两个包的 trusted
   publisher 都要配置为 `iFurySt/open-browser-use` + `npm-publish.yml`。
5. Python SDK 的 PyPI 发布走 `pypi-publish.yml`。首次发布前在 PyPI 账号
   Publishing 页面创建 pending trusted publisher：project name
   `open-browser-use-sdk`，owner `iFurySt`，repository
   `open-browser-use`，workflow `pypi-publish.yml`，environment `pypi`。
6. Homebrew 发布走 `homebrew-publish.yml`，需要仓库 secret
   `HOMEBREW_TAP_TOKEN` 能写入 tap repo。
7. 浏览器插件发布走 `docs/CHROME_WEB_STORE_RELEASE.md` 里的 Chrome Web
   Store API v2 流程；需要每个 tag 自动提交时，再开启 `CWS_AUTO_PUBLISH=true`。
   如果要让最新 tag 取消并替换审核中的旧版本，再开启
   `CWS_CANCEL_PENDING_SUBMISSION=true`。
8. 技术栈和环境稳定后，再补其他部署 job 或非阻塞的依赖巡检。
9. 即使交付方式变化，release 阶段的 SBOM 和 provenance 这类供应链能力也建议保留。

## 默认 release 产物

当前 GitHub Release 页面面向用户暴露：

- `open-browser-use-cli-<version>-darwin-amd64.tar.gz`
- `open-browser-use-cli-<version>-darwin-arm64.tar.gz`
- `open-browser-use-cli-<version>-linux-amd64.tar.gz`
- `open-browser-use-cli-<version>-linux-arm64.tar.gz`
- `open-browser-use-chrome-extension-<version>.zip`
- `open-browser-use-chrome-extension-<version>.crx`
- `open-browser-use-zen-extension-<version>.xpi`
- `open-browser-use-skill.zip`
- `open-browser-use.skill`

`open-browser-use-skill.zip` 和 `open-browser-use.skill` 都是
`skills/open-browser-use/` 的可安装 skill 包，解压后的顶层目录固定为
`open-browser-use/`。`.skill` 文件和 zip 内容一致，只是使用 agent skill
installer 更容易识别的扩展名。

workflow 还会在 `release-evidence` artifact 中保留内部追溯材料：

- `release-manifest.json`
- `repo-metadata.tgz`
- `cli/open-browser-use-cli-<version>-*.tar.gz`
- `chrome-extension/package-manifest.json`
- `chrome-extension/crx-manifest.json`
- `zen-extension/package-manifest.json`
- `skills/package-manifest.json`
- `sbom.spdx.json`
- 对 extension zip/CRX、Zen XPI 和 skill 包生成的 GitHub artifact attestation

也就是说，即使项目还没进入真实部署阶段，这个模板也已经把“可追溯的制品封装”这一步准备好了。
