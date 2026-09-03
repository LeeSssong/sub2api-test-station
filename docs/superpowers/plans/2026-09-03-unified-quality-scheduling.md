# Unified Quality Scheduling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove administrator-editable per-group `weight_overrides` from OpenAI text scheduling while making every text group use the same versioned multi-window quality score and preserving native profit, candidate-pool, continuity, and safety semantics.

**Architecture:** Keep one fixed, versioned quality-score calculator for ordinary OpenAI text requests. Run Sub native eligibility and profit-control checks before scoring, use each group only to define its eligible candidate set and non-quality continuity policy, then sort eligible candidates by the shared score. Read legacy group overrides for compatibility/audit only and never apply them at runtime; do not delete historical settings in this task.

**Tech Stack:** Go backend scheduler/service/repository/handler tests, Vue/TypeScript admin settings UI and Vitest, PostgreSQL-backed native settings already used by Sub2API, existing scheduler decision event ledger.

**Spec:** `docs/superpowers/specs/2026-09-03-unified-quality-score-and-native-profit-control-design.md`

## Global Constraints

- Only ordinary OpenAI-compatible HTTP text requests change; images, Responses WebSocket, alpha search, forced protocol bindings, and non-OpenAI paths stay unchanged.
- All text groups use the same quality formula: success 40%, P50 TTFT 24%, P90 TTFT 16%, output rate 10%, live load 10%.
- Quality windows are mutually exclusive W1=[now-1h, now), W24=[now-24h, now-1h), W7=[now-7d, now-24h), with 50/30/20 window weighting and confidence fallback.
- Profit and cost never enter quality score; Sub native rate multiplier and profit control remain the only commercial policy.
- Native eligibility, profit precheck/terminal check, capability, cooldown, failure-domain, concurrency, and safe-replay gates remain fail-closed and precede scoring.
- TTFT has no hard quality veto; the 60-second slow-output observation affects later ranking only and never cancels or creates another attempt.
- Legacy overrides remain readable for audit compatibility but are ignored at runtime and are not newly persisted.
- Do not modify root main, global progress/queue documents, production configuration, database data, or deployment state from the feature worktree.

---

### Task 1: Freeze the Shared Quality-Score Contract

Files:
- Modify: upstream/sub2api/backend/internal/service/openai_account_quality.go
- Modify: upstream/sub2api/backend/internal/service/openai_quality_score.go
- Modify: upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go
- Modify: upstream/sub2api/backend/internal/service/openai_quality_score_test.go
- Modify: upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler_test.go
- Create: upstream/sub2api/backend/internal/service/openai_unified_quality_score_test.go

Interfaces:
- Consumes: `OpenAIAccountQualitySnapshotProvider`, `OpenAIAccountQuality`, `AccountLoadInfo`, and the existing T114 mutually-exclusive window metrics.
- Produces: `OpenAIUnifiedQualityScoreVersion = "t122-v1"` and `calculateOpenAIUnifiedQualityScore(quality OpenAIAccountQuality, load *AccountLoadInfo, slow *OpenAIFirstOutputSlowTracker, accountID int64) OpenAIQualityBreakdown`. `OpenAIQualityBreakdown` exposes total, success, P50 TTFT, P90 TTFT, output-rate, live-load, confidence, and window provenance fields.

- [ ] Step 1: Write failing tests for identical inputs producing identical scores across group IDs, fixed component coefficients (40/24/16/10/10), missing evidence using neutral values, and cost/profit fields not changing the result.
- [ ] Step 2: Run go test ./internal/service -run TestOpenAIUnifiedQualityScore -count=1; confirm failure because the shared calculator/versioned contract does not exist.
- [ ] Step 3: Implement the shared calculator in `openai_quality_score.go`, with named constants `0.40`, `0.24`, `0.16`, `0.10`, `0.10`; make `buildOpenAIQualityBreakdowns` call it without accepting group policy or cost inputs.
- [ ] Step 4: Run the focused tests again and confirm all pass, including the same account quality score when evaluated for group 2 and group 20.
- [ ] Step 5: Commit with git add internal/service && git commit -m "feat: add unified openai quality score contract".

### Task 2: Remove Runtime Application of Legacy Group Weight Overrides

Files:
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler.go
- Modify: upstream/sub2api/backend/internal/service/settings_view.go
- Modify: upstream/sub2api/backend/internal/service/setting_parse.go
- Modify: upstream/sub2api/backend/internal/service/setting_update.go
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go
- Modify: upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go
- Create: upstream/sub2api/backend/internal/service/openai_legacy_weight_compat_test.go

Interfaces:
- Consumes: persisted openai_advanced_scheduler_group_overrides JSON and the shared calculator from Task 1.
- Produces: `OpenAISchedulerGroupPolicy.LegacyWeightOverrideIgnored bool` and `IgnoredWeightOverrideKeys []string`; runtime normalization preserves candidate-pool/continuity fields while never calling `applyOpenAIAdvancedSchedulerWeightOverrides` for ordinary text selection.

