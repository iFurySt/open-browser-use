# 供应链安全

这份文档定义模板默认采用的供应链安全做法。

## 默认控制项

- 为 release 产物生成 SBOM。
- 为 release 产物生成 build provenance attestation。
- 所有 GitHub Actions 都固定到不可变的 commit SHA，而不是漂移的版本标签。

## 当前对应关系

- `anchore/sbom-action`：生成 SPDX 格式的 SBOM。
- `actions/attest-build-provenance`：为 release artifact 生成签名 provenance。
- `scripts/check-action-pinning.sh`：如果 workflow 里出现浮动 tag 而不是 SHA，直接让 CI 失败。

## 限制和前提

- 当前不启用独立的 Pull Request 供应链安全 workflow。此前的 Dependency Review
  job 依赖仓库侧 Dependency Graph / GitHub Advanced Security 能力，当前仓库设置下会直接失败并阻塞普通 PR；OSV 全仓扫描也容易把既有开发依赖问题变成非增量 PR 噪音。
- 依赖漏洞巡检应在仓库设置和依赖基线稳定后，以非阻塞定时任务或明确 owner
  的修复流程重新接入。
- SBOM 的效果依赖仓库里存在可识别的依赖清单或 lockfile。
- `scripts/release-package.sh` 当前会产出 CLI 预编译 tarball、Chrome
  extension zip、CRX、Open Browser Use skill zip、`.skill` 包和内部追溯
  manifest；release workflow 会把 CLI、extension 与 skill 用户可下载包放到
  GitHub Release 页面，并对它们生成 provenance。user-owned private repository 不支持 GitHub
  artifact attestation，release workflow 会在 private repo 下跳过 provenance。
- npm 发布使用 trusted publishing/OIDC 分别发布 CLI 包 `open-browser-use` 和
  JavaScript SDK 包 `open-browser-use-sdk`。npm 在 public repository + public
  package + trusted publishing 条件下会自动生成 package provenance。
- PyPI 发布使用 trusted publishing/OIDC 发布 Python SDK distribution
  `open-browser-use-sdk`，避免在仓库 secret 中保存长期 PyPI token。Python
  import 模块名仍为 `open_browser_use`。
- OpenSSF Scorecard 默认不启用，因为新模板仓库还没有真实分支保护、release 历史和 SAST 姿态可以评分；等仓库规则配置完成后再按需加回。

## 项目落地后建议继续做的事

Electron IAB 首版使用 lockfile 固定的 Electron `44.7.0`、node-gyp `11.5.0`
以及 Go `golang.org/x/sys v0.41.0`。原生 peer adapter 通过 Node-API 8 从仓库
源码构建，不复制或分发参考 app 的闭源原生模块。宿主 ASAR 打包需 unpack
自己的 `.node` 文件；当前没有跨平台 prebuilt addon 制品或 Electron 包的
npm 发布任务，正式发布时应单独接入现有 SBOM/provenance 约定。

- 锁定并提交项目真实依赖的 lockfile。
- 让构建过程尽量可重复、可验证。
- 如果条件允许，在部署链路里增加对 provenance 的校验。
- 把 attestation 校验继续下沉到部署平台或准入层。
