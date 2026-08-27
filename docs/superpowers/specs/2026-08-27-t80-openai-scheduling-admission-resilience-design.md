# T80 OpenAI 长请求调度准入韧性设计

## 状态

2026-08-27 用户确认启动本任务，并确认冻结 T76 的实现写入。本文是 T80 的正式规格，待用户书面审阅批准后才可编写实施计划或生产代码。

## 问题与原生现状

2026-08-27 的 GPT-Pro 分组 2 事故中，账号 `286` 在 `08:43:48` 起接入 11 个 `gpt-5.6-sol` 请求。前四笔约十万 input token 的请求尚未返回首字时，后续请求仍持续进入同一账号；部分小请求随后等待超过 80 分钟。当天该分组的 TTFT P50 约 54 秒、P95 约 3,841 秒，账号 286 的成功样本 TTFT 多数在 2,098 至 5,187 秒。

现有 Sub2API 原生能力已经包含：

- `OpenAIAccountScheduler`：资格过滤、粘性、Top-K、加权随机、公平性、半开探测和账号并发槽。
- `OpenAISharedHealthStore`：Redis 中按账号和模型保存完成后的成功/失败、TTFT EWMA、冷却及半开 lease。
- `AccountSelectionResult.ReleaseFunc`：账号并发槽和半开 lease 的幂等释放入口。
- `OpenAIResilienceEvent`：有界调度选择/结果 ledger 和现有 Ops 聚合输入。
- HTTP 文本入口：在选路前已读取并验证请求体，可得到请求体字节数；流式转发路径在收到首个输出 chunk 时已经计算 `FirstTokenMs`。

现有能力无法覆盖本次事故的第一窗口：共享健康仅在请求完成后写入，进程内 TTFT 也在完成后更新。因此首批长请求没有已知慢信号时，Top-K 和账号并发上限允许多个请求同时进入同一上游。账号并发 `10` 是容量上限，不是“首输出前只允许一笔长请求”的保护。

另一个已证实的可靠性缺口是共享健康完成事件使用传入请求 context 创建 Redis timeout context。若 HTTP 请求已取消或 handler 收尾后 context 已取消，子 context 立即取消；Redis 自身可用时仍会记录 `shared_health_store_unavailable`。读路径仍应响应请求取消，写路径不能继续继承已结束请求的取消信号。

T76 的候选实现是“至少 5 个已完成样本后”的质量门控，不能在第一笔慢请求完成前限制后续准入。T76 已被冻结为只读证据，不是本任务的实现基线。

## 目标

1. 对 OpenAI HTTP 流式文本请求，在账号被最终选中且并发槽已获得后，原子登记跨实例的“首输出前”准入 lease。
2. 当某账号-模型已有未首输出的长请求或超过首输出前安全上限时，后续请求必须跳过该账号并继续在当前候选池中选择其他账号，避免账号 286 类批量堆积。
3. 将真实用户请求的完成后 TTFT 按请求体大小桶写入 Redis 共享质量状态，进程重启后不丢失，且 monitor probe 不得覆盖真实请求质量。
4. 修复共享健康写入继承已取消 request context 的问题，并提供不泄露请求内容的失败分类日志和 resilience 事件。
5. 让发生准入跳过或共享状态退化时，日志和有界 resilience ledger 足以还原选路层、请求大小桶、候选/过滤数量、准入原因及最终账号。

## 非目标

- 不改变 GPT-Pro 或任何分组的 Top-K、加权随机、探索率、公平性、账号优先级、粘性权重或利润策略。
- 不修改账号 `concurrency` 配置，也不永久禁用、删除或自动编辑账号。
- 不修改账务、usage log、模型映射、用户返回协议、API schema、前端页面或管理员设置页。
- 不做数据库迁移，不持久化原始请求体、Authorization、完整 session、用户输入、完整候选列表或密钥。
- 不对 WebSocket、图片、视频、`/count_tokens` 或非流式文本请求启用本批次的首输出前 admission lease；这些路径保留原行为，后续单独评估。
- 不把 Redis 不可用升级为用户请求失败；共享准入在 Redis 退化时 fail-open，但必须可观测。

