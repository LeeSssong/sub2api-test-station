# Unified Quality Priority Weight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add aggressive cold-start priority weighting and persistent daily priority weighting to the OpenAI unified-quality scheduler without changing hard eligibility gates or the traditional scheduler.

**Architecture:** Keep the existing unified-quality comparator and add two bounded priority signals to API-key candidates. Resolve caps from existing global scheduler settings and group policy JSON, with group values overriding global defaults. Preserve the legacy weight override contract and emit additive decision fields.

**Tech Stack:** Go, Viper, PostgreSQL-backed settings JSON, existing scheduler service, testify.

**Spec:** `docs/superpowers/specs/2026-09-12-unified-quality-priority-weight-design.md`

## Global Constraints

- Cold-start priority maximum is `50` points.
- Daily priority maximum is `20` points.
- Priority is soft and never bypasses eligibility or concurrency gates.
- `account_groups.priority` overrides global account priority for group requests.
- Traditional scheduler behavior remains unchanged.
- No database migration, production configuration change, or deployment during implementation.
- Implement in a clean task worktree derived from current `main`; preserve unrelated `tmp/`.

## File Map

- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`: signal helpers, candidate scoring, comparator, decision observability.
- `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`: global/group cap resolution; legacy scoring remains unchanged.
- `upstream/sub2api/backend/internal/service/settings_view.go`: serialized group-policy cap fields.
- `upstream/sub2api/backend/internal/service/setting_parse.go`: parse and normalize cap fields.
- `upstream/sub2api/backend/internal/config/config.go`: global defaults and validation.
- `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go`: typed admin settings update fields if required by the existing update path.
- `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`: ranking and signal tests.
- `upstream/sub2api/backend/internal/config/config_test.go` and existing scheduler settings tests: validation and round-trip tests.

### Task 1: Add bounded priority signal primitives

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- Test: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`

**Interfaces:**
- Add `openAIUnifiedQualityPriorityCaps{ColdStartMax float64, DailyMax float64}`.
- Add `openAIUnifiedQualityDailyPrioritySignal(priority int, max float64) float64`.
- Add a cap-aware cold-start helper while keeping existing call sites compatible.
- Store `coldStartPrioritySignal` and `dailyPrioritySignal` on `openAIUnifiedQualityCandidate`.

- [ ] Write table-driven tests for priority `1`=`+50` cold-start, priority `1`=`+20` daily, priority `50` neutral, priority `100` negative, maturity zeroing cold-start, and invalid caps returning zero.
- [ ] Run `cd upstream/sub2api/backend && go test ./internal/service -run 'TestOpenAIUnifiedQuality.*Priority' -count=1`; verify failure before implementation.
- [ ] Implement finite, clamped helpers using `clampInt` and `openAIUnifiedQualityMaturityConfidence`; use `((50-priority)/49)*max` and existing confidence strength.
- [ ] Re-run the focused tests and verify pass.
- [ ] Commit with `git commit -m "feat: add unified quality priority signals"`.

### Task 2: Add global and group cap configuration

**Files:**
- Modify: `upstream/sub2api/backend/internal/config/config.go`
- Modify: `upstream/sub2api/backend/internal/service/settings_view.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_parse.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go` only where needed by the typed settings contract.
- Test: `upstream/sub2api/backend/internal/config/config_test.go` and existing scheduler settings tests.

**Interfaces:**
- Global fields: `GatewayOpenAISchedulerConfig.UnifiedQualityPriorityColdStartMax` and `.UnifiedQualityPriorityDailyMax`.
- Group fields: `OpenAISchedulerGroupPolicy.UnifiedQualityPriorityColdStartMax *float64` and `.UnifiedQualityPriorityDailyMax *float64`; nil inherits global.
- Resolver: `openAIUnifiedQualityPriorityCapsForRequest(ctx context.Context, groupID int64) openAIUnifiedQualityPriorityCaps`.

- [ ] Add failing tests for defaults `50`/`20`, negative/NaN/Inf rejection, and group-policy JSON round-trip with omitted fields remaining nil.
- [ ] Run `cd upstream/sub2api/backend && go test ./internal/config ./internal/service -run '(PriorityCaps|SchedulerGroupPolicy|OpenAILegacyWeight)' -count=1`; verify failure.
- [ ] Add Viper defaults under `gateway.openai_ws`, validate finite non-negative values, and keep existing score-weight validation separate.
- [ ] Extend group-policy JSON parsing, normalization, and marshaling with the two fields; do not reinterpret `WeightOverrides` or `LegacyWeightOverrideIgnored`.
- [ ] Resolve global caps first, then independently override each cap from the selected group policy when non-nil; malformed values resolve safely to finite non-negative values.
- [ ] Re-run config/settings tests and verify pass.
- [ ] Commit with `git commit -m "feat: configure unified quality priority caps"`.

### Task 3: Integrate ranking and observability

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go` only for decision/cap plumbing.
- Test: `upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go`
- Test: scheduler log/repository tests if they assert decision JSON.

**Interfaces:**
- API-key score is `quality.QualityScore + coldStartPrioritySignal + dailyPrioritySignal`.
- Self-owned OAuth/SetupToken ordering remains native priority/load ordering.
- Decision JSON gains additive finite fields for selected priority, cold-start signal, and daily signal.

- [ ] Add failing tests for a priority-1 cold-start API key beating a peer with a modestly higher quality score; a mature priority-1 account receiving daily points; a materially better priority-50 account still winning; group priority overriding global priority; and runtime-blocked priority-1 exclusion.
- [ ] Run `cd upstream/sub2api/backend && go test ./internal/service -run 'TestOpenAIUnifiedQuality' -count=1`; verify failure.
- [ ] Resolve caps once per selection request. Calculate daily signal for every eligible API-key candidate and cold-start signal only for existing cold-start candidates.
- [ ] Update the API-key comparator to compare combined score, then preserve existing success-score, nullable TTFT, and account-ID tie breakers.
- [ ] Populate additive decision fields without logging credentials, raw API keys, request bodies, or settings secrets.
- [ ] Re-run focused scheduler tests and verify pass.
- [ ] Run `gofmt` on changed Go files, `git diff --check`, `go test ./internal/config ./internal/service ./internal/repository -run '(OpenAIUnifiedQuality|Scheduler|Settings)' -count=1`, and `go build ./cmd/server`; all must pass.
- [ ] Commit with `git commit -m "feat: include priority in unified quality scheduling"`.

## Final Handoff Checklist

- [ ] Report formulas, defaults, changed files, direct test results, build result, and diff-check result.
- [ ] Confirm no migration, credential, production-data, production-setting, or deployment changes.
- [ ] Keep the candidate worktree for root review; do not merge, push, deploy, or delete it from the implementation task.
- [ ] Before deployment, capture a Plus baseline and verify group priority and new decision fields online.
