# 技术债追踪

这里记录那些暂时不阻塞当前任务、但已经值得留档的技术债。

| 日期 | 区域 | 债务描述 | 为什么会存在 | 计划中的后续动作 |
| --- | --- | --- | --- | --- |
| 2026-10-13 | Electron IAB | Windows private descriptor 未实测，socket adapter 暂未提供；仅 Electron 44.7.0/macOS arm64 有真实 smoke | 首版选择可失败关闭的 macOS/Linux peer adapter | 补 Linux 实机、Windows DACL/peer、Electron 版本和 IME/iframe/最小化矩阵 |
| 2026-10-13 | Electron IAB 分发 | 原生 addon 安装时从源码构建，Electron 包尚未接入 npm/provenance 发布流程 | 当前交付工作区模块和示例 app | 配置独立发布链路、签名宿主验收、预构建或清晰的源码安装保障 |
| 2026-10-13 | Electron IAB 诊断 | 通信拒绝只关闭连接或返回通用错误，尚无结构化宿主诊断事件 | 避免在首版错误输出中泄漏凭据和资源信息 | 增加可脱敏的 auth/revoke/request 关联日志，保持客户端拒绝信息一致 |
