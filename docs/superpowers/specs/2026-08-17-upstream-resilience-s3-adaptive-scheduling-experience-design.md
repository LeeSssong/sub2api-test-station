# S3 自适应选择、粘性逃逸与调度体验观测设计

**日期：** 2026-08-17  
**状态：** 发布总控依 2026-08-15 代审授权审核批准  
**任务包：** S3  
**基线：** `main@83c4554792d3424751a439f3fd1cc38a0542ed5e`  
**工作区：** `codex/s3-adaptive-scheduling-experience`  
**发布属性：** 目标 `downtime_required=false`

## 1. 问题证据与当前事实

S1-R2 与 S2 已上线，当前代码已具备本任务需要的安全底座：

- `openai_account_scheduler.go` 在评分前已过滤原生不可调度状态、账号/模型 runtime block、S2 Redis 共享 cooldown 与 half-open，并消费当前请求的 `ExcludedIDs`。
- handler 在每次选号前传入 S2 已观测故障域，并由 `openai_retry_budget.go` 限制最大尝试、账号切换、故障域和 5 秒总预算。S3 不重复或放宽这些约束。
- 当前高级调度已根据优先级、负载、排队、错误率、TTFT、quota headroom、上游成本、previous response 和 session sticky 评分，但只使用固定 Top-K，没有按最佳候选质量动态收窄。
- 非加权 sticky 已可按进程本地错误率/TTFT 逃逸，共享健康 veto 也能阻止明确坏账号；但 `OpenAIAccountScheduleDecision` 没有统一的逃逸原因、有效候选数或质量门槛。
- `openai_resilience_observability.go` 已有有界、可按时间/平台/分组过滤的结构化事件台账，Ops 告警也已消费其派生计数；但 Ops 页面还不能直观回答“是否自动恢复、平均尝试几次、是否又命中坏账号、sticky 为何逃逸”。

因此 S3 的职责是消费现有 S1/S2 结果，改善健康候选中的选择和解释，而不是再建故障状态机或重试器。

## 2. 目标

1. 在 S1/S2 硬门槛之后，以“最佳健康候选分数 - 可配置差值”形成最低质量门槛，再以最大 K 限制有效池。
2. 明确输出候选总数、健康有效数、动态 `effective_top_k`、质量门槛和选择层。
3. 保留健康 sticky 的缓存优势，但对 S1/S2 veto、当前请求排除、高错误率和高 TTFT 生成可解释逃逸原因。
4. 在现有 Ops 总览中展示自动恢复率、平均/P95 尝试数、坏账号重复命中率、预算耗尽率、sticky 保留/逃逸率和 Top-K 过滤率，并显示分子、分母、样本量与新鲜度。
5. TTFT 只做 report-only 资格判定和估算事件，不发起第二个上游请求。

## 3. 非目标

- 不改变 S1 确定性分类、原生账号状态、恢复条件或 S2 重试上限。
- 不启用并行竞速、shadow 完整请求或真实预热；不对已输出、有副作用、工具调用或幂等性不明的请求重放。
- 不改变价格、倍率、账务、usage 幂等或外部控制面。
- 不新建平行管理页、持久化调度事件表或历史回填。
- 不修改 T15、T16 或历史候选 worktree。

## 4. 方案比较

| 方案 | 做法 | 优点 | 问题 | 结论 |
|---|---|---|---|---|
| A. 只调固定权重/Top-K | 修改现有设置 | 变更小 | 没有质量门槛与解释，无法稳定阻止劣质候选进入抽样 | 不采用 |
| B. 健康门槛 + 动态 Top-K + 可解释 sticky + Ops 派生指标 | 最小扩展现有 scheduler/event/Ops | 直接复用 S1/S2，无新事实源，可测试与回滚 | 需要在两条 OpenAI handler 循环中统一发事件 | **采用** |
| C. 默认并行 TTFT 竞速 | 同时请求多账号 | TTFT 改善上限高 | 成本、副作用、流式拼接和账务风险超出 S3 | 不采用；仅 report-only |

## 5. 调度契约

### 5.1 硬门槛顺序

```text
原生账号/分组/模型/传输资格
  -> S1 account/account-model veto
  -> S2 shared cooldown/domain preference/half-open lease
  -> 当前 logical request ExcludedIDs
  -> S2 retry budget 是否允许发起下一次尝试
  -> S3 质量评分和 sticky 奖励
```

S3 分数、优先级、成本或 sticky 不得让任何被前四层拒绝的账号重新入选。Redis 失效时继续执行 S2 已上线的本地快照/fail-safe 行为，S3 不改写该降级。

### 5.2 动态 Top-K

新增配置：

```yaml
gateway:
  openai_scheduler:
    adaptive_top_k_enabled: true
    adaptive_top_k_max: 7
    adaptive_top_k_score_gap: 0.15
    ttft_report_only_enabled: true
```

