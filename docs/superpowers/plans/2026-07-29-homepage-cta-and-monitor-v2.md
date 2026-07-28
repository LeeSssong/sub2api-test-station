# Homepage CTA and Monitor V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Normalize the homepage CTA to `立即开始` and add an authenticated,
isolated Monitor V2 that renders one operational card for every active public
Sub2API group while preserving the native channel monitor as automatic
fallback.

**Architecture:** Add a read-only Monitor V2 projection inside the Sub2API
backend. The projection selects active groups where `is_exclusive = false`,
reuses native channel-monitor, channel, Ops overview/trend, and OpenAI token
statistics services, and adds one bounded cache-hit aggregate. The frontend is
a self-contained `features/monitor-v2` island selected by a thin route view;
contract or service failure switches to the unchanged native
`ChannelStatusView`.

**Tech Stack:** Go 1.24, Gin, Ent/PostgreSQL, Google Wire, Vue 3,
TypeScript 5.6, Pinia, Vue I18n, Tailwind CSS, Vitest, Vue Test Utils,
Go `testing`, `testify`, and `go-sqlmock`.

## Global Constraints

- `/monitor` and `/api/v1/monitor-v2` remain behind the existing JWT and
  backend-mode user middleware.
- Card membership is exactly active native groups with
  `is_exclusive = false`; subscription type, `AllowedGroups`, API-key binding,
  and user rate overrides never affect it.
- Cards display the group's native public/default `rate_multiplier`; peak
  rules are separate metadata.
- No second database, scheduler, health checker, or `relay-ops` runtime
  dependency is introduced.
- The native `ChannelStatusView` and native channel-monitor APIs remain
  unchanged and importable as fallback.
- Supported windows are exactly `24h`, `7d`, and `30d`; all query inputs and
  response sizes are bounded.
- Responses contain no users, API keys, accounts, suppliers, credentials,
  balances, request IDs, IPs, user agents, prompts, monitor endpoints, or raw
  errors.
- Missing monitor, low samples, and unsupported evidence render explicitly;
  they never become fabricated `0ms`, `0%`, or `100%` values.
- Homepage pricing content and CTA destination/session behavior remain
  unchanged.
- Implementation is performed in an isolated Git worktree and is kept local:
  do not push, release, deploy, or promote a production image.

---

## File Structure

### Backend files

- Create `upstream/sub2api/backend/internal/service/monitor_v2.go`
  — projection contract, window validation, public-group selection, concurrent
  native metric reads, metric/sample states, and group assembly.
- Create `upstream/sub2api/backend/internal/service/monitor_v2_test.go`
  — pure/service tests for public selection, cross-user invariance, states,
  metric formulas, bounds, and partial native-data failures.
- Create `upstream/sub2api/backend/internal/repository/monitor_v2_repo.go`
  — one read-only, group-batched cache-hit aggregate over `usage_logs`.
- Create `upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go`
  — SQL bounds, grouping, empty result, and scan-error tests.
- Create `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`
  — versioned allowlisted DTO, `window` parsing, `Cache-Control: no-store`, and
  response mapping.
- Create `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`
  — invalid-window, response-contract, no-store, and forbidden-field tests.
- Modify `upstream/sub2api/backend/internal/repository/wire.go`
  — register the focused Monitor V2 repository.
- Modify `upstream/sub2api/backend/internal/service/wire.go`
  — provide the Monitor V2 service from existing concrete native services.
- Modify `upstream/sub2api/backend/internal/handler/handler.go`
  — add `MonitorV2 *MonitorV2Handler`.
- Modify `upstream/sub2api/backend/internal/handler/wire.go`
  — construct and expose the handler.
- Modify `upstream/sub2api/backend/internal/server/routes/user.go`
  — register authenticated `GET /monitor-v2`.
- Modify `upstream/sub2api/backend/cmd/server/wire_gen.go`
  — regenerate dependency wiring with Google Wire.

### Frontend files

