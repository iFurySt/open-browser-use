# Electron IAB 可选操作反馈

## 目标

首版已提交 6219075 并推送，MR/PR #31 已创建。继续实现可选 AI 鼠标指示和
页面内侧渐变边缘，仅在目标 tab 操作时显示，示例 app 提供开关。

## 设计边界

- 复用 main 的 scope/tab 归属和实际 CDP Input 坐标；视觉层不合成输入事件。
- 光效位于 guest 页面视口内，pointer-events:none，不遮挡点击或宿主 chrome。
- 独立 isolated world/closed shadow DOM，无宿主 preload、IPC、凭据或网站策略扩权。
- 视觉指示是可被页面移除/伪造的提示，不作为认证或安全信号。
- 库默认关闭；宿主显式开启、设置 tab 活跃状态，示例开启并允许随时关闭。
- Tab 切换、导航、停止、异常、scope/页面销毁后清理状态和动画。
- 尊重 reduced motion；隐藏页不等待动画完成或抢焦点。

## 进度

- [x] 提交、推送首版并创建 PR #31。
- [x] 实现可选 visual feedback API 与 guest 渲染。
- [x] 接入 CDP 输入坐标/点击反馈及 demo 的动作生命周期。
- [x] 示例开关、真实 Electron 视觉/点击/导航清理验证。
- [x] 文档/history/feature note 同步，形成后续可评审提交。

## 验证结果

- `make ci` 通过，包含 18 项 Electron runtime/transport 测试。
- `pnpm smoke:iab` 20 项通过（Electron 44.7.0 / macOS arm64）。
- 严格 CSP 下检查原 cursor bitmap 的实际 canvas 像素与 23×24 显示尺寸，
  点击波纹缩放中心保持在真实坐标；关闭、导航、临时活动到期和可信输入正常。
- 原生窗口自动播放，实际看到既有鼠标及内侧渐变、CLI 输入提交和关闭全部。
- 原素材与完整 spring renderer 生成入包，build 校验生成物是否过期；无独立
  鼠标造型。视觉提示不作为认证依据。

## 交付

首版 `6219075` 与本轮后续操作反馈通过同一 PR #31 评审；npm 发布和更多
Electron/OS 实机矩阵仍属于后续工作。
