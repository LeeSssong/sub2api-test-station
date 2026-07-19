# relay-ops 上游监控与预警 Agent 设计

## 状态

设计已由用户确认（2026-07-19）。本文件定义服务器端 `relay-ops` 的核心数据对象和预警 Agent 边界；尚未实施、部署或修改生产配置。

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
- 提供公开模型定价页、登录用户性能页和管理员运维后台。
- 事件状态机、飞书通知、每日摘要和只读预警 Agent。

### 不包含

- 自动切换上游、自动改价格、自动充值或自动修改用户余额。
- 让 Agent 读取或操作 API Key、Cookie、密码、提示词、响应正文或数据库写权限。
- Fork 或重写 Sub2API 核心代码。
- 把真实账单核对做成用户上线或生产路由切换的硬阻塞。
- 支付、公开无限注册、复杂推荐系统和多节点高可用。

## 总体架构

`relay-ops` 作为独立服务器服务，与 Sub2API 并行运行：

```text
Caddy
|- /api/*           -> Sub2API
|- /pricing         -> relay-ops 公共模型定价页
|- /performance     -> relay-ops 登录用户性能页
`- /ops             -> relay-ops 管理后台

relay-ops
|- public-group-discovery
|- upstream-adapters
|- candidate-probes
|- pricing-and-cost-normalizer
|- quality-window-aggregator
|- incident-state-machine
|- feishu-notifier
|- read-only-alert-agent
`- web-and-admin-api
```

- 通过 Sub2API Admin API 读取公开分组、用户用量聚合和配置快照，不直接写 Sub2API PostgreSQL。
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
- `health_gate`：`pending`、`qualified`、`degraded`、`blocked`。
- `last_seen_at`、`source_revision`。

规则：只有 `enabled=true` 且 `customer_visible=true` 的分组进入公开页面和公开分组质量告警。新分组必须先有合成请求和有效样本，才能出现在公开性能页。

### 2. `upstreams`

代表已接入生产或由管理员录入的候选站点。

关键字段：

- `upstream_id`、`display_name`、`role`：`production`、`candidate`、`paused`、`rejected`。
- `base_url`：规范化后的 OpenAI-compatible Base URL。
- `site_url`、`pricing_url`、`usage_url`、`performance_url`：公开或管理员参考页面。
- `adapter_type`：`sub2api`、`newapi`、`openai_compatible` 或 `unknown`。
- `monitor_key_secret_ref`：秘密引用、指纹和末四位，不保存原文。
- `billing_auth_secret_ref`：可选；只用于有稳定机器接口或授权会话的站点。
- `advertised_multiplier`、`multiplier_source`、`multiplier_observed_at`。
- `billing_evidence_status`：`not_requested`、`estimated_only`、`verified`、`stale`。
- `monitor_status`、`last_success_at`、`last_error_code`。

最小候选录入字段是名称、Base URL、低额度监测 Key、模型定价/倍率页 URL、用量/账单页 URL。站点类型、模型、端点、可比较公开分组和探测计划自动发现。

### 3. `secret_refs`

保存凭据元数据而非凭据内容本身。

- `secret_ref`、`kind`、`owner_scope`、`fingerprint`、`last_four`。
- `created_at`、`expires_at`、`last_used_at`、`status`。
- 凭据内容只存在服务器秘密存储或只读挂载文件中。

监测 Key 与账单会话必须分开。监测 Key 只能用于低消耗 API 探测；账单会话如果不存在，不影响质量监控。

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

`probe_profiles` 保存全局探测策略，所有生产上游和候选站默认继承：

- 页面/倍率采集：每 5 分钟（候选站页面每 15 分钟）。
- 生产质量与健康：每分钟轻量检查。
- 候选同步 + SSE：每 6 小时，默认每日最多 8 个模型请求。
- 无真实用户流量时，生产池每小时执行一次低消耗探测。
- 单次最大输出 Token、单站每日费用上限和全部候选站总预算上限。

`probe_runs` 记录一次可重放的探测批次：

- `run_id`、`upstream_id`、`group_id`、`model_id`、`probe_kind`。
- `started_at`、`finished_at`、`status`、`http_status`、`error_class`。
- `input_tokens`、`output_tokens`、`cache_tokens`。
- `ttft_ms`、`total_latency_ms`、`tps`、`sse_done`。
- `estimated_standard_cost`、`estimated_upstream_cost`、`actual_upstream_cost`（可空）。
- `evidence_level`、`request_fingerprint`、`redaction_version`。

探测结果全部入库，但不逐条发送飞书。飞书只消费经过状态机确认的事件。

### 6. `quality_windows`