- [ ] Step 1: Write failing tests that set different per-group weight_overrides and assert equal quality scores, equal score components, and no runtime use of upstream_cost, ttft, session_sticky, or previous_response override values.
- [ ] Step 2: Run go test ./internal/service -run TestOpenAILegacyWeightOverride -count=1 and confirm failure because the current resolver applies overrides.
- [ ] Step 3: Stop `resolveOpenAIAdvancedSchedulerWeights` and runtime group-policy normalization from applying `WeightOverrides`; retain the raw keys in the compatibility diagnostic and preserve candidate-pool mode, Top-K, fairness, native eligibility, profit checks, and failure-domain behavior.
- [ ] Step 4: Run focused scheduler tests and confirm legacy override values cannot change ranking, while candidate-pool membership and hard gates still do.
- [ ] Step 5: Commit with git add internal/service && git commit -m "refactor: ignore legacy openai group weight overrides".

### Task 3: Make Continuity Conditional Instead of Score-Weighted

Files:
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler.go
- Modify: upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go
- Modify: upstream/sub2api/backend/internal/handler/openai_gateway_handler.go
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler_quality_gate.go
- Modify: upstream/sub2api/backend/internal/service/openai_resilience_observability.go
- Create: upstream/sub2api/backend/internal/service/openai_continuity_priority_test.go

Interfaces:
- Consumes: native previous-response capability checks, sticky binding state, shared quality score, cooldown/concurrency/profit state.
- Produces: conditional continuity selection: protocol-required previous response stays hard-bound to compatible accounts; ordinary sticky is attempted only for still-eligible, non-escaped accounts and otherwise falls through to highest-quality eligible candidate.

- [ ] Step 1: Write failing tests for previous-response capability hard binding, sticky bypass on quality escape/cooldown/concurrency/profit rejection, and highest-quality fallback after sticky bypass.
- [ ] Step 2: Run go test ./internal/service ./internal/handler -run TestOpenAI.*Continuity -count=1 and confirm failure because legacy sticky/previous score additions still influence selection.
- [ ] Step 3: Implement the smallest continuity change: remove score additions sourced from legacy SessionSticky and Previous weights; preserve protocol-required account filtering and existing sticky escape reasons/events; perform sticky as a bounded preference before normal quality order.
- [ ] Step 4: Run focused service and handler tests, ensuring safe replay and client-disconnect paths are unchanged.
- [ ] Step 5: Commit with git add internal/service internal/handler && git commit -m "refactor: separate openai continuity from quality score".

### Task 4: Enforce Native Profit Control as the Sole Commercial Gate

Files:
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler.go
- Modify: upstream/sub2api/backend/internal/service/openai_unified_quality_scheduler.go
- Modify: upstream/sub2api/backend/internal/service/openai_profit_control.go
- Modify: upstream/sub2api/backend/internal/handler/openai_gateway_handler.go
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go
- Modify: upstream/sub2api/backend/internal/handler/openai_profit_slot_recheck_test.go
- Modify: upstream/sub2api/backend/internal/handler/openai_profit_veto_budget_test.go
- Create: upstream/sub2api/backend/internal/service/openai_native_profit_gate_test.go

Interfaces:
- Consumes: Sub native group rate_multiplier, profit_control_enabled, profit_min_margin, profit_safety_buffer, and existing precheck/terminal-check services.
- Produces: fail-closed commercial eligibility before score comparison and terminal recheck after slot acquisition, with non-sensitive rejection reason in decision events.

- [ ] Step 1: Write failing tests proving a cost/profit change cannot change an otherwise eligible account quality score, a native profit rejection removes the account before ranking, and a terminal profit rejection releases the slot and reselects.
- [ ] Step 2: Run go test ./internal/service -run TestOpenAINativeProfitGate -count=1 and confirm failure for any path that lets quality/cost ranking bypass native profit control.
- [ ] Step 3: Change `partitionOpenAIUnifiedQualityCandidates` so an active native profit gate with no qualified candidates returns no candidates and a typed rejection reason; retain the existing gateway terminal recheck and slot-release/reselection path without introducing another margin calculation.
- [ ] Step 4: Run focused profit/scheduler tests and confirm unknown native-control results fail closed.
- [ ] Step 5: Commit with git add internal/service && git commit -m "fix: keep native profit control outside quality score".

### Task 5: Collapse Settings API and Admin UI to Read-Only Unified Policy

