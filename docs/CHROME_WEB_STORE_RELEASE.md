# Chrome Web Store 发布

这份文档记录 Open Browser Use Chrome extension 的正式商店打包、GitHub
Release 产物和 Chrome Web Store 自动提交审核流程。

## 当前发布边界

Chrome Web Store 只分发 `apps/chrome-extension/` 里的 MV3 extension 包。
宿主机二进制 `open-browser-use` 仍然需要用户通过项目安装包、脚本或二进制
单独安装到本机。npm 和 Homebrew 安装只提供 CLI，并提示用户显式运行
`open-browser-use setup`。

Chrome extension 不能自己安装 native messaging host manifest。正式扩展 ID 是
`bgjoihaepiejlfjinojjfgokghnodnhd`，已经作为 CLI 默认值写入。正式安装路径是：

```bash
open-browser-use setup
```

`setup` 会调用 native host manifest 注册，写入 Chrome External Extensions
JSON。只有检测到浏览器插件未安装或版本低于当前 CLI 期望版本时，CLI 才打开
Chrome Web Store 正式扩展页，引导用户手动安装或启用扩展；Chrome 仍可能要求用户
重启并确认启用扩展。需要只修复 native host 时，仍可运行：

```bash
open-browser-use install-manifest
```

Chrome Web Store 条目已经公开发布。只有在商店条目临时不可用、或需要测试非商店
安装路径时，才使用 GitHub Release zip 的 beta/manual fallback：

```bash
open-browser-use setup beta
```

`setup beta` 会下载最新 release zip，在本机 unpacked 目录和待拖入 Chrome 的
ZIP 中写入稳定 beta public key。CLI 会用该 beta id 注册 native host allowed
origin。只有检测到浏览器插件未安装或版本低于当前 CLI 期望版本时，CLI 才会打开
`chrome://extensions/`，同时在 Finder 或系统文件管理器中定位这个 zip。用户需要
打开 Developer mode，把这个 ZIP 拖到 Chrome 扩展页面完成手动安装。setup 还会在
本机已有可用 `npx skills` 时 best-effort 执行
`npx skills update open-browser-use -g -y`；未检测到 skills 时只展示 Codex 和
Claude Code 的 skill 安装命令。

已有本地 extension ZIP 时，首选直接运行：

```bash
open-browser-use setup --zip /absolute/path/to/open-browser-use-chrome-extension.zip
```

该命令跳过 release 下载，并复用相同的 key 写入、native host 注册和手动安装
引导。`setup beta --zip <path>` 继续作为兼容写法。`--zip` 支持的输入是 ZIP，不是
`.crx` 或证书文件；命令会把写入 stable key 后的内容原地重新打包到传入路径。

这条 fallback 会安装为 beta extension id，而不是 Chrome Web Store 的正式
extension id。它适合“CI 已发新版 release，但 Chrome Web Store 仍在审核新版”的
窗口期。审核通过后，用户应回到正式路径运行 `open-browser-use setup`，让 native
host manifest 重新指向 Chrome Web Store extension id。

`install-manifest` 会把 manifest 的 `path` 写成稳定 native host link：
`~/Library/Application Support/OpenBrowserUse/native-host/open-browser-use`，并
让该 link 指向当前安装的真实二进制。开发 unpacked extension 时，可以通过
`--extension-id <unpacked-extension-id>` 覆盖 allowed origin。

## 本地打包

```bash
pnpm package:chrome-extension
```

打包脚本会执行基础校验：

- `manifest_version` 必须是 3。
- `background.service_worker` 必须指向 `background.js`。
- manifest 必须包含 `nativeMessaging` 权限。
- manifest 必须声明 `16`、`32`、`48`、`128` 四个 PNG icons，并把 toolbar
  action icon 指向 `16` 和 `32` 图标。
- `background.js`、`content-cursor.js`、`popup.js` 必须通过 `node --check`。

输出文件：

```text
dist/chrome-extension/open-browser-use-chrome-extension-<version>.zip
dist/chrome-extension/open-browser-use-chrome-extension-<version>.crx
dist/chrome-extension/package-manifest.json
dist/chrome-extension/crx-manifest.json
```

`.zip` 是正式 Chrome Web Store 上传包，不写入 `manifest.key`，文件名保持为
`open-browser-use-chrome-extension-<version>.zip`。不要把 GitHub Release 上的原始
zip 当作普通用户直接拖入安装入口；如果要在审核窗口期使用最新 release，运行
`setup beta`，让 CLI 在本机把这个 zip 改写成带稳定 beta key 的手动安装包。
GitHub Release 不再发布单独的 `*-manual.zip` 或 `*-beta.zip`。`.crx` 会保留在
GitHub Release 里用于制品归档、Chromium/企业策略/自托管更新等场景；Chrome
Stable 对非 Web Store CRX 会报 `CRX_REQUIRED_PROOF_MISSING`，不要把它作为普通
用户安装入口。

