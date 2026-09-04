# T130 流式容量失败与无首输出反馈闭环规格

**日期：** 2026-09-04  
**状态：** 正式规格，待用户审阅批准  
**范围：** OpenAI/Codex 普通 HTTP 流式请求的账号运行时反馈、后续请求调度降权、短冷却、慢首输出右删失证据及相关调度日志

## 1. 问题证据与当前行为

2026-09-04 主站只读日志、Caddy 流水和 `openai_scheduler_logs` 已确认两类高频故障。

第一类是上游在 SSE 已经开始后返回完整失败事件。近 24 小时共有 106 次 `openai.stream_upstream_failure`，其中账号 `229` 占 61 次。原始错误集中为：

- `Too many pending requests`
- `Concurrency limit exceeded for account`
- `Upstream rate limit exceeded`
- `Service temporarily unavailable`

这些请求均为 `output_started=true`、`safe_to_replay=false`。现有实现正确禁止当前请求自动切号，但失败事件的 HTTP 状态投影为 `0`，没有触发只面向 502/503 的 immediate cooldown。夹杂的一次成功又会通过 `RecordOpenAIAccountModelSuccess` 删除整个账号+模型瞬态状态，包括仍在 60 秒、5 分钟和 15 分钟窗口内的失败时间戳。结果是账号反复回到 `failure_streak=1 / cooldown_seconds=0`。

第二类是上游派发完成后长期没有响应头或有效语义输出。Caddy 记录到多次 `/responses` 在约 125 秒后以 `status=0 / size=0` 结束，API 没有 OOM、重启、EOF、TCP reset 或解码错误。客户端取消被当成普通 client disconnect，没有形成账号慢首输出反馈。一个明确样本在 08:40:50 接收约 1.28 MB 的 `gpt-5.6-sol` 请求，125 秒内没有首字节，08:42:55 由客户端断开。

凌晨部署的 T126 自适应候选池已经运行，但它只调整候选池模式、Top-K 和探索比例，不改变失败分类或健康状态。账号 229 在失败后仍出现 `selected_rank=1`、`live_load_score=100`、`quality_snapshot_stale=true`、`cooldown_seconds=0`，说明调度优化依赖的运行时反馈没有完整进入评分与资格门禁。

## 2. 第一性原则

本任务严格分开两个问题：

1. **当前请求能否重放。** 由输出、usage、副作用和幂等证据决定。已输出或无法证明安全时继续禁止重放。
2. **同一账号是否适合承接后续请求。** 由容量失败、短期失败窗口和慢首输出证据决定。当前请求不能重放，不代表后续请求应继续选择同一过载账号。

客户端取消本身不是上游失败。只有在已经完成上游派发且等待超过慢首输出阈值时，才能记录“至少慢到该时点”的右删失 TTFT 证据；不得据此推断上游未处理、未计费或已经失败。

## 3. 目标

1. 将明确的上游容量/拥塞失败映射为账号+规范模型的运行时容量信号，即使 SSE 已输出、状态码为 0，也立即影响后续请求。
2. 保持已输出请求不自动重放、不切号、不制造第二 attempt 的计费安全边界。
3. 成功请求不再删除尚未自然过期的失败窗口；瞬态状态按有界时间窗和滞回规则恢复。
4. 对“已派发上游、长期无首输出、最后由客户端取消”的请求保留右删失慢信号，立即进入 W1 首字评分。
5. 让本地运行时健康、共享 Redis 健康和多窗口质量评分看到一致的反馈，避免多实例或重启后立即重新集中到过载账号。
6. 在调度日志中明确区分容量失败、普通传输失败、纯客户端取消、慢首输出取消，以及它们是否触发冷却或降权。
7. 不改变账号持久化 `status`、用户价格、usage 计费、利润保护、原生并发槽和 T126 自适应候选池公式。

## 4. 非目标

- 不为已输出请求自动切号或重放。
- 不把所有 `context.Canceled` 都归因给上游账号。
- 不因单次慢首输出把账号永久禁用、写为 error 或修改管理员配置。
- 不新增 admission、slow-session 或 T100 已移除的额外并发限制。
- 不修改 Caddy/Cloudflare 压缩策略；`error decoding response body` 是客户端对未完成流的表象，不以关闭 gzip 掩盖根因。
- 不新增数据库表、历史回填、第二套评分事实源或外部控制面。
- 不修改 WebSocket、生图、非 OpenAI 平台或协议强制绑定路径。
- 不顺带修复 T129 Monitor 查询与缓存分母问题。

