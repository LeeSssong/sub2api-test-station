# relay-ops 上游监控与预警 Agent 设计

## 状态

设计已由用户确认并根据 Sub2API 原生能力完成第一轮修订（2026-07-19）。`relay-ops` 已实现并以 `read_only` 模式部署；真实飞书/Agent 凭据、候选记录和付费 probe 尚未启用，生产分组切换也不属于本次部署结果。

## 主线与范围

本设计服务于当前主线：

> 在首尔服务器上，让正式产品能够以稳定、透明、可控的方式接待首批真实用户，并持续优化服务质量。

设计优先级固定为：

1. 服务质量：成功率、SSE 完整率、TTFT、延迟、429/5xx、超时和容量。
2. 运营风险：上游失效、倍率/模型价格变化、Key 或会话失效、配置漂移。
3. 成本利润：默认相信上游标注的倍率和价格；真实账单只作为辅助证据和提示。

### 包含

- 自动发现 Sub2API 中所有已启用且对客户公开的分组。
- 监控生产上游、模型目录、公开价格、实际请求质量和估算成本。
- 管理员手动录入候选中转站，并用独立低额度 Key 探测。
- 用统一指标比较候选站与当前公开分组。
- 提供公开模型定价页，复用 Sub2API 登录用户渠道状态页，并补充管理员运维后台。
- 事件状态机、飞书通知、每日摘要和只读预警 Agent。

### 不包含

- 自动切换上游、自动改价格、自动充值或自动修改用户余额。
- 让 Agent 读取或操作 API Key、Cookie、密码、提示词、响应正文或数据库写权限。
- Fork 或重写 Sub2API 核心代码。
- 重做 Sub2API 已有的渠道监控、用户渠道状态页、Ops 指标、Usage 账单字段或后台任务监控。
- 把真实账单核对做成用户上线或生产路由切换的硬阻塞。
- 支付、公开无限注册、复杂推荐系统和多节点高可用。

## Sub2API 原生复用边界

截图中的“渠道状态”界面来自 Sub2API `v0.1.161` 原生实现，不是另一个需要引入的开源项目。原生功能已经具备：

- 用户侧 `/monitor`：状态卡、对话延迟、端点 PING、7/15/30 天可用率和近 60 次记录。
- 管理侧 Channel Monitor：名称、供应商、端点、API Key、主模型、附加模型、分组名、Chat Completions/Responses、间隔、抖动、自定义请求头/正文、立即运行和历史聚合。
- 管理侧 Ops：QPS/WebSocket、SLA、错误与上游错误、首 Token 延迟 P50/P90/P95/P99、总延迟、CPU/内存/健康评分、错误趋势/分布/详情、后台任务和分组可用性。
- Usage/计费：标准费用、实际费用、账号成本、用户扣费、账号扣费以及模型/分组/错误分类等现有数据。
- 可用渠道：登录用户可查看自己可访问的渠道、模型和定价；首版公开 `/pricing` 可以复用同一份 Sub2API 渠道定价数据，但匿名公开投影仍由 `relay-ops` 提供，不能假设原生页面支持免登录。

因此职责固定如下：

| 能力 | 事实来源与界面 | `relay-ops` 的职责 |
|---|---|---|
| 生产分组合成监控与历史 | Sub2API Channel Monitor | 保存 `monitor_id` 引用、读取聚合结果、触发跨来源告警，不复制原始历史 |
| 用户性能页 | Sub2API `/monitor` | 不重做页面，只控制哪些已合格监控可对用户显示 |
| 本站真实流量、TTFT、延迟、SLA、错误、资源和后台任务 | Sub2API Ops/Usage | 读取已有只读聚合或 API，生成跨站比较与飞书摘要 |
| 公开模型定价 | Sub2API 渠道定价为事实来源 | 输出脱敏、免登录的只读 `/pricing` 投影和变更时间 |
| 候选站、价格/倍率页变化、跨站机会比较 | 无原生完整能力 | 由 `relay-ops` 采集、归一化和持久化 |
| 事件去重、飞书、Agent 分析 | 无原生完整能力 | 由 `relay-ops` 状态机负责 |

原生实现证据：

