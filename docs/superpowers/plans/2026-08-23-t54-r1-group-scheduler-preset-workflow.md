# T54-R1 分组调度三步流程与全局命名预设实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 T54 设置页的分组来源与交互层级，并在原生 settings 中增加可跨分组复用的管理员命名预设。

**Architecture:** 分组继续来自 Sub 原生 `/admin/groups/all?platform=openai`，调度分组列表与默认订阅列表分开维护。后端在现有 settings JSON 中保存管理员预设和完整的分组策略快照，GET 派生统一可选预设列表；运行时继续消费策略快照，不改变调度算法。

**Tech Stack:** Go、Vue 3、TypeScript、Vitest、原生 Sub2API settings、现有蓝绿发布链。

**Spec:** `docs/superpowers/specs/2026-08-23-t54-r1-group-scheduler-preset-workflow-design.md`

## Global Constraints

- 页面顺序必须是 `选择分组 -> 选择策略模式 -> 配置参数/选择预设`。
- 策略模式只显示 `custom | preset` 对应的“自定义参数 | 预设模式”。
- 调度分组必须直接使用原生有效 OpenAI 分组，不按 `subscription_type` 过滤；默认订阅列表保持原过滤语义。
- 预设模式所有最终参数必须 `disabled`，并以服务端快照为唯一提交值。
- 管理员命名预设全局复用；内置预设不可变；被引用的管理员预设不得删除。
- 保存命名预设后当前分组仍保持自定义模式，不发生隐式切换。
- 旧 `weighted_override | fair` 配置必须无损读取；旧客户端省略新字段时不得清空新数据。
- 不改变 T54 调度算法、内置预设数值、S1/S2、sticky、并发、故障域或 Monitor V2。
- 不新增迁移、生产业务数据写入、依赖或 GitHub Actions；预计 `downtime_required=false`。

---

### Task 1: 后端命名预设与新策略契约

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/domain_constants.go`
- Modify: `upstream/sub2api/backend/internal/service/settings_view.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_parse.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_update.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/settings.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go`
- Test: `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`
- Test: `upstream/sub2api/backend/internal/service/setting_parse_test.go`
- Test: focused admin settings handler test discovered with `rg`.

**Interfaces:**
- Consumes: existing `OpenAISchedulerPolicyValues`, T54 built-in preset values, settings repository update transaction and group policy parsing.
- Produces: `OpenAISchedulerPresetDefinition`, `OpenAISchedulerCustomPreset`, `OpenAISchedulerGroupPolicyModeCustom`, `OpenAISchedulerGroupPolicyModePreset`, settings key `openai_advanced_scheduler_custom_presets`, normalized `openai_advanced_scheduler_available_presets`, and backward-compatible group policy parsing.

- [ ] **Step 1: Write failing service tests** for the three immutable built-ins plus persisted custom presets; `weighted_override -> custom`; `fair+pro -> preset+builtin:pro`; complete snapshot persistence; invalid name/ID/value/reference rejection; referenced-delete rejection; and omitted-field preservation.
- [ ] **Step 2: Run RED tests:** `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(OpenAIScheduler.*Preset|NormalizeOpenAISchedulerGroupPolicies|ParseOpenAISchedulerGroupPolicies|Setting.*Scheduler)' -count=1`. Expected: FAIL because the new contracts do not exist.
- [ ] **Step 3: Add minimal typed contracts and constants** in `settings_view.go` and `domain_constants.go`. Reuse `openAISchedulerPresetValues`; do not duplicate numeric constants in handlers.
- [ ] **Step 4: Implement strict parsing and normalization** in `setting_parse.go`: legacy conversion, complete snapshot materialization, temporary custom ID normalization, name/value validation and reference validation.
- [ ] **Step 5: Implement atomic settings update** in `setting_update.go` and handler update flow. Validate the combined previous/request state before writing either JSON key; omitted fields preserve prior values; available presets remain read-only.
- [ ] **Step 6: Add GET/PUT DTO coverage** proving GET exposes normalized policies and available presets and PUT rejects invalid combined state with 400.
- [ ] **Step 7: Run GREEN tests** using Step 2 plus focused handler tests. Expected: PASS.
- [ ] **Step 8: Commit** with `feat: add reusable scheduler presets`.

