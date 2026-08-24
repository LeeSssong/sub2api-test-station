# Hybrid Performance Monitor V4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fourth selectable hybrid monitoring presentation that combines active probes and strict successful user requests into five-minute availability buckets and shared P95 performance cards, without changing the three existing monitoring choices.

**Architecture:** Keep the existing `channel_monitor_mode` setting and legacy `v1`/`v2` values backward compatible. Add explicit `native_probe` and `hybrid_performance` values: legacy `v1` continues routing to the current custom Monitor V2 page, legacy `v2` continues routing to the native passive page, `native_probe` exposes the native active page, and `hybrid_performance` routes to a new API/service/page. Reuse existing group visibility and schedulable-account scope construction, extend the native account-monitor repository with a read-only hybrid projection query over `account_monitor_results` and `usage_logs`, and keep the new frontend contract/components isolated under `features/monitor-v4`.

**Tech Stack:** Go, Gin, PostgreSQL `percentile_cont`/`date_bin`, Vue 3, TypeScript, Vitest, existing Sub2API settings and release chain.

**Spec:** `docs/superpowers/specs/2026-08-25-hybrid-performance-monitor-v4-design.md`

## Global Constraints

- Preserve legacy `channel_monitor_mode=v1|v2` behavior and rollback semantics.
- The fourth mode is read-only over existing `account_monitor_results` and `usage_logs`; no migration, backfill, new fact table, or production data write.
- Strict user success reuses the current `actual_cost > 0` and non-unknown successful-service predicate.
- A performance sample must be successful and contain both `first_token_ms` and `duration_ms` after probe-field mapping.
- Availability uses UTC half-open five-minute buckets and counts a bucket once when either source has a successful event.
- TTFT P95, duration P95, and the single displayed sample count use the same unified sample set.
- The fourth card uses a complete closed ring, threshold colors, static center text, and breathing glow only; no particles, orbit dots, sweep, horizontal timeline, or color legend.
- Do not use GitHub Actions; deploy only through the reviewed local/host chain after root integration.

---

### Task 1: Extend the channel-monitor mode registry without changing legacy behavior

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/domain_constants.go`
- Modify: `upstream/sub2api/backend/internal/service/setting_public.go`
- Modify: `upstream/sub2api/backend/internal/service/settings_view.go`
- Modify: `upstream/sub2api/backend/internal/handler/dto/settings.go`
- Modify: `upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/channel_monitor_feature_gate_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/setting_handler_public_test.go`
- Modify: `upstream/sub2api/frontend/src/types/index.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/utils/featureFlags.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/SettingsView.vue`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.channel-monitor-mode.spec.ts`

**Interfaces:**
- Produces backend constants and predicates for `native_probe` and `hybrid_performance`, plus frontend `ChannelMonitorMode`/`isChannelMonitorNativeProbeMode`/`isChannelMonitorHybridMode` helpers.
- Legacy `v1` remains the custom Monitor V2 route mode; legacy `v2` remains native passive mode. Active-probe runtime gating must accept `v1`, `native_probe`, and `hybrid_performance`; passive aggregation remains gated by `v2` only.

