# 上游调度韧性二期 S2：共享健康、故障域与抗故障重试设计

**日期：** 2026-08-17  
**状态：** 发布总控代审批准，可进入实施计划  
**任务包：** S2  
**基线：** `main@ed48df777f06c83727c0db78a40f15010d45ae1a`  
**依赖：** S1-R2 已完成生产验收  
**发布目标：** `downtime_required=false`

## 1. 问题与当前代码证据

S1-R2 已把余额耗尽、明确凭据失效、明确模型不支持和未完成 SSE 等故障接入 Sub 原生隔离、账号模型 transient、half-open 与恢复语义，但 transient 与 EWMA 仍主要是单进程状态：

- `internal/service/openai_account_model_transient.go` 使用进程内 map 保存 10/45 秒 cooldown、failure streak 和 `halfOpenInFlight`；另一 API 实例或蓝绿新实例不会立即看到这些状态。
- `internal/service/openai_account_scheduler.go` 的账号 error-rate/TTFT EWMA 使用原子内存状态；多实例各自学习，蓝绿切换后重新变冷。
- `internal/handler/openai_gateway_handler.go` 的 Responses 与 Messages 循环分别维护 `failedAccountIDs`、同账号重试次数和账号切换次数，但没有统一的“最多 4 次尝试、最多 2 个普通故障域、总预算 5 秒”门。
- 当前 handler 已有 `logical_request_id` / `attempt_id`、流式输出检测、副作用检测、usage attempt 记录和账务 dedup；这些能力应被复用，不新建账务事实源。
- `internal/repository/redis.go` 已提供带连接池、超时和 TLS 的共享 Redis 客户端；scheduler snapshot/outbox 已形成 Redis 运行时投影与数据库事实分层的原生先例。
- 账号实体没有通用 `provider_id`、`channel_id` 或 `quota_pool_id` 列。请求分组可由原生 `ChannelService` 解析渠道 ID；额度池只能读取账号 `extra.quota_pool_id` 的显式值，禁止根据名称、URL 或凭据内容猜测。
- `usage_billing_dedup` 和 attempt metadata 已保证同一逻辑请求的失败尝试可审计、最终成功只扣一次；S2 必须保持这一边界。

S2 要解决的是：一个实例已观察到 transient 或正在执行 half-open 时，其他实例仍可能立即命中同一账号模型；同时单次请求在上游公共故障时可能把既有账号切换能力放大为过多尝试。

## 2. 目标

1. 以 Redis 共享可重建的账号模型 transient、cooldown、half-open lease 和 error-rate/TTFT EWMA；S1 的数据库确定性隔离继续拥有最高优先级。
2. 使用可证实的 `provider_channel`、显式 `quota_pool` 和 `unknown` 故障域，优先跨域切换并限制一个请求可触达的普通故障域数量。
3. 对 Responses 与 Messages 使用同一请求本地预算：最多 4 次上游尝试、3 次账号切换、2 个普通故障域、默认总重试预算 5 秒。
4. 429 尊重合法 `Retry-After`；5xx 与连接错误使用有界指数退避。
5. 已输出、含工具/外部副作用或幂等性未知的请求禁止自动完整重放。
6. Redis 读写失败不把共享健康误报为 healthy，不绕过 S1 veto，也不把 Redis 故障扩散成全站失败。

## 3. 非目标

- 不修改 S1 分类、账号主状态、模型原生限流、余额隔离或管理员恢复流程。
- 不改变 Top-K、sticky、利润门、价格、倍率、用户扣费、账号计费或 usage 日志语义。
- 不新增数据库迁移、第二套审计表、第二套账务源或外部控制面。
- 不实现 S3 的动态 Top-K、sticky escape、TTFT 并行竞速或新监控 UI。
- 不把 Redis 作为确定性隔离事实或用户账务事实。
- 不对图片、embedding、WebSocket、Live 或非 OpenAI 文本入口扩大本任务范围；这些路径保留现有边界。
- 不使用 GitHub Actions，不部署生产，直到用户另行明确发出部署指令。

## 4. 方案比较

| 方案 | 做法 | 优点 | 缺点 | 结论 |
|---|---|---|---|---|
| A. 继续每实例本地状态 | 只增加本地 cooldown 和随机抖动 | 变更最少 | 多实例重复撞击、蓝绿状态丢失、half-open 不互斥 | 不采用 |
| B. Redis 共享运行时投影 + 请求本地预算 | Redis 共享健康与租约；单个 HTTP 请求在 handler 内强制预算 | 复用原生 Redis/调度体系；不把每次请求强一致性绑到 Redis；Redis 故障可降级 | 需要版本化键、Lua 原子更新和本地可信快照 | **采用** |
| C. 所有健康与请求预算写 PostgreSQL | 每次 attempt 写库并由数据库裁决 | 审计性强 | 高频写放大、延迟和锁竞争；与“可重建运行时投影”边界冲突 | 不采用 |

