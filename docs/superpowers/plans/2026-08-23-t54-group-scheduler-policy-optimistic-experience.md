# T54 分组调度策略与乐观体验卡实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在原生 Sub 调度器和 Monitor V2 上增加可维护的分组策略选择、三种公平预设、受限权值覆盖和纯乐观体验卡展示，避免合格账号长期被冷落。

**Architecture:** 继续复用 `GET/PUT /api/v1/admin/settings`、现有 settings cache、原生账号资格链和 `LastUsedAt`。把现有 JSON 分组覆盖规范化为带 `mode/preset/weight_overrides/fairness` 的策略对象；旧 JSON 只读兼容为权值覆盖模式。调度器按请求 `group_id` 解析策略，在 S1/S2 等硬门槛之后执行策略选择；Monitor V2 保留后台探测和 API 字段，但前端体验卡只渲染成功形成的乐观指标。

**Tech Stack:** Go、PostgreSQL-backed native settings JSON、Vue 3、TypeScript、Vitest、现有 Sub2API release chain。

**Spec:** `docs/superpowers/specs/2026-08-23-t54-group-scheduler-policy-optimistic-experience-design.md`

## Global Constraints

- 只复用 Sub 原生调度、settings、Monitor V2 和发布链，不新增平行控制面、账务源、错误表或质量事实源。
- 每个分组的模式只能是 `weighted_override` 或 `fair`；公平模式预设只能是 `special_offer`、`balanced`、`pro`。
- 自定义权值必须是有限、非负数；TopK 为正整数；探索比例为 0-100；闲置阈值为 0 或 300-86400 秒；公平权重为 0-10。
- 公平探索不得绕过 S1/S2、模型能力、余额/资格、并发、故障域、sticky、previous response 或安全重放边界。
- 主卡不展示探测样本数、失败数、失败原因、失败状态、探测来源标签或探测旁证；探测只保留后台用途。
- 不新增迁移或生产数据写入；预计 `downtime_required=false`；禁止 GitHub Actions。

---

### Task 1: 规范化分组策略契约与预设展开

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/settings_view.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_parse.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_update.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/settings.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler.go`
- Test: `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`
- Test: `upstream/sub2api/backend/internal/service/setting_parse_test.go`

**Interfaces:**
- Consumes: existing `OpenAISchedulerFairnessSettings`, `OpenAISchedulerFairnessOverride`, ten native scheduler weight keys and current JSON `group_overrides` payload.
- Produces: `OpenAISchedulerGroupPolicy`, `OpenAISchedulerGroupPolicyMode`, `OpenAISchedulerPreset`, `normalizeOpenAISchedulerGroupPolicies`, `resolveOpenAISchedulerPolicyForGroup`, and a validated API DTO map keyed by `int64` group ID.

- [ ] **Step 1: Write failing contract tests** for the four strategy shapes: weighted override, each fair preset, omitted fields inheriting global values, and legacy `{candidate_pool_mode, exploration_ratio}` JSON reading as `weighted_override` without losing data.
- [ ] **Step 2: Add concrete preset constants and values** in `settings_view.go`, using the current runtime defaults as baseline:
  - special offer: `top_k=7`, priority `0.8`, load `0.8`, queue `0.5`, error `0.8`, TTFT `0.2`, reset `0`, quota `0`, upstream cost `2.5`, previous response `5`, session sticky `3`, pool `hybrid`, exploration `15`, starvation `21600`, fairness `2`;
  - balanced: current defaults (`7`, `1`, `1`, `0.7`, `0.8`, `0.5`, `0`, `0`, `0`, `5`, `3`, `hybrid`, `25`, `21600`, `3`);
  - pro: `top_k=10`, priority `1.2`, load `1.4`, queue `1.2`, error `2.5`, TTFT `2.0`, reset `0.5`, quota `0.2`, upstream cost `1.5`, previous response `5`, session sticky `3`, pool `hybrid`, exploration `40`, starvation `10800`, fairness `5`.
  These values are bounded within existing runtime ranges and make Pro stability/latency primary while retaining a cost tie-breaker.
- [ ] **Step 3: Implement normalization and fail-closed validation** so mode/preset mismatch, unknown policy fields, unknown group IDs, invalid finite numbers, out-of-range values, and a zero-total weight set return the existing bad-request error and leave the previous settings unchanged.
- [ ] **Step 4: Implement compatibility conversion** from the old pointer-field override object into `OpenAISchedulerGroupPolicy{Mode: weighted_override}` and serialize the new object without clearing omitted legacy fields.
- [ ] **Step 5: Run focused backend tests**.

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(OpenAIScheduler|NormalizeOpenAI|ResolveOpenAI|SettingParse)' -count=1`