- [ ] **Step 1: Write failing backend mode tests.** Add table cases asserting public normalization preserves `v1`/`v2`, accepts `native_probe`/`hybrid_performance`, rejects unknown values to the safe legacy default, and active-probe/passive guards select the expected mode.
- [ ] **Step 2: Run the focused backend tests and verify RED.**

  Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/server/routes ./internal/handler -run 'ChannelMonitor|channelMonitor' -count=1`

  Expected: failures for the new mode values and guards.
- [ ] **Step 3: Write failing frontend settings tests.** Assert the settings model accepts all four visible choices, the selected value survives load/save normalization, and the labels/hints for the fourth choice are present.
- [ ] **Step 4: Run the focused frontend test and verify RED.**

  Run: `cd upstream/sub2api/frontend && pnpm vitest run src/views/admin/__tests__/SettingsView.channel-monitor-mode.spec.ts`

  Expected: failures because the type and segmented control only expose `v1|v2`.
- [ ] **Step 5: Implement the mode registry and settings UI.** Add constants/normalization and runtime predicates, update DTOs and update payload validation, expand the settings control to four stable options, and add Chinese/English labels. Keep the existing v1/v2 labels and behavior intact.
- [ ] **Step 6: Run the focused tests and verify GREEN.**

  Run the two commands above; expected: all focused mode tests pass.
- [ ] **Step 7: Commit the mode-registry slice.**

  ```bash
  git add upstream/sub2api/backend/internal/service/domain_constants.go upstream/sub2api/backend/internal/service/setting_public.go upstream/sub2api/backend/internal/service/settings_view.go upstream/sub2api/backend/internal/handler/dto/settings.go upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go upstream/sub2api/backend/internal/server/routes/channel_monitor_feature_gate_test.go upstream/sub2api/backend/internal/handler/setting_handler_public_test.go upstream/sub2api/frontend/src/types/index.ts upstream/sub2api/frontend/src/api/admin/settings.ts upstream/sub2api/frontend/src/utils/featureFlags.ts upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts upstream/sub2api/frontend/src/views/admin/SettingsView.vue upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.channel-monitor-mode.spec.ts
  git commit -m "feat: add hybrid channel monitor mode selection"
  ```

### Task 2: Add the unified five-minute backend projection

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Modify: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Create: `upstream/sub2api/backend/internal/service/monitor_v4.go`
- Create: `upstream/sub2api/backend/internal/service/monitor_v4_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:**
- `AccountMonitorGroupProbeRepository` gains a read-only `ProjectHybridMonitorV4Groups(ctx, scopes, groupIDs, start, end, bucketSize)` method.
- `MonitorV4GroupProjection` carries `AvailabilityBucketCount`, `TotalBucketCount`, `TTFTP95MS`, `LatencyP95MS`, `SampleCount`, `SourceUpdatedAt`, `CurrentOperational`, and `MetricFallback`.
- `MonitorV4Service.Snapshot(ctx, userID, window, now)` returns visible group metadata plus the projection, with window values `24h|7d|30d`.

- [ ] **Step 1: Write RED unit tests for scope reuse and projection semantics.** Cover schedulable-account scopes, UTC five-minute boundary assignment, probe-only success, user-only success, mixed-source bucket de-duplication, strict `actual_cost`/unknown filtering, missing-field exclusion, common P95 sample count, and historical fallback.
- [ ] **Step 2: Run repository/service tests and verify RED.**

  Run: `cd upstream/sub2api/backend && go test ./internal/repository ./internal/service -run 'MonitorV4|Hybrid|MonitorV2' -count=1`

  Expected: compile/test failures because the new interface and projection do not exist.
- [ ] **Step 3: Extract the shared schedulable group-account scope builder.** Move the existing scope construction used by `ProjectMonitorV2Groups` into a private helper and make both v2 and v4 call it; preserve ordering, account status filters, and settings freshness semantics.
- [ ] **Step 4: Add the v4 projection types and repository method.** Implement a bounded SQL query that:
  - generates five-minute UTC buckets for the requested window;
  - unions successful probe events mapped to `first_token_ms`/`duration_ms` with strict successful `usage_logs` events;
  - counts availability from all successful events, independent of performance fields;
  - filters the unified performance sample to rows containing both fields;
  - calculates raw-sample `percentile_cont(0.95)` for both metrics and one common count;
  - derives current status from the latest fresh successful bucket;
  - uses the latest earlier successful unified bucket as the metric fallback, with deterministic zero values only when no historical sample exists.
- [ ] **Step 5: Add `MonitorV4Service` and reuse visible-group filtering.** Preserve active/enabled group filtering and exclusive-group availability checks; map projections into the stable API-facing service model and keep current availability independent from fallback metrics.
- [ ] **Step 6: Run the repository/service tests and verify GREEN.**

  Run the command from Step 2; expected: all v4 and existing monitor tests pass.
- [ ] **Step 7: Commit the backend projection slice.**

  ```bash
  git add upstream/sub2api/backend/internal/service/account_monitor_types.go upstream/sub2api/backend/internal/service/account_monitor_service.go upstream/sub2api/backend/internal/repository/account_monitor_repo.go upstream/sub2api/backend/internal/service/monitor_v2.go upstream/sub2api/backend/internal/service/monitor_v4.go upstream/sub2api/backend/internal/service/monitor_v4_test.go upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
  git commit -m "feat: aggregate hybrid monitor samples in five-minute buckets"
  ```

### Task 3: Expose the v4 snapshot API and wire runtime routing

**Files:**
- Create: `upstream/sub2api/backend/internal/handler/monitor_v4_handler.go`
- Create: `upstream/sub2api/backend/internal/handler/monitor_v4_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/wire.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/user.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/monitor_v2_routes_test.go`
- Create: `upstream/sub2api/backend/internal/server/routes/monitor_v4_routes_test.go`