采用 B。历史方案中 Redis `request-budget` 键不进入本次实现：一个入站 HTTP 请求始终由同一 handler 循环拥有，请求本地预算更可靠、更快，并且 Redis 不可用时仍能强制上限。Redis 只保存跨请求、跨实例有价值且可丢失重建的状态。

## 5. 架构与组件

### 5.1 `OpenAISharedHealthStore`

在 service 层定义接口，repository 层使用现有 `go-redis` 客户端实现：

```go
type OpenAISharedHealthStore interface {
    GetAccountModel(ctx context.Context, key OpenAISharedHealthKey) (OpenAISharedHealthSnapshot, error)
    RecordAttempt(ctx context.Context, event OpenAISharedHealthEvent) (OpenAISharedHealthSnapshot, error)
    AcquireHalfOpen(ctx context.Context, key OpenAISharedHealthKey, owner string, ttl time.Duration) (OpenAISharedHalfOpenLease, bool, error)
    CompleteHalfOpen(ctx context.Context, lease OpenAISharedHalfOpenLease, success bool, observedAt time.Time) error
}
```

repository 负责 key 编码、JSON/Lua、TTL、幂等 event ID 和 fencing token；service 负责业务分类、S1 优先级、本地 fallback 与日志。

### 5.2 Redis 键

```text
sub2api:openai:shared-health:v1:account-model:{account_id}:{model_hash}
sub2api:openai:shared-health:v1:domain:{domain_type}:{domain_hash}:{model_hash}
sub2api:openai:shared-health:v1:lease:{account_id}:{model_hash}
sub2api:openai:shared-health:v1:event:{event_hash}
```

- model/domain/event 使用 SHA-256 截断后的不可枚举摘要；值中可保存已限制长度的 canonical model 和脱敏域 ID，禁止保存 API Key、提示词、响应正文或用户原始 ID。
- event 键以 `SET NX` 或同一 Lua 脚本实现 10 分钟幂等；同一 attempt 不能重复增加 streak/EWMA。
- cooldown 状态 TTL 为 `max(cooldown_until-now, 0) + 10 分钟`；仅 EWMA 的 healthy 状态 TTL 30 分钟。
- 未识别 `schema_version` 时返回 unavailable/unknown，不能当作 healthy。

### 5.3 共享值

```json
{
  "schema_version": 1,
  "revision": 12,
  "account_id": 153,
  "canonical_model": "gpt-5.6-sol",
  "state": "soft_failed|cooldown|half_open|healthy",
  "failure_streak": 2,
  "cooldown_until_unix_ms": 1786944045000,
  "last_status_code": 503,
  "last_error_type": "transient_upstream",
  "ewma_error_rate": 0.36,
  "ewma_ttft_ms": 1810,
  "observed_at_unix_ms": 1786944000000
}
```

failure streak 与 cooldown 保持现有 S1-R2 语义：第一次 soft failure 允许一次安全的同账号短重试；第二次 10 秒；第三次及以后 45 秒。S2 不借机改变现有阈值。

### 5.4 本地可信快照

`OpenAIGatewayService` 保留现有本地 transient 状态，并增加最近共享快照缓存：

- Redis 正常：共享快照与本地状态取更保守结果；任一处 blocked 即 blocked。
- Redis 读失败：最多使用 30 秒内的最后共享快照，并继续排除当前请求失败账号。
- 快照超过 30 秒：共享状态为 unknown；请求预算把剩余自动尝试收窄为最多一次跨账号尝试，但绝不放行 S1 已知坏账号。
- Redis 写失败：本地状态照常更新；记录脱敏 warning，不额外等待超过 100ms，不阻塞用户响应。

## 6. 故障域事实契约

```go
type OpenAIFailureDomain struct {
    Type string // provider_channel | quota_pool | unknown
    ID   string
}
```

- `provider_channel`：`account.Platform + ":channel:" + channelID`。channelID 仅来自当前请求 group 经原生 `ChannelService` 的解析；无渠道或解析失败时不能伪造。
- `quota_pool`：仅接受账号 `Extra["quota_pool_id"]` 的非空字符串/整数显式值，并加 platform 前缀。
- `unknown`：没有上述事实时使用。多个 unknown 不视为相互独立；一次请求最多把 unknown 计作一个普通域。
- 账号模型键始终存在；域只影响排序偏好、跨域预算与共享 domain EWMA，不把一个域的观测升级为账号数据库硬禁用。