把真实用户请求和合成请求按来源分开聚合，按上游、公开分组、模型和时间窗口保存：

- `window_start`、`window_end`、`sample_count`。
- `success_rate`、`sse_completion_rate`、`rate_429`、`rate_5xx`、`timeout_rate`。
- `ttft_p50_ms`、`ttft_p95_ms`、`latency_p50_ms`、`latency_p95_ms`、`tps_p50`。
- `concurrency_peak`、`queue_time_p95_ms`、`cache_hit_rate`。
- `source_mix`：`real_traffic`、`synthetic` 或两者分别的统计。

用户错误，例如错误 Key、错误模型和参数错误，不计入平台成功率，但保留独立计数。

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

1. 每分钟执行健康检查和轻量用户链路探测，记录真实流量与合成流量的独立结果。
2. 每 5 分钟抓取生产上游的倍率、模型目录和公开定价；候选站每 15 分钟抓取。
3. 每 6 小时对候选站执行低成本同步/SSE；发现页面或模型变化时立即追加一次受限复核。
4. 每次请求只记录 Token、延迟、结果和估算费用，不记录提示词、响应正文或认证头。
5. 聚合器生成质量窗口、成本观察和候选比较。
6. 状态机确认事件后，先写 `incidents`，再调用 Feishu 和只读 Agent。
7. 每天发送一次价格、稳定性、延迟、成本估算、候选站和授权状态摘要。

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
  "evidence_refs": ["quality_window:...", "probe_run:..."],
  "allowed_actions": ["observe", "recommend_recheck", "request_human_review"]
}
```

输入禁止包含：完整 URL 中的 Key、Authorization、Cookie、密码、JWT、提示词、响应正文、用户邮箱、用户 IP 原文和完整上游后台页面 HTML。需要诊断用户网络时只传 ASN/运营商等已聚合字段。

### 只读工具白名单

Agent 只能调用以下本地只读工具：

- 查询指定时间窗口的 `quality_windows`。
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
    {"cause": "上游高峰排队", "confidence": 0.62, "evidence_refs": ["quality_window:..."]}
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
5. 链接：内部事件详情、公开性能页或上游重新登录地址。

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

无需登录。只显示客户可购买的正式模型和价格：输入、输出、缓存、端点能力、公开分组和更新时间。隐藏上游名称、采购倍率、余额、候选站和内部告警。

### `/performance`

要求用户登录。按公开分组和模型显示 24 小时/7 天可用率、TTFT P50/P95、总延迟 P50/P95、TPS、SSE 完整率和样本量。真实用户与合成探测可以在后台分开，页面只显示经过定义的聚合结果。

### `/ops`

仅管理员可见。显示生产和候选上游、页面/模型变化、估算或真实成本证据、授权状态、事件时间线、候选分项标签、Agent 分析和配置审计。

## 安全与降级

- 采集器按上游隔离错误；单站失败不能阻塞全局调度。
- 账单会话失效只降低账单证据等级，不影响质量监控和用户服务。
- 页面解析失败保留上次快照，同时标记 `stale`，不能把旧值当新值。
- Agent 超时、输出不可解析或模型不可用时，回退确定性飞书模板。
- 监控服务故障由容器健康检查和外部 HTTPS 探针发现；Sub2API 不依赖 Agent 才能运行。
- 所有事件、通知、分析和人工确认写入审计链，保留证据引用而不是敏感原文。

## 分阶段验收

1. 使用 Fake Provider 验证公开分组自动发现、价格快照 diff、探测预算、质量窗口、事件去重和恢复通知。
2. 使用候选站假数据验证分项标签、综合门槛、候选失败隔离和每日摘要。
3. 使用真实生产只读 API 验证所有公开分组被发现，且不会读取或修改 Sub2API 密钥和用户正文。
4. 使用一条低额度候选 Key 验证同步/SSE 费用估算、飞书状态机和超预算暂停。
5. 验证账单授权失效只影响账单状态，重新授权后自动恢复，不需要重新录入上游。
6. 验证 Agent 收到脱敏 JSON、只能使用只读工具、输出不可解析时回退模板，任何写操作均被拒绝。
7. 验证 `/pricing` 无需登录、`/performance` 需要登录、`/ops` 只允许管理员。
8. 通过域名/TLS、服务器资源和日志泄露检查后，才进入生产只读部署。

## 与主线的关系

本设计不改变 Neko/wawazz 的生产选择，不执行价格或路由变更，也不阻塞正式域名审核。它是“正式入口 → 真实用户 → 每日服务质量优化”之间的服务器端运营基础设施。实现顺序应保持：先完成 relay-ops 只读监控和页面，再做低额度验收，最后才开启首批真实用户入口。
