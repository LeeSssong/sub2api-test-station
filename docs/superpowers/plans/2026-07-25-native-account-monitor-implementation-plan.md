# Native Account Monitor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an administrator-only Sub2API account monitor that probes every `active + schedulable` account, stores bounded history, exposes a global cadence, and renders account cards with quality, multiplier, invocation, and usage-window data.

**Architecture:** Extract a structured probe core from `AccountTestService`, then build a persistence-backed account monitor service and runner around it. The admin API returns one secret-free projection that the new Vue page and relay-ops can consume. The monitor uses raw SQL migration/repository tables for settings and history so it does not require adding a generated Ent entity to the existing account schema.

**Tech Stack:** Go, Gin, PostgreSQL, `database/sql`, existing Sub2API account/test/usage services, Vue 3, TypeScript, Tailwind conventions, Vitest/Vue Test Utils.

## Global Constraints

- Probe only accounts satisfying `status=active && schedulable=true`.
- Real probes use the existing server-side account authentication, proxy, TLS, model mapping, and upstream validation.
- Record first valid content TTFT and total latency; discard generated content and raw upstream response.
- One account failure must not stop other accounts in the same run.
- Use one global refresh interval; card settings opens that global setting.
- Manual whole-pool and single-account probes share a run lock and per-account in-flight protection.
- Expose no credentials, headers, cookies, Base URLs, raw responses, or generated text.
- Retain monitoring history for a bounded seven-day aggregation window.
- Do not touch any local `sub` runtime, credentials, database rows, or deployment instance.

---

### Task 1: Add the persistence contract and migration

**Files:**
- Create: `upstream/sub2api/backend/migrations/187_account_monitor.sql`
- Create: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Create: `upstream/sub2api/backend/internal/repository/account_monitor_repo.go`
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`

**Interfaces:**
- Produces `service.AccountMonitorSettings`, `service.AccountMonitorResult`,
  `service.AccountMonitorAggregate`, and `service.AccountMonitorRepository`.
- `LoadSettings(ctx) (AccountMonitorSettings, error)`.
- `SaveSettings(ctx, AccountMonitorSettings) error`.
- `InsertResult(ctx, AccountMonitorResult) error`.
- `ListAggregates(ctx, accountIDs []int64, since time.Time) (map[int64]AccountMonitorAggregate, error)`.
- `ListHistory(ctx, accountID int64, limit int) ([]AccountMonitorResult, error)`.
- `DeleteBefore(ctx, before time.Time) error`.

- [ ] **Step 1: Write failing repository tests**

Cover singleton settings defaults, interval update, result insert, aggregate
success rate and P50/P95 calculation, deterministic account ordering,
seven-day deletion, and rejection of secret-shaped fields. Use `sqlmock` for
query shape and JSON/nullable field assertions.

- [ ] **Step 2: Run repository tests and verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestAccountMonitor' -count=1
```

Expected: FAIL because the service types, repository, and migration do not
exist.

- [ ] **Step 3: Create the schema**

Create:

```sql
CREATE TABLE account_monitor_settings (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    interval_seconds INTEGER NOT NULL,
    updated_by BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE account_monitor_results (
    id BIGSERIAL PRIMARY KEY,
    run_id UUID NOT NULL,
    account_id BIGINT NOT NULL,
    model_id TEXT NOT NULL,
    status TEXT NOT NULL,
    error_code TEXT NOT NULL DEFAULT '',
    http_status INTEGER,
    ttft_ms DOUBLE PRECISION,
    latency_ms DOUBLE PRECISION,
    checked_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX account_monitor_results_account_checked_idx
    ON account_monitor_results (account_id, checked_at DESC);
CREATE INDEX account_monitor_results_checked_idx
    ON account_monitor_results (checked_at);
```

Seed settings with the validated channel-monitor-compatible default interval.
Use the repository migration runner's idempotent conventions.

- [ ] **Step 4: Implement repository methods**

Use parameterized SQL, bounded `LIMIT`, UTC timestamps, and a seven-day
cleanup query. Aggregate only the requested positive account IDs. Return
`sql.ErrNoRows` only for missing singleton settings; the service layer
provides the default and persists it on first update.

- [ ] **Step 5: Run repository tests and migration checks**

```bash
cd upstream/sub2api/backend
go test ./internal/repository -run 'TestAccountMonitor|TestMigrations' -count=1
go test ./migrations -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/backend/migrations/187_account_monitor.sql \
  upstream/sub2api/backend/internal/service/account_monitor_types.go \
  upstream/sub2api/backend/internal/repository/account_monitor_repo.go \
  upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go
git commit -m "feat: persist native account monitor results"
```

### Task 2: Extract structured real-account probing

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_test_service.go`
- Create: `upstream/sub2api/backend/internal/service/account_monitor_probe.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_probe_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_test_service_*_test.go` only when existing behavior requires regression coverage.

**Interfaces:**
- Produces `AccountMonitorProbeResult`.
- Produces `AccountTestService.ProbeAccountConnection(ctx context.Context, accountID int64, modelID string, prompt string, mode string) (AccountMonitorProbeResult, error)`.
- Existing browser SSE method keeps its current response contract and delegates only shared platform request logic.

- [ ] **Step 1: Write failing probe tests**

Use `httptest.Server` fixtures for successful first-content SSE, delayed first
content, HTTP error, timeout, malformed SSE, empty stream, and explicit
insufficient-balance response. Assert TTFT is measured at the first non-empty
content event, latency is non-negative, response text is not retained, and
the result contains only stable error classifications.

- [ ] **Step 2: Run probe tests and verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorProbe' -count=1
```