## 5. 方案比较与选择

| 方案 | 做法 | 结论 |
| --- | --- | --- |
| A. 仅调大 T126 探索比例 | 让更多请求随机进入其他账号 | 不能阻止排名第一账号持续承接流量，也没有修复反馈丢失；不采用 |
| B. 所有断流或客户端取消统一冷却 | 任意中断都把账号踢出候选池 | 反应快，但会把用户取消、网络切换和客户端超时错误归因给上游；不采用 |
| C. 分类反馈 + 独立后续请求状态（采用） | 明确容量错误进入短冷却和失败窗口；慢首输出客户端取消只进入右删失 TTFT；当前请求重放规则保持独立 | 能修复根因，并保留计费与归因安全边界 |

## 6. 适用路径与控制流

适用于已经进入统一 OpenAI HTTP 文本调度的 Responses、Chat Completions 和 Messages 兼容流式路径。处理顺序如下：

```text
账号资格/模型能力/利润门
-> 运行时健康与容量冷却
-> T114 多窗口质量评分
-> T126 自适应候选池和 Top-K
-> 获取原生账号并发槽
-> 发起唯一上游 attempt
-> 观测响应头、首个语义输出和终态
-> 分类当前 attempt 结果
-> 分别决定：当前请求是否重放、后续请求如何反馈
```

反馈必须在当前请求结束前以尽力但有界的方式写入进程内状态和现有共享健康存储。共享存储失败不得阻塞响应；本地状态仍应生效，并在调度日志标记 `shared_feedback_written=false`。

## 7. 容量失败分类

新增内部受控分类 `upstream_capacity_pressure`，只匹配结构化上游错误字段或规范化后的非敏感错误消息。初始子类固定为：

| 子类 | 典型证据 |
| --- | --- |
| `pending_queue_full` | `Too many pending requests` |
| `account_concurrency_exceeded` | `Concurrency limit exceeded for account` |
| `upstream_rate_limited` | 明确的 upstream rate limit / 429 语义 |
| `service_temporarily_unavailable` | 上游明确的临时不可用事件 |

分类优先读取 SSE `response.failed` / error event 的结构化 code、type 和 status；仅在结构字段缺失时使用大小写无关、边界明确的消息匹配。不得匹配请求正文、用户输出或任意自由文本。

以下情况不进入容量分类：模型不存在、余额不足、401/403、内容安全拒绝、客户端参数错误、本站本地限流、客户端取消、未知 EOF/reset。它们继续走既有专用分类。

## 8. 后续请求容量反馈

### 8.1 单次明确容量失败

任一明确容量失败都对 `(account_id, canonical_model)` 立即写入现有短冷却，初始为 10 秒。该动作与 `output_started`、`safe_to_replay` 解耦：

- 当前请求已输出：不重放，但后续新请求在 10 秒内排除该账号+模型。
- 当前请求尚未输出且满足安全重放：继续使用既有有界 failover；冷却同时对其他请求生效。
- 账号持久化状态保持 `active`。

### 8.2 窗口升级

继续复用 T82 已有窗口，不新增阈值体系：

- 60 秒内 2 次容量/瞬态失败：冷却 60 秒；
- 5 分钟内 3 次：冷却 5 分钟；
- 15 分钟内 5 次：冷却 30 分钟。

失败时间戳按自然时间窗过期。一次普通成功不得清空 `recent_failures`，否则间歇性过载永远无法升级。

### 8.3 成功与恢复

成功事件分开处理“当前阻断状态”和“历史窗口”：

- 未处于 OPEN/half-open：成功降低连续失败 streak，但保留窗口时间戳，直到自然过期。
- 处于冷却到期后的 half-open：继续沿用连续 2 次成功恢复 HEALTHY 的滞回规则。
- 恢复 HEALTHY 后允许普通调度，但历史窗口仍保留；窗口内再次容量失败可按已有数量快速升级。
- 只有窗口自然过期或显式账号配置重置才清除历史失败；普通业务成功不是显式重置。

实现必须避免“一次成功立即完全恢复”和“历史失败永久不消退”两个极端。

## 9. 无首输出客户端取消反馈

### 9.1 归因条件

仅同时满足以下条件时记录 `client_abandoned_after_upstream_wait`：

