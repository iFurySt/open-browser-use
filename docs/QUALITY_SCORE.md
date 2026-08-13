# 质量评分

用这份文档按产品区域和架构层次记录当前质量水位，方便持续知道最薄弱的地方在哪。

## 建议的评分标准

- `A`：覆盖完整、行为稳定、文档清楚、运行风险低。
- `B`：整体可接受，但还有明确短板。
- `C`：能用，但需要针对性补强。
- `D`：脆弱、缺少规范，或很多行为尚未定义。

## 当前评分

| 区域 | 评分 | 原因 | 下一步 |
| --- | --- | --- | --- |
| 产品面 | B | Chrome extension、Go native host/CLI、JS SDK、Python SDK、Go SDK、file chooser、Chrome Web Store 发布链路、SDK registry 发布链路和真实 Chrome smoke 记录已成形。 | 补首轮用户安装说明和更多失败场景验收。 |
| 架构文档 | B | Chrome route completed plan、架构、安全、发布和 reference 文档已覆盖主要边界。 | 把 extension-host runtime 的状态机和错误恢复策略补成单独文档。 |
| 插件 UI | B | MV3 popup、icons、content cursor 和基础打包校验已具备。 | 补 popup 截图 smoke 和权限状态展示验收。 |
| 测试 | B | `make ci` 覆盖 docs/repo hygiene、action pinning、extension 打包、脚本语法、Go 测试、JS/Python/Go SDK 协议测试、Python SDK smoke 和 fake native host/extension peer relay 测试。 | 补真实 Chrome 自动化 smoke 和 popup 截图 smoke。 |
| 可观测性 | B | request id 已按 caller 映射，迟到 response 不再污染其他 client；operation id、结构化 lifecycle receipts 和 durable reconciliation 已覆盖 unknown outcome。 | 增加可脱敏 debug trace、operation retention metrics 和真实跨 client soak。 |
| 安全 | C | Chrome route 明确不内置上层站点策略，manifest origin 限制和本地 socket 文件权限已有默认约束。 | 补 socket token/peer 授权、失败路径安全测试和安装权限审计说明。 |