Expected: FAIL because the structured probe method is not defined.

- [ ] **Step 3: Implement the shared structured probe**

Introduce:

```go
type AccountMonitorProbeResult struct {
    AccountID  int64
    ModelID    string
    Status     string
    ErrorCode  string
    HTTPStatus *int
    TTFTMS     *float64
    LatencyMS  *float64
}
```

Reuse existing platform-specific request construction and account repository
lookups. Parse only enough stream data to detect the first valid content event,
then close the body. Map failures to `balance_exhausted`, `http_error`,
`timeout`, `malformed_stream`, `account_test_error`, or `model_unavailable`.

- [ ] **Step 4: Preserve the browser SSE path**

Run existing account-test tests, including Claude, OpenAI, Gemini, Grok,
Antigravity, image, compact, and error-state tests. Ensure the refactor does
not change the browser event names, prompt defaults, or successful-test
rate-limit recovery behavior.

- [ ] **Step 5: Run probe and regression tests**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorProbe|TestAccountTest' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/backend/internal/service/account_test_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_probe.go \
  upstream/sub2api/backend/internal/service/account_monitor_probe_test.go
git commit -m "feat: expose structured account probe results"
```

### Task 3: Build the account monitor service and runner

**Files:**
- Create: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Create: `upstream/sub2api/backend/internal/service/account_monitor_runner.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_runner_test.go`

**Interfaces:**
- `AccountMonitorService.List(ctx, filters) (AccountMonitorPage, error)`.
- `AccountMonitorService.RunAll(ctx, actorID) (AccountMonitorRun, error)`.
- `AccountMonitorService.RunOne(ctx, actorID, accountID) (AccountMonitorResult, error)`.
- `AccountMonitorService.UpdateSettings(ctx, actorID, intervalSeconds) (AccountMonitorSettings, error)`.
- `AccountMonitorService.History(ctx, accountID, limit) ([]AccountMonitorResult, error)`.
- `AccountMonitorRunner.Start()`, `Stop()`, and `TriggerNow()`.

- [ ] **Step 1: Write failing service tests**

Assert that the service selects only active/schedulable accounts, orders them
by ID, chooses a deterministic compatible text model, stores every result,
continues after one failure, calculates aggregate metrics, and rejects a
single-account run for an account outside the current pool.

- [ ] **Step 2: Write failing runner tests**

Assert one global ticker, validated interval changes, immediate first run,
whole-pool run lock, per-account duplicate suppression, stop cancellation,
and seven-day cleanup. Use fake clock and fake probe/repository interfaces.

- [ ] **Step 3: Run service/runner tests and verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitor(Service|Runner)' -count=1
```

Expected: FAIL because the service and runner are absent.

- [ ] **Step 4: Implement service behavior**

Use `AccountRepository.ListSchedulable(ctx)` as the source of truth, then
retain only accounts whose persisted status is `active`. Select the same
provider-neutral deterministic model policy used by the approved account
quality monitor design. Persist a result for every discovered account,
including failures, and publish the run only after all results are written.

- [ ] **Step 5: Implement runner behavior**

Use a single long-lived ticker driven by settings, a global mutex/atomic
running flag, a bounded worker pool, and a per-account in-flight map. On
startup load settings and trigger one run. On each run, invoke cleanup after
successful publication. Do not modify account schedulability or routing as a
side effect.

- [ ] **Step 6: Run service/runner tests**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitor(Service|Runner)' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_runner.go \
  upstream/sub2api/backend/internal/service/account_monitor_service_test.go \
  upstream/sub2api/backend/internal/service/account_monitor_runner_test.go
git commit -m "feat: schedule native account monitoring"
```

### Task 4: Wire admin API and backend runtime

**Files:**
- Create: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler.go`
- Create: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Create: `upstream/sub2api/backend/internal/handler/dto/account_monitor.go`
- Modify: `upstream/sub2api/backend/internal/handler/handler.go`
- Modify: `upstream/sub2api/backend/internal/handler/wire.go`
- Modify: `upstream/sub2api/backend/internal/repository/wire.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify: `upstream/sub2api/backend/internal/server/routes/admin.go`
- Modify: `upstream/sub2api/backend/internal/server/middleware/audit_log.go` only for the new admin routes.

**Interfaces:**
- Produces admin endpoints:
  `GET /api/v1/admin/account-monitors`,
  `PUT /api/v1/admin/account-monitors/settings`,
  `POST /api/v1/admin/account-monitors/run`,
  `POST /api/v1/admin/account-monitors/:account_id/run`,
  `GET /api/v1/admin/account-monitors/:account_id/history`.

- [ ] **Step 1: Write failing handler and route tests**

Cover admin success, non-admin rejection, malformed account ID, interval
validation, full-pool run response, single-account run response, bounded
history, empty results, and a JSON response scan that rejects credential-shaped
keys and raw upstream content.

- [ ] **Step 2: Run handler tests and verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin ./internal/server/routes -run 'AccountMonitor' -count=1
```

