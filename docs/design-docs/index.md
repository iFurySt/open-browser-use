# 设计文档索引

用这个目录集中管理架构设计和产品设计文档。

建议约定：

- 一个主题一份文档。
- 每份文档写清当前状态和简短摘要。
- 关联引入它的 execution plan 或 spec。

## 初始文档

- `core-beliefs.md`

## 架构设计

- [Electron IAB 集成设计](electron-iab.md)：可引入 Electron app 的 npm
  模块、示例 app、多 tab、命名 profile 和自动化协议边界。首版已实现，完整平台与发布链路待扩展。
- [IAB 本地连接鉴权设计](iab-transport-auth.md)：原生 peer 身份、私有
  credential 交付、scope lease、撤销和跨平台验证要求。首版已实现，完整平台与发布链路待扩展。