1. 上游请求已经成功派发；
2. 尚未收到有效语义首输出；
3. 已达到 T114 的 60 秒慢首输出阈值；
4. 最终错误是客户端 context 取消或向客户端写入失败；
5. 没有更强的上游 HTTP、SSE、EOF、reset、DNS 或代理错误证据。

不足 60 秒的客户端取消仍记为普通 `client_disconnected`，不影响账号质量或冷却。

### 9.2 调度作用

满足条件后，复用 T114 的 attempt 级右删失慢信号：

```text
ttft_lower_bound_ms = max(60000, observed_wait_ms)
```

该信号立即影响后续请求的 W1 `first_output_score`，但：

- 不计入失败率；
- 不触发账号冷却；
- 不推断 usage、成本或上游最终状态；
- 不创建第二 attempt；
- 不修改持久化账号状态。

若同一 attempt 随后取得真实终态或真实 TTFT，以真实证据替换占位，不重复计数。若客户端取消使结果永远未知，右删失信号保留到 W1 自然过期。

## 10. 与质量评分和自适应候选池的关系

- 容量冷却属于质量评分之前的硬运行时资格过滤，T126 不得把冷却账号重新加入候选池。
- 右删失慢信号只影响 T114 `first_output_score`，不改变成功率、输出速率或利润分区。
- `live_load_score` 继续表示本站原生槽位负载，不伪装成上游队列深度。容量压力通过独立健康状态表达。
- 质量快照过期时，运行时容量冷却仍必须即时生效；不得因 `quality_snapshot_stale=true` 忽略新失败。
- 共享健康写入成功后，其他 API 实例也应排除相同账号+模型；进程重启不应立即遗忘尚未过期的容量冷却。

## 11. 内部接口与日志契约

扩展现有失败事件和调度决策，不新增公共用户 API。内部字段至少包含：

```text
failure_class
capacity_subtype
feedback_scope
output_started
usage_produced
safe_to_replay
current_request_action
future_request_action
cooldown_seconds
cooldown_until
recent_failure_count_60s
recent_failure_count_5m
recent_failure_count_15m
client_disconnected
upstream_dispatched
first_visible_output_seen
observed_wait_ms
ttft_lower_bound_ms
shared_feedback_written
```

`current_request_action` 与 `future_request_action` 必须分别记录。例如：

```text
current_request_action=stop_unsafe_to_replay
future_request_action=cooldown_account_model
```

调度日志新增或复用以下事件：

- `openai.upstream_capacity_pressure`
- `openai.account_model_cooldown_started`
- `openai.client_abandoned_after_upstream_wait`
- `openai.first_output_slow`
- `openai.scheduler_selection`
- `openai.scheduler_request_outcome`

日志不得包含 Authorization、API key、Cookie、attestation、请求正文、响应正文或完整 SSE data。上游 request ID 可保留；错误消息必须经过现有脱敏器。

## 12. 并发、幂等与共享状态

- 同一 `attempt_id` 的容量失败和慢信号必须幂等，重复 handler/flush 路径不得重复累计。
- 本地状态和 Redis 共享健康使用现有原子更新/事件 ID 机制；不能通过读改写竞态丢失并发失败。
- 多实例同时记录失败时，窗口计数应单调合并；事件重复不得放大计数。
- Redis 不可用时本地状态继续工作，日志标记共享降级；Redis 恢复后不做历史数据库回填。
- 冷却到期继续使用现有单 half-open 租约，不能让并发请求同时绕过。

## 13. 失败与安全语义

- 已输出、usage 已知、存在副作用或幂等未知：当前请求绝不自动重放。
- 容量失败即使不能重放，也必须反馈给后续请求。
- 单纯客户端取消不得触发失败率或冷却。
- 慢首输出只提供右删失体验证据，不证明上游失败或未计费。
- 状态/共享存储写入失败不得覆盖上游原始错误，不得阻塞客户端响应。
- 所有候选均冷却时沿用既有可用性兜底和半开规则，不伪造健康，也不无限循环。

## 14. 兼容性、迁移与配置

- 不新增数据库迁移，不修改 `usage_logs` 或 `openai_scheduler_logs` schema。
- 不修改现有管理员设置；初始阈值复用 T82/T114 常量。
- 旧日志缺少新字段时继续显示“证据不足”，不回填。
- 配置读取失败继续使用现有默认值；不得因反馈组件异常阻断全部请求。
- 预期 `downtime_required=false`，最终以根目录干净 `main` 的发布预检为准。

## 15. 场景化验收矩阵