- Create `upstream/sub2api/frontend/src/features/monitor-v2/types.ts`
  — exact V2 types and discriminated metric/status states.
- Create `upstream/sub2api/frontend/src/features/monitor-v2/api.ts`
  — request function plus runtime allowlisted contract validator.
- Create
  `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/api.spec.ts`
  — supported/unsupported version and malformed-range tests.
- Create
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2RouteView.vue`
  — route-level V2/native-fallback switch.
- Create
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2View.vue`
  — AppLayout, window loading, refresh, overall status, privacy/metric notes,
  and fatal-error emission.
- Create
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue`
  — group status, multiplier, metrics, availability calls, timeline, and model
  details trigger.
- Create
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue`
  — text-accessible, sample-aware timeline.
- Create
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2ModelDialog.vue`
  — keyboard-accessible model details and metric definitions.
- Create
  `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`
  — V2 success and native fallback tests.
- Create
  `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`
  — complete/partial/empty state, window, accessibility, and no-secret tests.
- Modify `upstream/sub2api/frontend/src/router/index.ts`
  — point `/monitor` at the thin Monitor V2 route entry.
- Modify `upstream/sub2api/frontend/src/views/HomeView.vue`
  — use `home.getStarted` for both session states while preserving `:to`.
- Modify `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
  and `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`
  — add Monitor V2 labels and fallback notice.
- Create or modify
  `upstream/sub2api/frontend/src/views/__tests__/HomeView.spec.ts`
  — assert identical CTA copy and unchanged guest/authenticated destinations.

---

### Task 1: Normalize the homepage CTA without changing navigation

**Files:**

- Modify: `upstream/sub2api/frontend/src/views/HomeView.vue:128-139`
- Create or modify:
  `upstream/sub2api/frontend/src/views/__tests__/HomeView.spec.ts`

**Interfaces:**

- Consumes: `isAuthenticated`, `dashboardPath`, and i18n key
  `home.getStarted`.
- Produces: identical visible `立即开始`/`Get Started` label for guest and
  authenticated sessions; existing route destinations remain unchanged.

- [ ] **Step 1: Write failing CTA behavior tests**

  Mount `HomeView` twice with the auth store returning false and true. Stub
  `router-link` so its `to` prop remains inspectable:

  ```ts
  expect(wrapper.get('[data-test="home-primary-cta"]').text()).toContain('立即开始')
  expect(wrapper.getComponent({ name: 'RouterLink' }).props('to')).toBe('/login')

  expect(authenticated.get('[data-test="home-primary-cta"]').text()).toContain('立即开始')
  expect(authenticated.getComponent({ name: 'RouterLink' }).props('to')).toBe('/dashboard')
  ```

- [ ] **Step 2: Run the focused test and verify the authenticated assertion fails**

  Run:

  ```bash
  cd upstream/sub2api/frontend
  pnpm test:run src/views/__tests__/HomeView.spec.ts
  ```

  Expected: authenticated copy is `进入控制台`, while destination assertions
  already pass.

- [ ] **Step 3: Make the minimal template change**

  Add `data-test="home-primary-cta"` to the existing `router-link` and replace
  only the interpolated label:

  ```vue
  {{ t('home.getStarted') }}
  ```

  Do not change:

  ```vue
  :to="isAuthenticated ? dashboardPath : '/login'"
  ```

- [ ] **Step 4: Re-run the focused test**

  Run the Step 2 command. Expected: PASS.

- [ ] **Step 5: Commit**

  ```bash
  git add upstream/sub2api/frontend/src/views/HomeView.vue \
    upstream/sub2api/frontend/src/views/__tests__/HomeView.spec.ts
  git commit -m "fix: normalize homepage primary CTA"
  ```

---

### Task 2: Add the bounded cache-evidence repository

**Files:**

- Create: `upstream/sub2api/backend/internal/repository/monitor_v2_repo.go`
- Create:
  `upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/wire.go`

**Interfaces:**

