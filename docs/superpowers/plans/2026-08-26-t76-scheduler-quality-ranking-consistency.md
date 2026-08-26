# Scheduler and Quality Ranking Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the account-monitoring page show group-scoped quality scores/ranks and the scheduler's real read-only candidate priority in one explainable full-width account card, while preserving each group's independent scheduler policy and the existing quality-score weights.

**Architecture:** The monitor service will keep quality scoring separate from scheduling policy. It will request group-scoped quality evidence and a read-only scheduler projection from the same current group snapshot, then return explicit quality and scheduler rank/explanation fields. The scheduler projection will reuse the production candidate eligibility, effective group policy, comparator, and deterministic tie-break path; the frontend will render those server-provided results and will not calculate a second ranking formula.

**Tech Stack:** Go 1.26, Gin, existing Sub2API service/repository interfaces, PostgreSQL queries through `database/sql`, Vue 3, TypeScript, Tailwind CSS, Vitest, Vue Test Utils, `go test`, `gofmt`, `pnpm typecheck`, and Vite production build.

**Spec:** `docs/superpowers/specs/2026-08-26-t76-scheduler-quality-ranking-consistency-design.md`

## Global Constraints

- Do not add a model dimension to the monitoring UI or to the returned ranking scope.
- Preserve each group's existing scheduler policy and effective weights; quality score is not a fourth scheduler weight.
- `scheduler_rank` must come from the scheduler's actual read-only candidate projection, not a duplicated monitor-side formula.
- A scheduler rank is a current-snapshot candidate order, not a promise that every request selects rank 1.
- Keep `group_rank`, `quality_score`, and `score_breakdown` during compatibility; `group_rank` remains an alias for group quality rank only.
- Group quality aggregates must be filtered by the current `group_id`; requests from another group must not affect a group's score or rank.
- Full-site view may show quality ranking only; it must not fabricate a cross-policy scheduler ranking.
- No database migration, historical backfill, billing change, account regrouping, or production business-data write.
- Do not add external screenshot controls or maintenance semantics; retain the existing native account actions and model-detection entry points.
- Work only in a fresh feature worktree created from the latest clean root `main`; do not modify the root release ledger or queue from the feature worktree.

## File Map

- Modify `upstream/sub2api/backend/internal/service/account_monitor_types.go` to define the JSON contract for quality rank, scheduler rank, explanation facts, and bounded reason codes.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_service.go` to build a single group snapshot, load group-scoped quality evidence, attach quality ranks, and merge scheduler projections without changing quality-score weights.
- Modify `upstream/sub2api/backend/internal/service/account_monitor_types.go` and the account-monitor repository interface implementation in `upstream/sub2api/backend/internal/repository/account_monitor_repo.go` for group-scoped window aggregate reads.
- Create `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go` for the scheduler-owned read-only projection boundary if the existing scheduler file cannot hold it without mixing request side effects.
- Modify `upstream/sub2api/backend/internal/service/openai_account_scheduler.go` only where needed to expose the production candidate-building/comparator path to the read-only projection; selection, sticky state, cooldown, usage, and slot acquisition behavior must remain unchanged.
- Modify `upstream/sub2api/backend/internal/service/wire.go` to inject the scheduler projection provider into `AccountMonitorService` without breaking existing test constructors.
- Modify `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go` and `upstream/sub2api/backend/internal/service/account_monitor_service_test.go` for API and service contract coverage.
- Modify `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts` for the new optional response fields and reason/explanation types.
- Modify `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue` to use scheduler order for a concrete group, quality order for the full-site view, and a single-column full-width card grid.
- Modify `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue` to implement the approved full-width row card, two-rank focus, compact metrics, trend, actions, and expandable explanations.
- Modify `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts` and `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts` for rendering, ordering, responsive-structure, and action-regression coverage.

## Task 1: Freeze the monitor response contract

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Test: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`

