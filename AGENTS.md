# 交接给接手智能体

用户希望在现有 Go 条件检查器之外，单独做一个使用 egglog 的数据库 planner 优化器。本目录是新项目 `E:\Coding\egglog-planner` 的骨架；**不要把 planner 写进 `E:\Coding\egglog-helper`**。当前没有优化器实现，也没有可以宣称“已经优化查询”的功能。

先读 `docs/superpowers/specs/2026-10-24-egglog-planner-design.md` 和 `docs/superpowers/plans/2026-10-24-egglog-planner.md`。按计划逐项实现，每项先写能证明行为的测试，再写代码并运行测试。先确认 egglog 0.4.0 怎样可靠地枚举或按自定义代价提取候选；不能只使用默认语法树大小提取，却声称找到了最低执行成本计划。遇到 CLI 能力不足时，记录验证结果，提出有证据的方案，然后继续完成能独立验证的 Go 计划表示和成本模型。

首版只支持具有唯一关系名、限定列名、内连接和受限等值谓词的 JSON 计划；不解析完整 SQL，不支持外连接、聚合、NULL 三值逻辑或自动执行。每条重写必须保持输入/输出列语义与 bag semantics。选择下推仅在谓词明确只引用对应子树的列时允许；连接重排要保持连接条件附着正确。代价模型必须公开假设与输入统计。

验收前运行 `go test ./...`、`go vet ./...`、真实 egglog 集成测试和 README 中的端到端示例。不要自动推送 GitHub；用户会决定何时发布。
