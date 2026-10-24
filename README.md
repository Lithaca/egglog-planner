# egglog-planner

一个独立于 [egglog-helper](https://github.com/Lithaca/egglog-helper) 的 Go 项目，用 egglog 探索数据库查询的等价执行计划，再依据简单的表统计信息选择低成本候选。

**当前状态：项目骨架和交接文档已建立；planner 尚未实现。** 接手开发请先阅读 [AGENTS.md](AGENTS.md)、[设计说明](docs/superpowers/specs/2026-10-24-egglog-planner-design.md) 和 [实现计划](docs/superpowers/plans/2026-10-24-egglog-planner.md)。

首版通过 JSON 输入计划树及目录统计信息，不解析 SQL，也不执行查询。范围限定为 Scan、Filter、Project、InnerJoin；重点验证选择下推、内连接交换/结合、代价估算和计划提取。语义边界、验收案例及开发顺序均写在上述文档中。

目标环境：Windows PowerShell、Go 1.26.2、官方 egglog CLI 0.4.0。可以参考原项目调用 egglog 的进程协议，但两个项目保持独立。