Expected: FAIL because the handler and route are absent.

- [ ] **Step 3: Implement response projection and handlers**

Return redacted account metadata, current group IDs/names, multiplier, latest
result, aggregate metrics, usage-window summary when already available, and
today invocation stats. Keep write endpoints limited to interval settings and
probe triggers. Map service errors to existing response helpers.

- [ ] **Step 4: Wire repository, service, runner, handler, and routes**

Follow the existing ChannelMonitor provider order:
repository -> service -> runner -> handler -> `handler.Handlers` -> admin route.
Start the runner exactly once during application wiring and ensure cleanup
stops it.

- [ ] **Step 5: Run backend contract tests**

```bash
cd upstream/sub2api/backend
go test ./internal/handler/admin ./internal/server ./internal/service ./internal/repository -run 'AccountMonitor|ServerTiming' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/backend/internal/handler \
  upstream/sub2api/backend/internal/repository/wire.go \
  upstream/sub2api/backend/internal/service/wire.go \
  upstream/sub2api/backend/internal/server/routes/admin.go \
  upstream/sub2api/backend/internal/server/middleware/audit_log.go
git commit -m "feat: expose administrator account monitor API"
```

### Task 5: Add the administrator account-monitor page

**Files:**
- Create: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Create: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorSettingsDialog.vue`
- Create: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/api/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/router/index.ts`
- Modify: `upstream/sub2api/frontend/src/components/layout/AppSidebar.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/common.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/common.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- `adminAPI.accountMonitor.list()`.
- `adminAPI.accountMonitor.updateSettings(intervalSeconds)`.
- `adminAPI.accountMonitor.runAll()`.
- `adminAPI.accountMonitor.runOne(accountID)`.
- `adminAPI.accountMonitor.history(accountID, limit)`.

- [ ] **Step 1: Write failing Vue/API tests**

Mock the admin API and assert:

```ts
expect(screen.getByText('账号监控')).toBeInTheDocument()
expect(screen.getByText('0.10x')).toBeInTheDocument()
expect(screen.getByText('active')).toBeInTheDocument()
```

Also cover hidden route for non-admin, global interval update from header and
card settings, run-all, single-card run, stale/error/no-history states, and
rendering of `AccountTodayStatsCell` and `AccountUsageCell`.

- [ ] **Step 2: Run the focused frontend test and verify RED**

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts
```

Expected: FAIL because the API module, route, and view do not exist.

- [ ] **Step 3: Implement the API module and view**

Use the existing `apiClient`, `AppLayout`, `TablePageLayout`/responsive
layout, `HelpTooltip`, `Icon`, `AccountTodayStatsCell`, and `AccountUsageCell`.
Keep one global interval state, disable duplicate actions while running, and
re-filter cards when the backend returns a changed account pool.

- [ ] **Step 4: Add route, navigation, and translations**

Add `/admin/accounts/monitor` with `requiresAuth: true` and
`requiresAdmin: true`. Add the menu item adjacent to account management.
Provide complete Chinese and English strings for headings, statuses, errors,
interval labels, filters, and action tooltips.

- [ ] **Step 5: Run frontend verification**

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts
pnpm run typecheck
pnpm run build
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add upstream/sub2api/frontend/src/api/admin/accountMonitor.ts \
  upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue \
  upstream/sub2api/frontend/src/components/admin/account-monitor \
  upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts \
  upstream/sub2api/frontend/src/api/admin/index.ts \
  upstream/sub2api/frontend/src/router/index.ts \
  upstream/sub2api/frontend/src/components/layout/AppSidebar.vue \
  upstream/sub2api/frontend/src/i18n/locales/zh \
  upstream/sub2api/frontend/src/i18n/locales/en
git commit -m "feat: add administrator account monitor page"
```

### Task 6: Run the native monitor acceptance suite

**Files:**
- Test: `upstream/sub2api/backend/internal/repository/account_monitor_repo_test.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_*_test.go`
- Test: `upstream/sub2api/backend/internal/handler/admin/account_monitor_handler_test.go`
- Test: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`

- [ ] **Step 1: Run focused backend tests**

```bash
cd upstream/sub2api/backend
go test ./internal/repository ./internal/service ./internal/handler/admin -run 'AccountMonitor' -count=1
```

- [ ] **Step 2: Run focused frontend tests and typecheck**

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts
pnpm run typecheck
```

- [ ] **Step 3: Run broader regression**

```bash
cd upstream/sub2api/backend
go test ./internal/service ./internal/handler/... ./internal/server/... -count=1
cd ../../frontend
pnpm exec vitest run
pnpm run build
```

Expected: no regression in existing channel monitoring, account management, or
admin authorization tests.

- [ ] **Step 4: Commit verification notes**

```bash
git add docs/superpowers/reports
git commit -m "test: verify native account monitor"
```

