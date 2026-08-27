# T80 OpenAI 长请求调度准入韧性设计

## 状态

2026-08-27 用户确认启动本任务，并确认冻结 T76 的实现写入。用户已书面批准初版；同日追加的阻断性约束覆盖初版中任何 account-model admission/shared-quality 契约：T80 只能使用账号级跨模型、跨分组安全状态，禁止 Pro、分组名或模型特判。

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

1. 对所有走 OpenAI scheduler 的 HTTP 流式文本请求，在账号被最终选中且并发槽已获得后，原子登记跨实例、跨模型、跨分组的账号级“首输出前”准入 lease。
2. 当某账号已有未首输出的长请求、超过首输出前安全上限或处于慢会话保护时，任一路径的后续请求必须跳过该账号并继续在当前候选池中选择其他账号，避免账号 286 类批量堆积。
3. T80 仅维护账号级安全状态；真实质量的 `group_id` 归属、分组解释与投影沿用 T76 的既有方向，待 T80 稳定后刷新，不在 T80 新增 shared-quality key 或模型维度。
4. 修复共享健康写入继承已取消 request context 的问题，并提供不泄露请求内容的失败分类日志和 resilience 事件。
5. 让发生准入跳过或共享状态退化时，日志和有界 resilience ledger 足以还原选路层、请求大小桶、候选/过滤数量、准入原因及最终账号。

## 非目标

- 不改变 GPT-Pro 或任何分组的 Top-K、加权随机、探索率、公平性、账号优先级、粘性权重或利润策略。
- 不修改账号 `concurrency` 配置，也不永久禁用、删除或自动编辑账号。
- 不修改账务、usage log、模型映射、用户返回协议、API schema、前端页面或管理员设置页。
- 不做数据库迁移，不持久化原始请求体、Authorization、完整 session、用户输入、完整候选列表或密钥。
- 不对 WebSocket、图片、视频、`/count_tokens` 或非流式文本请求启用本批次的首输出前 admission lease；这些路径保留原行为，后续单独评估。
- 不把 Redis 不可用升级为用户请求失败；共享准入在 Redis 退化时 fail-open，但必须可观测。
- 不按 Pro、GPT-Pro、分组名称、模型、模型映射或账号名称创建 admission、慢会话保护、质量或配置分支；不得新增 account-model 质量/配置契约。
- 分组只能继续使用既有 scheduler 设置影响已通过安全过滤的候选排序、利润与体验取舍；分组目标、权重或策略绝不能绕过 T80 的统一安全门槛。

## 方案比较

### 方案 A：仅调低 Top-K 或账号并发

实现很小，但无法区分正常并行和“首输出迟迟未到”的上游阻塞。调低账号并发会直接损失健康账号吞吐，调低 Top-K 也不能防止排名靠前的慢账号接收一批请求。拒绝。

### 方案 B：完成后 TTFT 质量门控

这是 T76 候选的方向。它可在慢样本累积后减少选中慢账号，但第一批请求完成前没有任何可用证据，无法覆盖本次事故。保留为未来监控解释能力，不作为 T80 的保护机制。拒绝作为单独修复。

### 方案 C：账号级共享首输出前 lease + 账号级慢会话保护

在并发槽成功后，以 Redis Lua 原子登记账号级短租约；流式路径首个语义输出到达时立刻释放该 lease。长请求占用一个专用上限，且任何未首输出长请求会阻止跨模型、跨分组的后续请求进入同一账号。若现有真实请求结果判定慢会话，则写入同一账号级短 guard。该方案直接收敛首批批量进入的爆炸半径，同时保留已有 scheduler 的优先级和候选策略。选择此方案。

## 设计

### 请求大小桶

T80 不在选路前调用上游 `/input_tokens`，也不在 handler 做完整 JSON/tiktoken 解码。流式 HTTP 文本入口已经拥有通过长度限制和 JSON 验证的原始 body，因此只传递非敏感的大小桶：

- `normal`：body 小于或等于 `long_request_body_threshold_bytes`。
- `long`：body 大于该阈值。
- `unknown`：未携带 body-size context 的内部调用；不触发 admission lease，保持原行为。

默认阈值为 `65536` bytes。它是准入信号，不是计费 token 数，也不写入请求内容。完成后的实际 `usage.input_tokens` 继续用于使用记录；T80 的质量状态以这个请求前大小桶分隔，不把轻量 monitor probe 或正常短请求用于恢复长上下文质量。

