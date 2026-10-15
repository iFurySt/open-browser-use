# Electron IAB 示例样式与第三个 tab

> 阶段记录：本计划中的六步流程随后改为五步，取消关闭全部，支持结果保留和
> 手动重复动作。当前行为与运行方式以[示例 README](../../../examples/electron-iab/README.md)为准。

## 目标

按用户截图统一为黑白灰极简样式；移除彩色卡片、图标色块和额外说明，仅保留
必要动作和表单控件。增加第 5 步通过
CLI 私有通道打开 `https://github.com/ifuryst`，第 6 步关闭全部标签。

## 约束与验证

- 保留 280 DIP sidebar 与 134 DIP browser chrome，验证 native/DOM bounds。
- 示例演示仍使用 main + CLI + private，不引入新的接入按钮。
- 复用原 Browser Use 鼠标与操作提示，不改变库或通信鉴权。
- 本地 fixture 保留计数、可信输入/提交、popup、cookie 与严格 CSP 测试。
- 自动 smoke 用本地第三页验证编排，不依赖 GitHub 在线状态；原生 app
  单独验证实际 GitHub 页面。两者的范围分别记录。
- 同步 README、架构、前端、history 和 feature note。

## 进度

- [x] 宿主样式、icons、空状态、动作进度与本地示例页调整。
- [x] 六步手动/播放/停止/清理，并验证第三 tab 独立存在。
- [x] 真实 Electron smoke、原生视觉与 GitHub 验收、文档同步。

## 验证结果

- `make ci` 通过（既有 Electron package 18 项测试）。
- `pnpm smoke:iab` 21 项通过，第三页使用本地 URL；六步编排、旧页状态、
  trusted input、停止/清理、viewport 对齐及极简控件布局通过。
- 原生窗口核对最终黑白灰宿主与本地页，实际执行前五步；真实 GitHub 页面
  已加载，步骤标记完成。远端页面先开空白页再 CLI 导航，文档就绪即可完成，
  解决等待全部资源引发 CLI deadline 超时。导航失败时清理新创建的第三页。
- 文档/history/feature note 同步；本轮改动保留在工作区用于继续迭代。