- [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api/tree/v0.1.161)
- [用户渠道状态页面](https://github.com/Wei-Shaw/sub2api/blob/v0.1.161/frontend/src/views/user/ChannelStatusView.vue)
- [渠道状态卡片](https://github.com/Wei-Shaw/sub2api/blob/v0.1.161/frontend/src/components/user/monitor/MonitorCardGrid.vue)
- [渠道状态中文文案](https://github.com/Wei-Shaw/sub2api/blob/v0.1.161/frontend/src/i18n/locales/zh/dashboard.ts)
- [渠道监控用户 API](https://github.com/Wei-Shaw/sub2api/blob/v0.1.161/frontend/src/api/channelMonitor.ts)
- [渠道监控管理页面](https://github.com/Wei-Shaw/sub2api/blob/v0.1.161/frontend/src/views/admin/ChannelMonitorView.vue)
- [渠道监控服务](https://github.com/Wei-Shaw/sub2api/blob/v0.1.161/backend/internal/service/channel_monitor_service.go)
- [调度器与历史聚合](https://github.com/Wei-Shaw/sub2api/blob/v0.1.161/backend/internal/service/channel_monitor_runner.go)

Sub2API 原生 Channel Monitor 的间隔范围是 15 至 3600 秒，无法表达候选站每 6 小时一次。因此生产公开分组使用原生监控，候选站仍由 `relay-ops` 独立调度；这是补足能力，不是重复实现。

## 总体架构

`relay-ops` 作为独立服务器服务，与 Sub2API 并行运行：

```text
Caddy
|- /api/*           -> Sub2API
|- /monitor         -> Sub2API 原生登录用户渠道状态页
|- /pricing         -> relay-ops 公共模型定价页
`- /ops             -> relay-ops 管理后台

Sub2API
|- native-channel-monitor
|- native-user-monitor-ui
|- native-ops-and-usage
`- native-channel-pricing

relay-ops
|- sub2api-read-model
|- v2-qualification-orchestrator
|- upstream-adapters
|- candidate-probes
|- pricing-and-cost-normalizer
|- comparison-window-materializer
|- incident-state-machine
|- feishu-notifier
|- read-only-alert-agent
`- web-and-admin-api
```

- 通过 Sub2API 现有 Admin/Monitor/Ops/Usage API 读取公开分组、原生监控聚合、用户用量聚合和配置快照，不直接读写 Sub2API PostgreSQL。
- `relay-ops` 不保存 Sub2API 已有的逐次生产监控历史，只保存稳定外部引用、价格快照、候选探测、事件和为跨来源比较生成的物化窗口。
- `relay_ops` 使用独立 PostgreSQL database 或 schema 保存监控数据；不复用 D04 SQLite 账本。
- 首版单实例内部调度即可；需要多进程时再使用 Redis 锁，不能因此改变监控语义。
- 所有真实凭据通过服务器秘密文件或等效秘密存储注入，禁止出现在 Git、URL、命令参数、日志和 Agent 输入中。

## 核心数据对象

以下对象是第一版的最小持久化边界。原始观察与价格快照追加写入；聚合窗口和当前状态可以重算或更新。

### 1. `public_groups`

代表 Sub2API 中对客户公开的分组，是用户性能页和告警的第一层归属。

关键字段：

- `group_id`：Sub2API 数字或稳定外部 ID。
- `name`：对用户显示的正式名称，例如 `GPT-Pro`、`GPT-Plus`。
- `enabled`、`customer_visible`：由 Sub2API 同步。
- `user_multiplier`：本站对用户的计费倍率。
- `model_allowlist`：允许对客户开放的模型。
- `upstream_route_refs`：关联的上游账号或路由引用，不保存 Key 原文。
- `sub2api_channel_monitor_ids`：关联的 Sub2API 原生 Channel Monitor ID；生产探测历史仍由 Sub2API 持有。
- `qualification_run_id`、`qualified_at`：最近一次完整 V2 准入结果及时间。
- `health_gate`：`pending`、`qualified`、`degraded`、`blocked`。
- `last_seen_at`、`source_revision`。

规则：只有 `enabled=true` 且 `customer_visible=true` 的分组才有资格进入用户渠道状态页和公开分组质量告警，但不能仅凭这两个开关直接公开。新分组必须依次完成：

1. 使用现有 `ops/upstream-benchmark-v2.rb` 和 `mvp-text-v2` profile 完成模型发现、逐文本模型同步/SSE、TTFT/总耗时、容量阶梯和价格证据验收。
2. V2 结果达到 `verified`，或仅因非计费、非同步/SSE 的次要证据缺口成为 `partial` 且管理员明确接受，并绑定已批准的 proposal。模型价格未知、同步失败或 SSE 不完整的 `partial` 不得公开。
3. 在 Sub2API 原生 Channel Monitor 中配置该公开分组的主模型和必要附加模型。
4. 原生监控至少产生一条有效成功样本后，才允许出现在 `/monitor`。

完整 V2 是新分组准入和重大变更后的重新验收工具，不作为每分钟或每 6 小时的常规在线监控。`relay-ops` 只编排 V2、保存结果引用和控制 `health_gate`，不重写 V2 evaluator。

### 2. `upstreams`

代表已接入生产或由管理员录入的候选站点。

关键字段：

- `upstream_id`、`display_name`、`role`：`production`、`candidate`、`paused`、`rejected`。
- `base_url`：规范化后的 OpenAI-compatible Base URL。
- `site_url`、`pricing_url`、`usage_url`、`performance_url`：公开或管理员参考页面。
- `adapter_type`：`sub2api`、`newapi`、`openai_compatible` 或 `unknown`。
- `candidate_probe_key_secret_ref`：仅候选站使用；指向管理员在该上游创建的专用低额度 API Key，保存秘密引用、指纹和末四位，不保存原文。
- `sub2api_channel_monitor_id`：生产/公开分组使用；指向由 Sub2API 原生 Channel Monitor 加密保存的探测 Key 和配置，`relay-ops` 不再保存第二份 Key。
- `billing_auth_secret_ref`：可选；只用于有稳定机器接口或授权会话的站点。
- `advertised_multiplier`、`multiplier_source`、`multiplier_observed_at`。
- `billing_evidence_status`：`not_requested`、`estimated_only`、`verified`、`stale`。
- `monitor_status`、`last_success_at`、`last_error_code`。

最小候选录入字段是名称、Base URL、低额度监测 Key、模型定价/倍率页 URL、用量/账单页 URL。这里的低额度监测 Key 就是管理员在候选上游创建的 API Key；它应独立于将来可能接入生产的 Key。站点类型、模型、端点、可比较公开分组和探测计划自动发现。

### 3. `secret_refs`

保存凭据元数据而非凭据内容本身。

- `secret_ref`、`kind`、`owner_scope`、`fingerprint`、`last_four`。
- `created_at`、`expires_at`、`last_used_at`、`status`。
- 凭据内容只存在服务器秘密存储或只读挂载文件中。

候选监测 Key 与账单会话必须分开。候选监测 Key 只能用于低消耗 API 探测；账单会话如果不存在，不影响质量监控。生产/公开分组的探测 Key 由 Sub2API 原生 Channel Monitor 加密保管，`relay-ops` 的 `secret_refs` 不再复制它。

### 4. `model_catalog` 与 `pricing_snapshots`

`model_catalog` 表示从公开分组、上游 `/models` 和价格页观察到的模型身份；`pricing_snapshots` 表示某一时间点的证据。

模型字段：

- `canonical_model`、`provider_model`、`upstream_id`。
- `input_price_per_1m`、`output_price_per_1m`、`cache_read_price_per_1m`、`cache_write_price_per_1m`。
- `supports_sync`、`supports_sse`、`supports_image`、`supports_audio` 等能力。
- `first_seen_at`、`last_seen_at`、`availability`。

快照字段：

- `source_url`、`source_type`、`fetched_at`、`content_hash`。
- `normalized_payload`：不含 Cookie、Key、账号标识和用户数据。
- `diff_summary`：新模型、删除模型、价格变化、倍率变化和无法解析字段。
- `evidence_level`：`public_page`、`api_response`、`live_probe`、`manual_billing`。

### 5. `probe_profiles` 与 `probe_runs`

`probe_profiles` 只保存 `relay-ops` 自有的页面变化和候选探测策略；生产分组在线质量调度由 Sub2API 原生 Channel Monitor 配置：

- 生产价格/倍率页：每 5 分钟采集一次；内容 hash 未变化时不重复解析和通知。
- 候选站：固定每 6 小时执行一个采集周期，周期内完成价格/倍率页、`/models` 和受限同步/SSE 探测。
- 候选站日常周期复用 V2 的模型发现、请求、指标和脱敏组件，但使用低成本 `candidate-watch` profile；不运行完整容量/RPM 阶梯。
- 候选站准备转为公开分组，或已公开分组的模型/协议发生重大变化时，单独运行完整 V2 准入验收；仅录入候选站不会自动运行昂贵的容量/RPM 阶梯。
- 生产质量与健康、无真实流量时的合成样本均由 Sub2API 原生 Channel Monitor 负责，`relay-ops` 不重复发请求。
- 单次最大输出 Token、单站每日费用上限和全部候选站总预算上限。

`probe_runs` 记录一次可重放的探测批次：

- `run_id`、`upstream_id`、`group_id`、`model_id`、`probe_kind`。
- `started_at`、`finished_at`、`status`、`http_status`、`error_class`。
- `input_tokens`、`output_tokens`、`cache_tokens`。
- `ttft_ms`、`total_latency_ms`、`tps`、`sse_done`。
- `estimated_standard_cost`、`estimated_upstream_cost`、`actual_upstream_cost`（可空）。
- `evidence_level`、`request_fingerprint`、`redaction_version`。

探测结果全部入库，但不逐条发送飞书。飞书只消费经过状态机确认的事件。

### 6. `metric_refs` 与 `comparison_windows`

Sub2API 已经保存生产合成历史、日聚合和本站真实流量 Ops/Usage 指标，`relay-ops` 不复制这些原始数据。`metric_refs` 保存查询范围、原生 monitor ID、Ops/Usage 查询版本、窗口结束时间和内容 hash；`comparison_windows` 只物化跨来源比较所需的统一口径：

- `window_start`、`window_end`、`sample_count`。
- `success_rate`、`sse_completion_rate`、`rate_429`、`rate_5xx`、`timeout_rate`。
- `ttft_p50_ms`、`ttft_p95_ms`、`latency_p50_ms`、`latency_p95_ms`、`tps_p50`。
- `concurrency_peak`、`queue_time_p95_ms`、`cache_hit_rate`。
- `source_kind`：`sub2api_real_traffic`、`sub2api_native_monitor` 或 `relay_ops_candidate_probe`，三者分别统计，不能混成一个成功率。
- `source_ref`：Sub2API 原生查询引用、Channel Monitor ID/窗口或候选 `probe_run` 引用。

用户错误，例如错误 Key、错误模型和参数错误，沿用 Sub2API SLA 口径，不计入平台成功率；跨站比较必须记录使用的口径版本。只有需要与候选站对齐的时间窗口才物化，其他管理查看直接读取 Sub2API 原生 Ops/Usage 页面。

### 7. `cost_observations`

成本只作为辅助证据，不自动改变路由。

- `source`：`advertised`、`estimated_from_tokens`、`provider_reported`、`manual_billing`。
- `standard_cost`、`actual_cost`、`effective_multiplier`。
- `model_id`、`group_id`、`observed_at`、`confidence`。
- `comparison_note`：说明缺少账单 API、页面登录态过期或价格推算限制。

当没有上游机器可读账单时，系统显示“按公开定价估算”，不把估算值写成真实扣费。

### 8. `candidate_comparisons`

将候选站与所有模型重叠的公开分组比较，输出分项标签而非单一分数：

- `more_stable`：成功率、SSE 完整率和错误率更好。
- `faster_ttft`：TTFT P95 至少改善 20%。
- `cheaper`：有效或公开成本至少降低 10%。
- `overall_better`：服务质量不低于当前、风险门槛通过，并且成本更低或延迟明显更快。
- `evidence_window`、`sample_count`、`comparison_status`、`recommended_action`。

只有 `overall_better=true` 才能发送“建议考虑复测/切换”的高优先级机会提醒；候选站永远不会自动切入生产。

### 9. `incidents` 与 `notification_deliveries`

`incidents` 是告警状态机的事实来源：

- `incident_key`：站点、分组、模型、指标和规则的稳定组合。
- `severity`：`P0`、`P1`、`P2`。
- `state`：`observed`、`confirmed`、`degraded`、`escalated`、`recovered`、`muted`。
- `first_seen_at`、`last_seen_at`、`last_notified_at`、`next_review_at`。
- `baseline_value`、`current_value`、`sample_count`、`evidence_refs`。
- `recommended_action`、`human_decision`、`closed_by`。

`notification_deliveries` 保存每次飞书发送结果、去重键、消息 hash 和恢复/升级关系。相同事件不重复发送；只在首次确认、恶化升级、恢复或产生新证据时更新。

### 10. `auth_sessions`

只记录上游用量页或机器接口的授权状态，不保存账号密码：

- `upstream_id`、`secret_ref`、`auth_mode`、`status`。
- `expires_at`、`last_refresh_at`、`last_success_at`、`last_failure_reason`。
- `scope`：只能是账单/用量读取，不得扩展为路由、余额或账号管理。

授权失效不叫“账单过期”，而是“上游用量读取会话失效”。质量监控和价格监控继续运行；只有账单核对暂停。

### 11. `agent_analyses` 与 `audit_events`

`agent_analyses` 保存结构化分析结果：

- `analysis_id`、`incident_id`、`model_provider`、`prompt_contract_version`。
- `summary`、`hypotheses`、`evidence_refs`、`confidence`、`recommended_action`。
- `requires_human_approval`、`delivery_status`、`created_at`。

`audit_events` 保存管理员新增/停用上游、修改全局探测预算、重新授权、确认候选站和关闭告警等行为。只记录操作者、对象、前后摘要和时间，不记录秘密内容。

## 采集与数据流

1. Sub2API 原生 Channel Monitor 按生产配置持续探测公开分组，原生 Ops/Usage 持续汇总真实用户请求、TTFT、总延迟、SLA、错误、资源和计费。
2. `relay-ops` 只读同步公开分组配置、Channel Monitor 聚合、Ops/Usage 窗口和渠道定价；保存引用与 hash，不复制原始生产监控历史。
3. 生产上游价格/倍率页每 5 分钟抓取；内容 hash 未变化时结束本轮，不解析、不告警。
4. 候选站严格每 6 小时执行一次周期：抓取价格/倍率页、发现模型，并使用 V2 共用组件执行受限同步/SSE。页面变化只进入本周期结果，不额外插入一次付费探测。
5. 新公开分组先运行完整 V2；通过后创建或关联 Sub2API 原生 Channel Monitor；原生监控产生有效样本后再公开。
6. 每次候选请求只记录 Token、延迟、结果和估算费用，不记录提示词、响应正文或认证头。
7. 比较器把 Sub2API 原生真实流量、原生合成监控和候选探测按来源分别对齐，生成必要的比较窗口、成本观察和候选标签。
8. 状态机确认事件后，先写 `incidents`，再调用 Feishu 和只读 Agent。
9. 每天发送一次价格、稳定性、延迟、成本估算、候选站和授权状态摘要。

## 预警 Agent 章节

### 角色定位

预警 Agent 是事件触发的只读分析器，不是 24 小时监控器，也不是生产控制器。

- 确定性采集器负责发现异常、窗口聚合、去重和阈值判断。
- Agent 负责解释结构化证据、归类可能原因、生成处置建议和日报段落。
- 飞书负责通知；管理员负责确认涉及价格、路由、凭据和用户影响的动作。
- Agent 不持有 Sub2API Admin API Key、用户 Key、上游 Key、Cookie、密码或数据库写权限。

### 触发事件

Agent 只接受以下事件：

- `P0`：用户链路持续失败、TLS/DNS/核心服务不可用、上游认证或余额完全失效、明确出现亏损风险。
- `P1`：倍率/模型价格变化、模型删除、SSE 完整率下降、429/5xx/超时持续升高、TTFT P95 相对基线恶化 30% 以上、公开分组配置漂移。
- `P2`：候选站分项变优、综合门槛通过、新模型出现、小幅价格或性能变化、日报分析。
- 授权事件：上游用量读取会话失效、刷新失败或账单接口 schema 变化。

同一事件的重复探测不会重复调用 Agent。只有首次确认、严重性升级、恢复或新证据超过最小变化阈值时才触发。

### 输入契约

Agent 输入必须是版本化 JSON，示例：

```json
{
  "contract_version": "relay-ops-incident-v1",
  "incident_id": "inc_01J...",
  "severity": "P1",
  "upstream": "Neko",
  "public_group": "GPT-Pro",
  "model": "gpt-5.6-sol",
  "window": {"minutes": 15, "samples": 126},
  "metric": {"name": "ttft_p95_ms", "baseline": 2400, "current": 3800},
  "related_rates": {"success": 0.987, "sse_completion": 0.992, "rate_429": 0.012},
  "evidence_refs": ["sub2api_ops_window:...", "sub2api_monitor_window:...", "probe_run:..."],
  "allowed_actions": ["observe", "recommend_recheck", "request_human_review"]
}
```

输入禁止包含：完整 URL 中的 Key、Authorization、Cookie、密码、JWT、提示词、响应正文、用户邮箱、用户 IP 原文和完整上游后台页面 HTML。需要诊断用户网络时只传 ASN/运营商等已聚合字段。

### 只读工具白名单

Agent 只能调用以下本地只读工具：

- 查询指定时间窗口的 `metric_refs`、`comparison_windows` 和对应 Sub2API 原生只读聚合。
- 查询指定上游/模型的 `probe_runs` 摘要。
- 查询价格快照差异和倍率观察。
- 查询公开分组当前配置摘要和历史配置 hash。
- 查询候选站比较结果和证据窗口。
- 查询相关 runbook 和最近同类事件。

禁止工具：

- 写入 Sub2API、修改分组/价格/路由/余额。
- 读取秘密存储、打印 Key、Cookie、密码或 Token。
- 发起任意外网请求。
- 删除观察、关闭告警、修改预算或签发邀请。

### 输出契约

Agent 必须返回可解析 JSON，之后再渲染成人类消息：

```json
{
  "summary": "GPT-Pro 首字延迟持续升高，但请求成功率仍正常",
  "what_was_done": ["同步 6 次", "SSE 6 次", "读取价格快照 1 次"],
  "result": ["同步成功率 100%", "SSE 完整率 100%", "TTFT P95 3.8s"],
  "change": "相对基线 2.4s 上升 58%，连续 3 个窗口",
  "focus": "继续观察；未达到切换条件",
  "hypotheses": [
    {"cause": "上游高峰排队", "confidence": 0.62, "evidence_refs": ["sub2api_ops_window:..."]}
  ],
  "recommended_action": "observe",
  "requires_human_approval": false,
  "confidence": 0.78
}
```

如果证据不足，Agent 必须返回“无法确定”，不能编造原因或把估算费用说成实际账单。

### 飞书消息规则

每条重要消息固定包含：

1. 做了什么：同步/SSE 数量、模型、窗口和样本。
2. 得到什么：成功率、SSE、TTFT P50/P95、错误率、成本估算或实际费用证据。
3. 发生什么变化：当前值、基线、持续时间和变化幅度。
4. 需要关注什么：无须操作、继续观察、重新登录、建议复测或需要人工决定。
5. 链接：内部事件详情、Sub2API `/monitor` 或上游重新登录地址。

正常探测不逐条发送。只在状态变化、升级、恢复、价格/倍率变化、候选综合变优、授权失效或预算阈值触发时通知；每天固定发一次汇总日报。

### 上游授权失效

授权失效消息必须使用明确语言：

```text
上游用量读取会话失效：wawazz
影响：质量和公开价格监控正常，真实消费核对暂停
需要操作：重新打开授权链接并登录一次
```

优先使用长期 Token 或 Refresh Token；如果上游只有网页登录态，服务器保留独立监控浏览器会话。系统在过期前刷新，遇到 401 只自动重试一次；连续 24 小时不能读取时发一条提醒，不循环刷屏。不得通过飞书传输密码，也不得把密码写入 Agent 或日志。

### LLM 调用与预算

- LLM 供应商通过服务器环境配置选择，必须是 OpenAI-compatible API 或等价受控适配器。
- 单次输入只包含一个事件的结构化摘要；日报可按多个已去重事件批量分析。
- 设置请求超时、最大输入 Token、最大输出 Token、每日费用上限和并发上限。
- LLM 不可用时，飞书发送确定性模板消息，不能阻塞监控和事件落库。
- 同一事件只允许一次主分析和一次人工请求的重新分析。
- 保存模型名、契约版本、成本估算、输出 hash 和分析结果，不保存秘密或原始页面。

### 人工确认边界

Agent 可以提出：继续观察、增加一次复测、重新授权、保留候选、人工核对定价、人工考虑切换。

以下动作必须由管理员明确确认并在后续实施计划中单独处理：

- 改用户价格或上游成本倍率。
- 启用、停用或切换生产上游。
- 修改公开分组和模型白名单。
- 充值、退款、调整用户余额。
- 修改凭据、预算或授权范围。
- 重启数据库、清理数据或执行不可逆操作。

第一版不提供 Agent 执行按钮；后台只展示建议和人工确认入口。

## 页面与权限

### `/pricing`

无需登录。以 Sub2API 渠道定价/可用渠道数据为事实来源，生成只读、脱敏的匿名投影；只显示客户可购买的正式模型和价格：输入、输出、缓存、端点能力、公开分组和更新时间。隐藏上游名称、采购倍率、余额、候选站和内部告警。首版不重写 Sub2API 定价管理逻辑，只补匿名公开读取和适合用户查看的页面。

### `/monitor`

直接使用 Sub2API 原生登录用户“渠道状态”页面，不再建设自定义 `/performance`。用户侧保持截图中的简单信息层级：状态、对话延迟、端点 PING、7/15/30 天可用率和近期状态时间线。P50/P90/P95/P99、SLA 口径、错误分类、真实/合成来源和样本细节只在管理员 Ops/`/ops` 中展示。

新分组只有完成 V2 准入、关联原生 Channel Monitor 且产生有效样本后才可见；停用或取消客户可见的分组从用户页移除，但历史证据保留。

### `/ops`

仅管理员可见。本站 QPS、SLA、TTFT、错误、资源、任务和生产 Channel Monitor 详情优先链接或嵌入 Sub2API 原生管理能力；`relay-ops` 只新增生产/候选价格与模型变化、候选探测、跨站比较、估算或真实成本证据、授权状态、事件时间线、候选标签、Agent 分析和配置审计。

## 安全与降级

- 采集器按上游隔离错误；单站失败不能阻塞全局调度。
- 账单会话失效只降低账单证据等级，不影响质量监控和用户服务。
- 页面解析失败保留上次快照，同时标记 `stale`，不能把旧值当新值。
- Agent 超时、输出不可解析或模型不可用时，回退确定性飞书模板。
- 监控服务故障由容器健康检查和外部 HTTPS 探针发现；Sub2API 不依赖 Agent 才能运行。
- 所有事件、通知、分析和人工确认写入审计链，保留证据引用而不是敏感原文。

## 分阶段验收

1. 使用 Fake Provider 验证价格快照 diff、候选探测预算、比较窗口、事件去重和恢复通知。
2. 运行现有 V2 测试与 fixture，验证新分组必须经过完整模型发现、逐模型同步/SSE、容量和价格证据门禁，`relay-ops` 不复制 evaluator。
3. 使用候选站假数据验证严格 6 小时调度、分项标签、综合门槛、候选失败隔离和每日摘要；页面变化不得额外触发付费探测。
4. 使用真实生产只读 API 验证所有公开分组、Channel Monitor、Ops/Usage 和渠道定价可被引用，且不会读取或修改 Sub2API 密钥、原始 PostgreSQL 或用户正文。
5. 验证生产分组只使用 Sub2API 加密保存的 Channel Monitor Key，`relay-ops` 不保留副本；候选站只使用专用低额度上游 API Key 的秘密引用。
6. 使用一条低额度候选 Key 验证同步/SSE 费用估算、飞书状态机和超预算暂停。
7. 验证账单授权失效只影响账单状态，重新授权后自动恢复，不需要重新录入上游。
8. 验证 Agent 收到脱敏 JSON、只能使用只读工具、输出不可解析时回退模板，任何写操作均被拒绝。
9. 验证 `/pricing` 无需登录、Sub2API 原生 `/monitor` 需要登录、`/ops` 只允许管理员；用户 `/monitor` 不出现高级运维术语。
10. 通过域名/TLS、服务器资源和日志泄露检查后，才进入生产只读部署。

## 与主线的关系

本设计不改变 Neko/wawazz 的生产选择，不执行价格或路由变更，也不阻塞正式域名审核。它是“正式入口 → 真实用户 → 每日服务质量优化”之间的服务器端运营基础设施。实现顺序应保持：先启用并验证 Sub2API 原生 Channel Monitor/`/monitor`，再实现 relay-ops 补充能力和公开 `/pricing`，然后做低额度候选验收，最后才开启首批真实用户入口。
