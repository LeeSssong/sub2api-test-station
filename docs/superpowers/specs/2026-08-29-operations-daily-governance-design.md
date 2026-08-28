# 运营日报、错误生命周期、模型准入与调度质量治理设计

日期：2026-08-29  
状态：设计已获用户批准，待拆分实施任务包  
范围：Sub2API 原生后端、原生管理员运营视图、原生调度/错误/账务投影

## 1. 背景与真实证据

本设计基于主站 PostgreSQL、`usage_logs`、`ops_error_logs`、账号/分组配置和 T82/T83 发布记录的只读核对，不使用估算数据或人工制造请求。

已核对事实：

- 2026-08-27 共 9,808 条 usage logs，其中管理员 7,377 条、普通用户 2,431 条。
- 管理员 `actual_cost` 为 $115.067963；普通用户 `actual_cost` 为 $111.824727。
- 全站有效上游成本为 $140.388174，其中管理员 $73.902456、普通用户 $66.485718。
- 有效成本公式为：`COALESCE(account_cost, COALESCE(account_stats_cost, total_cost) * account_rate_multiplier)`。
- 2026-08-27：最终 4xx/5xx 为 1,839 条；HTTP 200 且存在 `upstream_errors` 的自动恢复请求为 107 条；最终失败中带 failover 轨迹 761 条、无 failover 轨迹 1,078 条。
- 2026-08-28：最终 4xx/5xx 为 1,963 条；自动恢复请求 28 条；带 failover 轨迹的最终失败 223 条；无 failover 轨迹 1,740 条。
- T82 生产发布时间为 2026-08-28 12:22:18（北京时间）。按该时间切分，T82 后路由 503 绝对数低于 T82 前，但在错误构成中的占比上升；不能把“占比上升”写成“绝对数量上升”。
- T82 前后有效文本请求的聚合性能发生变化：P50 总耗时约 8,993ms→15,208ms，P95 总耗时约 42,630ms→82,494ms，P50 TTFT 约 6,125ms→8,020ms，P95 TTFT 约 23,206ms→41,880ms。由于请求量、模型和分组构成不同，必须分层后再下因果结论。
- 生产 GPT-Pro、GPT-Plus、【专属】GPT-PRO、GPT-特惠的 `models_list_config` 已启用且列表不含 `gpt-5.6-luna`；但账号 278、279、280、281、282、289、290、291 的账号级 `credentials.model_mapping` 仍含 Luna 映射，且实际日志继续出现 Luna 请求。

## 2. 目标与非目标

### 2.1 目标

1. 让运营日报明确区分普通用户收入、管理员内部消耗和全站有效上游成本。
2. 按一次逻辑请求聚合所有尝试，区分用户可见失败、系统自动恢复、重试后失败和不可安全重放停止。
3. 让启用自定义模型列表的分组把该列表同时作为请求准入规则；本站不允许 Luna 时，在调度前直接拒绝。
4. 解释并拆分路由 503 与上游 502/503，补齐 T82 admission、慢会话保护和安全重放的性能观测。
5. 让 2026-08-27 及后续日报的比较窗口、分母、样本资格和错误密度定义可复算。

### 2.2 非目标

- 不改变 Sub 原生扣费、充值、退款、账号余额或价格写入算法。
- 不新增平行账务源、模型能力表、错误存储或外部控制面。
- 不自动把 Luna 降级为 Sol/Terra。
- 不把管理员使用伪装成外部营收，也不从全站成本中删除管理员对应的真实成本。
- 不为了性能指标制造探测流量；继续遵守 T83 当前 5 分钟真实请求空桶准入规则。

## 3. 原生能力盘点与总体架构

现有原生能力可直接复用：

- 账务：`usage_logs.actual_cost`、`account_cost`、`account_stats_cost`、`total_cost`、`account_rate_multiplier`。
- 请求错误：`ops_error_logs` 的 `request_id`、`client_request_id`、`error_phase`、`status_code`、`upstream_errors`、`duration_ms`、`time_to_first_token_ms`、`upstream_*` 字段。
- 请求尝试：`logical_request_id`、`attempt_id`、`failover` 相关结构化日志。
- 模型能力：`DiagnoseModelAvailabilityForPlatform`、`ListModelAvailabilityCandidates`、账号 `IsModelSupported`/`GetMappedModel`。
- 分组模型目录：`groups.models_list_config`。
- 调度质量：T82 shared-health、admission lease、slow-session guard、`unsafe_to_replay`、`switch_allowed`。