Expected: PASS, including legacy JSON, preset expansion, invalid payload fail-closed, and group fallback cases.

- [ ] **Step 6: Commit** with `feat: add validated per-group scheduler policies`.

### Task 2: Apply policies to native OpenAI scheduler without bypassing hard gates

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_fairness_test.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_adaptive_test.go`

**Interfaces:**
- Consumes: `resolveOpenAISchedulerPolicyForGroup`, normalized policy values, existing `LastUsedAt`, `fairnessExplorationDue`, S1/S2 candidate filtering and selection order.
- Produces: policy-aware `buildOpenAISelectionOrder` behavior with bounded longest-idle exploration and explicit per-group policy mode.

- [ ] **Step 1: Write failing scheduler tests** proving weighted mode does not apply fairness fields, fair mode selects an overdue eligible account before the regular Top-K list, and sticky/forced-retry/half-open paths keep their existing precedence.
- [ ] **Step 2: Thread the resolved policy through both scheduler planning and selection-order paths** so the same group policy is used for scoring, Top-K and exploration; remove any second global fairness lookup in the same request path.
- [ ] **Step 3: Implement longest-idle guard**: sort only the current group’s already-qualified candidates by `LastUsedAt`, treat nil as oldest, add at most one overdue candidate, and when no candidate crosses the threshold use deterministic session-hash exploration ratio. Preserve final concurrency/lease checks.
- [ ] **Step 4: Add a bounded minimum-opportunity rule** for fair mode: when a group has at least two qualified candidates and the primary candidate has been selected repeatedly while another candidate is older than the configured threshold, the older candidate is inserted once; never add candidates rejected by S1/S2 or current-request exclusion.
- [ ] **Step 5: Run scheduler focused tests**.

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(OpenAI.*Scheduler|Fairness|SelectionOrder|Sticky|RetryBudget)' -count=1`

Expected: PASS; no regression to sticky, previous-response, forced retry, S1/S2 veto, or half-open tests.

- [ ] **Step 6: Commit** with `feat: apply per-group scheduler fairness policies`.

### Task 3: Replace JSON editor with group-first policy controls

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- Modify: `upstream/sub2api/frontend/src/api/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts`

**Interfaces:**
- Consumes: settings API policy map keyed by group ID and active group list already available to admin settings view.
- Produces: group selector, two-option mode control, preset selector with automatic read-only values, bounded custom inputs, clear-to-inherit action, and backward-compatible PUT payload.