- Produces:

  ```go
  type MonitorV2CacheStats struct {
      RequestCount int64
      HitCount     int64
  }

  type MonitorV2Repository interface {
      GetCacheStats(
          ctx context.Context,
          groupIDs []int64,
          start, end time.Time,
      ) (map[int64]MonitorV2CacheStats, error)
  }
  ```

- `repository.NewMonitorV2Repository(db *sql.DB)` returns
  `service.MonitorV2Repository`.

- [ ] **Step 1: Define the interface and write failing repository tests**

  The SQL test must expect exactly one grouped query:

  ```sql
  SELECT
    group_id,
    COUNT(*)::bigint AS request_count,
    COUNT(*) FILTER (WHERE cache_read_tokens > 0)::bigint AS hit_count
  FROM usage_logs
  WHERE created_at >= $1
    AND created_at < $2
    AND group_id = ANY($3)
  GROUP BY group_id
  ```

  Cover:

  ```go
  require.Equal(t, service.MonitorV2CacheStats{
      RequestCount: 20,
      HitCount: 8,
  }, got[publicGroupID])
  ```

  Also assert an empty group list returns an empty map without issuing SQL,
  invalid/zero ranges return errors, and rows for no traffic produce no
  fabricated group entry.

- [ ] **Step 2: Run tests and verify missing symbols fail**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/repository -run MonitorV2 -count=1
  ```

  Expected: compile failure because `MonitorV2Repository` and constructor do
  not exist.

- [ ] **Step 3: Implement the focused repository**

  Use `pq.Array(groupIDs)`, UTC timestamps, `rows.Err()`, and an explicit
  maximum of 100 group IDs. Return an error before SQL when the bound is
  exceeded.

- [ ] **Step 4: Register the constructor and re-run repository tests**

  Add `NewMonitorV2Repository` to `repository.ProviderSet`, then run the Step 2
  command. Expected: PASS.

- [ ] **Step 5: Commit**

  ```bash
  git add upstream/sub2api/backend/internal/repository/monitor_v2_repo.go \
    upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go \
    upstream/sub2api/backend/internal/repository/wire.go
  git commit -m "feat: add monitor v2 cache projection"
  ```

---

### Task 3: Build the native Monitor V2 projection service

**Files:**

- Create: `upstream/sub2api/backend/internal/service/monitor_v2.go`
- Create: `upstream/sub2api/backend/internal/service/monitor_v2_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`

**Interfaces:**

- Consumes:

  ```go
  type MonitorV2ChannelReader interface {
      ListAvailable(context.Context) ([]AvailableChannel, error)
  }

  type MonitorV2ProbeReader interface {
      ListUserView(context.Context) ([]*UserMonitorView, error)
  }

  type MonitorV2OpsReader interface {
      GetDashboardOverview(context.Context, *OpsDashboardFilter) (*OpsDashboardOverview, error)
      GetThroughputTrend(context.Context, *OpsDashboardFilter, int) (*OpsThroughputTrendResponse, error)
      GetErrorTrend(context.Context, *OpsDashboardFilter, int) (*OpsErrorTrendResponse, error)
      GetOpenAITokenStats(context.Context, *OpsOpenAITokenStatsFilter) (*OpsOpenAITokenStatsResponse, error)
  }
  ```

- Produces:

  ```go
  type MonitorV2Window string

  const (
      MonitorV2Window24H MonitorV2Window = "24h"
      MonitorV2Window7D  MonitorV2Window = "7d"
      MonitorV2Window30D MonitorV2Window = "30d"
  )

  func (s *MonitorV2Service) Snapshot(
      ctx context.Context,
      window MonitorV2Window,
      now time.Time,
  ) (*MonitorV2Snapshot, error)
  ```

- [ ] **Step 1: Write public-membership and identity-invariance tests**

  Feed the group stub:

  ```go
  []Group{
      {ID: 1, Name: "公开标准", Status: StatusActive, IsExclusive: false, RateMultiplier: 0.2},
      {ID: 2, Name: "公开订阅", Status: StatusActive, IsExclusive: false, SubscriptionType: SubscriptionTypeSubscription, RateMultiplier: 0.3},
      {ID: 3, Name: "专属", Status: StatusActive, IsExclusive: true},
      {ID: 4, Name: "已停用", Status: StatusDisabled, IsExclusive: false},
  }
  ```

  Assert only IDs 1 and 2 appear, in repository order, with `0.2` and `0.3`.
  Call `Snapshot` twice without any user ID input and assert identical
  membership and rates. This proves the service has no user-selection input.

- [ ] **Step 2: Write failing state and formula tests**

  Cover exact rules:

  ```go
  // Current state
  noMatchingEnabledMonitor => "unconfigured"
  allOperational          => "operational"
  operationalAndFailed    => "degraded"
  allFailedOrError        => "unavailable"
  monitorWithoutLatest    => "insufficient_data"

  // Historical evidence
  availability = success_count / request_count_sla * 100
  request_count_sla = success_count + error_count_sla
  latency/TTFT visible when sample_count >= 5
  TPS visible when native token-stat request_count >= 5
  cache visible when cache request_count >= 5
  otherwise state = "insufficient_data"
  ```

  Use exact expected count copy:

  ```go
  require.Equal(t, int64(9842), card.Availability.SuccessCount)
  require.Equal(t, int64(9910), card.Availability.EligibleCount)
  require.InDelta(t, 99.3138, *card.Availability.Value, 0.0001)
  ```

- [ ] **Step 3: Write failing window/bounds/timeline tests**

  Assert:

  ```go
  24h => start=now-24h, bucketSeconds=3600
  7d  => start=now-7d,  bucketSeconds=21600
  30d => start=now-30d, bucketSeconds=86400
  invalid window => error
  more than 100 active public groups => bounded error
  ```

  Combine native throughput success counts and native SLA error counts by
  `bucket_start`. Buckets with zero eligible requests use `no_data`, not
  `100%`.

- [ ] **Step 4: Write failing model/probe/partial-error tests**

  Assert active `AvailableChannel` model names are deduplicated per group and
  monitor primary/extra models are included. Map monitors to groups using the
  trimmed, case-insensitive native `GroupName`; group names are unique.

  Ops/cache failure is group-local historical evidence degradation, not a
  whole-contract failure. Group-repository failure is fatal. A public group
  with no monitor remains present as `unconfigured`.

- [ ] **Step 5: Implement the projection with bounded concurrency**

  Add constants:

  ```go
  const (
      monitorV2MaxGroups       = 100
      monitorV2MetricWorkers   = 4
      monitorV2MinimumSamples  = 5
      monitorV2ContractVersion = "2"
  )
  ```

  Load groups, channels, monitors, and cache stats once. Run per-group Ops reads
  through an `errgroup` semaphore of four workers. For non-OpenAI groups, TPS
  may be `not_provided`; never substitute gateway aggregate token throughput
  for output-token TPS.

- [ ] **Step 6: Add the Wire provider and run service tests**

  `ProvideMonitorV2Service` accepts concrete `*ChannelService`,
  `*ChannelMonitorService`, and `*OpsService`, plus `GroupRepository` and
  `MonitorV2Repository`, then passes them to the interface-oriented
  constructor.

  Run:

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/service -run MonitorV2 -count=1
  ```

  Expected: PASS.