总体数据流：

```text
原生 usage_logs + ops_error_logs + groups/accounts 配置
        ↓
固定时间窗/资格过滤/逻辑请求聚合
        ↓
日报读模型与管理员诊断投影
        ↓
收入-成本、错误生命周期、路由原因、TTFT/耗时分层指标
```

四个实施任务包保持垂直边界：

1. 账务与日报口径；
2. 错误生命周期投影；
3. 分组模型准入与 Luna 清理；
4. T82 性能与路由 503 观测治理。

## 4. 账务与日报口径

### 4.1 指标定义

```text
external_revenue = SUM(actual_cost) WHERE user is ordinary user
admin_internal_consumption = SUM(actual_cost) WHERE user is administrator
effective_upstream_cost = SUM(
  COALESCE(account_cost,
           COALESCE(account_stats_cost, total_cost) * account_rate_multiplier)
)
operating_contribution = external_revenue - effective_upstream_cost
```

管理员使用是内部消耗，不计入外部收入；其对应上游成本仍计入全站有效成本。

### 4.2 对账不变量

日报生成时验证：

```text
ordinary_actual_cost + admin_actual_cost = all_actual_cost
ordinary_effective_cost + admin_effective_cost = all_effective_cost
```

不满足时输出 `reconciliation_status=failed`，禁止生成带利润结论的成功状态。

失败占位流水按既有 T49 规则排除：`usage_completeness='unknown'` 不进入正常收入/成本分母；不删除、不改写历史流水。

### 4.3 2026-08-27 基线

日报应展示：普通用户收入 $111.824727、管理员内部消耗 $115.067963、全站有效上游成本 $140.388174、对外经营贡献 -$28.563447。金额展示精度可按页面规范四舍五入，但底层计算保留原始精度。

## 5. 错误生命周期投影

### 5.1 聚合键

优先使用 `logical_request_id`；缺失时使用 `request_id`，并保留 `client_request_id` 作为辅助关联。一个逻辑请求的多个 attempt 必须只产生一条最终用户结果。

### 5.2 终态分类

```text
auto_retry_recovered
  = final_status=200 AND upstream_errors IS NOT NULL

retry_exhausted_user_visible
  = final_status in 4xx/5xx AND attempt_count>1

single_attempt_user_visible
  = final_status in 4xx/5xx AND attempt_count=1

stopped_unsafe_to_replay
  = unsafe_to_replay=true OR switch_allowed=false
    OR switch_reason='retry_policy'
```

分类优先级：先识别 `stopped_unsafe_to_replay`，再识别自动恢复，最后按 attempt 数区分单次失败与重试耗尽。所有最终非 2xx 均标记 `user_visible=true`，除非明确记录客户端断开且没有服务端终态。

### 5.3 运营字段

日报读模型/接口至少提供：`attempt_count`、`failover_count`、`upstream_error_count`、`final_status`、`terminal_reason`、`unsafe_to_replay`、`switch_allowed`、`user_visible`、`auto_retry_recovered`、`stopped_by_retry_policy`、`error_phase`。

### 5.4 流式契约

- 流未开始：按既有 HTTP 错误响应返回。
- 流已开始：保持 HTTP 200，发送单个终止事件；不得伪造新的 HTTP 400，也不得自动重放已输出内容。
- 用户响应只返回脱敏错误；管理员诊断保留账号、分组、上游状态和安全重放证据。

## 6. 分组模型准入与 Luna 治理

### 6.1 为什么现有配置未生效

`models_list_config` 当前只被 `/v1/models` 用于过滤展示列表；实际请求路径直接读取客户端 `model` 并进入账号选择。账号级 `model_mapping` 仍含 Luna，因此“目录不可见”没有成为“请求不可用”。

### 6.2 请求准入规则

当分组 `models_list_config.enabled=true` 且模型列表非空时：

```text
requested_model 不在分组允许列表
  → routing 阶段直接拒绝
  → 不选择账号、不 failover、不访问上游、不产生上游成本
```

当前四个生产 OpenAI 分组均不允许 `gpt-5.6-luna`，因此 Luna 请求应在入口被拒绝。

建议错误契约：

```http
HTTP/1.1 400
```

