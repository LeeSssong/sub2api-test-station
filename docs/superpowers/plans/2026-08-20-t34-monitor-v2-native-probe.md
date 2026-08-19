# T34 Monitor V2 Native Probe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (or superpowers:subagent-driven-development) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace Monitor V2's mixed Channel Monitor/`usage_logs` path with a native `AccountMonitorService` projection backed only by `account_monitor_results`, and ship the v7 API and UI semantics approved in the T34 specification.

**Architecture:** `AccountMonitorService` remains the sole owner of current account eligibility, account-to-group expansion, monitor settings, and freshness. A narrow optional repository capability performs one batched SQL read for all `(group_id, account_id)` scopes and returns fixed-bucket status plus native TTFT P50/average latency. `MonitorV2Service` consumes that projection and only handles visible group metadata, v7 serialization, and rendering inputs; the frontend renders backend values without recomputing availability.

**Tech Stack:** Go, PostgreSQL SQL, `sqlmock`, Gin handler tests, Vue 3, TypeScript, Vue Test Utils, Vitest, `vue-tsc`, Vite.

**Spec:** `docs/superpowers/specs/2026-08-20-t34-monitor-v2-native-probe-design.md`

## Global Constraints

- Native source is only `account_monitor_results`; the Monitor V2 request path contains no `usage_logs` query and no `ChannelMonitorService` dependency.
- Current eligibility is the snapshot-time `Account.IsSchedulable()` set; historical results are evaluated only for that current set.
- Fixed buckets are `24h=24 x 1h`, `7d=28 x 6h`, `30d=30 x 24h`; a bucket without a successful native result is `unavailable`.
- Current status is operational when any currently schedulable account's latest native result is `success` and within `2 * interval_seconds`; otherwise it is unavailable.
- TTFT is native successful-sample P50; average latency is native successful-sample `AVG(latency_ms)`; no v6 five-sample gate.
- API contract is v7 and removes `is_flagship`, TPS, both P95 fields, and the old latency field; no compatibility shell is retained.
- Physical native-result retention is 30 days while existing score/management aggregation remains 7 days; no historical backfill.
- Keep the existing Monitor V2 page width (`max-w-[1500px]`) and responsive shell; do not modify scheduling, scoring, billing, procurement, CodexRadar, or other pages.
- Before implementation starts, obtain root approval for this plan. During implementation do not modify global queue/progress ledgers, main, release evidence, production state, or GitHub Actions.

---

## File Map

- `upstream/sub2api/backend/internal/service/account_monitor_types.go`: native Monitor V2 projection types and optional repository capability.
- `upstream/sub2api/backend/internal/service/account_monitor_service.go`: snapshot-time eligibility and group-scope projection method.
- `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`: RED tests for eligibility, scope expansion, freshness forwarding, and error propagation.
- `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`: one batched `account_monitor_results` query and 30-day cleanup boundary.
- `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`: SQL shape, bucket, latest, aggregate, and retention tests.
- `upstream/sub2api/backend/internal/service/monitor_v2.go`: v7 contract model and native projection mapping.
- `upstream/sub2api/backend/internal/service/monitor_v2_test.go`: RED tests for fixed buckets, v7 metrics, visibility, and native-reader errors.
- `upstream/sub2api/backend/internal/service/wire.go`: provider signature accepting `MonitorV2NativeProbeReader`.
- `upstream/sub2api/backend/cmd/server/wire_gen.go`: minimal construction-order/provider update.
- `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`: v7 JSON contract and no-account-data assertions.
- `upstream/sub2api/backend/internal/repository/monitor_v2_repo.go`: delete obsolete `usage_logs` repository.
- `upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go`: delete obsolete `usage_logs` tests.
- `upstream/sub2api/frontend/src/features/monitor-v2/types.ts`: v7 TypeScript types.
- `upstream/sub2api/frontend/src/features/monitor-v2/api.ts`: v7 runtime validator and removed-field rejection.
- `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue`: backend availability/metrics, multiplier placement, and direct labels.
- `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue`: fixed native bucket rendering and labels if required by the new tests.
- `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`: Chinese v7 labels and removal of flagship/P95/TPS strings.
- `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`: English counterparts for the same contract.
- `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`: v7 card and copy tests.
- `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`: v7 route fixture.
- `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`: 24/28/30-point timeline and status tests.
- `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Api.spec.ts`: v7 validator tests (new file).