- [ ] **Step 7: Commit**

  ```bash
  git add upstream/sub2api/backend/internal/service/monitor_v2.go \
    upstream/sub2api/backend/internal/service/monitor_v2_test.go \
    upstream/sub2api/backend/internal/service/wire.go
  git commit -m "feat: add native monitor v2 projection"
  ```

---

### Task 4: Expose a versioned authenticated allowlisted API

**Files:**

- Create:
  `upstream/sub2api/backend/internal/handler/monitor_v2_handler.go`
- Create:
  `upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go`
- Modify: `upstream/sub2api/backend/internal/handler/handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/wire.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/user.go`
- Modify: `upstream/sub2api/backend/cmd/server/wire_gen.go`

**Interfaces:**

- Produces authenticated:

  ```http
  GET /api/v1/monitor-v2?window=24h|7d|30d
  Cache-Control: no-store
  ```

- Produces top-level JSON data:

  ```json
  {
    "contract_version": "2",
    "window": "7d",
    "generated_at": "2026-07-29T12:00:00Z",
    "groups": []
  }
  ```

- [ ] **Step 1: Write handler contract tests**

  Use a stub exposing:

  ```go
  Snapshot(context.Context, service.MonitorV2Window, time.Time) (*service.MonitorV2Snapshot, error)
  ```

  Assert valid windows map into the service, invalid windows return 400, the
  response contains `contract_version = "2"`, and
  `Cache-Control = "no-store"`.

