# Verification: S3 自适应选择、粘性逃逸与调度体验观测

## Scope

- 任务包：S3
- 初始基线：`main@83c4554792d3424751a439f3fd1cc38a0542ed5e`
- 已验证运行时 tip：`0f4506bca758377be204b4acea446d72edd74221`
- 范围：S1/S2 硬门槛后的动态 Top-K、sticky 保留/逃逸解释、TTFT report-only 资格、selection/outcome 运行时事件、Ops 派生指标/API 和现有 Ops Dashboard 卡片。

## Commands / Results

- `go test ./internal/config -run 'TestGatewayOpenAISharedHealthDefaultsAndHardLimits|TestValidateConfig_AutoScaleDisabledIgnoreAutoScaleFields|TestValidateConfigErrors' -count=1`: PASS.
- `go test ./internal/service -run 'TestApplyOpenAIAdaptiveTopK|TestOpenAIAccountSchedulerAdaptive|TestOpenAITTFTReportEligible|TestAggregateOpenAISchedulerExperience|TestOpsServiceGetOpenAISchedulerExperience|TestRecordOpenAISchedulerSelectionAndOutcome' -count=1`: PASS.
- `go test ./internal/handler -run 'TestOpenAIResponses_APIKeyPassthroughPool5xxRetriesThenExhaustsMaxSwitches|TestOpenAIMessages_TransientFailureRetriesOnceThenFailsOver|TestOpenAIResponses_PostOutputFailureNeverReplays' -count=1`: PASS.
- `go test ./internal/handler/admin -run 'TestOpsSchedulerExperienceHandler' -count=1`: PASS.
- `go test ./internal/server/routes -run 'TestOpenAISchedulerExperienceRouteRequiresAdminAuthentication' -count=1`: PASS.
- `go test ./internal/server -run '^$' -count=1`: PASS, compile-only.
- `go build ./cmd/server`: PASS.
- `pnpm exec vitest run src/views/admin/ops/components/__tests__/OpsOpenAISchedulerExperienceCard.spec.ts src/views/admin/ops/__tests__/OpsDashboard.schedulerExperience.spec.ts src/views/admin/__tests__/DashboardView.spec.ts`: PASS, 3 files / 9 tests.
- `pnpm typecheck`: PASS.
- `pnpm build`: PASS, 1046 modules transformed and production assets generated.
- Affected Go files `gofmt -w`: completed; no resulting worktree diff.
- `git diff --check 83c455479..HEAD`: PASS after removing five Markdown hard-break trailing spaces from the new S3 specification.
- Migration and `.github/workflows` diff count: 0.

The frontend commands retain existing repository warnings about the legacy `pnpm.overrides` location, Node localStorage test environment, stale Browserslist data, Vite dynamic/static imports and chunk size. They did not fail tests, typecheck or build and are not introduced by S3.

## Verified Behavior

- Defaults are exactly `adaptive_top_k_enabled=true`, `adaptive_top_k_max=7`, `adaptive_top_k_score_gap=0.15`, `ttft_report_only_enabled=true`; invalid max outside `1..32` and score gap outside `0..10` are rejected.
- Dynamic Top-K only narrows candidates that already passed native/S1/S2 eligibility; excluded or shared-health-blocked accounts cannot re-enter.
- Weighted sticky below the quality floor escapes with an explanation; existing sticky/TTFT/error-rate behavior remains bounded by S1/S2 veto.
- TTFT report-only is a pure eligibility signal and does not issue a second upstream request.
- Selection and one terminal request outcome are recorded with logical-request/attempt context in the existing bounded in-memory resilience ledger.
- Ops aggregation returns recovery, average/P95 attempts, repeated bad-account selection excluding half-open, budget exhaustion, sticky, Top-K and TTFT eligibility metrics; denominator below 5 is `insufficient_data`, zero data is `no_data`.
- Admin route `GET /api/v1/admin/ops/openai-scheduler-experience` retains admin/monitoring gates and time/platform/group filters.
- The Ops card preserves preset or exact custom time filters, remains visible independently of the OpenAI token-stats display switch, confines errors to the card, renders runtime window/latest-event/sample evidence, and uses a 390px-safe one-column-first grid.

## Changes / Boundaries

- Database migrations or historical backfill: none.
- Pricing, multiplier, billing, usage idempotency or external control plane changes: none.
- Production account/data mutation: none.
- GitHub Actions changes: none.
- The event ledger is a bounded, reconstructible process-local projection. After API restart the card can legitimately show `no_data` until new natural traffic produces events; it is not persistent historical truth.
- `downtime_required`: expected `false`, pending the root release precheck on the merged and pushed `main`.
- Runtime rollback switches: set `gateway.openai_scheduler.adaptive_top_k_enabled=false` and `gateway.openai_scheduler.ttft_report_only_enabled=false` to return to S1/S2 filtering plus existing fixed Top-K/sticky behavior. Authoritative binary rollback is the previous immutable blue-green image.

## Not Verified

- No full-repository, pressure, soak, mutation, race or unrelated browser matrix was run.
- No synthetic production failure, account mutation, forced sticky escape, budget exhaustion or second-upstream request was induced.
- Production natural-traffic metrics, API response and card rendering remain pending root merge, push, precheck and blue-green release.

## Remaining Risks

- Runtime metrics reset on process restart by design; a fresh deployment initially showing `no_data` is expected.
- Low natural traffic may keep individual rate metrics in `insufficient_data`; production acceptance must not manufacture failures merely to increase denominators.
- Existing frontend build warnings remain outside S3 scope.