```json
{
  "error": {
    "type": "invalid_request_error",
    "code": "model_not_supported",
    "message": "本站当前分组不支持 gpt-5.6-luna，请切换模型重试",
    "param": "model",
    "retryable": false,
    "resume_supported": false
  }
}
```

不发送 `Retry-After`，不触发服务端自动重试。诊断查询失败时保守回退到既有 503，不把数据库故障误判为模型不支持。

### 6.3 账号配置清理

对生产账号 278、279、280、281、282、289、290、291 的 Luna 映射执行原生管理员配置清理，并在清理前后记录脱敏配置摘要和哈希。清理不是入口准入的替代品；入口准入必须长期生效。

### 6.4 覆盖范围

Chat Completions、Responses、Messages、OpenAI-compatible 入口，以及流式/非流式路径必须共享同一准入 helper。图片、embedding、count tokens 等不接受 Luna 的入口也必须返回一致的 `model_not_supported` 或其既有协议映射，不得继续进入账号轮询。

## 7. T82 性能与路由 503 治理

### 7.1 术语

`error_phase=routing`、HTTP 503、`local_capacity_exhausted`、`account_id=null` 表示本站在调用上游前没有选出合资格账号；它不是上游返回的 503。

`error_phase=upstream`、HTTP 502/503 表示已经选中账号，失败来自上游连接、网关或上游服务。

### 7.2 原因分解

路由 503 至少投影为：

```text
no_candidate
transient_account_block
admission_lease_rejected
slow_session_guard
model_policy_rejected
no_available_channel
diagnosis_failure
```

模型策略拒绝在本设计实施后应优先转为 400 `model_not_supported`，不再伪装成 503。

### 7.3 性能字段

补齐并统一毫秒单位：`auth_latency_ms`、`routing_latency_ms`、`admission_wait_ms`、`account_selection_ms`、`upstream_connect_ms`、`time_to_first_token_ms`、`response_duration_ms`。

同时记录：`admission_result`、`admission_reject_reason`、`slow_session_guard_hit`、`safe_replay_decision`、`switch_allowed`、`switch_reason`。

### 7.4 比较方法

任何 T82 前后比较必须固定：北京时间时间窗、`group_id`、请求模型、流式标志、请求类型、有效文本请求资格和最终状态；按分组/模型/账号分别计算 P50/P95，不得用不同样本构成的全量聚合直接宣称因果。

错误密度分母固定为同一资格集合中的逻辑请求数；主动探测不得混入有真实请求的 5 分钟桶。

### 7.5 安全重放与长尾

当 `unsafe_to_replay=true`、`switch_allowed=false` 或 `switch_reason=retry_policy` 时，终止切号并记录 `stopped_by_retry_policy=true`。该行为保留正确性安全边界，但必须单独统计等待时间和终态，避免把它误算成普通路由失败。

## 8. 测试与验收

每个任务包只运行直接相关测试；组合验收至少覆盖：

1. 管理员与普通用户收入/成本分栏及对账不变量；
2. 107/28 条自动恢复样本的生命周期分类；
3. 单次失败、failover 后失败、不可安全重放停止；
4. 四类 API 入口、流式/非流式的模型准入；
5. Luna 不产生上游 attempt 和计费流水；
6. 分组目录缺失但账号映射存在时仍被入口拒绝；
7. 诊断查询失败回退 503；
8. 路由 503 与上游 503 的相位、状态码、原因分解；
9. T82 admission/slow-session/safe-replay 字段可复算；
10. 固定时间窗、有效请求分母和 T83 空桶门禁不回归。

## 9. 发布、回滚与权限边界

- 本规格只允许本地设计与测试，不包含生产配置写入或主站部署授权。
- 生产账号 Luna 映射清理属于明确的配置变更，必须在独立任务包中记录前后脱敏快照，并按验收站/主站全局授权规则发布。
- 主站只有用户明确“测试站验收通过，部署主站”或“快速部署到主站”时才可发布。
- 回滚使用既有蓝绿上一已验证版本；不删除历史 usage/ops 日志，不回滚已生成的日报证据。
- 新增字段或读模型必须向后兼容旧日志；缺失字段按保守未知处理，不猜测为成功或永久不支持。

## 10. 完成判据

设计阶段完成：本文件已写入并自审，无 TBD/TODO，四个任务包边界明确。  
实施阶段完成：每个任务包的功能实现与直接相关测试通过。  
发布阶段完成：按项目全局约束完成推送、部署、线上验证；在此之前不得标记任务 DONE。
