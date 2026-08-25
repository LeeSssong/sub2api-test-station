# T68 Scheduler Priority Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the operator-facing OpenAI group scheduler weight form with per-group numeric business priorities, three discrete operational controls, and truthful pre-save scenario previews while preserving native qualification, concurrency, sticky, fairness, and billing boundaries.

**Architecture:** Extend the existing native settings DTO and group-policy normalization in place. Store a business-facing policy plus a server-owned compiled snapshot in the existing settings JSON, compile the policy into the current scheduler weight/fairness inputs after hard qualification gates, and keep the current atomic concurrency acquisition and fallback chain unchanged. Replace only the T54 scheduler policy editor region in `SettingsView.vue`; all preview text is derived from the same draft policy used for serialization.

**Tech Stack:** Go service/handler tests, existing settings repository and JSON contracts, Vue 3 + TypeScript + Vitest, Tailwind classes already used by `SettingsView`, pnpm typecheck/build.

**Spec:** `docs/superpowers/specs/2026-08-25-t68-scheduler-policy-priority-design.md`

## Global Constraints

- Reuse Sub native settings, scheduler, account qualification, concurrency, sticky, S1/S2, error, billing, and Monitor facts; do not create a parallel control plane or scheduler.
- Services must keep hard qualification, model capability, balance/quota, cooldown, failure-domain, S1/S2, final concurrency acquisition, fresh-account recheck, and DB recheck before selection succeeds.
- Business priority values are integers `1..3`; equal values are a single equal-priority tier; the client never submits or computes decimal scheduler weights.
- Operational controls are only `balance=low|standard|high`, `peak_protection=strict|standard|open`, and `session_continuity=keep|standard|switch`.
- No database migration, production-data write, history backfill, new dependency, or GitHub Actions workflow.
- Direct validation only: focused Go/Vitest tests, `go build ./cmd/server`, `pnpm typecheck`, `pnpm build`, and `git diff --check`.

### Task 1: Add failing service contract tests for business policies

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go`
- Modify: `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go` (the existing scheduler fairness test file owns parser/normalization contracts)

**Interfaces:**
- Consumes: existing `OpenAISchedulerGroupPolicy`, `OpenAISchedulerPolicyValues`, normalization helpers, and scheduler test fixtures.
- Produces: RED tests that define `OpenAISchedulerBusinessPriority`, `OpenAISchedulerOperations`, policy normalization, legacy conversion, and compiled-input behavior for later implementation tasks.

- [ ] **Step 1: Write failing tests for four recommended defaults and equal priority.**

  Add table-driven cases asserting:

  ```go
  wantSpecial := OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 2, Latency: 3}
  wantPlus := OpenAISchedulerBusinessPriority{Profit: 1, TTFT: 1, Latency: 1}
  wantPro := OpenAISchedulerBusinessPriority{Profit: 3, TTFT: 1, Latency: 2}
  ```

  Assert Plus preserves one equal tier rather than imposing an order.

- [ ] **Step 2: Write failing tests for operational enum validation and defaults.**

  Cover valid `low|standard|high`, `strict|standard|open`, `keep|standard|switch`, rejection of unknown values, and the documented default `standard|strict|standard`.

- [ ] **Step 3: Write failing tests for legacy policy reads.**

  Assert old `weighted_override`, `fair`, and `preset` payloads remain readable and produce a business policy plus the original compiled snapshot without changing the old stored values.

- [ ] **Step 4: Write failing scheduler tests for hard-gate ordering.**

  Use the existing scheduler fixtures to assert a high-priority exploration candidate that is full, stale, or not schedulable is skipped, while a healthy candidate with capacity is selected. Assert a strict peak policy suppresses pure exploration but still permits capacity-spreading fallback.

- [ ] **Step 5: Run the focused tests and verify RED.**

  Run:

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service -run 'Test(NormalizeOpenAISchedulerBusiness|ParseOpenAISchedulerBusiness|OpenAISchedulerBusiness|OpenAIScheduler.*Priority|OpenAIScheduler.*Peak)' -count=1
  ```

  Expected: FAIL because the business policy types and compiler do not yet exist.

- [ ] **Step 6: Commit the RED tests.**

  ```bash
  git add upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go upstream/sub2api/backend/internal/service/openai_account_scheduler_test.go
  git commit -m "test: define T68 scheduler business policy contracts"
  ```