选号优先级：S1 veto → 当前请求账号排除 → 共享 cooldown → 未失败 provider_channel → 未失败 quota_pool → 已见域的健康候选 → 单一 half-open。

## 7. 请求预算与退避

```go
type OpenAIRetryBudget struct {
    StartedAt         time.Time
    Deadline          time.Time
    Attempts          int
    AccountSwitches   int
    Domains           map[string]struct{}
    MaxAttempts       int // 4，含首次
    MaxSwitches       int // 3
    MaxDomains        int // 2
}
```

- 每次真正发起上游 HTTP 调用前消费一次 attempt；利润门否决和单纯选号失败不消费 attempt。
- 同账号重试也消费 attempt，但不消费 account switch/domain。
- 切换到不同账号时消费 switch；首次域不算“切换”，但计入最多 2 个普通域。
- 任何等待前检查 `deadline`；退避超过剩余时间时立即返回预算耗尽错误。
- 503/502/504/520–524 与连接错误：`120ms, 240ms, 480ms, 960ms`，加稳定 0–20% 抖动，单次不超过 2 秒。
- 429：从 `UpstreamFailoverError.ResponseHeaders` 解析 delta-seconds 或 HTTP-date。合法值优先；如果等待不能在总预算内完成，不立即重试该域，转向不同健康域或终止。
- 输出已开始、`tools`、`function_call_output`、Anthropic `tool_result`、图片意图或其他已知副作用：自动完整重放次数为 0。
- 预算耗尽保持现有客户端错误外形，增加服务端稳定 reason：`attempt_limit`、`account_switch_limit`、`failure_domain_limit`、`retry_deadline`、`unsafe_to_replay`。

## 8. 端到端控制流

```text
创建 logical_request_id 和本地预算
  -> 读取/执行 S1 确定性过滤
  -> 解析本请求可证实的故障域
  -> 批量或逐候选读取共享健康（短超时）
  -> 本地 + 共享取更保守结果
  -> 选择账号并消费 attempt
  -> 发起上游调用
      -> 成功：本地成功 + Redis success/EWMA；half-open fencing 完成
      -> transient 且安全：记录幂等事件；计算域与退避；预算允许才重试/切换
      -> 429：遵守 Retry-After；预算不足时跨域或终止
      -> S1 硬故障：沿用原生隔离；S2 不追加 transient 重试
      -> 已输出/副作用：记录失败与恢复事件，不自动完整重放
      -> Redis 错误：本地 fail-safe，预算收窄，不造成全站拒绝
```

## 9. Half-open lease

- 共享 cooldown 到期后，候选仍保持 blocked，直到成功获取 Redis lease。
- lease 包含 owner、fencing token、issued_at、expires_at，TTL 15 秒。
- 同一账号模型同时只允许一个实例获取；未获取实例继续选择其他候选。
- 完成时必须携带 fencing token。旧 owner 的迟到完成不能覆盖新 lease/revision。
- 成功清除 failure streak/cooldown 并更新 success EWMA；失败恢复 10/45 秒 cooldown。
- Redis 不可用时不新发共享 half-open；仅允许本地已有、未过期的 lease 完成，避免多个实例同时探测。

## 10. 计费、流式与兼容性

- 继续使用 `OpenAIRequestAttemptMetadata` 的 `logical_request_id`/`attempt_id`。
- 失败 attempt 只写审计 usage；最终完整 usage 仍由 `usage_billing_dedup` 负责一次性扣费。
- unknown/partial usage 不因重试被伪造成完整扣费；已有 reconciliation 事件保持不变。
- 已输出 SSE 继续走现有 recovery/Continue 语义，不拼接第二条模型流。
- 外部 API 路径、错误主体和现有 `Retry-After` 透传保持向后兼容。
- 不新增数据库迁移。新增配置仅位于现有 YAML/config 结构并具有默认值与上下界校验。

## 11. 配置

```yaml
gateway:
  openai_shared_health:
    enabled: true
    redis_timeout_ms: 75
    stale_after_seconds: 30
    max_attempts: 4
    max_account_switches: 3
    max_failure_domains: 2
    total_retry_budget_ms: 5000
    backoff_initial_ms: 120
    backoff_max_ms: 2000
    half_open_lease_seconds: 15
```

硬上限：attempts ≤ 4、switches ≤ 3、domains ≤ 2、total budget ≤ 5000ms。管理员只能下调；配置越界时启动校验失败，不静默扩大。

