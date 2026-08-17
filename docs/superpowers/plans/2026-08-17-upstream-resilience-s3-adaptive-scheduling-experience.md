# S3 Adaptive Scheduling Experience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变 S1/S2 健康 veto、重试预算和账务语义的前提下，交付动态 Top-K、可解释 sticky 逃逸、TTFT report-only 与原生 Ops 调度体验卡片。

**Architecture:** 在已通过 S1/S2 硬门槛的候选中应用质量门槛，把选择 decision 和最终 outcome 写入现有有界 resilience ledger，由 Ops service 按时间/平台/分组派生指标。前端只在现有 Ops Dashboard 增加一张卡片。

**Tech Stack:** Go, Gin, Vue 3, TypeScript, Vitest, 现有 Sub2API config/service/handler/Ops 模式。

## Global Constraints

- S1/S2 veto 和 S2 `4/3/2/5000ms` 预算永远优先于 S3 分数和 sticky。
- TTFT report-only 不发起第二个上游请求。
- 无数据库迁移、回填、账务/价格/倍率变更、生产账号修改或 GitHub Actions。
- 只运行 S3 直接相关测试、必要编译/typecheck/build 和发布门禁。

---

### Task 1: 配置契约

**Files:**
- Modify: `upstream/sub2api/backend/internal/config/config.go`
- Modify: `upstream/sub2api/backend/internal/config/config_test.go`

**Produces:** `AdaptiveTopKEnabled bool`, `AdaptiveTopKMax int`, `AdaptiveTopKScoreGap float64`, `TTFTReportOnlyEnabled bool` on `GatewayOpenAISchedulerConfig`.

- [ ] Write failing tests asserting defaults `true/7/0.15/true` and rejecting max outside `1..32` or gap outside `0..10`.
- [ ] Run RED: `cd upstream/sub2api/backend && go test ./internal/config -run 'OpenAISchedulerAdaptive|openai_scheduler' -count=1`.
- [ ] Add mapstructure fields, explicit-false-safe defaults, and exact-key validation.
- [ ] Run `gofmt`, rerun GREEN, then commit `feat: configure adaptive OpenAI scheduling`.

### Task 2: 动态 Top-K 与 decision 字段

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_shared_health_test.go`

**Produces:**

```go
func applyOpenAIAdaptiveTopK(candidates []openAIAccountCandidateScore, configuredTopK, maxTopK int, scoreGap float64) (selected []openAIAccountCandidateScore, threshold float64, fallback bool)
```

Extend `OpenAIAccountScheduleDecision` with `EligibleCount`, `EffectiveTopK`, `MinimumScoreThreshold`, `SelectionLayer`, `StickyKept`, `StickyEscapeReason`, `TTFTReportEligible`.

- [ ] Write RED table tests: score threshold, candidate shortage, max cap, disabled compatibility, NaN/Inf fail-safe.
- [ ] Run RED: `go test ./internal/service -run 'TestApplyOpenAIAdaptiveTopK|TestOpenAIAccountSchedulerAdaptive' -count=1`.
- [ ] Implement the pure helper after current native/S1/S2 filtering; never re-add rejected accounts.
- [ ] Write RED proving a highest-score S1/S2-blocked account cannot re-enter and half-open remains a separate layer.
- [ ] Wire plan fields through `selectByLoadBalance` and `Select`; keep legacy `TopK` as a compatibility alias of `EffectiveTopK`.
- [ ] Run GREEN for adaptive/shared-health scheduler tests, `gofmt`, and commit `feat: adapt OpenAI scheduler top k`.

### Task 3: Sticky 原因和 TTFT report-only

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler_test.go`

**Changes:** `selectBySessionHash` returns an escape reason; add a pure predicate for TTFT report eligibility.

- [ ] Write RED tests for `ttft`, `error_rate`, `excluded`, `shared_cooldown`, `capability`, `quality_floor`, and healthy `StickyKept=true`.
- [ ] Implement reason propagation without deleting a binding solely because of quality/TTFT/error-rate escape.
- [ ] Write RED predicate tests: safe pre-output + two candidates + budget = true; output started, side effects, one candidate, or exhausted budget = false.
- [ ] Add a handler test asserting exactly one upstream forward when report eligibility is true.
- [ ] Add decision fields to existing structured debug logs; do not add goroutines or a second forward.
- [ ] Run focused service/handler GREEN, `gofmt`, and commit `feat: explain OpenAI sticky scheduling decisions`.

### Task 4: Selection/outcome 事件

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_resilience_observability_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- Create: `upstream/sub2api/backend/internal/handler/openai_gateway_scheduler_observability_test.go`

**Produces:** events `openai.scheduler_selection` and `openai.scheduler_request_outcome`, plus helpers `RecordOpenAISchedulerSelection` and `RecordOpenAISchedulerRequestOutcome`.