HTTP Responses、OpenAI-compatible Messages 和 Chat Completions 在调用 `SelectAccountWithSchedulerForCapability` 前调用一个 service context helper 写入 `normal` 或 `long`。该分类仅使用原始 body 字节数与全局安全阈值，不读取分组名、分组策略、Pro 或模型。重试/切号沿用同一原始 body 的桶。未修改的调用者得到 `unknown`，兼容现有内部调用和测试构造。

### 通用性与分组边界

T80 是所有 OpenAI scheduler 路径共享的安全层。其 Redis key、Lua 判断、阈值、TTL、renew、fail-open、stalled 和 slow-session 语义仅以 `account_id` 为资源边界。分组既有的 `OpenAISchedulerGroupPolicy` 仍仅在安全过滤后决定候选排序、利润与体验取舍；不得增加 admission override，也不得将分组目标转换为安全门槛例外。调度日志必须能说明“安全门槛已统一应用；剩余排序差异来自既有 group policy”，但 T80 不新增分组质量状态或模型维度。

### 共享准入 lease

新增在 `OpenAISharedHealthStore` 同一 Redis namespace 下的 admission store 能力，不创建第二个 Redis client、第二个控制面或数据库表。键仅由 `account_id` 和固定 schema version 构成；lease ID 是逻辑 attempt ID 的不可逆哈希加随机后缀。Redis 不存储模型、请求体、用户 ID、session、API key 或完整 request ID。

对每个 `account_id`，Redis Lua 脚本必须在一次原子操作中：

1. 清理过期 lease；
2. 读取未首输出 `normal` 和 `long` lease 数；
3. 判断当前请求是否可进入；
4. 可进入时写入/续期当前 lease 并返回 lease token、当前计数和决策；
5. 不可进入时不写入当前 lease，并返回确定的拒绝原因。

默认策略：

- `long` 请求最多一个未首输出 lease。
- `normal` 请求最多两个未首输出 lease。
- 只要存在未首输出 `long` lease，后续 `normal` 或 `long` 请求都拒绝进入该账号，不受模型或分组影响。
- 任一未首输出 lease 已超过 `stalled_before_first_output_seconds`（默认 30 秒）时，后续请求拒绝进入该账号，不受模型或分组影响，即使该 lease 是 `normal`。
- unknown shape、非流式、非文本或 admission disabled 的请求不申请 lease，也不改变当前 scheduler 行为。

新 lease 初始 TTL 为 90 秒。选中账号后启动有界续期，默认每 25 秒更新一次 TTL；收到首个语义输出、请求正常结束、失败、切号或 `ReleaseFunc` 执行时取消续期并删除本 lease。进程异常退出后，最后一次 lease 在至多 90 秒后自动清理，避免僵尸状态永久封锁账号。lease 的 acquire、renew、release 均使用短时、独立后台 context；任何 Redis 错误都释放本地控制流并 fail-open，不阻塞用户请求。

lease 只覆盖“上游尚未产生首个语义输出”的阶段。流式 Responses、Messages、Chat Completions 的既有 first-token/first-chunk 检测点必须调用同一个 `MarkFirstOutput` helper，立即删除 lease；不把一个已正常首字、但仍在持续输出的请求长期当作拥塞。

### 选路与释放控制流

准入检查发生在 scheduler 已完成账号资格/熔断/能力选择、handler 已获取账号并发槽之后：

1. scheduler 选中账号并取得并发槽；
2. 若请求带 `normal` 或 `long` 流式文本 shape，尝试原子 admission acquire；
3. acquire 被拒绝时，立即执行刚取得的 `ReleaseFunc`，把该账号加入当前请求的排除集合，并在同一既有 scheduler 调用链重新选择；
4. acquire 成功时，把 admission lease 组合进 handler 当前的账号-slot release closure，确保 handler 的所有已有 `defer`、失败、重试和半开释放路径都清理 lease；
5. 首输出到达时 lease 先释放；最终 `ReleaseFunc` 仍为幂等兜底。

forced account、sticky hit、Top-K 加权和 half-open 都进入同一“选中且获取并发槽后”的 admission hook。若所有本来可用候选均因 admission 拒绝而被排除，返回现有无可用账号语义；不伪造成功，不修改 HTTP 错误协议。该情况必须有 `admission_exhausted` 调度诊断日志。

### 慢会话保护与质量归属