**Interfaces:**
- Registers authenticated, heavy-rate-limited `GET /api/v1/monitor-v4?window=24h|7d|30d`.
- Returns the exact v4 snapshot fields from the spec, including non-null metric values, `sample_count`, `source_updated_at`, and `is_fallback_metric`.
- Rejects unsupported windows with the existing localized API error behavior and never exposes raw credentials/request bodies.

- [ ] **Step 1: Write RED handler and route tests.** Cover auth, default `7d`, valid windows, invalid window 400, mode-independent route registration, and response field serialization.
- [ ] **Step 2: Run focused route/handler tests and verify RED.**

  Run: `cd upstream/sub2api/backend && go test ./internal/handler ./internal/server/routes -run 'MonitorV4|monitor_v4' -count=1`

  Expected: missing handler/wiring/route failures.
- [ ] **Step 3: Implement handler response mapping and DI wiring.** Add the service provider, handler provider, handler registry field, authenticated route, cache-control `no-store`, subject extraction, and v4 response structs.
- [ ] **Step 4: Run focused route/handler tests and verify GREEN.**
- [ ] **Step 5: Commit the API slice.**

  ```bash
  git add upstream/sub2api/backend/internal/handler/monitor_v4_handler.go upstream/sub2api/backend/internal/handler/monitor_v4_handler_test.go upstream/sub2api/backend/internal/handler/handler.go upstream/sub2api/backend/internal/handler/wire.go upstream/sub2api/backend/internal/service/wire.go upstream/sub2api/backend/internal/server/routes/user.go upstream/sub2api/backend/internal/server/routes/monitor_v2_routes_test.go upstream/sub2api/backend/internal/server/routes/monitor_v4_routes_test.go
  git commit -m "feat: expose hybrid monitor v4 snapshot API"
  ```

### Task 4: Build the frontend v4 contract, loader, and mode dispatch