### Task 2: Implement native service policy types, normalization, and compilation

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/settings_view.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_parse.go`
- Modify: `upstream/sub2api/backend/internal/service/domain_constants.go` only if a new existing-settings key is required by the established settings pattern
- Modify: `upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go`

**Interfaces:**
- Consumes: Task 1 RED contracts and existing legacy `OpenAISchedulerGroupPolicy` JSON marshal/unmarshal.
- Produces: `OpenAISchedulerBusinessPriority`, `OpenAISchedulerOperations`, normalized `OpenAISchedulerBusinessGroupPolicy`, and a server-only compiler returning existing `OpenAISchedulerPolicyValues`/fairness inputs.

- [ ] **Step 1: Add typed business policy structures and JSON fields.**

  Extend the existing group policy view types with:

  ```go
  type OpenAISchedulerBusinessPriority struct {
      Profit  int `json:"profit"`
      TTFT    int `json:"ttft"`
      Latency int `json:"latency"`
  }

  type OpenAISchedulerOperations struct {
      Balance           string `json:"balance"`
      PeakProtection    string `json:"peak_protection"`
      SessionContinuity string `json:"session_continuity"`
  }
  ```

  Keep legacy fields and existing custom preset fields for backward-compatible reads.

- [ ] **Step 2: Implement strict normalization and defaults.**

  Add helpers with stable signatures:

  ```go
  func normalizeOpenAISchedulerBusinessPriority(priority OpenAISchedulerBusinessPriority) (OpenAISchedulerBusinessPriority, error)
  func normalizeOpenAISchedulerOperations(operations OpenAISchedulerOperations) (OpenAISchedulerOperations, error)
  func recommendedOpenAISchedulerBusinessPolicy(groupName string) OpenAISchedulerBusinessGroupPolicy
  ```

  Validate integer ranges and enum membership; do not coerce invalid writes. For missing new fields on a legacy policy, derive the documented recommendation from the existing preset ID/values, retain the legacy compiled values, and mark the policy as legacy-compatible until the next new-format save.

- [ ] **Step 3: Implement server-owned business-to-native compilation.**

  Add a helper such as:

  ```go
  func compileOpenAISchedulerBusinessPolicy(policy OpenAISchedulerBusinessGroupPolicy, base OpenAISchedulerPolicyValues) (OpenAISchedulerPolicyValues, error)
  ```

  The compiler must map priority tiers to the existing score factors (profit -> upstream cost, TTFT -> TTFT, latency -> load/queue/response completion factors) using finite, bounded internal values owned by the service. Equal tiers receive equal precedence. `balance`, `peak_protection`, and `session_continuity` alter existing fairness/top-k/sticky inputs only; they never disable hard gates. Keep the exact mapping in one helper so tests can lock it without frontend duplication.

- [ ] **Step 4: Normalize new group policies without trusting client compiled snapshots.**

  Extend the existing `normalizeOpenAISchedulerGroupPoliciesWithPresets` path to accept `business_priority`, normalize it, compile it server-side, and serialize the compiled snapshot from the normalized values. If a client sends `compiled_snapshot`, ignore it during validation and replacement.

- [ ] **Step 5: Run the focused tests and verify GREEN.**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service -run 'Test(NormalizeOpenAISchedulerBusiness|ParseOpenAISchedulerBusiness|OpenAISchedulerBusiness|OpenAIScheduler.*Priority|OpenAIScheduler.*Peak)' -count=1
  ```

  Expected: PASS, with legacy scheduler tests still passing.

- [ ] **Step 6: Commit the service contract.**

  ```bash
  git add upstream/sub2api/backend/internal/service/settings_view.go upstream/sub2api/backend/internal/service/setting_parse.go upstream/sub2api/backend/internal/service/domain_constants.go upstream/sub2api/backend/internal/service/scheduler_fairness_settings_test.go
  git commit -m "feat: compile T68 scheduler business policies"
  ```