## Implementation Tasks

### Task 1: Define the native projection boundary and lock service eligibility with RED tests

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`

**Interfaces:**
- Add `MonitorV2NativeGroupProjection` with `Status`, `OperationalBucketCount`, `TotalBucketCount`, `TTFTP50MS`, `AverageLatencyMS`, `TTFTSampleCount`, `LatencySampleCount`, and ordered native timeline points.
- Add `MonitorV2NativeProbeReader` with `ProjectMonitorV2Groups(ctx context.Context, groupIDs []int64, start, end time.Time, bucketSize time.Duration) (map[int64]MonitorV2NativeGroupProjection, error)`.
- Add `MonitorV2GroupAccountScope { GroupID int64; AccountID int64 }` and an optional `AccountMonitorGroupProbeRepository` method with the exact signature `ProjectMonitorV2Groups(ctx context.Context, scopes []MonitorV2GroupAccountScope, start, end, freshSince time.Time, bucketSize time.Duration) (map[int64]MonitorV2NativeGroupProjection, error)`.
- Implement `(*AccountMonitorService).ProjectMonitorV2Groups` so it loads all accounts, filters through `Account.IsSchedulable()`, expands only requested groups, loads monitor settings, and propagates repository errors.

- [ ] **Step 1: Write RED service tests.** Add table-driven cases covering disabled/manual-unschedulable/expired/cooldown/rate-limit/overload/quota-exhausted accounts, one account in two groups, duplicate group relationships, no eligible accounts, default settings, and repository error propagation. Assert the repository stub receives only valid scopes and the expected freshness interval.

```go
func TestAccountMonitorProjectMonitorV2GroupsUsesSchedulableScopes(t *testing.T) {
    // Arrange accounts with every IsSchedulable() exclusion plus one account in two groups.
    // Act: svc.ProjectMonitorV2Groups(ctx, []int64{7, 8}, start, end, 6*time.Hour).
    // Assert: only unique eligible (group_id, account_id) pairs reach the native repo.
}
```

- [ ] **Step 2: Run the focused test and confirm failure.**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountMonitorProjectMonitorV2GroupsUsesSchedulableScopes|TestAccountMonitorProjectMonitorV2Groups' -count=1`

Expected: FAIL because the projection interface/method and projection-aware repository stub are not implemented.

- [ ] **Step 3: Implement the narrow service boundary.** Add the projection structs and optional repository interface. In `ProjectMonitorV2Groups`, deduplicate account IDs and group scopes, call `IsSchedulable()`, load settings using the existing `loadSettings`, calculate `freshSince := end.Add(-2 * time.Duration(settings.IntervalSeconds) * time.Second)`, and require the optional repository capability instead of falling back to `ListAggregates`, `ListLatest`, or any other source.

- [ ] **Step 4: Run the focused service tests to green.**

Run: `cd upstream/sub2api/backend && go test ./internal/service -run 'TestAccountMonitorProjectMonitorV2Groups' -count=1`

Expected: PASS, including empty-scope and repository-error cases.

- [ ] **Step 5: Commit the boundary slice.**

Run: `git add upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/service/account_monitor_service_test.go && git commit -m "feat: expose native monitor v2 projection boundary"`

### Task 2: Add the single batched native SQL query and decouple 30-day retention

**Files:**
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go` to expose the exact scope and projection types to the repository package.
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go` to call the concrete optional repository capability and preserve error propagation.

**Interfaces:**
- Implement the optional repository capability from Task 1 on `accountMonitorRepository`.
- The SQL receives the deduplicated `[]MonitorV2GroupAccountScope`, expands it into a `scope(group_id, account_id)` CTE, generates the exact fixed bucket series, selects latest result per eligible account, and returns all group rows/timeline points in one `QueryContext` call.