**Files:**
- Create: `upstream/sub2api/frontend/src/features/monitor-v4/types.ts`
- Create: `upstream/sub2api/frontend/src/features/monitor-v4/api.ts`
- Create: `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceView.vue`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2RouteView.vue`
- Create: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/api.spec.ts`
- Create: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`

**Interfaces:**
- `getHybridPerformanceSnapshot(window, signal?)` calls `/monitor-v4` and validates the v4 contract.
- `HybridPerformanceView` owns `24h|7d|30d` selection, current snapshot, refresh interval, abort/retry lifecycle, and renders one card per group.
- Existing `MonitorV2RouteView` dispatches: `v2` to the native `ChannelStatusView`, `native_probe` to the native active `ChannelStatusV1View` path, legacy `v1` to the unchanged custom `MonitorV2View`, and `hybrid_performance` to `HybridPerformanceView`.

- [ ] **Step 1: Write RED contract tests.** Validate the contract version, windows, group limits, non-null P95 values, `0` sample counts, threshold availability range, and fallback flag.
- [ ] **Step 2: Write RED route/view tests.** Assert the four mode branches and that legacy v1/v2 branches remain unchanged.
- [ ] **Step 3: Run frontend tests and verify RED.**

  Run: `cd upstream/sub2api/frontend && pnpm vitest run src/features/monitor-v4/__tests__/api.spec.ts src/features/monitor-v4/__tests__/HybridPerformanceView.spec.ts src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`

  Expected: missing module/branch failures.
- [ ] **Step 4: Implement v4 types/API validator and view lifecycle.** Follow the existing Monitor V2 abort, loading, refresh, and error-retention patterns; do not alter the legacy view's data contract.
- [ ] **Step 5: Implement route dispatch and native active-mode helper.** Make the `native_probe` branch render the existing native active status view without changing the legacy v1 custom branch.
- [ ] **Step 6: Run focused frontend tests and verify GREEN.**
- [ ] **Step 7: Commit the frontend loader/dispatch slice.**

  ```bash
  git add upstream/sub2api/frontend/src/features/monitor-v4 upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2RouteView.vue upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts
  git commit -m "feat: route channel monitor v4 frontend mode"
  ```

### Task 5: Implement the symmetric breathing-ring group card

**Files:**
- Create: `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceGroupCard.vue`
- Create: `upstream/sub2api/frontend/src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/features/monitor-v4/HybridPerformanceView.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/monitorV2.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/monitorV2.ts`

**Interfaces:**
- Card consumes one validated `HybridPerformanceGroup` and renders no null metric branch.
- `availabilityTone(value)` returns `green` for `>=85`, `amber` for `>=50`, and `red` otherwise.
- Card markup exposes stable test hooks for `availability`, `multiplier`, `ttft-p95`, `latency-p95`, `sample-count`, `monitoring-status`, and `ring`.

- [ ] **Step 1: Write RED component tests.** Cover exact threshold boundaries 85/50, `倍率：0.3x`, one sample label, equal-width centered metric columns, static center percentage, closed-ring structure, and absence of particle/sweep/orbit classes.
- [ ] **Step 2: Run the component test and verify RED.**
- [ ] **Step 3: Implement the card.** Use the approved A layout: centered group header, large closed ring occupying about half the card's visual area, static center text, symmetric P95 columns, centered sample/freshness rows, and `prefers-reduced-motion` CSS. Only the ring's outer glow/opacity breathes; no transform rotates the ring or center content.
- [ ] **Step 4: Add Chinese/English card strings without changing the required “基于 N 次真实请求” copy.** Keep the multiplier label exactly `倍率：{value}x` in Chinese.
- [ ] **Step 5: Run component/view tests and verify GREEN.**
- [ ] **Step 6: Commit the card slice.**

  ```bash
  git add upstream/sub2api/frontend/src/features/monitor-v4 upstream/sub2api/frontend/src/i18n/locales/zh/monitorV2.ts upstream/sub2api/frontend/src/i18n/locales/en/monitorV2.ts
  git commit -m "feat: add breathing hybrid monitor group cards"
  ```

### Task 6: Complete direct verification and handoff

**Files:**
- Modify: `docs/product/performance-monitoring-calculation.md`
- Create: `docs/handoffs/2026-08-25-t59-hybrid-performance-monitor-v4-handoff.md`

**Interfaces:**
- Product calculation documentation states the fourth mode separately from the existing custom Monitor V2 page.
- Handoff records baseline SHA, implementation commits, changed files, direct test evidence, no migration/config changes, expected `downtime_required`, rollback setting, and residual query-performance risk.

- [ ] **Step 1: Update the calculation document.** Add the fourth-mode section: five-minute availability, unified probe/user sample, shared P95/sample count, closed breathing ring, and historical metric fallback; explicitly keep the existing page section unchanged.
- [ ] **Step 2: Run direct backend verification.**

  Run: `cd upstream/sub2api/backend && go test ./internal/service ./internal/repository ./internal/handler ./internal/server/routes -run 'MonitorV4|ChannelMonitor|channelMonitor' -count=1 && go build ./cmd/server`

  Expected: PASS.
- [ ] **Step 3: Run direct frontend verification.**

  Run: `cd upstream/sub2api/frontend && pnpm vitest run src/features/monitor-v4 src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts src/views/admin/__tests__/SettingsView.channel-monitor-mode.spec.ts && pnpm typecheck && pnpm build`

  Expected: PASS; only existing build warnings are acceptable.
- [ ] **Step 4: Run formatting and diff checks.**

  Run: `cd upstream/sub2api/backend && gofmt -w internal/service/account_monitor_types.go internal/service/account_monitor_service.go internal/service/monitor_v2.go internal/service/monitor_v4.go internal/service/monitor_v4_test.go internal/repository/account_monitor_repo.go internal/repository/account_monitor_repo_test.go internal/handler/monitor_v4_handler.go internal/handler/monitor_v4_handler_test.go internal/handler/handler.go internal/handler/wire.go internal/service/wire.go internal/server/routes/user.go internal/server/routes/monitor_v4_routes_test.go && cd ../../.. && git diff --check`

  Expected: no diff-check output and no formatting changes after the final test run.
- [ ] **Step 5: Perform visual verification.** Check 1440px and 390px authenticated pages with green/yellow/red data, confirm card alignment/no horizontal overflow, verify only breathing glow moves, and confirm the center percentage remains visually fixed.
- [ ] **Step 6: Write the handoff and stop at `READY_FOR_ROOT_REVIEW`.** Do not merge, push, deploy, or mark the task `DONE`; root release control performs integration and production verification.

## Plan Self-Review

- Mode compatibility is covered by Task 1 and route dispatch tests in Task 4.
- Five-minute availability, unified samples, shared P95, missing-field exclusion, and fallback are covered by Task 2 tests and Task 6 documentation.
- API contract and auth are covered by Task 3; visual thresholds, alignment, copy, and breathing-only animation are covered by Task 5.
- No migration, data writes, GitHub Actions, unrelated pages, or old Monitor V2 contract changes are included.
- No unresolved `TBD`, `TODO`, or implementation-placeholder steps remain.