**Interfaces:**
- Produces `AccountMonitorQualityExplanation`, `AccountMonitorSchedulerExplanation`, and bounded reason-code values used by Tasks 2-5.
- Produces optional `QualityRank`, `QualityRankTotal`, `SchedulerRank`, and `SchedulerRankTotal` fields on `AccountMonitorAccount`; group rows use the same fields through `AccountMonitorGroupAccount` embedding.
- Preserves `GroupRank` as the compatibility alias for the group quality rank and does not use it for scheduler order.

- [ ] **Step 1: Write the failing Go contract test**

Add a service test that serializes a projected account and asserts the JSON contains `quality_rank`, `quality_rank_total`, `scheduler_rank`, `scheduler_rank_total`, `quality_explanation`, and `scheduler_explanation`, while the legacy `group_rank` field remains present.

```go
func TestAccountMonitorProjectionExposesExplainableQualityAndSchedulerRanks(t *testing.T) {
	qualityRank, schedulerRank, legacyRank := 2, 1, 2
	row := AccountMonitorAccount{
		AccountID: 7,
		QualityRank: &qualityRank, QualityRankTotal: 5,
		SchedulerRank: &schedulerRank, SchedulerRankTotal: 4,
		GroupRank: &legacyRank,
		QualityExplanation: &AccountMonitorQualityExplanation{Window: "24h", SampleCount: 12},
		SchedulerExplanation: &AccountMonitorSchedulerExplanation{PolicyLabel: "利润优先", PrimaryReasonCode: "strategy"},
	}
	body, err := json.Marshal(row)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	require.Equal(t, float64(2), payload["quality_rank"])
	require.Equal(t, float64(5), payload["quality_rank_total"])
	require.Equal(t, float64(1), payload["scheduler_rank"])
	require.Equal(t, float64(4), payload["scheduler_rank_total"])
	require.Equal(t, float64(2), payload["group_rank"])
	require.NotNil(t, payload["quality_explanation"])
	require.NotNil(t, payload["scheduler_explanation"])
}
```

- [ ] **Step 2: Run the focused test and verify it fails for missing fields**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run TestAccountMonitorProjectionExposesExplainableQualityAndSchedulerRanks -count=1`

Expected: FAIL because the new rank and explanation fields do not exist yet.

- [ ] **Step 3: Add the typed contract and frontend mirrors**

Define string constants or a typed alias for `strategy`, `quality_gate`, `runtime_load`, `cooldown`, `tie_break`, `data_freshness`, and `not_eligible`. Add JSON fields with `omitempty` for global rows that do not have group scheduler context. Put readable labels/facts in the response contract; keep raw policy keys and database names out of the default UI.

- [ ] **Step 4: Run the focused backend and frontend type checks**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run TestAccountMonitorProjectionExposesExplainableQualityAndSchedulerRanks -count=1`

Run: `cd upstream/sub2api/frontend && pnpm typecheck`

Expected: the new contract test passes and TypeScript accepts the optional fields.

- [ ] **Step 5: Commit the contract change**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/frontend/src/api/admin/accountMonitor.ts upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git commit -m "feat: add explainable account ranking contract"
```

## Task 2: Extract a side-effect-free scheduler candidate projection

**Files:**
- Create: `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go`
- Modify: `upstream/sub2api/backend/internal/service/openai_account_scheduler.go`
- Test: `upstream/sub2api/backend/internal/service/openai_account_scheduler_projection_test.go`

**Interfaces:**
- Consumes the existing OpenAI account candidates, group policy resolution, effective scheduler weights, load snapshot, cooldown/eligibility checks, and `isOpenAIAccountCandidateBetter` tie-break semantics.
- Produces `OpenAIAccountSchedulerProjection` with `SnapshotAt`, `PolicyKey`, `PolicyLabel`, `EffectiveWeights`, `CandidateCount`, and ordered `OpenAIAccountSchedulerProjectionCandidate` values containing account ID, rank, eligibility, and primary reason code.
- Does not acquire/release concurrency slots and does not write sticky sessions, cooldowns, usage, response bindings, or audit records.

- [ ] **Step 1: Write failing projection tests**

Cover the production behaviors required by the spec: a profit-priority group can rank a less performant account first without changing the quality score; ineligible/cooldown accounts have no normal scheduler rank; equal scores use the existing stable comparator; and the projection does not invoke slot acquisition or mutate selection state.

The first test should create two concrete `Account` fixtures, configure one group with a profit-priority policy, call `Project` with a fixed UTC timestamp, and assert that the returned candidate IDs follow the production comparator and that the first candidate carries `strategy` when its order differs from quality order. The second test should snapshot runtime state before and after `Project` and assert no slot, sticky, cooldown, usage, or audit state changed.

- [ ] **Step 2: Run the projection tests and verify the intended missing API failure**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestOpenAIAccountSchedulerProjection' -count=1`

