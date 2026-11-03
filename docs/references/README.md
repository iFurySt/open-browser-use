# 外部参考资料

这个目录用于沉淀那些值得长期放进仓库、供 Agent 直接读取的外部参考材料。

适合放这里的内容包括：

- 团队会反复依赖的框架、部署或接入说明。
- 设计系统参考、API 使用约定。
- 对外标准、合作方协议或外部文档的简要整理版。

不要把大段供应商文档原样塞进来。这里应该是经过筛选和整理后的资料。

长期演进的项目知识放在 `docs/wiki/`。例如 Browser Use 客户端的增量逆向
结论维护在 `docs/wiki/browser-client/`，这里的 reference 文档保留首次分析
的来源型记录。

## 已沉淀资料

- `chrome/0.1.7/skills/chrome/SKILL.md`：OpenAI bundled Chrome plugin
  `0.1.7` 的官方 Chrome skill 原文快照，用于对照官方 browser session、
  tab claiming、file upload 和 tab cleanup 约定；后续版本按同一路径新增。
- `codex-chrome-extension-1.1.4/`：Codex Chrome Web Store extension
  `hehggadaopoacecdllhhajmbjkdcmajg` 的本机安装快照、格式化入口文件和
  native messaging host 形状记录。

- `zen-native-messaging.md`：Zen/Firefox native manifest 源码依据和 Windows 验证边界。
