# 账号探测口径与管理员流水成本详情 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变现有视觉样式的前提下，让账号卡片完全使用主动探测评估服务质量，并让管理员流水详情同时展示上游原生流水、本站计费和有证据等级的成本/毛利。

**Architecture:** 账号监控后端只从 `account_monitor_results` 计算探测样本、探测成功率、探测 TTFT/总耗时和当前状态；真实业务请求不再参与账号卡片质量评分。管理员流水详情继续复用 Sub2API 原生 usage log，同时通过 relay-ops 成本对账查询按本站请求 ID / 上游请求 ID关联 Sub2API 或 New API 原生账单流水；没有精确成本证据时显示待对账，不伪造毛利。

**Tech Stack:** Go、PostgreSQL、Vue 3、TypeScript、Vitest、现有 relay-ops billing adapters、现有 Tailwind 组件。

## Global Constraints

- 保持现有账号卡片和流水详情的布局、颜色、组件风格不变，只调整字段、中文文案和数据口径。
- 账号卡片质量字段只能来自主动探测；真实业务请求只能在流水/经营页面展示，不能覆盖探测状态。
- Sub2API 使用 `/v1/usage/records` 的 `actual_cost`；New API 使用 `/api/log/token` 的 `quota` 与 `/api/status` 的 `quota_per_unit` 换算实际扣费。
- 上游请求 ID 必须与本站请求 ID分开保存和展示；不能覆盖 `usage_logs.request_id`。
- 成本证据分为已确认、估算、待对账；只有已确认成本才能显示确认毛利。
- 所有新增接口、DTO、计算逻辑必须有回归测试；完成前必须通过 typecheck、build、Go tests、前端测试和生产视觉/API 验证。

---