Expected: FAIL because the read-only projection API and projection-owned candidate result do not exist.

- [ ] **Step 3: Implement the minimal projection boundary by reusing scheduler-owned logic**

Move or factor only pure candidate ordering and explanation code. The projection must resolve the same group policy and effective weights used by request selection, build the deterministic pre-Top-K candidate order, and annotate why a row is not ranked. Keep Top-K random selection, session sticky, forced-account retry, and half-open lease acquisition in the request path; expose their static facts only as explanation data when already available.

- [ ] **Step 4: Run scheduler regression and projection tests**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestOpenAIAccountSchedulerProjection|TestOpenAIGatewayService_SelectAccountWithScheduler|TestAdvanced' -count=1`

Expected: PASS with the existing scheduler tests still exercising the request-selection path.

- [ ] **Step 5: Commit the scheduler projection**

```bash
git add upstream/sub2api/backend/internal/service/openai_account_scheduler.go upstream/sub2api/backend/internal/service/openai_account_scheduler_projection.go upstream/sub2api/backend/internal/service/openai_account_scheduler_projection_test.go
git commit -m "feat: expose read-only scheduler candidate projection"
```

## Task 3: Make quality evidence and ranking group-scoped

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go` or the existing account-monitor repository test file
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Consumes the Task 1 response types and the Task 2 scheduler projection provider.
- Produces group-specific quality score/rank fields and scheduler projection fields on each `AccountMonitorGroupAccount`.
- Keeps full-site rows on the existing global quality ranking path and leaves scheduler fields empty outside a concrete group.

- [ ] **Step 1: Write failing group-isolation and ranking tests**

Add a repository test that verifies the group window query includes the requested `group_id`, and a service test with the same account represented in two groups whose evidence and policies differ. Assert each group gets its own score/rank/scheduler rank, the account's other-group requests do not affect the active group, and an account without valid score evidence has no quality rank.

The service test should construct two group rows for the same account, provide different group-scoped aggregate fixtures and group policies, call `ListWindow(context.Background(), "24h")`, then assert the two `AccountMonitorGroupAccount` values have different quality evidence/rank and scheduler rank. A third account with no valid evidence must have both a nil quality rank and no fabricated score.

- [ ] **Step 2: Run the focused tests and confirm the pre-change failure**

Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/repository -run 'TestAccountMonitor(ListWindowUsesGroupScopedQualityEvidence|.*Group.*)' -count=1`

Expected: FAIL because group rows currently reuse account-keyed window aggregates and do not expose scheduler projection fields.

- [ ] **Step 3: Add group-scoped aggregate loading and one snapshot timestamp**

Add the smallest repository method needed to load request evidence by `(group_id, account_id)` for the selected window. In `ListWindow`, capture one UTC `observedAt`, load groups/settings/policies and group members for that snapshot, calculate quality evidence and ranks per group, and pass the same snapshot time to the read-only scheduler projection. Preserve historical fallback behavior and existing score weights.

- [ ] **Step 4: Attach quality and scheduler explanations without frontend inference**

Populate four component scores with their existing maxima, window, sample count, source, observed time, rank total, policy label, effective scheduler weights, candidate total, eligibility, snapshot time, tie-break label, and one server-selected reason code. Use `not_eligible` or a null rank for excluded accounts; never convert missing evidence into score `0` or a fabricated rank.