约束：`1 <= adaptive_top_k_max <= 32`，`0 <= adaptive_top_k_score_gap <= 10`。非法值在启动校验时拒绝，不静默放宽。

对通过硬门槛的候选完成现有评分后：

1. 找出 `best_score`，质量门槛为 `best_score - adaptive_top_k_score_gap`。
2. 保留分数不低于门槛的候选，再取前 `min(configured LBTopK, adaptive_top_k_max)` 个。
3. 若只有 1 个健康候选，`effective_top_k=1`；不得为填满 K 加回被 veto/cooldown 的账号。
4. 若质量门槛因 NaN/Inf 或意外输入无法计算，fail-safe 保留排名第一的健康候选并记录 `quality_fallback`。
5. half-open 候选仍使用 S2 独占 lease，不与普通动态 Top-K 混合。

`OpenAIAccountScheduleDecision` 增加：

```text
EligibleCount
EffectiveTopK
MinimumScoreThreshold
SelectionLayer = previous_response_id | session_hash | adaptive_top_k | half_open_probe
StickyKept
StickyEscapeReason = none | excluded | deterministic_health | shared_cooldown |
                     error_rate | ttft | capability | concurrency | quality_floor
TTFTReportEligible
```

### 5.3 Sticky 逃逸

- previous-response 不可移动时继续沿用原生硬粘性，但 S1/S2/能力/并发硬门槛仍可拒绝。
- 非加权 session sticky 继续使用现有错误率 `0.5` 和 TTFT `15000ms` 基线，但把逃逸原因回传到 decision/event。
- 加权 sticky 仅在进入动态质量池后才能置顶；如 sticky 分数低于质量门槛，记录 `quality_floor` 并逃逸。
- 因负载/并发已满而排队不记为坏账号命中；只有失败后又被普通选取才进入“坏账号重复命中”分子。

### 5.4 TTFT report-only

`TTFTReportEligible=true` 仅表示“如未来存在安全预热任务，当前尝试具备候选资格”。本任务不发起、不计费、不取消任何第二请求。资格至少要求：

- 尚未输出；
- 当前请求无图像生成、工具/函数调用或其他已知副作用；
- 还存在另一个通过 S1/S2 硬门槛的候选；
- S2 预算尚未耗尽。

## 6. 观测数据契约

复用 `OpenAIResilienceEvent` 的有界事件台账，增加不含敏感数据的字段：

```text
SelectionLayer, CandidateCount, EligibleCount, EffectiveTopK
MinimumScoreThreshold, StickyKept, StickyEscapeReason
TTFTReportEligible, RetryBudgetExhausted, FinalOutcome
```

每次实际尝试在已有 `logical_request_id/attempt_id/attempt_number/account_id/model/group/platform` 维度下记录 selection；成功、终端失败和预算耗尽记录 request outcome。不写 prompt、API Key、凭据、原始请求体或原始上游 body。

Ops 派生指标：

| 指标 | 定义 |
|---|---|
| 自动恢复率 | 有失败且最终成功的 logical request / 有失败的 logical request |
| 平均/P95 尝试数 | 每个 logical request 的实际 attempt 数 |
| 坏账号重复命中率 | 失败后又普通选中同账号模型的次数 / 失败后普通选择次数；half-open 单列且不进分子 |
| 预算耗尽率 | 因 S2 budget 终止的 logical request / 进入重试的 logical request |
| sticky 保留/逃逸率 | 保留或逃逸次数 / 存在 sticky 候选的选择次数 |
| Top-K 过滤率 | `(eligible_count - effective_top_k) / eligible_count` |
| TTFT report-only 资格率 | 具备资格的尝试 / 尚未输出的选择次数 |

样本分母小于 5 时显示 `insufficient_data`，不用 0%/100% 诱导。事件台账为运行时观测投影，进程重启后可为空；页面必须显示最新事件时间和“当前运行窗口”，不伪装为持久历史。

## 7. 原生 Ops 界面

在现有 `OpsDashboard.vue` 的 OpenAI Token Stats 下方增加 `OpsOpenAISchedulerExperienceCard.vue`：

- 沿用顶部时间、platform 和 group 过滤，不新建页面或导航。
- 一行摘要显示自动恢复率、平均/P95 尝试、坏账号重复命中率、预算耗尽率。
- 第二行显示 sticky 保留/逃逸、Top-K 过滤、TTFT report-only 资格。
- 每项显示分子/分母、样本和最新时间；无数据显示明确空态，API 失败仅使本卡片进入错误/重试态，不影响 Ops 其他卡片。
- 中英文文案同步，390px 宽度单列、无横向滚动。

## 8. API 契约

新增管理员原生端点：

```text
GET /api/v1/admin/ops/openai-scheduler-experience
query: start_time, end_time, platform?, group_id?
```

响应：

