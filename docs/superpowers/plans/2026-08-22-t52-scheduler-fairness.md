# T52 Scheduler Fairness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task with verification checkpoints.

**Goal:** Add native hybrid account exploration and live admin controls so healthy accounts that have been idle for too long are not starved by fixed Top-K priority ranking.

**Architecture:** Persist four global fairness settings plus JSON group overrides in the existing `settings` key/value store. Extend the existing OpenAI scheduler runtime snapshot to resolve group-aware settings, keep sticky/forced/failure-domain gates first, and add an exploration selection order only for eligible non-sticky requests. Reuse `SettingsView` and the existing settings API; no migration or second control plane.

**Tech Stack:** Go service/config/handler tests, PostgreSQL-backed settings repository, Vue 3 + TypeScript + Vitest, existing admin settings API and i18n.

**Spec:** `docs/superpowers/specs/2026-08-22-t52-scheduler-fairness-design.md`

## Global Constraints

- Preserve S1/S2 qualification, sticky bindings, retries, concurrency, billing, and usage logs.
- Invalid settings fail closed with HTTP 400 and leave prior values unchanged.
- Missing settings use hybrid/20%/21600 seconds/fairness weight 2 defaults.
- No database migration, production data write, external control plane, or GitHub Actions.
- All new UI fields include concise effect explanations and range/units.

### Task 1: Add setting contracts and parser validation

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/domain_constants.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_parse.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_service.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/settings.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler.go`
- Test: `upstream/sub2api/backend/internal/service/setting_service_update_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/setting_handler_test.go`

**Interfaces:**
- Add `OpenAISchedulerFairnessSettings` with `CandidatePoolMode string`, `ExplorationRatio int`, `StarvationThresholdSeconds int`, `FairnessWeight float64`, and `GroupOverrides map[int64]OpenAISchedulerFairnessOverride`.
- Add constants for the five settings keys and defaults.
- Add `Parse/NormalizeOpenAISchedulerFairnessSettings` helpers that return a bad-request error for unknown mode, ratio outside 0..100, threshold not 0 or 300..86400, fairness weight outside 0..10, invalid group id, or malformed JSON.
- Add response/request JSON fields and wire them through the existing whole-document PUT preservation logic.

- [ ] Write failing service tests for missing defaults, each invalid range, valid group override, and omitted-field preservation.
- [ ] Run `go test ./internal/service ./internal/handler/admin -run 'SchedulerFairness|SettingHandler' -count=1` and confirm the new assertions fail for missing symbols/behavior.
- [ ] Implement constants, structs, parser normalization, and handler/service mapping using the existing settings patterns.
- [ ] Run the focused Go tests and confirm they pass.
- [ ] Run `gofmt` on changed Go files and `git diff --check`.

### Task 2: Implement group-aware runtime fairness selection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_fairness_test.go`

**Interfaces:**
- `openAIAdvancedSchedulerRuntimeSettings` carries fairness defaults and group overrides.
- `openAISchedulerFairnessForRequest(ctx, groupID)` resolves the effective group setting.
- `buildOpenAISelectionOrder` returns existing order for `top_k`, all ranked eligible accounts for `all_eligible`, and a starvation/ratio exploration prefix plus the existing Top-K order for `hybrid`.

- [ ] Write failing tests for all-eligible coverage, hybrid threshold, hybrid ratio, sticky precedence, and qualification exclusion.
- [ ] Run the scheduler fairness tests and confirm they fail before implementation.
- [ ] Implement runtime parsing/cache fields and group resolution; keep cache TTL/invalidation unchanged.
- [ ] Implement deterministic idle ordering using `last_used_at` (nil first, then oldest) and ratio-based exploration without bypassing final acquire checks.
- [ ] Run scheduler fairness tests plus existing Top-K/sticky tests and confirm green.
- [ ] Run `go test ./internal/service -run 'OpenAIAccountScheduler|OpenAI.*TopK|SchedulerFairness' -count=1`.

### Task 3: Add admin controls and explanatory UI

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts`

**Interfaces:**
- Add typed fairness settings and group override records to the admin settings payload.
- Add a fairness section under the existing OpenAI advanced scheduler area with mode select, ratio/threshold/weight inputs, group selector, override rows, effective-value preview, and save/reset behavior.
- Each control renders a short description of purpose and the result of increasing/decreasing the value.

- [ ] Write failing Vitest tests for field labels/descriptions, mode/range validation, group override fallback/clear, effective preview, and mobile single-column class contract.
- [ ] Run the focused SettingsView tests and confirm failure before UI implementation.
- [ ] Implement typed form state, sanitization, controls, group loading via existing `adminAPI.groups.getAll`, and localized copy.
- [ ] Run focused SettingsView tests and confirm green.
- [ ] Run `pnpm typecheck` and the relevant Vitest suite.

### Task 4: Integrated verification and handoff

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `docs/handoffs/2026-08-22-t52-scheduler-fairness-handoff.md`

- [ ] Run Go focused service/handler tests, `go build ./cmd/server`, frontend focused tests, `pnpm typecheck`, `pnpm build`, and `git diff --check` in the candidate worktree.
- [ ] Verify no migration files, no `.github/workflows` changes, and no production writes.
- [ ] Record baseline SHA, candidate SHA, changed files, tests, defaults, rollback, `downtime_required=false`, and residual risks in the handoff.
- [ ] Update the task ledger to `READY_FOR_ROOT_REVIEW`; do not merge, push, deploy, or delete the worktree from the feature task.

