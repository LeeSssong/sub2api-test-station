# T43 调度 Top-K 放宽设计

## 目标

降低自适应 Top-K 对浅账号池的候选收窄，恢复更接近 Sub 原生的账号覆盖与长期未使用账号探索，同时保留 S1/S2 资格过滤、故障域、并发、sticky 和恢复预算语义。

## 当前证据

- `gateway.openai_scheduler.adaptive_top_k_enabled` 当前默认 `true`。
- 默认 `adaptive_top_k_max=7`、`adaptive_top_k_score_gap=0.15`。
- 自适应层先按最高分减 `score_gap` 建质量门槛，再按上限截断；候选分数差异较大时，`effective_top_k` 可降到 1。
- `lb_top_k` 仍是固定候选池上限；S1/S2 硬过滤在评分和 Top-K 之前执行。

## 方案与选择

1. 只增大 `adaptive_top_k_max`：保留分数门槛，低分且长期未使用账号仍可能被排除。
2. 关闭自适应 Top-K 默认值，保留固定 `lb_top_k`：最小变更，回退到 S1/S2 过滤 + 固定 Top-K/sticky，运行时仍可显式开启自适应。
3. 取消所有 Top-K：覆盖最宽，但会放大排序和并发探测成本，也失去现有运行时保护。

选择方案 2。默认关闭质量分数收窄，保留 `lb_top_k=7`、可观测字段和显式回滚/灰度开关。该方案不改调度接口、数据结构、账务或故障恢复。

## 数据流与行为

1. 原生 `listSchedulableAccounts`、模型/传输能力、S1 veto、S2 runtime/failure-domain 过滤保持原样。
2. 评分仍计算并用于固定 Top-K 内的加权顺序。
3. 默认 `AdaptiveTopKEnabled=false` 时跳过 `applyOpenAIAdaptiveTopK`，所有健康候选进入固定 Top-K 排序；`decision.SelectionLayer` 保持 load-balance，`EligibleCount` 为过滤后候选数，`EffectiveTopK` 为 `min(lb_top_k, EligibleCount)`。
4. 显式设置 `adaptive_top_k_enabled=true` 时，现有自适应行为和质量门槛完全保留。
5. `lb_top_k`、sticky、overflow fallback、半开探测和失败恢复预算不变。

## 配置契约

- `gateway.openai_scheduler.adaptive_top_k_enabled` 默认改为 `false`。
- `adaptive_top_k_max` 与 `adaptive_top_k_score_gap` 默认值保持 `7` 与 `0.15`，用于显式开启时的兼容行为。
- 示例配置明确标注：浅账号池默认关闭自适应收窄；如需质量优先可显式打开。
- 配置校验范围不变。

## 验收矩阵

- 默认加载：`AdaptiveTopKEnabled=false`。
- 默认调度：3 个健康候选、分数差异明显时 `EffectiveTopK=3`（而不是 1），且仍按固定 Top-K 顺序选择。
- 显式开启：已有自适应测试继续证明 `EffectiveTopK=1` 和 `quality_floor` sticky 逃逸。
- 资格过滤：quota paused/runtime blocked 账号仍在 Top-K 前被剔除。
- 配置校验：现有 max/score gap 边界测试保持通过。
- 无迁移、无 API 契约变化、无生产数据写入。

## 发布与回滚

- 预期 `downtime_required=false`，以根发布预检为准。
- 功能回滚：设置 `gateway.openai_scheduler.adaptive_top_k_enabled=true` 恢复旧自适应行为；二进制回滚沿用上一已验证蓝绿镜像。
- 观测重点：24 小时未使用账号数量、账号覆盖率、Top-K 过滤率、平均尝试次数和自动恢复率。

## 非目标

- 不改变账号资格、分组、模型能力、并发、sticky 绑定写入、故障域或重试预算。
- 不新增数据库迁移、管理员页面、指标事实源或外部控制面。