- [ ] Write RED service tests for context enrichment, bounded ledger, exact platform/group/correlation filtering, and terminal-outcome dedupe.
- [ ] Extend `OpenAIResilienceEvent` with decision fields, `RetryBudgetExhausted`, and `FinalOutcome`; keep sensitive request material absent.
- [ ] Write RED handler tests for first-attempt success, retry recovery, terminal selection failure, and budget exhaustion in Responses and Messages loops.
- [ ] Emit selection only after `attemptSequence.next`; emit exactly one terminal outcome at existing return boundaries.
- [ ] Run focused service/handler GREEN, `gofmt`, and commit `feat: observe OpenAI scheduling outcomes`.

### Task 5: Ops 派生指标与管理 API

**Files:**
- Create: `upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience.go`
- Create: `upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience_test.go`
- Modify: `upstream/sub2api/backend/internal/service/ops_service.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/ops_dashboard_handler.go`
- Create: `upstream/sub2api/backend/internal/handler/admin/ops_scheduler_experience_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`

**Produces:**

```go
type OpsSchedulerRateMetric struct { Numerator int64; Denominator int64; Value *float64; Status string }
type OpsSchedulerAttemptsMetric struct { SampleSize int64; Value *float64; P95 *int; Status string }
func (s *OpsService) GetOpenAISchedulerExperience(ctx context.Context, filter *OpsDashboardFilter) (*OpsOpenAISchedulerExperienceResponse, error)
```

Route: `GET /api/v1/admin/ops/openai-scheduler-experience`.

- [ ] Write RED golden aggregation tests for recovery, attempts/P95, repeated bad-account selection excluding half-open, budget exhaustion, sticky, Top-K, TTFT eligibility, latest event, no-data and denominator `<5`.
- [ ] Implement pure correlation-based aggregation using the current runtime ledger; `Value=nil` for no-data/insufficient-data while retaining numerator/denominator.
- [ ] Write RED handler tests for valid filters, invalid time/group, Ops disabled and service unavailable.
- [ ] Reuse `parseOpsTimeRange`, admin auth and `RequireMonitoringEnabled`; no repository or migration.
- [ ] Run focused service/admin-handler GREEN, `gofmt`, and commit `feat: expose OpenAI scheduler experience metrics`.

### Task 6: Ops Dashboard 卡片

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/ops.ts`
- Create: `upstream/sub2api/frontend/src/views/admin/ops/components/OpsOpenAISchedulerExperienceCard.vue`
- Create: `upstream/sub2api/frontend/src/views/admin/ops/components/__tests__/OpsOpenAISchedulerExperienceCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/ops/OpsDashboard.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/DashboardView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/ops.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/ops.ts`

**Produces:** TypeScript response mirrors and `opsAPI.getOpenAISchedulerExperience(...)`.

- [ ] Write RED component tests for normal, insufficient-data, no-data, error/retry, numerator/denominator, P95, latest event, and sibling dashboard isolation.
- [ ] Add API types/client method and a card following `OpsOpenAITokenStatsCard` cancellation/error conventions.
- [ ] Render responsive `grid-cols-1 sm:grid-cols-2 xl:grid-cols-4`, use `break-words`, and mount after OpenAI token stats.
- [ ] Add Chinese/English copy and a 390px no-horizontal-overflow assertion.
- [ ] Run `npm run test:unit -- ...OpsOpenAISchedulerExperienceCard.spec.ts ...DashboardView.spec.ts`, `npm run typecheck`, then commit `feat: show OpenAI scheduler experience in ops`.

### Task 7: 候选验证与交接

**Files:**
- Create: `docs/superpowers/reports/2026-08-17-s3-adaptive-scheduling-experience-verification.md`
- Create: `docs/handoffs/2026-08-17-s3-adaptive-scheduling-experience-handoff.md`

- [ ] Run backend focused tests for config, scheduler/shared-health, resilience aggregation, both OpenAI handler loops, Ops service/handler, `go test ./internal/server -run '^$'`, and `go build ./cmd/server`.
- [ ] Run the two focused frontend specs, typecheck and build.
- [ ] Run `gofmt` on touched Go files, `git diff --check main...HEAD`, and verify no migration or `.github/workflows` changes.
- [ ] Record exact candidate SHA, commands/results, changed files, unverified items, four config defaults, no migration, `downtime_required=expected false pending root precheck`, runtime-ledger restart semantics, rollback switches, and remaining risks.
- [ ] Commit handoff/report as `docs: hand off S3 adaptive scheduling candidate` and stop at `READY_FOR_ROOT_REVIEW`; do not merge, push, precheck, deploy, or edit global ledgers from this worktree.