## 方案比较

### 方案 A：仅调低 Top-K 或账号并发

实现很小，但无法区分正常并行和“首输出迟迟未到”的上游阻塞。调低账号并发会直接损失健康账号吞吐，调低 Top-K 也不能防止排名靠前的慢账号接收一批请求。拒绝。

### 方案 B：完成后 TTFT 质量门控

这是 T76 候选的方向。它可在慢样本累积后减少选中慢账号，但第一批请求完成前没有任何可用证据，无法覆盖本次事故。保留为未来监控解释能力，不作为 T80 的保护机制。拒绝作为单独修复。

### 方案 C：共享首输出前 lease + 完成后真实质量分桶

在并发槽成功后，以 Redis Lua 原子登记账号-模型的短租约；流式路径首个语义输出到达时立刻释放该 lease。长请求占用一个专用上限，且任何未首输出长请求会阻止后续短请求进入同一账号-模型。完成后把真实请求 TTFT 写入独立的请求大小桶质量状态。该方案直接收敛首批批量进入的爆炸半径，同时保留已有 scheduler 的优先级和候选策略。选择此方案。

## 设计

### 请求大小桶

T80 不在选路前调用上游 `/input_tokens`，也不在 handler 做完整 JSON/tiktoken 解码。流式 HTTP 文本入口已经拥有通过长度限制和 JSON 验证的原始 body，因此只传递非敏感的大小桶：

- `normal`：body 小于 `long_request_body_threshold_bytes`。
- `long`：body 大于或等于该阈值。
- `unknown`：未携带 body-size context 的内部调用；不触发 admission lease，保持原行为。

默认阈值为 `65536` bytes。它是准入信号，不是计费 token 数，也不写入请求内容。完成后的实际 `usage.input_tokens` 继续用于使用记录；T80 的质量状态以这个请求前大小桶分隔，不把轻量 monitor probe 或正常短请求用于恢复长上下文质量。

HTTP Responses、OpenAI-compatible Messages 和 Chat Completions 在调用 `SelectAccountWithSchedulerForCapability` 前调用一个 service context helper 写入 `normal` 或 `long`。重试/切号沿用同一原始 body 的桶。未修改的调用者得到 `unknown`，兼容现有内部调用和测试构造。

### 共享准入 lease

新增在 `OpenAISharedHealthStore` 同一 Redis namespace 下的 admission store 能力，不创建第二个 Redis client、第二个控制面或数据库表。键仅由 `account_id`、canonical model hash 和固定 schema version 构成；lease ID 是逻辑 attempt ID 的不可逆哈希加随机后缀。Redis 不存储请求体、用户 ID、session、API key 或完整 request ID。

对每个 `(account_id, canonical_model)`，Redis Lua 脚本必须在一次原子操作中：

1. 清理过期 lease；
2. 读取未首输出 `normal` 和 `long` lease 数；
3. 判断当前请求是否可进入；
4. 可进入时写入/续期当前 lease 并返回 lease token、当前计数和决策；
5. 不可进入时不写入当前 lease，并返回确定的拒绝原因。

默认策略：

- `long` 请求最多一个未首输出 lease。
- `normal` 请求最多两个未首输出 lease。
- 只要存在未首输出 `long` lease，后续 `normal` 或 `long` 请求都拒绝进入该账号-模型。
- 任一未首输出 lease 已超过 `stalled_before_first_output_seconds`（默认 30 秒）时，后续请求拒绝进入该账号-模型，即使该 lease 是 `normal`。
- unknown shape、非流式、非文本或 admission disabled 的请求不申请 lease，也不改变当前 scheduler 行为。

新 lease 初始 TTL 为 90 秒。选中账号后启动有界续期，默认每 25 秒更新一次 TTL；收到首个语义输出、请求正常结束、失败、切号或 `ReleaseFunc` 执行时取消续期并删除本 lease。进程异常退出后，最后一次 lease 在至多 90 秒后自动清理，避免僵尸状态永久封锁账号。lease 的 acquire、renew、release 均使用短时、独立后台 context；任何 Redis 错误都释放本地控制流并 fail-open，不阻塞用户请求。

