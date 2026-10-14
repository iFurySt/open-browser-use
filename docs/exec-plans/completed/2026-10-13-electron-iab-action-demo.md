# Electron IAB CLI 私有通道动作演示

## 目标

示例 app 只展示 Electron main + Go CLI + 私有通道。左侧五步演示可逐步执行
或一键播放：打开 tab1/tab2，回到 tab1 点击，回到 tab2 点击输入提交，关闭全部。
Node/CLI × private/socket 四种接入保留在代码和文档，界面不再平铺接入方式。

## 实施边界

- 所有演示页面的创建、读取、真实 CDP 输入和关闭通过真实 obu iab 子进程。
- main 只负责 scope、展示选择、私有 bootstrap 和步骤编排；不以 DOM 合成
  点击或直接 main 操作页面代替 CLI 演示。
- 顺序依赖、并发防护、重复播放、停止和页面被用户关闭的失败需清晰处理。
- fixture 增加输入提交结果，前端只显示动作、运行状态和可展开的结果。
- 继续验证四种参考 transport，增加手动五步、自动播放和关闭/重播的 smoke。

## 进度

- [x] 读取当前代码、CLI 和本地规则。
- [x] 拆出参考集成与 CLI 私有通道 launcher。
- [x] 实现五步编排、fixture 提交和播放/停止。
- [x] 前端改为动作列表和播放按钮，保留右侧 tabs。
- [x] 本地 smoke、原生窗口验证、文档/history 同步。

## 完成结果

- `make ci` 与脚本语法、repo/docs、diff 检查通过。
- Electron 44.7.0/macOS arm64 的真实 smoke 18 项通过，新增五步手动、自动
  重播、停止和清理；四种参考 transport 持续验证。
- CLI 输入检查确认点击、input 和 submit 事件均 isTrusted；关闭全部对 opener
  自动销毁 popup 的情况重新读取 live tabs，避免使用失效 tabId。
- 2026-10-14：原生窗口中逐一执行五步，观察计数为 1、输入提交结果和 0 tabs；
  再点击自动播放，最终全部步骤完成并清空页面。截图复核右侧 panel 与网页对齐。
- UI 只表达 main+CLI+private；参考接法与文档留在 integrations.mjs/README。