- [ ] **Step 2: Write serialized privacy tests**

  Marshal a complete response and reject forbidden keys/substrings:

  ```go
  forbidden := []string{
      "user_id", "api_key", "account_id", "account_name", "supplier",
      "credential", "balance", "request_id", "client_ip", "user_agent",
      "prompt", "endpoint", "raw_error",
  }
  ```

  The test also asserts response group IDs, names, public rates, metrics,
  models, and timeline survive mapping.

- [ ] **Step 3: Implement DTO mapping and route**

  Keep handler DTO structs private and explicit. Do not serialize service
  objects directly. Register:

  ```go
  authenticated.GET("/monitor-v2", h.MonitorV2.Snapshot)
  ```

  This placement inherits JWT, backend-mode user guard, and audit middleware
  but does not read the auth subject in the handler.

- [ ] **Step 4: Update dependency injection**

  Add `MonitorV2 *MonitorV2Handler` to `Handlers`, add
  `NewMonitorV2Handler` to `handler.ProviderSet`, then regenerate:

  ```bash
  cd upstream/sub2api/backend
  go run github.com/google/wire/cmd/wire ./cmd/server
  ```

- [ ] **Step 5: Run handler, route, and compile tests**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/handler ./internal/server/routes -run 'MonitorV2|UserRoutes' -count=1
  go test ./cmd/server -run '^$'
  ```

  Expected: PASS.

- [ ] **Step 6: Commit**

  ```bash
  git add upstream/sub2api/backend/internal/handler/monitor_v2_handler.go \
    upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go \
    upstream/sub2api/backend/internal/handler/handler.go \
    upstream/sub2api/backend/internal/handler/wire.go \
    upstream/sub2api/backend/internal/server/routes/user.go \
    upstream/sub2api/backend/cmd/server/wire_gen.go
  git commit -m "feat: expose authenticated monitor v2 API"
  ```

---

### Task 5: Add the frontend contract and automatic native fallback

**Files:**

- Create: `upstream/sub2api/frontend/src/features/monitor-v2/types.ts`
- Create: `upstream/sub2api/frontend/src/features/monitor-v2/api.ts`
- Create:
  `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/api.spec.ts`
- Create:
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2RouteView.vue`
- Create:
  `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/router/index.ts:483-492`

**Interfaces:**

- Produces:

  ```ts
  export type MonitorV2Window = '24h' | '7d' | '30d'
  export const MONITOR_V2_CONTRACT_VERSION = '2'
  export async function getMonitorV2Snapshot(
    window: MonitorV2Window,
    signal?: AbortSignal,
  ): Promise<MonitorV2Snapshot>
  ```

- `MonitorV2RouteView` renders `MonitorV2View` after a valid contract and
  `ChannelStatusView` after any HTTP/contract failure.

- [ ] **Step 1: Write failing runtime-contract tests**

  Accept a complete version-2 payload. Reject:

  ```ts
  contract_version !== '2'
  unsupported window
  groups not array
  duplicate group IDs
  rate_multiplier < 0
  availability outside 0..100
  hit_count > request_count
  sample_count < 0
  unknown status-state discriminants
  ```

  Unknown extra fields remain ignored for forward-compatible minor additions.