| 场景 | 当前请求 | 后续请求 |
| --- | --- | --- |
| 已输出后 `Too many pending requests` | 返回原上游失败，不重放 | 账号+模型立即冷却 10 秒 |
| 已输出后账号并发超限 | 不重放 | 容量分类、短冷却生效 |
| 未输出且可安全重放的容量失败 | 按既有预算切号 | 原账号同时进入冷却 |
| 60 秒内第二次容量失败 | 各请求保持原重放边界 | 冷却升级为 60 秒 |
| 成功夹在两次失败之间 | 成功正常完成 | 第一次失败时间戳不被清空，第二次可升级 |
| 失败窗口自然过期 | 无影响 | 旧失败不再参与升级 |
| 上游派发后 125 秒无首输出，客户端取消 | 不重放、不推断费用 | 写右删失 TTFT，W1 首字分下降，不冷却 |
| 30 秒内客户端主动取消 | 立即结束 | 不写慢信号、不降权、不冷却 |
| 客户端取消前已有明确上游 reset | 按真实上游传输失败处理 | 使用真实失败分类，不降级为客户端取消 |
| Redis 写入失败 | 返回原请求结果 | 本地反馈生效，日志标记共享降级 |
| 质量快照过期 | 原请求不受额外阻断 | 新容量冷却仍先于质量评分生效 |
| 所有候选均冷却 | 不无限重试 | 既有半开/可用性兜底生效 |

## 16. 测试策略

采用 TDD，先写直接失败用例，再实施最小修复。

后端单元测试至少覆盖：

- 四种容量错误分类及非容量错误反例；
- `status_code=0 + response.failed + output_started=true` 仍触发后续短冷却；
- 当前请求不重放与后续请求冷却相互独立；
- 成功不清空窗口、窗口自然过期、half-open 两次成功恢复；
- 并发/重复 attempt 事件幂等；
- 60 秒以上无首输出客户端取消形成右删失样本；
- 60 秒以内取消和已有语义输出不形成慢样本；
- 更强上游错误优先于客户端取消；
- Redis 成功、失败和多实例共享反馈；
- T126 不重新加入冷却账号，T114 W1 分数即时下降；
- 既有 502/503、429、余额、401、计费安全和 retry budget 回归。

最小验证：相关 service/handler/repository Go 测试、`go build ./cmd/server`、`gofmt`、`git diff --check`。不运行无关前端测试、全仓压力测试或制造生产失败流量。

## 17. 发布、线上验证与回滚

本规格不授权合并、推送或部署。T129 仍处于 `IMPLEMENTING` 且发布暂停，T130 可以设计和实现，但不得抢占唯一整合/部署车道。

发布前必须：

1. 候选从届时最新 `origin/main` 刷新并完成直接相关测试；
2. 合入并推送根目录干净 `main`；
3. 确认 `HEAD` commit/tree 与 `origin/main` 一致；
4. 发布预检明确输出 `downtime_required=false`；若为 `true`，在任何停服、迁移、重启或切换前暂停并请求授权；
5. 只有满足项目定义的主站明确授权语义后才能部署主站。

线上只读验收不主动制造付费失败，使用自然流量观察：

- 容量失败后调度日志同时出现当前请求停止原因和后续冷却动作；
- 同一账号在冷却期不再被普通请求选中；
- 成功夹杂时窗口计数不被清零；
- 长时间无首输出客户端取消产生右删失慢信号但不产生失败/计费推断；
- API、worker、Caddy 无重启/OOM，健康探针通过。

回滚为恢复上一已验证镜像或对修复 commit 做可审计 revert 后从根 `main` 重新发布。无数据库迁移、历史回填或生产数据回滚。

## 18. 明确不可推广内容

- 本次排查读取到的生产日志、账号名称、请求关联 ID和上游 request ID不得写入发布制品或公共接口。
- 主站与验收站的数据库、Redis、账号、凭据、日志和业务数据不得互相复制。
- 不得把验收站服务器目录、容器或运行镜像作为主站代码来源。

## 19. 未决事项

无产品未决事项。容量短冷却使用现有 10 秒常量，窗口升级复用 T82 的 60 秒/5 分钟/30 分钟规则，慢首输出阈值复用 T114 的 60 秒规则；实施不得另增可调参数或管理页面。

## 20. 批准记录

- 2026-09-04：用户认可两项根因及优化方向，并要求输出修复规格书。
- 当前仍需用户审阅并明确批准本书面规格，批准后才能编写实施计划和开始实现。