## 12. 测试与验收矩阵

| 场景 | 必须证明 |
|---|---|
| 同一 event 重复写 Redis | failure streak/EWMA 只更新一次 |
| 实例 A 冷却，实例 B 选号 | B 跳过相同账号模型 |
| cooldown 到期并发竞争 | 只有一个 fencing lease 成功 |
| 旧 owner 迟到完成 | 不覆盖新 revision |
| Redis 读失败且快照 10 秒 | 使用更保守的本地/快照结果 |
| Redis 失败且快照 31 秒 | 状态 unknown，最多剩余一次跨账号尝试，S1 veto 仍生效 |
| 连续 503 | 最多 4 次 attempt，退避有界，总预算不超过 5 秒 |
| 429 `Retry-After: 45` | 不在当前 5 秒预算内原地重试该域 |
| 第一 provider_channel 失败，第二域健康 | 优先第二域，域数不超过 2 |
| 全部候选 unknown | unknown 只计一个域，不假设彼此独立 |
| 输出后断流 | 0 次完整自动重放，保留恢复事件 |
| tools/function_call_output/tool_result | 0 次自动完整重放 |
| 失败 attempt 后成功 | 账务只扣一次，attempt 审计完整 |

直接相关验证：shared-health repository/service 单元与 Redis 集成测试；scheduler 共享 cooldown/domain 过滤测试；Responses/Messages 预算、Retry-After、流式/副作用和 billing focused tests；受影响包 compile-only、server build、`gofmt`、`git diff --check`。不运行全仓、压力、mutation、soak 或无关浏览器矩阵。

## 13. 发布、线上验证与回滚

- 候选目标为无迁移、`downtime_required=false`；实际值只能由合并后根发布预检确认。
- 当前用户指令为“不部署，等待下一条部署指令”。本任务候选完成后停在 `READY_FOR_ROOT_REVIEW`；不得执行发布预检、蓝绿切换或生产写入。
- 获得后续部署指令后，从已验证且已推送的根 `main` 使用既有本地/宿主蓝绿链；禁止 GitHub Actions。
- 线上专项验收只使用脱敏 Redis key 元数据、健康端点和受控合成错误，不修改生产账号主状态，不制造账务写入。
- 回滚：关闭 `openai_shared_health.enabled`，蓝绿切回上一镜像；v1 key 由 TTL 自然过期。仅在核验精确前缀和数量后才允许清理本发布 key；不删除 S1 数据库事实。

## 14. 风险与边界

- Redis 热键：event 幂等、短超时、TTL 与键长限制；写失败不阻塞请求。
- 候选池读取放大：实现优先批量读取；若只能逐键读取，必须设置请求级短超时并在计划中限制最大读取数。
- 渠道事实变化：每次请求按原生 group/channel 解析，不持久化为账号字段。
- `quota_pool_id` 缺失：降级 unknown，不从 URL、账号名或 API Key 推断。
- 多版本混跑：键带 v1 schema；旧实例不读取，新实例对缺失值按 unknown 处理。
- S3 依赖：S2 只产出稳定健康/预算事件与内部 reason，不实现 S3 的 UI 或排序体验。

## 15. Brainstorming、自审与批准记录

### 已完成的 brainstorming

- 已探索当前代码、S1-R2 行为、Redis 连接池、scheduler snapshot/outbox、handler 重试循环、流式恢复和 billing dedup。
- 已比较 A/B/C 三种方案并选择“Redis 共享运行时投影 + 请求本地预算”。
- 已按架构、数据契约、故障域、预算、失败降级、测试与发布分段复核；没有待决产品问题。
- 视觉 companion 不适用：本任务没有需要用户比较的 UI/布局决策。

### 规格自审

- Placeholder：无 TBD/TODO/待定项。
- 一致性：Redis 只保存可重建投影；S1 数据库事实和 billing dedup 始终优先。
- 范围：仅 OpenAI Responses/Messages 文本主链，不扩展到 S3/UI/其他端点。
- 歧义：unknown 域、quota pool 来源、429 超预算、Redis stale 降级和 no-replay 均已明确。
- 安全：无凭据、提示词、响应正文或不可逆数据操作；无迁移、回填或外部付费。

### 发布总控代审批准

依据 `native-sub-incremental-delivery-constraints.md` 2.3 节中 2026-08-15 起的用户授权，唯一发布总控于 2026-08-17 完成书面代审并批准本规格进入 `writing-plans`。批准仅覆盖既定 S2 范围，不授权部署、不可逆数据操作、安全例外、外部付费或 `downtime_required=true` 动作。