- [ ] **Step 1: Write RED sqlmock tests.** Add tests asserting one query contains `account_monitor_results`, `generate_series`/equivalent fixed buckets, `[start,end)` predicates, latest-per-account ordering by `checked_at DESC, id DESC`, `status = 'success'`, `percentile_cont(0.50)` for TTFT, `AVG(latency_ms)`, bucket-level operational aggregation, and the freshness cutoff. Add tests that fail if `usage_logs` appears or if a second query is expected. Add empty scope, duplicate scope, invalid window/bucket, and multi-group scan fixtures.

```go
func TestAccountMonitorRepositoryProjectMonitorV2GroupsUsesOneNativeQuery(t *testing.T) {
    // Expect exactly one QueryContext over account_monitor_results.
    // Return rows for two groups and fixed buckets, then assert all projections.
}
```

- [ ] **Step 2: Run repository RED tests.**

Run: `cd upstream/sub2api/backend && go test ./internal/repository -run 'TestAccountMonitorRepositoryProjectMonitorV2Groups' -count=1`

Expected: FAIL because the repository method and query do not exist.

- [ ] **Step 3: Implement the single query.** Use a scope CTE, a bucket CTE for the caller-supplied bucket width, a latest-per-account CTE for current status, and aggregate CTEs for successful TTFT/latency and per-bucket status/latency. Return zero-result buckets through a left join. Scan rows into a map keyed by group ID, sort timeline points ascending, round millisecond values in Go, and reject malformed input before querying. Do not import or call the old Monitor V2 repository.

- [ ] **Step 4: Make retention explicit without changing 7-day aggregates.** Add `AccountMonitorResultRetentionDays = 30` beside the existing `AccountMonitorHistoryDays = 7`; change only the two `DeleteBefore` call sites to use the new retention constant. Leave all `ListAggregates`/score lookbacks at seven days. Add a sqlmock assertion for the 30-day cutoff argument at the service cleanup boundary.

- [ ] **Step 5: Run native SQL and retention tests to green.**

Run: `cd upstream/sub2api/backend && go test ./internal/repository ./internal/service -run 'TestAccountMonitorRepositoryProjectMonitorV2Groups|Test.*Retention|Test.*Cleanup' -count=1`

Expected: PASS; `mock.ExpectationsWereMet()` confirms one native query and the 30-day delete cutoff.

- [ ] **Step 6: Commit the repository slice.**

Run: `git add upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/service/account_monitor_service.go && git commit -m "feat: aggregate monitor v2 from native probe results"`

### Task 3: Replace Monitor V2 service semantics with the native v7 projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`

**Interfaces:**
- Change `MonitorV2ContractVersion` to `"7"`.
- Change `MonitorV2Service` to depend on `MonitorV2NativeProbeReader` and `MonitorV2SettingsReader`; remove `MonitorV2ProbeReader`, `MonitorV2Repository`, performance scopes/stats, primary-model logic, and Channel Monitor timeline helpers.
- Define v7 `MonitorV2Group` fields as group metadata, `Status`, `Availability`, `TTFT`, `AverageLatency`, and fixed native timeline; retain only the two metric states `available` and `insufficient_data`.

- [ ] **Step 1: Write RED service/handler tests.** Replace old performance/probe fixtures with a native reader stub. Cover public/admin exclusive filtering, visible groups with no eligible accounts, 24/28/30 fixed timeline lengths, `operational_bucket_count` percentage rounding, native TTFT/average latency mapping, no-sample placeholders, stable group order without flagship sorting, and native-reader error propagation. Update handler assertions to require contract `7`, `availability`, `ttft`, `average_latency`, and absence of deleted fields.