### Task 2: 前端策略状态模型与原生分组来源

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts`

**Interfaces:**
- Consumes: `adminAPI.groups.getAll('openai')`, normalized custom/preset policy map, available preset definitions and existing settings save call.
- Produces: separate scheduler group state, per-group drafts, custom preset drafts, legacy normalization and serialization helpers.

- [ ] **Step 1: Write failing Vitest cases** proving the groups API uses `openai`; standard groups remain visible; default subscriptions retain their old filter; no group is auto-selected; switching groups preserves drafts; legacy payloads normalize; preset serialization ignores mutated draft fields.
- [ ] **Step 2: Run RED tests:** `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts -t 'scheduler'`. Expected: FAIL on standard group visibility, mode names and draft isolation.
- [ ] **Step 3: Extend TypeScript API types** for `custom | preset`, preset IDs, definitions, custom preset map and parameter snapshots.
- [ ] **Step 4: Split group loading state** so scheduler groups contain every native active OpenAI group while `subscriptionGroups` keeps subscription-only filtering. Preserve a visible scheduler group load error.
- [ ] **Step 5: Implement draft helpers** in the existing view: legacy normalization, group draft load/store, preset snapshot clone, custom preset add/rename/delete and final request serialization.
- [ ] **Step 6: Run GREEN tests** using Step 2. Expected: PASS.
- [ ] **Step 7: Commit** with `fix: load native scheduler groups`.

### Task 3: 三步设置界面与命名预设交互

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts`

**Interfaces:**
- Consumes: Task 2 draft helpers and typed preset definitions.
- Produces: visible three-step editor, custom/preset segmented mode, conditional parameter/preset area, naming interaction and admin preset rename/delete controls.

- [ ] **Step 1: Write failing component tests** for exact DOM order, unselected-group gate, two mode labels, separate preset field, all preset parameters disabled, custom inputs enabled, naming, no implicit mode switch, referenced-delete error, and removal of the misleading top-level parameter grids from this workflow.
- [ ] **Step 2: Run RED tests:** `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts -t 'scheduler'`. Expected: FAIL because the current UI exposes global fields first and old mode labels.
- [ ] **Step 3: Rebuild only the T54 group-policy section** in the existing page style. Use native select, segmented buttons, preset select, disabled preset values and bounded custom inputs; do not add nested cards.
- [ ] **Step 4: Add naming interaction** with 1-40 character validation. Keep the current group custom after creation; group options by built-in/admin; expose rename/delete only for admin presets; locally disable referenced deletion.
- [ ] **Step 5: Remove misleading top-level editable weight/fairness grids from this workflow** while retaining global compatibility values in form state and PUT serialization. Keep unrelated scheduler enable, sticky and subscription-priority controls.
- [ ] **Step 6: Add concise Chinese/English labels and errors** for steps, modes, preset operations, disabled effective values and group-load failure.
- [ ] **Step 7: Run GREEN tests** using Step 2. Expected: PASS.
- [ ] **Step 8: Commit** with `feat: enforce scheduler preset workflow`.

### Task 4: Direct validation and handoff

**Files:**
- Create: `docs/handoffs/2026-08-23-t54-r1-group-scheduler-preset-workflow-handoff.md`
- Modify only for a real focused failure: files already listed in Tasks 1-3.

**Interfaces:**
- Consumes: completed backend/frontend implementation.
- Produces: candidate SHA, exact validation evidence, deployment properties and rollback statement.

- [ ] **Step 1: Run focused backend tests:** `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler/admin -run 'Test(OpenAIScheduler|.*Scheduler.*Setting|.*Setting.*Scheduler)' -count=1`.
- [ ] **Step 2: Build backend:** `cd upstream/sub2api/backend && go build ./cmd/server`.
- [ ] **Step 3: Run focused frontend tests:** `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts`.
- [ ] **Step 4: Run frontend gates:** `cd upstream/sub2api/frontend && pnpm typecheck && pnpm build`.
- [ ] **Step 5: Run range checks:** `git diff --check main...HEAD`; then ensure `git diff --name-only main...HEAD` contains neither migrations nor `.github/workflows`.
- [ ] **Step 6: Write handoff** with baseline/candidate SHA, changed files, results, unverified items, migration/config change, `downtime_required`, rollback and risks.
- [ ] **Step 7: Commit** with `docs: hand off t54 r1 scheduler workflow`.

## Plan Self-Review

- Spec coverage: all four user requirements map to Tasks 2-3; persistence, compatibility and deletion safety map to Task 1; release evidence maps to Task 4.
- Placeholder scan: no TBD/TODO/later placeholders; focused commands and expected outcomes are explicit.
- Type consistency: `custom | preset`, `preset_id`, snapshots, `openai_advanced_scheduler_custom_presets` and `openai_advanced_scheduler_available_presets` are consistent.
- Scope check: no scheduler runtime algorithm, Monitor V2, migration, dependency or production-data work is included.
- Approval record: user selected global reusable administrator presets, approved the design, and instructed direct implementation after the specification.