T80 不新增 `OpenAISharedRequestQualitySnapshot`，不写 account-model 或 account-group quality Redis key，也不把 TTFT 样本变成第二套 scheduler 质量事实源。慢会话保护只使用账号级、短 TTL 的 `slow_session_guard`：当现有真实请求完成路径提供可信 slow-session 结论时，按 `account_id` 原子写入/延长 guard；guard 在所有 OpenAI scheduler 分组与模型路径上生效，并在 TTL 到期后自然恢复。monitor probe、`/count_tokens`、失败、unknown shape 和未产生首字的取消请求不得触发该 guard。

质量归属、按 `group_id` 的样本隔离、监控解释和“组策略导致的调度差异”沿用 T76 的既有解释投影方向，待 T80 安全层稳定后由根总控授权刷新；T80 只留下无模型维度的 account-level safety event。任何组的快速样本、权重或体验目标都不能清除尚未到期的账号级 guard。

### 共享健康写可靠性

保留 `GetAccountModel` 和 selection/half-open acquire 使用调用方 context，以便用户取消立即停止读取/选路。新增明确的 write context helper，供以下变更操作使用：

- `RecordAttempt` 成功或失败；
- `CompleteHalfOpen`；
- admission acquire/renew/release；
- 账号级 slow-session guard 写入/释放。

该 helper 总是从 `context.Background()` 创建，使用配置的 Redis timeout 上限，不继承结束 HTTP 请求的取消状态。每次操作仍受 timeout 约束，不启动无界重试。日志不再把所有错误折叠成 `shared_health_store_unavailable`；增加受控字段 `operation`、`error_kind`（`context_canceled`、`deadline_exceeded`、`redis_unavailable`、`script_error`）和 account hash，禁止记录模型、Redis URL、凭据或请求内容。

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
- `slow_session_guard_seconds=600`，范围 30 至 3,600

这些是全局代码默认值和环境配置读取，不新增分组 admission 配置或管理员页面。分组只继续通过现有 scheduler 设置影响通过安全过滤后的排序、利润与体验取舍。紧急回滚开关为 `admission_enabled=false`；账号级 safety state 保留短 TTL 并自然过期。T80 不迁移数据库；发布预检预期 `downtime_required=false`，以实际根发布预检为准。

### 可观测性与隐私

扩展现有 `OpenAIAccountScheduleDecision` 和 `OpenAIResilienceEvent` 的内部字段，至少记录：

- request shape；
- admission `acquired`、`rejected`、`released`、`first_output`、`store_degraded` 结果；
- admission 拒绝原因、active normal/long 计数、是否因 stalled lease 或 slow-session guard 拒绝；
- admission-filtered candidate count、最终 selected rank、selection layer 和最终 account ID；
- account-level safety guard 是否命中/触发，以及安全过滤后由既有 group policy 造成的排序差异。

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
    AccountID int64
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