### Task 3: Apply compiled policies in the native scheduler and handler contract

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go`
- Modify: relevant handler/service tests under `upstream/sub2api/backend/internal/service/*scheduler*test.go` and `upstream/sub2api/backend/internal/handler/admin/*setting*test.go`

**Interfaces:**
- Consumes: Task 2 normalized business policy and compiled native snapshot.
- Produces: Native selection behavior that preserves hard qualification/concurrency/sticky ordering and GET/PUT DTO behavior for T68.

- [ ] **Step 1: Write failing handler tests for GET/PUT business policy payloads.**

  Assert GET returns `priority`, `operations`, and a server-produced `compiled_snapshot`; PUT with a forged `compiled_snapshot` persists the policy but returns a snapshot generated by the server. Assert an unknown enum or priority `0/4` returns 400 and leaves the repository unchanged.

- [ ] **Step 2: Extend the settings update DTO and response projection.**

  Add the typed business policy map to the existing `SettingsUpdateRequest` and ensure the current settings response includes the normalized policy map. Preserve pointer-field omission semantics so old clients do not clear T68 fields.

- [ ] **Step 3: Write failing scheduler tests for operational interactions.**

  Cover: strict peak suppresses the exploration branch but not load-spreading; high balance inserts the oldest safe candidate; a full candidate is skipped by the existing acquire path; `switch` session continuity does not bypass previous-response output safety; equal priorities preserve all same-tier score factors.

- [ ] **Step 4: Apply the compiled policy after native qualification and before existing ranking.**

  In `buildOpenAIAccountLoadPlan`/`buildOpenAISelectionOrder`, resolve the group policy and apply only its compiled weights/fairness. Keep the existing sequence: account filtering -> load read -> scoring -> candidate ordering -> atomic acquire -> fresh account -> DB recheck. Do not add a second acquire path.

- [ ] **Step 5: Add peak and balance gates around the existing fairness exploration branch.**

  Use the operational enum to select existing top-k/hybrid/all-eligible behavior. Strict peak must skip pure exploration when the current candidate pool is capacity-tight; all modes still call the existing final acquire and fallback. High balance may add one oldest safe candidate, never an unbounded round-robin loop.

- [ ] **Step 6: Run handler and scheduler tests.**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service ./internal/handler/admin -run 'Test(OpenAIScheduler|NormalizeOpenAIScheduler|ParseOpenAIScheduler|Setting.*Scheduler|Admin.*Setting)' -count=1
  ```

  Expected: PASS.

- [ ] **Step 7: Build the server and commit.**

  ```bash
  go build ./cmd/server
  git add upstream/sub2api/backend/internal/service/openai_account_scheduler.go upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go upstream/sub2api/backend/internal/handler/admin/setting_handler_auth_source_defaults_test.go upstream/sub2api/backend/internal/service
  git commit -m "feat: enforce T68 scheduler policy guards"
  ```

### Task 4: Replace the SettingsView scheduler editor with numeric priorities and segmented controls

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- Modify: `upstream/sub2api/frontend/src/types/index.ts` only if shared DTO types are used by this view; otherwise keep view-local types with existing conventions
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts`

**Interfaces:**
- Consumes: Task 2/3 JSON fields `priority`, `operations`, and normalized `compiled_snapshot`.
- Produces: A group-first editor with per-metric `1/2/3` controls, three three-segment controls, draft isolation, reset-to-recommended, and preview text derived from the draft.

- [ ] **Step 1: Write failing Vue tests for the approved interaction.**

  Add tests that mount the existing settings view and assert:

  - GPT-特惠 defaults to `1/2/3`, Plus to `1/1/1`, Pro and 专属 Pro to `3/1/2` in the metric order profit/TTFT/latency.
  - Clicking a number updates only the selected group draft and the summary; equal numbers render one tier in the summary.
  - Each operational card renders exactly three labeled buttons and the selected state changes without numeric inputs.
  - Preview text changes for balance, peak, and session selections.
  - Switching groups preserves each in-memory draft; reset restores only the selected group's recommendation.
  - The outgoing payload contains business fields and does not allow a client-edited compiled snapshot to override server values.

- [ ] **Step 2: Run the focused Vue tests and verify RED.**

  ```bash
  cd upstream/sub2api/frontend
  pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts -t 'business priority|segmented|scheduler preview|recommended'
  ```

  Expected: FAIL because the current editor still renders legacy mode, weight, and fairness number inputs.

- [ ] **Step 3: Replace the scheduler draft model and computed summaries.**

  Add view-local types matching the handler contract, default drafts for the four named groups, `summaryForPriority`, `previewForDraft`, and group-scoped draft storage. Keep the existing settings form save pipeline and custom preset compatibility until the server contract is fully migrated.

- [ ] **Step 4: Replace the template region.**

  Render group selection, fixed service guard, metric rows with native buttons for 1/2/3, three segmented controls, live summary, three scenario preview blocks, reset recommendation, and the existing save action. Keep keyboard focus, `aria-pressed`, disabled/loading/error states, mobile single-column layout, and existing dark/light classes.

- [ ] **Step 5: Replace locale strings.**

  Add Chinese and matching English labels for the three metrics, all nine segment labels, hard guard text, preview scenario labels, reset action, and validation errors. Do not expose internal JSON keys, decimal weights, Top-K, exploration ratio, or starvation seconds in the primary editor.

- [ ] **Step 6: Run the focused Vue tests and verify GREEN.**

  ```bash
  pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts
  ```

  Expected: all existing scheduler tests plus the new interaction tests pass.

- [ ] **Step 7: Commit the frontend editor.**

  ```bash
  git add upstream/sub2api/frontend/src/views/admin/SettingsView.vue upstream/sub2api/frontend/src/types/index.ts upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts
  git commit -m "feat: add operator scheduler priority controls"
  ```

### Task 5: Run direct gates, self-review, and prepare handoff

**Files:**
- Modify: `docs/superpowers/specs/2026-08-25-t68-scheduler-policy-priority-design.md` only for implementation decisions discovered during execution
- Create: `docs/handoffs/2026-08-25-t68-scheduler-policy-priority-handoff.md`
- Create: `.superpowers/sdd/2026-08-25-t68-scheduler-policy-priority/task-report.md`

**Interfaces:**
- Consumes: Tasks 1-4 committed implementation and focused test evidence.
- Produces: A `READY_FOR_ROOT_REVIEW` handoff with exact commit/tree, changed files, tests, migration/config status, downtime result, rollback, and residual risks.

- [ ] **Step 1: Run direct backend gates.**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service ./internal/handler/admin -run 'Test(OpenAIScheduler|NormalizeOpenAIScheduler|ParseOpenAIScheduler|Setting.*Scheduler|Admin.*Setting)' -count=1
  go build ./cmd/server
  ```

- [ ] **Step 2: Run direct frontend gates.**

  ```bash
  cd upstream/sub2api/frontend
  pnpm vitest run src/views/admin/__tests__/SettingsView.spec.ts
  pnpm typecheck
  pnpm build
  ```

- [ ] **Step 3: Run scope and formatting checks.**

  ```bash
  git diff --check
  git diff --name-only main...HEAD
  git diff --name-only main...HEAD | rg '(^|/)(migrations|\.github/workflows)/' && exit 1 || true
  ```

  Expected: only approved backend service/handler tests, frontend settings/types/locales/tests, and T68 docs are changed; no migration or workflow paths appear.

- [ ] **Step 4: Perform self-review against the spec.**

  Verify every spec acceptance row has a test or explicit online verification step; verify no preview sentence promises a fixed success rate or guaranteed per-account interval; verify the server ignores client compiled snapshots; verify hard gates remain before every successful selection.

- [ ] **Step 5: Write the handoff and task report.**

  Include baseline `main@c70f11193`, final candidate commit/tree, changed files, focused test commands/results, unverified items, migration/config status, `downtime_required`, rollback slot, and the fact that no production data was written.

- [ ] **Step 6: Commit the handoff and report.**

  ```bash
  git add docs/superpowers/specs/2026-08-25-t68-scheduler-policy-priority-design.md docs/handoffs/2026-08-25-t68-scheduler-policy-priority-handoff.md .superpowers/sdd/2026-08-25-t68-scheduler-policy-priority/task-report.md
  git commit -m "docs: hand off T68 scheduler priority policy"
  ```

## Execution Notes

- Implement in this worktree only: `.worktrees/t68-scheduler-policy-priority`.
- Do not merge, push, deploy, modify root `main`, or change global queue/progress files from the candidate after the plan starts; root release control performs those actions.
- If a direct test reveals that the current runtime cannot express a business priority without changing hard qualification or adding a new fact source, stop and update the spec before expanding scope.