lease 只覆盖“上游尚未产生首个语义输出”的阶段。流式 Responses、Messages、Chat Completions 的既有 first-token/first-chunk 检测点必须调用同一个 `MarkFirstOutput` helper，立即删除 lease；不把一个已正常首字、但仍在持续输出的请求长期当作拥塞。

### 选路与释放控制流

准入检查发生在 scheduler 已完成账号资格/熔断/能力/并发槽选择之后、返回 handler 之前：

1. scheduler 选中账号并取得并发槽；
2. 若请求带 `normal` 或 `long` 流式文本 shape，尝试原子 admission acquire；
3. acquire 被拒绝时，立即执行刚取得的 `ReleaseFunc`，把该账号加入当前请求的排除集合，并在同一既有 scheduler 调用链重新选择；
4. acquire 成功时，把 admission lease 包装进 `AccountSelectionResult.ReleaseFunc`，确保 handler 的所有已有 `defer`、失败、重试和半开释放路径都清理 lease；
5. 首输出到达时 lease 先释放；最终 `ReleaseFunc` 仍为幂等兜底。

forced account、sticky hit、Top-K 加权和 half-open 都进入同一“选中且获取并发槽后”的 admission hook。若所有本来可用候选均因 admission 拒绝而被排除，返回现有无可用账号语义；不伪造成功，不修改 HTTP 错误协议。该情况必须有 `admission_exhausted` 调度诊断日志。

### 真实请求质量分桶

在现有成功/失败共享健康事件之外新增独立、可过期的 `OpenAISharedRequestQualitySnapshot`。它按 `(account_id, canonical_model, request_shape)` 保存：

- `real_sample_count`；
- `ewma_ttft_ms`；
- `last_ttft_ms`；
- `observed_at`；
- 短 TTL 和 schema version。

只在 T80 已标识的真实 HTTP 流式文本请求完成且含首字时间时更新。monitor probe、`/count_tokens`、失败、unknown shape、未产生首字的取消请求都不得写入成功质量样本。该 state 供 scheduler 读取：对同 shape 有至少一个新鲜真实样本且 `last_ttft_ms` 或 EWMA 超过 `slow_ttft_ms`（默认 30 秒）的账号，设置有限 `slow_quality` admission 冷却，默认 10 分钟；冷却期间将它作为资格过滤原因跳过。正常桶的成功不得清除 long 桶冷却，long 桶新鲜快速样本才可恢复 long 桶。

这不是 T76 的长期排名、P50 或 UI 投影。T80 只在真实且同大小桶的强慢信号出现后做有 TTL 的账号-模型准入隔离，解决重启后共享学习丢失和 probe 误导问题。

### 共享健康写可靠性

保留 `GetAccountModel` 和 selection/half-open acquire 使用调用方 context，以便用户取消立即停止读取/选路。新增明确的 write context helper，供以下变更操作使用：

- `RecordAttempt` 成功或失败；
- `CompleteHalfOpen`；
- admission acquire/renew/release；
- 真实请求质量写入。

该 helper 总是从 `context.Background()` 创建，使用配置的 Redis timeout 上限，不继承结束 HTTP 请求的取消状态。每次操作仍受 timeout 约束，不启动无界重试。日志不再把所有错误折叠成 `shared_health_store_unavailable`；增加受控字段 `operation`、`error_kind`（`context_canceled`、`deadline_exceeded`、`redis_unavailable`、`script_error`）和 account/model hash，禁止记录 Redis URL、凭据或请求内容。

### 配置与兼容性

在现有 `gateway.openai_shared_health` 下增加一组默认启用且严格校验的 admission 字段：

- `admission_enabled=true`
- `long_request_body_threshold_bytes=65536`，范围 4 KiB 至 4 MiB
- `max_pre_first_output_normal=2`，范围 1 至 8
- `max_pre_first_output_long=1`，范围 1 至 4
- `stalled_before_first_output_seconds=30`，范围 5 至 120
- `admission_lease_ttl_seconds=90`，范围 30 至 300
- `admission_renew_seconds=25`，必须小于 lease TTL 且不小于 5
- `slow_ttft_ms=30000`，范围 1,000 至 120,000
- `slow_quality_cooldown_seconds=600`，范围 30 至 3,600