```

`OpenAISharedHealthStore` 需要扩展为账号级 admission acquire/renew/release 和账号级 slow-session guard；所有 mutation 必须由该 store 的 Redis Lua 脚本以 lease ID 为条件执行，过期 lease 的清理和上限判断不得在 Go 侧做 read-then-write。handler 的组合 release closure 必须成为 admission lease 的唯一最终清理链，调用多次不会重复释放并发槽、half-open lease 或 admission lease。

## 失败与安全语义

- Redis admission read/write 超时、脚本错误或连接失败：记录 `store_degraded`，不拒绝用户请求，不影响既有并发槽释放或 failover 安全边界。
- admission acquire 被拒绝：只释放刚取得的账号并发槽，加入本请求 exclusions，然后在现有候选逻辑中重选；不得重复向同一账号发送上游请求。
- 所有候选都被 admission 过滤：使用既有 no-available-account 错误协议，记录 `admission_exhausted`。
- 请求取消、上游失败、客户端断开、首输出后的异常和 handler panic：现有 `defer ReleaseFunc` 必须清理 lease；没有首输出的异常路径不得触发 slow-session guard。
- 进程崩溃：Redis TTL 清理 orphan lease；不会修改账号数据库状态。
- slow-session guard：只按账号跨模型、跨分组生效并自动过期；不改变手工 `schedulable=false`、硬禁用、余额隔离或现有半开语义。现有 group policy 不能绕过 guard。
- 未识别 endpoint/shape：零新准入行为，保留当前选路兼容性。

## 验收矩阵

| 场景 | 预期 |
| --- | --- |
| 首笔 long 流式请求获得账号 286 | 原子登记一个 long lease，后续请求可看到其 active 状态。 |
| 该请求尚未首输出时，另一个模型或分组到达 normal 或 long 请求 | 286 被以 `long_pre_first_output` 跨模型、跨分组排除，已获得的槽被释放，scheduler 在该请求候选中继续选择。 |
| 两个并发实例争抢同一 long 账号 | Redis 脚本仅允许一个 long lease；另一实例得到确定拒绝。 |
| normal 请求已超过 30 秒未首输出 | 该账号被视为 stalled，跨模型、跨分组后续请求被过滤。 |
| 流式首个语义输出到达 | lease 立即释放；后续请求可按原 scheduler 竞争，不必等待整个输出结束。 |
| 请求失败、客户端取消或 handler 退出 | release 幂等执行；Redis 中不残留活动 lease。 |
| 进程在首输出前死亡 | 未续期 lease 最多 90 秒后过期，不永久阻塞。 |
| 一笔真实流式请求被判为慢会话 | 写入账号级 slow-session guard；重启后的另一实例、其他模型和其他分组仍会跳过该账号。 |
| 其他分组的轻量 probe 或快速请求成功 | 不解除尚未过期的账号级 guard。 |
| 不同分组有不同现有 scheduler policy | 安全门槛完全相同且不可绕过；通过安全过滤后的排序差异在解释投影中归因于 existing group policy。 |
| 成功/失败回调收到已取消 request context | shared health/quality mutation 使用独立 bounded context 成功调用 store；正常 selection read 仍响应取消。 |
| Redis 不可用 | 请求遵循原有 scheduler 和用户协议；记录不含敏感信息的 store-degraded event。 |
| admission disabled 或 unknown shape | 无额外 Redis admission 操作，既有调度结果不变。 |

## 测试策略

- 先写 service RED 测试：long lease 跨模型/跨分组阻止 normal/long、normal 上限、stalled 跨模型/跨分组阻止、first-output release、release 幂等、账号级 slow-session guard、全局安全门槛不能被 group policy 绕过、Redis fail-open。
- 先写 repository RED 测试（miniredis）：两个并发 acquire 只有一个胜者、过期清理、renew 延长 TTL、release 只能释放本 lease、脚本不包含原始 request/correlation 值。
- 为共享健康写 context 添加 RED 测试：取消的 parent context 仍能让 mutation store 观察到可用 bounded context；`GetAccountModel` 在取消 context 下仍返回取消错误。
- 为 handler/forward 添加 RED 测试：所有 OpenAI scheduler 分组的 HTTP stream 在最终 selection/抢槽后申请账号级 lease、首个 chunk 立即 release、失败和 failover 释放、跨模型/跨分组重选跳过 admission-rejected account；不为 `/count_tokens`、images 或 non-stream 创建 lease，也不出现 Pro、分组名或模型分支。
- 直接相关验证包括 Go 定向 service/repository/handler 测试、`go test ./internal/service -run '^$'`、`go build ./cmd/server`、`gofmt`、`git diff --check`。不扩大为全仓压力/soak 或无关前端测试。
- 发布后只做安全的线上专项验证：确认配置有效、结构化 admission 事件可见、Redis 状态可读、健康端点正常；不主动发送长上下文或付费上游探测。自然真实请求出现后再只读核对 lease/quality 事件和分组 TTFT 变化。

## 发布、回滚与风险

本任务无数据库迁移、无账号数据写入、无 GitHub Actions。根发布总控仅在候选合并后的 `main` 上执行直接相关测试、构建、发布预检和既有蓝绿链。若预检明确返回 `downtime_required=true`，任何切换前停止等待用户授权；预期为 `false`，但以真实输出为准。

紧急回滚首先把 `gateway.openai_shared_health.admission_enabled=false`，恢复既有 scheduler 行为；完整回滚为恢复上一已验证蓝绿镜像。Redis admission/quality key 使用短 TTL，无需数据回滚。主要剩余风险是阈值过低会减少健康账号的短时吞吐、Redis 故障时保护退化为 fail-open，以及本批次未覆盖 WebSocket/非流式路径；这些风险必须在 handoff 和线上验证中保留。

## 批准记录

2026-08-27：用户确认采用“共享首输出前准入保护优先，后续质量信号分桶”的方向，并确认冻结 T76 的实现写入，避免与 T80 并行修改 OpenAI 选路链。用户随后明确阻断性修正：admission 与慢会话保护的共享状态只以 `account_id` 为资源键，跨模型、跨分组；不新增 account-model 质量或配置契约；分组现有策略不能绕过统一安全门槛；`group_id` 质量归属/解释由 T76 在 T80 稳定后刷新。本修订已纳入实现门槛。
