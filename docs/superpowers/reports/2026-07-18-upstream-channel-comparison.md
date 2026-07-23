| Channel | Lifecycle | Latest status | Evidence | Billing | Models | Stream | Max verified concurrency |
|---|---|---|---|---|---|---|---:|
| Aliu | paused | partial | historical_report | actual_multiplier=0.04, reconciliation=partial, api_usage_vs_gateway_debit=matched, upstream_dashboard=matched_at_display_precision | available_count=20, verified_models=["gpt-5.4-mini"], target_models_present=unknown | complete=true, verified_models=["gpt-5.4-mini"] | 1 |
| NekoAPI Pro | production | partial | historical_report,live_billing,live_direct,live_gateway,manual_terms | actual_multiplier=0.07, reconciliation=display_precision_delta_not_same_request, api_usage_vs_upstream_billing_tokens=different, gateway_debit=unknown, status=partial, gateway_display_debit_usd=0.0001, gateway_usage_tokens={"sync_total_tokens"=>16, "stream_usage_reported"=>false}, provider_key_display_delta_usd=0.0006, api_usage_vs_gateway_display=partial | available_count=8, verified_models=["gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4-mini"], target_models_present=true, http_status=200 | complete=true, first_event_ms=2060, total_ms=6270, request_count=3, success_count=3, success_rate=1.0, latency={"count"=>3, "p50_ms"=>2766.0069999983534, "p95_ms"=>2965.0170000095386} | 3 |
# 上游渠道统一基准对比

**日期：** 2026-07-18  
**范围：** D03 上游与定价验证  
**当前结论：** Neko 已按用户明确批准切为生产；Aliu 调度关闭，保留为暂停候选。2026-07-19 容量短测的独立证据见 `2026-07-19-neko-capacity-verification.md`。

## 当前角色

 - **NekoAPI Pro：生产。** 生产账号 `neko-production-primary`，Neko 账号倍率 `0.07`，账号并发 `3`；GPT 用户组售价仍为 `0.15x`。
 - **Aliu：暂停候选。** 账号保留用于回滚和后续复测，但当前不承接用户流量。

## 切换后冒烟

 切换后通过公开 Sub2API 网关完成一次同步和一次 SSE：同步 HTTP 200，`gpt-5.6-sol` 返回 usage `11/5/16` Token；SSE HTTP 200，收到内容事件和一个 `[DONE]`，总耗时约 2.94 秒。临时低余额用户和下游 Key 已清理。

 这次生产冒烟的网关页面显示扣费约 `$0.0001`；Neko 密钥页面在相邻设置和请求期间显示余额变化约 `$0.0006`，无法按同一请求精确拆分，因此台账标为部分计费核对。此前 `live_billing` 的同请求三方精确对账仍是成本资格的权威证据：225 Token，Sub2API `$0.005600`，Neko 实际 `$0.000393`，显示倍率约 `0.06895x`。

## 风险与复核触发

 Neko 的 24 至 72 小时稳定性、首尔生产机到大陆用户的真实链路、长流错误率和商业转售授权仍未知；供应方可调整价格、模型、额度和规则。出现新的同步/SSE/5xx/429 失败、余额跌破运营底线、供应商目录或倍率变化、商业条款确认，或流量超过已验证并发时，重新复核 Neko 与 Aliu。

## 容量短测补充（2026-07-19）

同步并发 1–50 路全部 HTTP 200；SSE 并发 3/5/10 路全部收到 `[DONE]`；60/120/180 RPM 一分钟窗口全部成功；240 RPM 出现 1 次超时。该结果是观测下界，不是供应商硬上限或 SLA。当前生产账号并发 `3` 未修改。