Files:
- Modify: upstream/sub2api/backend/internal/handler/dto/settings.go
- Modify: upstream/sub2api/backend/internal/handler/admin/setting_handler.go
- Modify: upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go
- Modify: upstream/sub2api/backend/internal/handler/admin/setting_handler_audit.go
- Modify: upstream/sub2api/backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go
- Modify: upstream/sub2api/backend/internal/server/api_contract_test.go
- Modify: upstream/sub2api/backend/internal/service/setting_parse.go
- Modify: upstream/sub2api/backend/internal/service/setting_update.go
- Modify: upstream/sub2api/frontend/src/views/admin/SchedulerSettingsView.vue
- Modify: upstream/sub2api/frontend/src/api/admin/index.ts
- Modify: upstream/sub2api/frontend/src/views/admin/__tests__/SchedulerSettingsView.spec.ts
- Modify: upstream/sub2api/frontend/src/views/admin/scheduler/__tests__/schedulerPolicy.spec.ts
- Modify: upstream/sub2api/frontend/src/views/admin/scheduler/schedulerPolicy.ts
- Create: upstream/sub2api/frontend/src/views/admin/__tests__/UnifiedQualitySchedulingSettings.spec.ts

Interfaces:
- Consumes: legacy settings JSON, unified score version/constants, candidate-pool/continuity settings, native profit-control projection.
- Produces: DTO fields `openai_quality_score_version`, `openai_quality_score_coefficients`, and `openai_legacy_weight_overrides_ignored`; `UpdateSettingsRequest` rejects `weight_overrides` inside newly submitted group policies before `settingService.UpdateSettings` runs.

- [ ] Step 1: Write failing API and Vue tests for hidden/removed editable weight controls, read-only coefficient display, legacy override diagnostics, rejection of newly submitted group weight maps, and unchanged candidate-pool/continuity writes.
- [ ] Step 2: Run focused Go/Vitest tests and confirm failure while current UI/API still exposes and saves group weight overrides.
- [ ] Step 3: Implement API validation and UI removal: stop generating new weight_overrides, preserve old JSON on read, return explicit ignored diagnostics, and keep non-quality settings functional.
- [ ] Step 4: Run focused Go tests and targeted Vitest suite, checking old clients that omit new fields remain compatible.
- [ ] Step 5: Commit with git add upstream/sub2api/backend/internal/handler/admin upstream/sub2api/backend/internal/service upstream/sub2api/frontend && git commit -m "feat: expose unified openai quality policy".

### Task 6: Decision Events, Regression Matrix, and Release Readiness

Files:
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go
- Modify: upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience.go
- Modify: upstream/sub2api/backend/internal/service/openai_account_scheduler_projection_test.go
- Modify: upstream/sub2api/backend/internal/service/ops_openai_scheduler_experience_test.go
- Modify: upstream/sub2api/backend/internal/service/openai_resilience_observability.go
- Modify: upstream/sub2api/backend/internal/service/scheduler_events.go
- Modify: upstream/sub2api/backend/internal/handler/admin/ops_scheduler_experience_handler_test.go
- Create: docs/superpowers/reports/2026-09-03-unified-quality-scheduling-verification.md

Interfaces:
- Consumes: Tasks 1–5 score, eligibility, continuity, and settings projections.
- Produces: decision events containing score version, component breakdown/confidence, candidate/eligible/effective Top-K counts, native profit rejection reason, and sticky keep/escape reason with sensitive fields excluded.

- [ ] Step 1: Write failing event-contract tests for score version, component breakdown, ignored legacy keys, profit rejection reason, sticky outcome, and absence of credentials/request bodies.
- [ ] Step 2: Run focused event tests and confirm missing fields fail the contract.
- [ ] Step 3: Implement additive event projection without changing billing, retry, failover, or usage semantics.
- [ ] Step 4: Run the direct validation matrix:
  - go test ./internal/service -run TestOpenAI -count=1
  - go test ./internal/handler/admin -run Test.*Scheduler -count=1
  - cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/UnifiedQualitySchedulingSettings.spec.ts
  - go build ./cmd/server
  - cd upstream/sub2api/frontend && pnpm typecheck && pnpm build
  - gofmt on changed Go files and git diff --check
- [ ] Step 5: Write the verification report with test output, unverified items, migration/config changes (none), downtime (unknown until root preflight), rollback (previous verified root main/slot), and remaining risks.
- [ ] Step 6: Commit with git add . && git commit -m "test: verify unified quality scheduling policy".

## Completion Criteria

- All ordinary OpenAI text groups use the same versioned quality-score calculator.
- No runtime path applies administrator-supplied per-group weight overrides.
- Profit/cost does not enter quality score and native Sub profit control remains fail-closed.
- Candidate-pool and hard-gate semantics remain explicit and tested.
- Previous-response and ordinary sticky behavior remain safe and explainable.
- Direct tests, build/typecheck, and diff-check pass.
- Worktree remains unpushed and undeployed until root review and explicit release authorization.