### Task 1: 账号监控纯探测投影与状态评分

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/__tests__/AccountMonitorCard.spec.ts`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Produce account monitor fields `probe_success_rate`, `probe_sample_count`, `probe_success_count`, `probe_ttft_p50_ms`, `probe_latency_p95_ms`, `availability_status`, `score_status`.
- Preserve existing response compatibility by retaining legacy fields only as aliases with probe-derived values.

- [ ] **Step 1: Write failing backend tests** for 0/24 failed probes, 24/24 successful probes, one failed probe, three consecutive failures, stale probes, and no samples.
- [ ] **Step 2: Run the focused Go tests** and confirm the old real-request fallback fails the new expectations.
- [ ] **Step 3: Implement probe-only aggregation** from `account_monitor_results`; calculate success rate as successful probes / all probes, TTFT P50 and latency P95 from successful probes only.
- [ ] **Step 4: Implement availability gates**: disabled, unavailable after three consecutive failures or fatal auth/quota errors, abnormal after one fresh failure, normal only after a fresh successful probe.
- [ ] **Step 5: Implement score eligibility**: unavailable/disabled/stale/no-sample accounts show no score and do not rank; abnormal accounts are capped; normal accounts use probe success, TTFT, and latency weights.
- [ ] **Step 6: Update card labels and details** to say “探测成功率”“探测样本”“首 Token 延迟 P50”“完整响应耗时 P95”; remove real-request wording from score evidence.
- [ ] **Step 7: Add frontend regressions** proving 0/24 probes cannot render “正常”、100 分或组内排名, while 33/33 real requests are irrelevant to the card.
- [ ] **Step 8: Run focused backend/frontend tests and commit** `feat: make account monitor probe-only`.

### Task 2: Native upstream cost detail contract

**Files:**
- Modify: `relay-ops-service/internal/billing/sub2api.go`
- Modify: `relay-ops-service/internal/billing/newapi.go`
- Modify: `relay-ops-service/internal/billing/adapter.go`
- Modify: `relay-ops-service/internal/reconciliation/*`
- Modify: `relay-ops-service/internal/http/reconciliation.go`
- Test: `relay-ops-service/internal/billing/adapter_test.go`
- Test: `relay-ops-service/internal/reconciliation/*_test.go`

**Interfaces:**
- Produce an admin-only request cost detail containing `local_request_id`, `upstream_request_id`, `source_id`, `adapter_type`, `model`, `prompt_tokens`, `completion_tokens`, `upstream_actual_cost`, `upstream_standard_cost`, `cost_source`, `confidence`, `matched_at`, and `status`.
- Sub2API direct charge source is `actual_cost`; New API direct charge source is `quota / quota_per_unit`; New API type 6 is a refund.

- [ ] **Step 1: Add failing adapter tests** for Sub2API direct actual cost, New API quota conversion, refund rows, missing upstream request ID, and invalid unit price.
- [ ] **Step 2: Add failing reconciliation query tests** for exact local ID matching, exact upstream ID matching, and ambiguous token/model/time fallback being marked pending rather than confirmed.
- [ ] **Step 3: Implement the admin request-cost read endpoint** that returns only the matched native transaction and evidence metadata; do not expose credentials or raw upstream responses.
- [ ] **Step 4: Implement cost source labels**: `上游逐笔账单`, `上游价格表推算`, `自购账号成本分摊`, `待对账`.
- [ ] **Step 5: Run relay-ops billing/reconciliation tests and commit** `feat: expose native upstream request cost details`.

### Task 3: Persist and expose separate upstream request ID in Sub2API usage details

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`
- Modify: `upstream/sub2api/backend/internal/service/usage_service.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_insert.go`
- Modify: `upstream/sub2api/backend/internal/repository/usage_log_repo_query.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/types.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/mappers.go`
- Modify: `upstream/sub2api/backend/internal/store/migrations/*`
- Test: corresponding usage log repository/service/DTO tests

**Interfaces:**
- Add nullable `upstream_request_id` to usage log persistence and administrator-only DTO.
- Keep `request_id` as the local request ID; never replace it with an upstream value.

- [ ] **Step 1: Write failing repository and mapper tests** proving local and upstream IDs are returned separately.
- [ ] **Step 2: Add the nullable migration and model field.**
- [ ] **Step 3: Pass `ForwardResult.RequestID` as upstream ID while resolving the local request ID independently.**
- [ ] **Step 4: Add the admin DTO field only; ordinary user DTO must not include it.**
- [ ] **Step 5: Run Go repository/service tests and commit** `feat: persist upstream request ids separately`.

### Task 4: Redesign administrator usage detail contents without changing layout

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`
- Modify: `upstream/sub2api/frontend/src/components/usage/usageDetail.ts`
- Modify: `upstream/sub2api/frontend/src/types/*`
- Modify: `upstream/sub2api/frontend/src/api/admin/usage.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`
- Test: `upstream/sub2api/frontend/src/components/usage/__tests__/*`

**Interfaces:**
- Preserve the existing dialog sections and layout.
- Add admin-only rows: 本站请求 ID、上游请求 ID、本站标准费用、本站实际扣费、上游实际扣费、成本依据、本站分组倍率、上游倍率、本次计入成本、毛利状态。

- [ ] **Step 1: Add failing component tests** for confirmed cost, estimated cost, pending cost, and missing upstream request ID.
- [ ] **Step 2: Add the top summary values**: 本站扣费、计入成本、毛利/待确认.
- [ ] **Step 3: Add request identity rows** while keeping user scope free of account, upstream, cost, and profit fields.
- [ ] **Step 4: Add the native upstream billing rows** and evidence badge; do not recalculate an upstream amount from the site price table when native actual cost exists.
- [ ] **Step 5: Add formulas**: confirmed gross margin = site actual cost - upstream actual cost; estimated gross margin is explicitly labeled 预计; missing cost stays 待对账.
- [ ] **Step 6: Run focused Vitest, typecheck, build, and commit** `feat: clarify administrator usage cost details`.

### Task 5: Whole-branch review and verification

**Files:**
- Review all files changed by Tasks 1–4.
- Update: `docs/project/project-progress.md`

- [ ] **Step 1: Run backend tests, frontend tests, typecheck, build, and `git diff --check`.**
- [ ] **Step 2: Run an independent review for probe-only semantics, user/admin field separation, cost evidence confidence, and request-ID persistence.**
- [ ] **Step 3: Run production-like API fixtures for Sub2API and New API native billing responses.**
- [ ] **Step 4: Perform visual checks at desktop and mobile sizes without changing the existing style.**
- [ ] **Step 5: Update the project progress ledger only with the actual deployment/verification state.**