这些是代码默认值和环境配置读取，不新增管理员页面或生产配置写入。紧急回滚开关为 `admission_enabled=false`；共享质量/health 状态保留短 TTL 并自然过期。T80 不迁移数据库；发布预检预期 `downtime_required=false`，以实际根发布预检为准。

### 可观测性与隐私

扩展现有 `OpenAIAccountScheduleDecision` 和 `OpenAIResilienceEvent` 的内部字段，至少记录：

- request shape；
- admission `acquired`、`rejected`、`released`、`first_output`、`store_degraded` 结果；
- admission 拒绝原因、active normal/long 计数、是否因 stalled lease 拒绝；
- admission-filtered candidate count、最终 selected rank、selection layer 和最终 account ID；
- shared quality bucket 是否命中、是否触发 slow-quality 冷却。

这些字段仅进入结构化服务日志和已有有界 process-local resilience ledger，不增加用户 API 或管理员 API。candidate IDs 不作为批量列表写入 event；现有选中账号 ID、计数、rank 和原因足以重建决策，而无需保留整份候选池或敏感请求内容。发生 admission reject、admission exhausted、shared-store write failure 时使用 Info/Warn 级别保留生产日志。

## 接口契约

实现计划必须以以下契约为准，并可在不改变公开 API 的前提下使用同名内部 helper：

```go
type OpenAIAdmissionRequestShape string

const (
    OpenAIAdmissionShapeUnknown OpenAIAdmissionRequestShape = "unknown"
    OpenAIAdmissionShapeNormal  OpenAIAdmissionRequestShape = "normal"
    OpenAIAdmissionShapeLong    OpenAIAdmissionRequestShape = "long"
)

type OpenAISharedAdmissionRequest struct {
    Key       OpenAISharedHealthKey
    LeaseID   string // hashed/non-sensitive
    Shape     OpenAIAdmissionRequestShape
    ObservedAt time.Time
}

type OpenAISharedAdmissionDecision struct {
    Allowed       bool
    Reason        string
    ActiveNormal  int
    ActiveLong    int
    Stalled       bool
    LeaseExpiresAt time.Time
}

type OpenAISharedRequestQualitySnapshot struct {
    Key             OpenAISharedHealthKey
    Shape           OpenAIAdmissionRequestShape
    RealSampleCount int
    EWMATTFT        time.Duration
    LastTTFT        time.Duration
    ObservedAt      time.Time
    CooldownUntil   time.Time
}
```

`OpenAISharedHealthStore` 需要扩展为 admission acquire/renew/release 和 real-quality read/write；所有 mutation 必须由该 store 的 Redis Lua 脚本以 lease ID 为条件执行，过期 lease 的清理和上限判断不得在 Go 侧做 read-then-write。`AccountSelectionResult.ReleaseFunc` 必须成为 admission lease 的唯一最终清理链，调用多次不会重复释放并发槽、half-open lease 或 admission lease。

## 失败与安全语义

- Redis admission read/write 超时、脚本错误或连接失败：记录 `store_degraded`，不拒绝用户请求，不影响既有并发槽释放或 failover 安全边界。
- admission acquire 被拒绝：只释放刚取得的账号并发槽，加入本请求 exclusions，然后在现有候选逻辑中重选；不得重复向同一账号发送上游请求。
- 所有候选都被 admission 过滤：使用既有 no-available-account 错误协议，记录 `admission_exhausted`。
- 请求取消、上游失败、客户端断开、首输出后的异常和 handler panic：现有 `defer ReleaseFunc` 必须清理 lease；没有首输出的异常路径不得把 quality 写成成功。
- 进程崩溃：Redis TTL 清理 orphan lease；不会修改账号数据库状态。
- shared quality slow cooldown：只影响相同账号、canonical model 和请求大小桶，自动过期；不改变手工 `schedulable=false`、硬禁用、余额隔离或现有半开语义。
- 未识别 endpoint/shape：零新准入行为，保留当前选路兼容性。

## 验收矩阵