- [ ] **Step 1: Write failing Vitest contracts** for selecting a group, switching modes, preset auto-fill, read-only preset values, clearing a group to inherit global values, range rejection, and legacy payload round-trip.
- [ ] **Step 2: Add typed frontend policy models and preset display metadata** matching backend field names and limits; keep the old JSON string parser only as a hidden compatibility reader for existing payloads.
- [ ] **Step 3: Replace the textarea** with a group-first panel. Use a select for groups, segmented control for `weighted_override`/`fair`, select for three fair presets, and compact numeric inputs for weighted mode. Each field must show the existing purpose/range hint and remain within stable responsive grid tracks.
- [ ] **Step 4: Implement auto-fill and dirty-state behavior**: choosing a fair preset replaces the selected group’s policy with the preset values; switching to weighted mode enables inputs; clearing removes only the selected group override and restores global effective values.
- [ ] **Step 5: Serialize the new policy map** in the existing PUT settings request; preserve unrelated settings and omitted groups, reject invalid local values before request, and show the existing API error without mutating the last saved form.
- [ ] **Step 6: Run focused frontend tests**.

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts -t 'scheduler'`

Expected: PASS, including desktop and mobile layout contracts and no JSON editor visible in the new flow.

- [ ] **Step 7: Commit** with `feat: add group-first scheduler policy controls`.

### Task 4: Keep Monitor V2 cards purely optimistic

**Files:**
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Consumes: existing Monitor V2 account projection and successful metric fields.
- Produces: unchanged API contract with a card that renders only optimistic availability, TTFT, latency and timeline state.

- [ ] **Step 1: Write failing component assertions** that sample count, failure count, error reason, `evidence_source`, and probe summary are absent from the rendered card while successful metric values remain.
- [ ] **Step 2: Remove the visible sample detail, probe summary, source label, failure-state copy and probe-derived failure count from the card template and computed display helpers; keep types/API fields intact for non-card consumers.
- [ ] **Step 3: Preserve successful timeline bars and no-result visual state** without rendering failure diagnostics or probe provenance.
- [ ] **Step 4: Run focused Monitor V2 tests**.

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`

Expected: PASS with only optimistic card content visible.

- [ ] **Step 5: Commit** with `feat: keep monitor cards optimistic`.

### Task 5: Integrated validation and root handoff

**Files:**
- Modify: `docs/handoffs/2026-08-23-t54-group-scheduler-policy-optimistic-experience-handoff.md`
- Modify: `docs/project/project-progress.md` (root-only status update after candidate is ready)

- [ ] **Step 1: Run backend direct gates**.

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(OpenAIScheduler|NormalizeOpenAI|ResolveOpenAI|SettingParse)' -count=1 && go build ./cmd/server`

- [ ] **Step 2: Run frontend direct gates**.

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts && pnpm typecheck && pnpm build`

- [ ] **Step 3: Run scope and formatting checks**.

Run: `git diff --check && git diff --name-only -- upstream/sub2api/backend upstream/sub2api/frontend | rg -v '^(upstream/sub2api/backend/internal/service/(settings_view|setting_parse|setting_update|openai_account_scheduler|scheduler_fairness_settings_test|setting_parse_test|openai_account_scheduler_fairness_test)|upstream/sub2api/backend/internal/handler/(dto/settings|admin/setting_handler_update|admin/setting_handler)|upstream/sub2api/frontend/src/(views/admin/SettingsView|api/admin/settings|i18n/locales/(zh|en)/admin/settings|components/admin/account-monitor/AccountMonitorCard|views/admin/__tests__/SettingsView.spec|components/admin/account-monitor/AccountMonitorCard.spec|views/admin/__tests__/AccountMonitorView.spec))'`

Expected: no out-of-scope implementation files and no whitespace errors.

- [ ] **Step 4: Write handoff evidence** with baseline `main` SHA, candidate SHA, changed files, tests, no migration/config schema change, `downtime_required=false`, rollback to previous blue-green image or clear policy overrides, and known residual risk that `LastUsedAt` is account-level rather than a persisted per-group quota.
- [ ] **Step 5: Stop at `READY_FOR_ROOT_REVIEW`**. Do not merge, push, deploy or update the global queue from the feature worktree.

## Verification Summary

The candidate is ready only when all direct backend/frontend gates pass, the optimistic card assertions prove that probe evidence is not rendered, scheduler tests prove fair-mode progress without hard-gate bypass, `git diff --check` passes, and the handoff explicitly records unverified production-only items. Root then performs the single merge, publish, and online verification lane.
