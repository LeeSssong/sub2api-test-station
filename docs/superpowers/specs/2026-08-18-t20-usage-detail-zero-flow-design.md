# T20 用量详情过时提示清理与盈利页零流水账号补齐

## 状态与批准

- 状态：已批准，2026-08-18 由用户明确批准并要求立即排队实施。
- 顶层范围：删除用量详情中误导性的严格账单不可用提示；让盈利页分组包含时间窗内无流水但仍有效绑定的账号。
- 非目标：不改变 T17 有效账号成本口径、价格/倍率/账务写入、API 字段结构、证据接口、迁移、回填、生产数据或 T21/T22。

## 当前行为与证据

`UsageDetailDialog` 已使用 `effectiveAccountCost` 计算主上游扣费和利润，但仍根据 `usage_upstream_cost_evidence.evidence_status=unavailable` 渲染过时的 unavailable reason 提示。该提示把辅助核验状态误读为主金额不可用。

`ReadAccountFinancialUsage` 读取全部账号与分组元数据，流水聚合按 `usage_logs.group_id/account_id` 返回；`AccountFinancialService` 只在处理流水或探测行时创建分组账号。因此有效 `account_groups` 绑定在所选时间窗无流水时不会出现在分组明细，尽管顶层有效账号已存在。

## 方案

1. 仅在服务层根据已返回的有效账号和分组补零：无需仓储新事实，但无法区分实际绑定关系，会把每个有效账号显示在每个分组。
2. 在现有 usage reader 事务中读取 `account_groups` 与有效账号/分组，向快照增加 membership，再由服务层按 membership 种子化分组账号。推荐此方案，复用原生绑定表，关系准确，API 和聚合公式不变。
3. 重写 SQL 为分组全量 LEFT JOIN 流水和探测聚合：可减少服务逻辑，但会重复现有两个聚合查询、放大 SQL 复杂度和测试面。

选择方案 2。数据流为：repeatable-read 事务读取账号、分组、有效绑定；流水和探测聚合保持原查询；服务先创建活跃分组与绑定账号的零值节点，再叠加行，最后沿用现有派生金额和利润收口。

## 契约与失败语义

- 新增内部 `AccountFinancialUsageMembership` 快照字段，不改变 HTTP JSON。
- 只纳入未删除账号和未删除分组的 `account_groups` 关系；历史流水仍可按现有逻辑形成历史分组/账号。
- 绑定查询失败时整个读模型返回错误；探测聚合失败仍沿用现有 fail-soft `ProbeDataError` 行为。
- evidence 接口、`evidence_status`、`reason_code` 继续可查询，仅不再决定详情主金额提示。

## 验收与测试

- 前端：evidence unavailable、endpoint unsupported 和请求失败仍显示有效账号成本/利润，但不显示 unavailable reason 文案；保留 evidence 请求与辅助字段。
- 后端服务：有效绑定的零流水账号出现在对应分组且全部金额为零；有流水账号金额与现有结果一致；共享账号可出现在多个绑定分组。
- 仓储 sqlmock：验证 membership 查询、扫描和错误传播；更新既有查询顺序合同。
- 运行最小直接相关 Go service/repository 测试、前端 UsageDetailDialog 测试、受影响包 compile-only/build、前端 typecheck/build、gofmt 和 diff-check。

## 发布与回滚

无迁移、无配置变化、无生产写入，预期 `downtime_required=false`。候选必须从根 `main` 合并、推送并通过既有蓝绿链；若预检返回 `true`，停止在停机授权门禁。回滚使用上一已验证根提交/活动槽。