- [ ] **Step 2: Implement types and validator**

  Use explicit type guards; do not cast raw Axios data directly. Throw
  `MonitorV2ContractError` on a required-field/range/version violation.

- [ ] **Step 3: Write failing route fallback tests**

  Mock `getMonitorV2Snapshot`. Assert:

  ```ts
  resolved valid snapshot => MonitorV2View
  rejected HTTP request   => ChannelStatusView + restrained fallback notice
  rejected contract       => ChannelStatusView + restrained fallback notice
  ```

  The notice must not render endpoint names, response bodies, or stack traces.

- [ ] **Step 4: Implement the thin route entry and change one router import**

  `MonitorV2RouteView` owns only the V2/native selection. Point `/monitor` to
  it while keeping the existing `requiresAuth: true` metadata unchanged.

- [ ] **Step 5: Run focused tests**

  ```bash
  cd upstream/sub2api/frontend
  pnpm test:run \
    src/features/monitor-v2/__tests__/api.spec.ts \
    src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts
  ```

  Expected: PASS.

- [ ] **Step 6: Commit**

  ```bash
  git add upstream/sub2api/frontend/src/features/monitor-v2/types.ts \
    upstream/sub2api/frontend/src/features/monitor-v2/api.ts \
    upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2RouteView.vue \
    upstream/sub2api/frontend/src/features/monitor-v2/__tests__/api.spec.ts \
    upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2RouteView.spec.ts \
    upstream/sub2api/frontend/src/router/index.ts
  git commit -m "feat: add monitor v2 contract fallback"
  ```

---

### Task 6: Build the isolated Monitor V2 user interface

**Files:**

- Create:
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2View.vue`
- Create:
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2GroupCard.vue`
- Create:
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2Timeline.vue`
- Create:
  `upstream/sub2api/frontend/src/features/monitor-v2/MonitorV2ModelDialog.vue`
- Create:
  `upstream/sub2api/frontend/src/features/monitor-v2/__tests__/MonitorV2View.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts`

**Interfaces:**

- `MonitorV2View` consumes `initialSnapshot: MonitorV2Snapshot` and emits
  `fatal` only for contract/service reload failure.
- `MonitorV2GroupCard` consumes one `MonitorV2Group`.
- `MonitorV2Timeline` consumes `MonitorV2TimelinePoint[]`.
- `MonitorV2ModelDialog` consumes selected group/model evidence and emits
  `close`.

- [ ] **Step 1: Write failing complete and partial-state tests**

  Assert cards show:

  ```text
  public group name
  0.2× base multiplier
  9,842 / 9,910 次有效调用成功
  TTFT P50
  average output TPS
  total latency P50
  cache-hit rate
  model count
  ```

  Partial evidence must render the exact Chinese states:
  `未配置监控`, `样本不足`, and `未提供`. Zero evidence must not render
  `0ms`, `0%`, or `100%`.

- [ ] **Step 2: Write failing interaction/accessibility tests**

  Cover `24 小时`/`7 天`/`30 天` window switching, aborting the previous
  request, model-dialog open/close, Escape close, focus return, status text
  independent of color, and one-column behavior classes for narrow screens.

- [ ] **Step 3: Implement the page and group cards**

  Follow the approved mockup information hierarchy:

  - compact title/overall-state header;
  - segmented window control;
  - two-column desktop, one-column mobile grid;
  - state badge and base multiplier in the card header;
  - four primary metric cells;
  - call-count-first availability row;
  - accessible timeline with text labels/tooltips;
  - model detail button/dialog;
  - metric and privacy footnotes.

  Use existing design tokens, `AppLayout`, `Icon`, focus-ring utilities, dark
  mode, and `prefers-reduced-motion`. Do not copy the competitor's brand.

- [ ] **Step 4: Implement reload and failure behavior**

  Initial data comes from the route view. Window switching calls
  `getMonitorV2Snapshot`; an abort is ignored, while any non-abort
  HTTP/contract failure emits `fatal` so the route entry mounts the native
  fallback.

- [ ] **Step 5: Add Chinese and English copy**

  Add a dedicated `monitorV2` locale object under dashboard locales. Do not
  repurpose existing `channelStatus` keys used by the native fallback.

- [ ] **Step 6: Run focused tests and typecheck**

  ```bash
  cd upstream/sub2api/frontend
  pnpm test:run src/features/monitor-v2/__tests__
  pnpm typecheck
  ```

  Expected: PASS.

- [ ] **Step 7: Commit**

  ```bash
  git add upstream/sub2api/frontend/src/features/monitor-v2 \
    upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts \
    upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts
  git commit -m "feat: build isolated monitor v2 UI"
  ```

---

### Task 7: Verify authentication, privacy, fallback, and visual behavior

**Files:**

- Modify only files already listed if verification exposes a defect.
- Do not modify deployment manifests or production configuration.

**Interfaces:**

- Consumes all prior tasks.
- Produces evidence that the written design's done-when conditions hold.

- [ ] **Step 1: Run backend focused suites**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/repository ./internal/service ./internal/handler \
    ./internal/server/routes -run 'MonitorV2|ChannelMonitor|OpenAITokenStats' -count=1
  ```

