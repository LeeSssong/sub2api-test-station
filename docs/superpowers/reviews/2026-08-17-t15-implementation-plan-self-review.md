# T15 实施计划自审

## 结论

计划覆盖规格的数据库、原生模型选择、sidecar 边界、持久化去重、固定时隙、admin API、卡片交互和候选交接。任务按可独立验证的纵向边界拆分，允许内联执行。

## 自审结果

- 规格覆盖：所有批准合同均映射到 Task 1–4；许可证门禁与禁止部署映射到 Global Constraints 和 Task 5。
- 占位符扫描：无 TBD/TODO/“类似前文”等不可执行占位描述。
- 类型一致性：service/repository/sidecar/API/UI 均使用 `AccountModelDetection*` 前缀；状态枚举统一为 `untested/queued/running/normal/abnormal/insufficient/failed/unsupported`。
- 兼容性：现有 monitor 构造器保持可在 detection=nil 时工作，避免扩大旧测试修改。
- 验证范围：只包含 migration、focused backend、focused frontend、typecheck/build 与 diff check；未加入全仓或额外 reviewer。