`setup beta` 默认会用 CLI 内置的 beta public key 计算 extension id；如果需要
手工覆盖 native host allowed origin，可以运行：

```bash
open-browser-use setup beta --extension-id <extensionId>
```

## GitHub Release

推送 `v*` tag 或手动触发 `.github/workflows/release.yml` 后，GitHub Release
页面放六个 CLI 制品、两个 extension 制品和两个 agent skill 制品：

- `dist/cli/open-browser-use-cli-<version>-darwin-amd64.tar.gz`
- `dist/cli/open-browser-use-cli-<version>-darwin-arm64.tar.gz`
- `dist/cli/open-browser-use-cli-<version>-linux-amd64.tar.gz`
- `dist/cli/open-browser-use-cli-<version>-linux-arm64.tar.gz`
- `dist/cli/open-browser-use-cli-<version>-windows-amd64.zip`
- `dist/cli/open-browser-use-cli-<version>-windows-arm64.zip`

- `dist/chrome-extension/open-browser-use-chrome-extension-<version>.zip`
- `dist/chrome-extension/open-browser-use-chrome-extension-<version>.crx`
- `dist/skills/open-browser-use-skill.zip`
- `dist/skills/open-browser-use.skill`

skill 两种文件名内容一致，解压后顶层目录都是 `open-browser-use/`。提供
`.zip` 是为了通用下载和手工解压；提供 `.skill` 是为了让支持 skill bundle
扩展名的 agent installer 可以直接识别。

`release-manifest.json`、`package-manifest.json`、`crx-manifest.json`、
`skills/package-manifest.json`、`repo-metadata.tgz` 和 `sbom.spdx.json` 会保留在 workflow 的
`release-evidence` artifact 中，用于追溯和排查，不作为用户下载项展示。
release workflow 会对 Chrome extension zip/CRX 和 skill 包生成 provenance attestation。

workflow 新建 GitHub Release 时使用 `gh release create --generate-notes`，由
GitHub 自动生成 `What's Changed`、`New Contributors` 和 `Full Changelog`。
tag push 后仍要检查 release body；如果自动正文只有 `Full Changelog`，根据
`docs/releases/feature-release-notes.md`、`git log <previous-tag>..<tag> --oneline`
和本轮 history 手动补一段简短的 `What's Changed`。

## 自动提交 Chrome Web Store

Chrome Web Store API v2 用于上传 extension zip，并可选提交审核。官方约束：

- Google 开发者账号需要启用 2-step verification。
- 首次发布前，需要在 Chrome Web Store Developer Dashboard 填好 Store
  listing 和 Privacy tabs。
- API 上传目标是已经存在的 store item，因此需要先拿到 Publisher ID 和
  Extension ID。
- 上传新包时，`apps/chrome-extension/manifest.json` 的 `version` 必须比
  已发布版本更高。
- 如果已有 active submission（例如 `PENDING_REVIEW` 或 `STAGED`），API 不允许继续
  上传或提交新版本。仓库脚本会先调用 `fetchStatus` 检查状态，默认跳过本轮商店提交
  并写出 JSON 结果，避免 GitHub Release 因商店审核队列而失败。
- 首次提交 Dashboard 文案、权限说明和隐私字段时，先使用
  `docs/CHROME_WEB_STORE_LISTING.md` 里的 listing draft。

推荐使用 Chrome Web Store API v2 service account 给 CI/CD 授权。需要在
Google Cloud 启用 Chrome Web Store API，创建 service account，并在 Chrome
Web Store Developer Dashboard 的 Account 设置里添加该 service account email。
官方限制是 publisher 当前只能添加一个 service account。

service account 路径需要在 GitHub repository secrets 里配置：

```text
CWS_SERVICE_ACCOUNT_JSON
CWS_PUBLISHER_ID
CWS_EXTENSION_ID
```

可用 `gh` 写入：

```bash
gh secret set CWS_SERVICE_ACCOUNT_JSON < /path/to/service-account.json
gh secret set CWS_PUBLISHER_ID
gh secret set CWS_EXTENSION_ID
```

也可以继续使用 OAuth refresh token 路径；这种方式需要配置：

```text
CWS_CLIENT_ID
CWS_CLIENT_SECRET
CWS_REFRESH_TOKEN
CWS_PUBLISHER_ID
CWS_EXTENSION_ID
```

`CWS_REFRESH_TOKEN` 可以用本地 OAuth helper 生成。先在 Google Cloud 中为同一
项目启用 Chrome Web Store API，创建 OAuth client，并确保 loopback redirect
URI 可用：

```text
http://127.0.0.1:53682/oauth2callback
```

然后运行：

```bash
CWS_CLIENT_ID=<oauth-client-id> \
CWS_CLIENT_SECRET=<oauth-client-secret> \
pnpm chrome-web-store:oauth
```