- [ ] **Step 2: Run backend package regression and static checks**

  ```bash
  cd upstream/sub2api/backend
  go test ./internal/repository ./internal/service ./internal/handler ./internal/server/routes -count=1
  go test ./cmd/server -run '^$'
  go vet ./internal/repository ./internal/service ./internal/handler ./internal/server/routes
  ```

- [ ] **Step 3: Run frontend tests, lint, typecheck, and production build**

  ```bash
  cd upstream/sub2api/frontend
  pnpm test:run \
    src/views/__tests__/HomeView.spec.ts \
    src/features/monitor-v2/__tests__
  pnpm lint:check
  pnpm typecheck
  pnpm build
  ```

- [ ] **Step 4: Audit serialized privacy and selection invariants**

  ```bash
  rg -n "AllowedGroups|group_rates|user_id|api_key|account_id|supplier|balance|endpoint|raw_error" \
    upstream/sub2api/backend/internal/handler/monitor_v2_handler.go \
    upstream/sub2api/frontend/src/features/monitor-v2
  ```

  Expected: no forbidden response field or user-bound filter; permitted test
  strings are confined to negative assertions.

- [ ] **Step 5: Run local visual verification**

  Start only the local development stack or frontend with a mocked V2 response.
  Inspect desktop and mobile widths, dark mode, 200% zoom, keyboard focus,
  reduced motion, full/partial/unconfigured cards, and native fallback.
  Capture screenshots as local review evidence; do not deploy them.

- [ ] **Step 6: Confirm no production action occurred**

  Verify:

  ```bash
  git status --short
  git log --oneline --decorate -10
  git remote -v
  ```

  Do not run `git push`, deployment scripts, release workflows, host updaters,
  Docker image promotion, or production SSH commands.

- [ ] **Step 7: Final local completion commit if verification required fixes**

  ```bash
  git add \
    upstream/sub2api/backend/internal/repository/monitor_v2_repo.go \
    upstream/sub2api/backend/internal/repository/monitor_v2_repo_test.go \
    upstream/sub2api/backend/internal/service/monitor_v2.go \
    upstream/sub2api/backend/internal/service/monitor_v2_test.go \
    upstream/sub2api/backend/internal/handler/monitor_v2_handler.go \
    upstream/sub2api/backend/internal/handler/monitor_v2_handler_test.go \
    upstream/sub2api/frontend/src/features/monitor-v2 \
    upstream/sub2api/frontend/src/views/HomeView.vue \
    upstream/sub2api/frontend/src/views/__tests__/HomeView.spec.ts
  git commit -m "test: complete monitor v2 verification"
  ```

  Skip this commit when the worktree is already clean.