- [ ] **Step 5: Run focused service, repository, and handler tests**

Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/repository ./internal/handler/admin -run 'AccountMonitor|OpenAIAccountSchedulerProjection' -count=1`

Expected: PASS, including existing monitor fallback, group recommendation, score-weight, and handler compatibility tests.

- [ ] **Step 6: Commit group-scoped monitor projection**

```bash
git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/service/wire.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go
git commit -m "feat: project group-scoped quality and scheduler ranks"
```

## Task 4: Lock the HTTP contract and ordering semantics

**Files:**
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go` only if response wrapping or error mapping requires it
- Modify: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`

**Interfaces:**
- Consumes the service projection from Task 3.
- Produces an admin monitor response where concrete group rows have quality and scheduler rank/explanation fields, full-site rows do not have cross-policy scheduler rank, and projection failure is an explicit unavailable state rather than a frontend fallback sort.

- [ ] **Step 1: Write failing handler contract tests**

Assert that a group response returns `quality_rank`, `quality_rank_total`, `scheduler_rank`, `scheduler_rank_total`, readable strategy facts, and a bounded reason code; assert full-site response rows do not receive a fabricated scheduler rank; assert a projection error maps to an unavailable response without changing real scheduler behavior.

- [ ] **Step 2: Run the handler tests and verify the missing fields/failure behavior**

Run: `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'TestAccountMonitorHandler.*(Rank|Projection|Group)' -count=1`

Expected: FAIL until the service response and error mapping are wired.

- [ ] **Step 3: Implement response compatibility and explicit unavailable semantics**

Keep the existing endpoint, authentication, and response envelope. Add no new endpoint. Ensure projection errors are represented by the existing monitor error shape or an explicit scheduler-unavailable field accepted by the frontend; do not sort with a copied formula in the handler.

- [ ] **Step 4: Run all direct handler tests**

Run: `cd upstream/sub2api/backend && go test ./internal/handler/admin -run 'AccountMonitor' -count=1`

Expected: PASS with legacy fields and existing score-weight/concurrency/model-detection endpoints unchanged.

- [ ] **Step 5: Commit the HTTP contract**

```bash
git add upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go upstream/sub2api/frontend/src/api/admin/accountMonitor.ts
git commit -m "test: lock account monitor ranking response contract"
```

## Task 5: Rebuild the frontend as full-width explainable account rows

**Files:**
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

**Interfaces:**
- Consumes the Task 4 TypeScript contract.
- Produces a single-column account list where each card fills the available content width; concrete groups sort by `scheduler_rank`, the full-site view sorts by quality rank, and the card exposes compact server-provided explanations on demand.

- [ ] **Step 1: Write failing component and view tests**

Add tests that assert the card shows score, quality rank, scheduler priority, active policy label, and one mismatch reason; clicking the score/scheduler control reveals the four score components and scheduler facts with `aria-expanded`/`aria-controls`; the view uses one full-width grid and group order follows scheduler rank while full-site order follows quality rank; existing account info/edit/delete/more/cost/model-detection events remain wired.

```ts
it('renders both rankings and expands readable explanations', async () => {
  const wrapper = mountCard({ account: { ...account, quality_rank: 3, quality_rank_total: 12, scheduler_rank: 1, scheduler_rank_total: 9, scheduler_explanation: { policy_label: '利润优先', primary_reason_code: 'strategy', primary_reason_label: '当前分组策略与质量排序目标不同' } } })
  expect(wrapper.get('[data-test="quality-rank"]').text()).toContain('第 3 / 12')
  expect(wrapper.get('[data-test="scheduler-rank"]').text()).toContain('第 1 / 9')
  await wrapper.get('[data-test="ranking-explanation-toggle"]').trigger('click')
  expect(wrapper.get('[data-test="ranking-explanation"]').text()).toContain('利润优先')
  expect(wrapper.get('[data-test="ranking-explanation-toggle"]').attributes('aria-expanded')).toBe('true')
})
```

- [ ] **Step 2: Run the focused frontend tests and confirm the pre-change failure**

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`

Expected: FAIL because the current card uses `group_rank`, the current view uses a two-column grid, and the new explanation controls do not exist.

- [ ] **Step 3: Implement the full-width card structure**