脚本会打印授权 URL，使用 Chrome Web Store 发布者账号完成授权后，会在终端
输出 `CWS_REFRESH_TOKEN`。不要把该值提交到仓库；用 `gh secret set
CWS_REFRESH_TOKEN` 写入 GitHub repository secret。

手动触发 release workflow 时，把 `publish_chrome_web_store` 设为 `true`，workflow
会：

1. 打包 Chrome extension zip。
2. 用 refresh token 换取 access token。
3. 调用 Chrome Web Store API v2 `upload`。
4. 上传成功后调用 `publish`，提交审核。

如果 Chrome Web Store 里已经有审核中或 staged 的提交，workflow 默认不会取消它；
脚本会跳过上传并在 `chrome-web-store-result.json` 里记录
`skipped.reason=ACTIVE_SUBMISSION`。这让 GitHub Release、npm、PyPI 和 Homebrew
发布可以继续保持绿色。要让最新版本替换旧的审核中提交，手动触发 workflow 时把
`chrome_cancel_pending_submission` 设为 `true`。这会先调用
`cancelSubmission` 取消当前 active submission，再上传并提交本次 extension zip。

`scripts/publish-chrome-web-store.mjs` 的认证优先级是：

1. `CWS_ACCESS_TOKEN`：短期 access token，适合本地一次性验证。
2. `CWS_SERVICE_ACCOUNT_JSON`：推荐的 CI/CD service account JSON key。
3. `CWS_CLIENT_ID`、`CWS_CLIENT_SECRET`、`CWS_REFRESH_TOKEN`：OAuth refresh
   token fallback。

`chrome_publish_type` 默认为 `DEFAULT_PUBLISH`，通过审核后自动发布；如果想
审核通过后再手动发布，选择 `STAGED_PUBLISH`。`chrome_deploy_percentage`
留空时使用 Developer Dashboard 当前设置。

如果要让每个 `v*` tag 自动上传并提交 Chrome Web Store，在 repository variables
中设置：

```text
CWS_AUTO_PUBLISH=true
```

可选 variables：

```text
CWS_PUBLISH_TYPE=DEFAULT_PUBLISH
CWS_DEPLOY_PERCENTAGE=
CWS_SKIP_REVIEW=false
CWS_CANCEL_PENDING_SUBMISSION=false
```

tag 自动发布仍然需要上面的 `CWS_*` secrets。`CWS_SKIP_REVIEW=true` 只会请求
Chrome Web Store 跳过审核；官方 API 会验证是否符合条件，不符合时会返回错误。
Open Browser Use 当前包含 `<all_urls>`、`debugger`、`tabs`、`downloads` 等敏感
能力，普通代码更新应预期继续进入审核流程。

`CWS_CANCEL_PENDING_SUBMISSION=true` 会让 tag 自动发布在发现 active submission
时先取消旧提交再提交当前 tag 的 zip。这个动作会改变 Chrome Web Store 审核队列，
建议只在你明确希望“最新 tag 覆盖旧审核版本”时开启；否则保持默认 `false`，等当前
审核结束后用补发 workflow 提交最新 release。

## 从已有 GitHub Release 补发商店

如果 GitHub Release 已经创建完成，只需要把其中的插件 zip 上传到 Chrome
Web Store，使用 `.github/workflows/chrome-web-store-publish.yml`。这个
workflow 不会重新创建 GitHub Release，适合 `v0.1.5` 这类 release asset
已经存在、但当时还没配置 Chrome Web Store secrets 的情况。

手动触发时传入：

```text
release_tag=v0.1.5
asset_name=
submit=true
publish_type=DEFAULT_PUBLISH
deploy_percentage=
skip_review=false
cancel_pending_submission=false
```

`asset_name` 留空时会按 `release_tag` 精确下载
`open-browser-use-chrome-extension-<version>.zip`。如果需要上传非默认 asset，可以
显式填写完整 asset 文件名。

如果补发时商店里仍有旧版本在审核中，先确认是否要替换旧审核版本；确认后把
`cancel_pending_submission=true`。否则 workflow 会跳过上传并保留当前审核队列。

## 官方参考

- Chrome Web Store API 使用指南：
  <https://developer.chrome.com/docs/webstore/using-api>
- Chrome Web Store API service account：
  <https://developer.chrome.com/docs/webstore/service-accounts>
- Chrome Web Store API v2 upload：
  <https://developer.chrome.com/docs/webstore/api/reference/rest/v2/media/upload>
- Chrome Web Store API v2 publish：
  <https://developer.chrome.com/docs/webstore/api/reference/rest/v2/publishers.items/publish>
- Chrome Web Store API v2 fetchStatus：
  <https://developer.chrome.com/docs/webstore/api/reference/rest/v2/publishers.items/fetchStatus>
- Chrome Web Store API v2 cancelSubmission：
  <https://developer.chrome.com/docs/webstore/api/reference/rest/v2/publishers.items/cancelSubmission>
