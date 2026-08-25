# T65 账号监控历史最终结果回退设计

## 状态

已获用户确认，进入实现前规格审阅。

## 目标

账号监控页面在当前检测证据不足、检测失败或当前评分窗口不可评分时，不显示空白；复用原生历史记录显示最近一次“最终有效”检测结果与评分，并明确标注这是历史回退，不把历史值伪装成实时值。

## 原生现状与证据

- `account_model_detection_runs` 已保存每次模型检测的最终状态、模型、指纹候选、错误和完成时间。
- `AccountModelDetectionService.ProjectionForAccount` 已返回当前投影及最近运行摘要，但账号监控 service 在投影失败时静默丢弃结果。
- 账号监控评分由 `accountMonitorWindowScoreProjection` 根据当前窗口 evidence 判定；当样本不足时返回 `ineligible`，页面因此显示空评分。
- 现有 `account_monitor_results`、窗口聚合和同一评分算法可作为历史评分重算依据，不新增第二张评分事实表。

## 设计

### 模型检测

1. 先使用当前 `ProjectionForAccount` 结果。
2. 当当前状态为 `insufficient`、`failed`、`service_unavailable`、`service_unconfigured` 或投影读取失败时，读取该账号最近一次已完成、状态为 `normal` 或 `abnormal`、且包含有效最终证据的运行记录。
3. 页面保留当前失败/证据不足状态和错误信息，同时在 `recent` 中显示历史最终结果，并新增来源标记与来源完成时间：`source=historical_final`、`source_finished_at`。
4. 若从未有有效历史结果，继续显示证据不足/未检测，不猜测模型。

### 评分

1. 当前窗口证据充分时，沿用现有实时评分流程。
2. 当前窗口证据不足时，读取最近一次可评分的历史监控窗口/证据，使用现有 `accountMonitorWindowScoreBreakdown` 重算同一评分，不新增平行算法。
3. 返回 `quality_score`、`score_breakdown`、`score_status`，并新增 `score_source=historical_final`、`score_observed_at`。
4. 账号永久禁用、明确不适用或没有任何历史有效证据时，不强行回退，仍显示不可评分。

### 前端

- 模型检测卡片显示当前状态；有历史回退时显示“沿用上次最终检测结果（时间）”。
- 评分区域显示历史分值及“沿用上次最终评分（时间）”，实时可用时不显示历史标签。
- API/类型、中文文案和现有测试同步更新；不改变账号管理页。

## 不在范围

- 不新增数据库表、缓存事实源或独立评分服务。
- 不改变检测算法、评分权重、调度资格或账号状态。
- 不把历史值用于新的调度决策，只用于监控展示和排序的已有读模型。

## 验收标准

- 当前检测 `insufficient` 但存在上次 `normal` 时，页面同时显示当前不足原因和上次最终结果/时间。
- 当前检测失败但没有历史成功时，页面仍显示失败/证据不足而不是伪造结果。
- 当前窗口无有效评分但存在上次有效评分时，`quality_score` 非空并带历史来源/时间。
- 明确禁用或永久不适用账号不被错误赋予历史评分。
- 后端 service/repository、API 合同、前端卡片和页面测试通过。