| 场景 | 预期 |
| --- | --- |
| 首笔 long 流式请求获得账号 286 | 原子登记一个 long lease，后续请求可看到其 active 状态。 |
| 该请求尚未首输出时到达 normal 或 long 请求 | 286 被以 `long_pre_first_output` 排除，已获得的槽被释放，scheduler 在其他候选中继续选择。 |
| 两个并发实例争抢同一 long 账号 | Redis 脚本仅允许一个 long lease；另一实例得到确定拒绝。 |
| normal 请求已超过 30 秒未首输出 | 该账号-模型被视为 stalled，后续请求被过滤。 |
| 流式首个语义输出到达 | lease 立即释放；后续请求可按原 scheduler 竞争，不必等待整个输出结束。 |
| 请求失败、客户端取消或 handler 退出 | release 幂等执行；Redis 中不残留活动 lease。 |
| 进程在首输出前死亡 | 未续期 lease 最多 90 秒后过期，不永久阻塞。 |
| 一笔 long 真实请求 TTFT 为 40 秒后完成 | 仅 long quality bucket 写入，触发 long slow-quality 冷却；重启后的另一实例仍会跳过该账号的 long 请求。 |
| 轻量 probe 成功 | 不覆盖 long 真实请求质量，也不解除 long cooldown。 |
| 成功/失败回调收到已取消 request context | shared health/quality mutation 使用独立 bounded context 成功调用 store；正常 selection read 仍响应取消。 |
| Redis 不可用 | 请求遵循原有 scheduler 和用户协议；记录不含敏感信息的 store-degraded event。 |
| admission disabled 或 unknown shape | 无额外 Redis admission 操作，既有调度结果不变。 |

## 测试策略

- 先写 service RED 测试：long lease 阻止 normal/long、normal 上限、stalled 阻止、first-output release、release 幂等、quality 分桶隔离、slow-quality TTL 过滤、Redis fail-open。
- 先写 repository RED 测试（miniredis）：两个并发 acquire 只有一个胜者、过期清理、renew 延长 TTL、release 只能释放本 lease、脚本不包含原始 request/correlation 值。
- 为共享健康写 context 添加 RED 测试：取消的 parent context 仍能让 mutation store 观察到可用 bounded context；`GetAccountModel` 在取消 context 下仍返回取消错误。
- 为 handler/forward 添加 RED 测试：HTTP stream 在最终 selection 后申请 lease、首个 chunk 立即 release、失败和 failover 释放、重选跳过 admission-rejected account；不为 `/count_tokens`、images 或 non-stream 创建 lease。
- 直接相关验证包括 Go 定向 service/repository/handler 测试、`go test ./internal/service -run '^$'`、`go build ./cmd/server`、`gofmt`、`git diff --check`。不扩大为全仓压力/soak 或无关前端测试。
- 发布后只做安全的线上专项验证：确认配置有效、结构化 admission 事件可见、Redis 状态可读、健康端点正常；不主动发送长上下文或付费上游探测。自然真实请求出现后再只读核对 lease/quality 事件和分组 TTFT 变化。

## 发布、回滚与风险

本任务无数据库迁移、无账号数据写入、无 GitHub Actions。根发布总控仅在候选合并后的 `main` 上执行直接相关测试、构建、发布预检和既有蓝绿链。若预检明确返回 `downtime_required=true`，任何切换前停止等待用户授权；预期为 `false`，但以真实输出为准。

紧急回滚首先把 `gateway.openai_shared_health.admission_enabled=false`，恢复既有 scheduler 行为；完整回滚为恢复上一已验证蓝绿镜像。Redis admission/quality key 使用短 TTL，无需数据回滚。主要剩余风险是阈值过低会减少健康账号的短时吞吐、Redis 故障时保护退化为 fail-open，以及本批次未覆盖 WebSocket/非流式路径；这些风险必须在 handoff 和线上验证中保留。

## 批准记录

2026-08-27：用户确认采用“共享首输出前准入保护优先，后续质量信号分桶”的方向，并确认冻结 T76 的实现写入，避免与 T80 并行修改 OpenAI 选路链。本文完成后等待用户对书面规格的明确批准。