Remove the thick `border-l-4` status treatment. Build stable desktop columns for identity/status, quality, scheduler priority, key metrics, trend, and actions using CSS Grid with `minmax` tracks. Keep the account maintenance controls, refresh state, concurrency state, trend data, cost entry, model detection, and existing native dialogs. Use icons and existing tooltip patterns for unfamiliar controls.

- [ ] **Step 4: Implement compact expandable explanations and mobile layout**

Use a semantic button with `aria-expanded` and `aria-controls`. Keep the default card focused on the two rankings and one short reason; render score components, sample/window/source time, policy/effective weights, eligibility, candidate count, snapshot, and tie-break only when expanded. Below the mobile breakpoint switch to stacked labeled rows, allow long names/reasons to wrap, and prevent page-level horizontal overflow.

- [ ] **Step 5: Update view sorting and grid semantics**

Use `scheduler_rank` only when `activeGroup` is selected and a rank exists; place unranked/ineligible rows after ranked rows using stable account ID order. Use `quality_rank`/legacy `group_rank` for full-site quality ordering. Change the grid to one column and keep the active group summary and filters unchanged.

- [ ] **Step 6: Run focused frontend tests, typecheck, and build**

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`

Run: `cd upstream/sub2api/frontend && pnpm typecheck && pnpm build`

Expected: PASS with no TypeScript errors and a successful production bundle.

- [ ] **Step 7: Commit the account-card redesign**

```bash
git add upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts
git commit -m "feat: redesign account monitor as explainable full-width rows"
```

## Task 6: Run the direct validation gate and prepare handoff

**Files:**
- Verify: all files changed by Tasks 1-5
- Do not modify: root release ledger, queue, production records, or unrelated worktrees

- [ ] **Step 1: Run backend formatting and direct tests**

Run: `cd upstream/sub2api/backend && gofmt -w internal/service/account_monitor_types.go internal/service/account_monitor_service.go internal/service/openai_account_scheduler.go internal/service/openai_account_scheduler_projection.go internal/repository/account_monitor_repo.go internal/service/wire.go internal/handler/admin/account_monitor_handler.go`

Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/repository ./internal/handler/admin -run 'AccountMonitor|OpenAIAccountSchedulerProjection' -count=1`

Run: `cd upstream/sub2api/backend && go build ./cmd/server`

Expected: PASS with no formatting changes left after the command.

- [ ] **Step 2: Run frontend formatting checks and direct tests**

Run: `cd upstream/sub2api/frontend && pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts`

Run: `cd upstream/sub2api/frontend && pnpm typecheck && pnpm build`

Expected: PASS with the full-width card tests, view ordering tests, typecheck, and production build all green.

- [ ] **Step 3: Run repository hygiene checks**

Run: `git diff --check`

Run: `git status --short`

Expected: no whitespace errors; only the feature worktree's intended commits/files are present.

- [ ] **Step 4: Record implementation evidence for the root release controller**

Record the commit IDs, focused test commands and outputs, build/typecheck results, no-migration confirmation, and any projection-unavailable behavior in the task handoff document. Do not mark T76 `DONE` from this worktree; completion remains gated by root integration, push, deployment, and online verification.

- [ ] **Step 5: Request targeted review or handoff**

Use the approved project workflow to hand the feature worktree to the root release controller. Preserve the worktree if merge, build, deployment, or online verification fails; do not delete or reset it as a shortcut.

## Self-Review Checklist

- [x] The plan has a task for every spec section: group-scoped score, group-owned scheduler projection, reasons, compatible API fields, full-width card, mobile layout, regression tests, and release gate.
- [x] No task adds a model ranking dimension or turns quality score into scheduler weight.
- [x] The frontend never infers scheduler order from score or rank differences.
- [x] Profit-priority and other per-group policies remain authoritative in scheduler projection tests.
- [x] The plan contains no placeholder implementation step; every task names files, interfaces, tests, commands, and expected outcomes.
- [x] Type names and JSON names are consistent between Go and TypeScript: `quality_rank`, `quality_rank_total`, `scheduler_rank`, `scheduler_rank_total`, `quality_explanation`, and `scheduler_explanation`.