- [ ] **Step 2: Run the focused RED tests.**

Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler -run 'TestMonitorV2|Test.*MonitorV2' -count=1`

Expected: FAIL because existing tests and implementation still use v6, usage-log stats, and Channel Monitor probes.

- [ ] **Step 3: Implement the v7 service projection.** Compute window bounds/bucket widths in one helper, call the native reader once per snapshot, map native projection fields, round availability to `0..100`, preserve visible group metadata and current page width assumptions, and return native-reader errors instead of ignoring them. Keep system refresh-setting fallback behavior only for refresh interval.

- [ ] **Step 4: Run backend Monitor V2 tests to green.**

Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler ./internal/server/routes -run 'TestMonitorV2|Test.*MonitorV2' -count=1`

Expected: PASS with no account-level fields in serialized JSON.

- [ ] **Step 5: Commit the service/API slice.**

Run: `git add upstream/sub2api/backend/internal/service/monitor_v2.go upstream/sub2api/backend/internal/service/monitor_v2_test.go upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go && git commit -m "feat: project monitor v2 from native probe data"`

### Task 4: Update providers and remove the obsolete usage-log Monitor V2 repository

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go`
- Delete: `upstream/sub2api/backend/internal/repository/monitor_v2_repo.go`
- Delete: `upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go`

**Interfaces:**
- `ProvideMonitorV2Service(groupRepo GroupRepository, nativeProbeReader MonitorV2NativeProbeReader, settingService *SettingService) *MonitorV2Service`.
- Construct Account Monitor before Monitor V2 in `wire_gen.go`, then pass `accountMonitorService` as the native reader. Keep `channelMonitorService` available for the legacy Channel Monitor handlers/runner.

- [ ] **Step 1: Write a RED provider/build check.** Update the provider-level compile test or add a focused test that constructs `ProvideMonitorV2Service` with an `AccountMonitorService` native reader and confirms the resulting service is non-nil. Add a repository grep assertion in the task review command that Monitor V2 source no longer references `usage_logs`.

- [ ] **Step 2: Run the provider RED check.**

Run: `cd upstream/sub2api/backend && go test ./cmd/server ./internal/service -run 'TestProvideMonitorV2Service|TestMonitorV2' -count=1`

Expected: FAIL until provider signatures and generated construction order are changed.

- [ ] **Step 3: Implement the minimal wire change.** Update `ProvideMonitorV2Service`, move the existing Account Monitor construction block ahead of Monitor V2 in `wire_gen.go`, pass `accountMonitorService`, and remove only `monitorV2Repository` construction. Do not remove the legacy `channelMonitorService` construction used elsewhere.

- [ ] **Step 4: Delete obsolete repository code and run build checks.** Remove both old Monitor V2 repository files, run `gofmt` on touched Go files, and verify no production Monitor V2 path references `usage_logs` or `ChannelMonitorService`.

Run: `! rg -n "usage_logs|MonitorV2Repository|monitorV2Repository|ProvideMonitorV2Service\([^\n]*channelMonitorService" upstream/sub2api/backend/internal/service/monitor_v2.go upstream/sub2api/backend/cmd/server/wire_gen.go && test ! -e upstream/sub2api/backend/internal/repository/monitor_v2_repo.go && test ! -e upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go` (Expected: exit 0; legacy Channel Monitor references outside the Monitor V2 provider remain valid.)

Run: `cd upstream/sub2api/backend && go test ./cmd/server ./internal/service ./internal/handler ./internal/server/routes -run 'TestMonitorV2|TestProvideMonitorV2Service' -count=1 && go build ./cmd/server`

Expected: PASS and a successful server build.

- [ ] **Step 5: Commit the provider slice.**

Run: `git add upstream/sub2api/backend/internal/service/wire.go upstream/sub2api/backend/cmd/server/wire_gen.go upstream/sub2api/backend/cmd/server/wire_gen_test.go upstream/sub2api/backend/internal/repository/monitor_v2_repo.go upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go && git commit -m "refactor: wire monitor v2 to account monitor service"`

### Task 5: Lock the v7 frontend contract and localized labels with RED tests

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/types.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/api.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`
- Create or modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Api.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`

**Interfaces:**
- Set `MONITOR_V2_CONTRACT_VERSION = '7'`.
- Define `MonitorV2Group` with `availability`, `ttft`, `average_latency`, and native timeline; remove `is_flagship`, `ttft_p95`, `tps`, `latency_p95`, and old `latency`.
- Validator rejects v6 and every deleted field, accepts only `available | insufficient_data`, and enforces fixed timeline lengths at the view boundary where the selected window is known.

- [ ] **Step 1: Write RED Vitest fixtures/assertions.** Change fixtures to v7 and add validator cases for v6 rejection, deleted-field rejection, `average_latency` acceptance, missing required native fields, null insufficient metrics, and fixed 24/28/30 timeline lengths. Update locale test stubs to include exact Chinese labels `可用性：` / `首字速度：` / `平均耗时：`.

- [ ] **Step 2: Run frontend RED tests.**

Run: `cd upstream/sub2api/frontend && npm run test:run -- src/features/monitor-v2/__tests__/MonitorV2Api.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`

Expected: FAIL because TypeScript types, validator, fixtures, and locale keys still target v6.

- [ ] **Step 3: Implement v7 types and validation.** Change the contract constant and interfaces, parse `average_latency`, remove old fields and flagship parsing, and retain strict numeric/date/bounds validation. Make the validator reject deleted keys before constructing the normalized object.

- [ ] **Step 4: Update zh/en locale contracts.** Add direct metric labels and remove obsolete flagship, availability-derived, P95, TPS, and “call samples” keys that no longer have consumers. Keep status/window/peak/timeline keys used by the existing shell.

- [ ] **Step 5: Run contract tests to green.**

Run: `cd upstream/sub2api/frontend && npm run test:run -- src/features/monitor-v2/__tests__/MonitorV2Api.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`

Expected: PASS with v7-only normalized snapshots.

- [ ] **Step 6: Commit the contract slice.**

Run: `git add upstream/sub2api/frontend/src/features/monitor-v2/types.ts upstream/sub2api/frontend/src/features/monitor-v2/api.ts upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Api.spec.ts upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts && git commit -m "feat: adopt monitor v2 native probe contract"`

### Task 6: Render backend metrics directly and verify desktop/390px layouts

**Files:**
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue` only where fixed bucket/label tests require it.
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`

**Interfaces:**
- Card receives the v7 `MonitorV2Group`; it must never derive availability from `timeline`.
- `MonitorV2Timeline` receives fixed backend points and renders all points in order; it does not aggregate or fill them.

- [ ] **Step 1: Write RED component tests.** Assert exact Chinese output for availability/TTFT/average latency, multiplier immediately adjacent to group name, current status visibility, no “旗舰”/TPS/P95 text, no timeline availability arithmetic, and 24/28/30 points rendered without layout growth. Include an insufficient-data fixture showing `—`.

- [ ] **Step 2: Run component RED tests.**

Run: `cd upstream/sub2api/frontend && npm run test:run -- src/features/monitor-v2/__tests__/MonitorV2View.spec.ts src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts`

Expected: FAIL because the card still computes percentage availability and renders old metric keys/flagship.

- [ ] **Step 3: Implement the minimal card projection.** Replace the computed timeline percentage with `metricValue(group.availability, formatAvailability)`, render direct `可用性：`/`首字速度：`/`平均耗时：` labels, place the multiplier beside the group heading, render current status independently, and format milliseconds as seconds with at most two decimals and no trailing zeros. Keep `max-w-[1500px]` and existing responsive grid.

- [ ] **Step 4: Keep timeline rendering fixed and accessible.** Render all supplied points, retain status/latency tooltip semantics, keep the existing timeline label semantics, and do not add browser-side bucket math.

- [ ] **Step 5: Run component tests and typecheck.**

Run: `cd upstream/sub2api/frontend && npm run test:run -- src/features/monitor-v2/__tests__/MonitorV2View.spec.ts src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts && npm run typecheck`

Expected: PASS with no TypeScript errors.

- [ ] **Step 6: Perform required visual checks.** Start the existing Vite dev server on an available port, use Playwright/browser automation to capture the Monitor V2 page at a desktop viewport and a 390px viewport, and inspect that the page width is unchanged, multiplier/name/status do not overlap, labels fit, and timeline remains bounded. Record screenshot paths in the implementation handoff; do not modify production evidence.

- [ ] **Step 7: Commit the rendering slice.**

Run: `git add upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2Timeline.spec.ts && git commit -m "feat: render native monitor v2 metrics"`

### Task 7: Run the bounded T34 verification set and prepare handoff

**Files:**
- Modify only files already listed above if a directly related test, format, or generated-wire correction is required.
- Create: `docs/superpowers/reports/2026-08-20-t34-native-probe-verification.md`

- [ ] **Step 1: Run focused backend verification.**

Run: `cd upstream/sub2api/backend && gofmt -l internal/service/account_monitor_types.go internal/service/account_monitor_service.go internal/service/monitor_v2.go internal/service/wire.go internal/repository/account_monitor_repo.go cmd/server/wire_gen.go && go test ./internal/service ./internal/repository ./internal/handler ./internal/server/routes -run 'Test(AccountMonitor|MonitorV2|ProvideMonitorV2)' -count=1 && go build ./cmd/server`

Expected: no `gofmt -l` output, all selected tests pass, and the server build succeeds.

- [ ] **Step 2: Run focused frontend verification.**

Run: `cd upstream/sub2api/frontend && npm run test:run -- src/features/monitor-v2 && npm run typecheck && npm run build`

Expected: all Monitor V2 Vitest suites pass, `vue-tsc` passes, and Vite build succeeds.

- [ ] **Step 3: Run source and diff guards.**

Run: `! rg -n "usage_logs|MonitorV2Repository|monitorV2Repository|ProvideMonitorV2Service\([^\n]*channelMonitorService" upstream/sub2api/backend/internal/service/monitor_v2.go upstream/sub2api/backend/cmd/server/wire_gen.go && ! rg -n "is_flagship|ttft_p95|latency_p95|\btps\b" upstream/sub2api/frontend/src/features/monitor-v2 upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts && git diff --check && git status --short --branch`

Expected: no forbidden Monitor V2 source matches, clean diff check, and only T34 branch files changed.

- [ ] **Step 4: Write the verification report and handoff.** Record commands, results, desktop/390px screenshot paths, known 30-day cold-start behavior, and any residual risk. Set task status to `READY_FOR_ROOT_REVIEW` in the handoff/report only; do not update the global progress ledger.

- [ ] **Step 5: Commit the verification report.**

Run: `git add docs/superpowers/reports/2026-08-20-t34-native-probe-verification.md && git commit -m "docs: record t34 native probe verification"`

## Acceptance Checklist

- [ ] The Monitor V2 production path contains no `usage_logs` or `ChannelMonitorService` dependency.
- [ ] Account eligibility is delegated to `Account.IsSchedulable()` and scopes are deduplicated.
- [ ] One native SQL query returns fixed buckets, latest freshness, TTFT P50, and average latency for all requested groups.
- [ ] 30-day physical retention is separate from the unchanged 7-day score/management lookback.
- [ ] v7 API and frontend render only availability, native TTFT P50, native average latency, current two-state status, and fixed timeline.
- [ ] Multiplier is adjacent to the group name, “旗舰” is absent, and page width remains `max-w-[1500px]`.
- [ ] Focused backend/frontend tests, typecheck/build, source guards, diff check, and desktop/390px visual inspection are recorded.
- [ ] Branch is `READY_FOR_ROOT_REVIEW`; no merge, push, deploy, or production action was performed.

## Risks

- Native result storage grows from seven to thirty days; existing account/checked indexes and the single batched query must remain in place.
- During the first thirty days after retention change, missing historical buckets intentionally show `unavailable` and are not backfilled.
- Current eligibility is evaluated at snapshot time because historical eligibility is not persisted.
- v7 is a deliberate breaking contract; frontend and backend must advance together and fail closed on v6.