```json
{
  "start_time": "2026-08-17T00:00:00Z",
  "end_time": "2026-08-17T01:00:00Z",
  "generated_at": "2026-08-17T01:00:01Z",
  "latest_event_at": "2026-08-17T00:59:58Z",
  "sample_size": 42,
  "metrics": {
    "auto_recovery_rate": {"numerator": 8, "denominator": 10, "value": 0.8, "status": "ok"},
    "average_attempts": {"sample_size": 42, "value": 1.24, "p95": 2, "status": "ok"}
  }
}
```

率类指标统一返回 `numerator/denominator/value/status`；无数据或低样本为 `status=no_data|insufficient_data`。端点只允许管理员，复用 Ops 时间参数解析与分组权限。

## 9. 失败与安全语义

- 动态 Top-K 计算失败时仅在已通过 S1/S2 硬门槛的候选中 fail-safe 到最佳候选，不放行已知坏账号。
- 观测事件写入失败不阻塞用户请求；Ops 卡片允许 no-data/error。
- 已输出的流保持 S1/S2 现有禁止完整重放规则。
- 不向普通用户返回账号 ID、评分、故障域或逃逸原因；这些只在管理员 Ops 和受控结构化日志中出现。
- 不增加数据库迁移、外部调用、付费或不可逆数据操作。

## 10. 测试与验收矩阵

| 场景 | 预期 |
|---|---|
| 10 个健康候选、固定 K=7 | 只在质量门槛上且最多 7 个候选中选择 |
| 3 个健康候选、K=7 | `effective_top_k<=3`，不补入 veto/cooldown 账号 |
| 最高分账号被 S1/S2 veto | 无论 sticky/优先级/成本奖励多高都不入选 |
| 非加权 sticky 超 TTFT/错误率 | 逃逸且原因分别为 `ttft`/`error_rate` |
| 加权 sticky 低于质量门槛 | 不置顶，原因为 `quality_floor` |
| half-open 仅剩候选 | 只有一个 lease 胜者，标记 `half_open_probe` |
| Redis 不可用且本地快照过期 | 沿用 S2 降级，不增加重试或放行明确坏账号 |
| TTFT report-only 资格 | 只记事件，上游请求数不增加 |
| 已输出/工具/生图 | `TTFTReportEligible=false`，不重放 |
| 重试后成功 | 自动恢复分子/分母和尝试数正确 |
| 预算耗尽 | 只记一个终端 outcome，预算耗尽率正确 |
| 低样本 Ops | 显示 `insufficient_data` 及分子/分母 |
| 390px Ops 页 | 卡片单列，无横向溢出，错误状态不影响其他区块 |

最小验证范围：

- scheduler 纯函数/服务单测；
- 两条 OpenAI HTTP handler 的 decision/event/outcome 定向合同测试；
- resilience 事件聚合与 Ops handler 单测；
- Ops API/TypeScript/卡片组件定向测试；
- 受影响 Go package compile-only、前端 typecheck/build、`gofmt`、`git diff --check`、迁移/GitHub Actions 零变更检查。

不运行全仓测试、压力、soak、mutation 或无关浏览器矩阵。

## 11. 发布、线上验证与回滚

- 只能在 S3 候选完成直接相关验证、合入并重验根 `main`、推送后，使用既有本地/宿主蓝绿链发布；禁止 GitHub Actions。
- 预检为 `downtime_required=false` 时，发布总控直接继续发布、健康检查与 S3 专项验收；为 `true` 时才在任何停服、迁移、重启或切换前暂停请求授权。
- 线上验收使用自然流量和只读证据：三个健康端点、API/worker 镜像一致、调度卡片 API，以及新增结构化事件中的 dynamic Top-K/sticky/TTFT report-only 字段。不修改生产账号，不人为制造上游故障。
- 功能快速回滚通过关闭 `adaptive_top_k_enabled` 和 `ttft_report_only_enabled`，退回 S1/S2 健康过滤 + 现有固定 Top-K/sticky 行为。二进制回滚使用上一已验证蓝绿镜像；观测投影为可重建运行时数据，无需数据回滚。

## 12. 规格自审与批准记录

- **原生优先：** 复用当前 scheduler、S1/S2 veto/budget、resilience event ledger 和 Ops 总览；未新建平行调度器、监控页或事实源。
- **范围：** 仅 S3 动态选择、sticky 解释、TTFT report-only 和 Ops 观测；不含并行竞速、账务或他任务。
- **一致性：** 数据流、API、测试、发布和回滚均保持 S1/S2 优先于 S3 分数。
- **模糊性：** 无 `TBD`/`TODO`；低样本、重启丢失运行投影、NaN/Inf、half-open、已输出和 Redis 降级均有明确语义。
- **安全：** 无迁移、回填、数据删除、外部付费、凭据风险或停机要求。

2026-08-17，唯一发布总控依用户已授予的队列规格/计划代审权完成书面审查，确认本规格不扩大已批准 S3 范围，批准进入 `writing-plans`。
