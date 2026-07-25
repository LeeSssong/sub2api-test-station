# Account Monitor Multiplier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the account monitor's local billing multiplier with a trustworthy upstream API-key group multiplier, add a group filter, and keep relay-ops recommendations read-only and fail-closed.

**Architecture:** Add a focused `AccountMultiplierService` that projects fresh native Sub2API declarations and measures compatible New API upstreams only when no fresh declaration exists. Persist sanitized measured snapshots in `accounts.extra` through an identity-checked repository update, then inject the service into account-monitor list and refresh paths. Bump the shared monitor projection to schema v2 and make the frontend and relay-ops consume the explicit multiplier value/source/status contract.

**Tech Stack:** Go, Ent/PostgreSQL JSONB, existing Sub2API HTTP/proxy/TLS helpers, Vue 3/TypeScript/Vitest, relay-ops Go service, Docker Compose.

## Global Constraints

- Change server-side project sources only; do not modify or deploy a separate local Sub2API checkout.
- Monitor only non-deleted accounts whose persisted state is `status=active` and `schedulable=true`.
- Do not change routing, account selection, account groups, prices, credentials, or scheduling state.
- Native `GET /v1/sub2api/billing` declarations take precedence.
- New API measurement is allowed only after declaration returns `404` or `405`.
- Automatic measurements are cached for 24 hours; seconds-level connectivity runs must reuse a fresh snapshot.
- A single-account manual refresh forces multiplier refresh; refresh-all measures only missing or expired snapshots.
- A multiplier failure must never change connectivity health, routing, or account state.
- Persist and expose no API keys, Base URLs, raw quota values, request bodies, response bodies, or raw upstream errors.
- relay-ops and Feishu may suggest that account B is better but must never switch an account or route.
- Do not stage `.codegraph/`, `upstream/sub2api/frontend/pnpm-workspace.yaml`, unrelated user-detail changes, or mechanical lockfile churn.

---

### Task 1: Define The Schema V2 Multiplier Contract

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Test: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- Produces: `AccountMonitorMultiplier{Value *float64, Source string, Status string, ObservedAt *time.Time}`
- Produces: `AccountMonitorSchemaVersion = 2`
- Removes: numeric fallback from `AccountMonitorAccount.Multiplier`

- [ ] **Step 1: Write failing projection tests**

Add table tests asserting that list output contains a declared snapshot, a measured snapshot, and unavailable/stale/failed states without falling back to `accounts.rate_multiplier`.

- [ ] **Step 2: Verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorService_List.*Multiplier' -count=1
```

Expected: compile or assertion failure because schema v2 multiplier fields do not exist.

- [ ] **Step 3: Add the minimal projection types**

Use this JSON shape:

```go
type AccountMonitorMultiplier struct {
	Value      *float64  `json:"value,omitempty"`
	Source     string    `json:"source,omitempty"`
	Status     string    `json:"status"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}
```

Embed it as `Multiplier AccountMonitorMultiplier 'json:"multiplier"'`, set schema version to `2`, and remove the `BillingRateMultiplier()` assignment.

- [ ] **Step 4: Verify GREEN**

Run the focused service tests and `go test ./internal/service -run AccountMonitor -count=1`.

- [ ] **Step 5: Commit**

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_types.go \
  upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git commit -m "feat: define account monitor multiplier projection"
```

### Task 2: Resolve Native Declarations

**Files:**
- Create: `upstream/sub2api/backend/internal/service/account_multiplier.go`
- Create: `upstream/sub2api/backend/internal/service/account_multiplier_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify generated provider call only: `upstream/sub2api/backend/cmd/server/wire_gen.go`

**Interfaces:**
- Produces: `AccountMultiplierService.Resolve(*Account, time.Time) AccountMonitorMultiplier`
- Consumes: `decodeUpstreamBillingProbeSnapshot(account.Extra)`
- Rule: fresh `effective_rate_multiplier`, then fresh `resolved_rate_multiplier`, becomes `source=declared,status=ok`

- [ ] **Step 1: Write failing declaration tests**

Cover fresh effective multiplier, resolved fallback, expired declaration, unsupported declaration, malformed/non-finite data, and declaration precedence over a measured snapshot.

- [ ] **Step 2: Verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMultiplier.*Declared' -count=1
```

- [ ] **Step 3: Implement sanitized resolution**

Decode only known fields from `UpstreamBillingProbeExtraKey`; require a positive finite value and `FreshUntil > now`. Return `stale`, `unsupported`, `failed`, or `unavailable` without copying `LastError`.

- [ ] **Step 4: Inject the resolver**

Extend `NewAccountMonitorService` with the multiplier service and set each row from `Resolve`. Update Wire providers and regenerate or minimally update `wire_gen.go` consistently.

- [ ] **Step 5: Verify and commit**

Run focused multiplier/account-monitor tests, then commit:

```bash
git add upstream/sub2api/backend/internal/service/account_multiplier.go \
  upstream/sub2api/backend/internal/service/account_multiplier_test.go \
  upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/wire.go \
  upstream/sub2api/backend/cmd/server/wire_gen.go
git commit -m "feat: project declared upstream account multipliers"
```

### Task 3: Measure And Persist New API Multipliers

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_multiplier.go`
- Modify: `upstream/sub2api/backend/internal/service/account_multiplier_test.go`
- Modify: `upstream/sub2api/backend/internal/service/account.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_repo.go`
- Create: `upstream/sub2api/backend/internal/repository/account_repo_account_multiplier_cas_test.go`

**Interfaces:**
- Produces: `AccountMultiplierMeasurementSnapshot` under `extra.account_monitor_multiplier_measurement`
- Produces: `Refresh(ctx context.Context, account *Account, force bool) error`
- Repository contract: `UpdateAccountMultiplierMeasurement(ctx, expected *Account, snapshot *AccountMultiplierMeasurementSnapshot) error`
- Measurement: three valid samples, median multiplier, maximum relative spread at or below `0.15`

- [ ] **Step 1: Write failing pure-calculation tests**

Test quota conversion `quotaDelta/quotaPerUnit`, official token cost through `BillingService.CalculateCost(model, usage, 1)`, median selection, rejection of zero/negative/non-finite delta, missing usage, unknown pricing, and spread above `15%`.

- [ ] **Step 2: Verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMultiplier.*(Measurement|Sample|Median|Spread)' -count=1
```

- [ ] **Step 3: Write failing HTTP capability tests**

With `httptest.Server`, assert:

```text
GET /api/usage/token/ -> usage snapshot
GET /api/status -> quota_per_unit
POST /v1/chat/completions -> deterministic non-stream response with usage
```

Also verify proxy/header/model mapping reuse, 404/403/malformed endpoint rejection, bounded response bodies, and secret-free persisted snapshots.

- [ ] **Step 4: Implement measurement**

Use the account test transport and URL safety checks already used by upstream billing probes. Send a fixed non-streaming request with the mapped monitor model, `temperature=0`, and a small output limit. Persist only:

```go
type AccountMultiplierMeasurementSnapshot struct {
	Version      int        `json:"version"`
	Status       string     `json:"status"`
	Source       string     `json:"source"`
	Value        *float64   `json:"value,omitempty"`
	ModelID      string     `json:"model_id,omitempty"`
	SampleCount  int        `json:"sample_count,omitempty"`
	RelativeSpread *float64 `json:"relative_spread,omitempty"`
	ObservedAt   *time.Time `json:"observed_at,omitempty"`
	FreshUntil   *time.Time `json:"fresh_until,omitempty"`
	LastAttemptAt time.Time `json:"last_attempt_at"`
}
```

- [ ] **Step 5: Add identity-checked CAS persistence**

Match account ID, platform, type, credentials JSON, proxy ID, prior measurement JSON, and non-deleted state before updating only the dedicated `extra` key. Return the existing identity-changed conflict when no row matches.

- [ ] **Step 6: Verify and commit**

Run focused service and repository tests, then commit:

```bash
git add upstream/sub2api/backend/internal/service/account_multiplier.go \
  upstream/sub2api/backend/internal/service/account_multiplier_test.go \
  upstream/sub2api/backend/internal/service/account.go \
  upstream/sub2api/backend/internal/repository/account_repo.go \
  upstream/sub2api/backend/internal/repository/account_repo_account_multiplier_cas_test.go
git commit -m "feat: measure new api account multipliers"
```

### Task 4: Apply Refresh Semantics Without Affecting Health

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`

**Interfaces:**
- `RunAll`: connectivity for every pool account; multiplier `Refresh(force=false)`
- `RunOne`: connectivity for target account; multiplier `Refresh(force=true)`
- Errors: log/retain multiplier status but never replace `AccountMonitorProbeResult`

- [ ] **Step 1: Write failing refresh tests**

Assert that run-all skips fresh measured snapshots, refreshes missing/expired snapshots, run-one always forces refresh, and multiplier errors still persist and return the connectivity result.

- [ ] **Step 2: Verify RED**

```bash
cd upstream/sub2api/backend
go test ./internal/service -run 'TestAccountMonitorService_Run(All|One).*Multiplier' -count=1
```

- [ ] **Step 3: Implement refresh orchestration**

Call multiplier refresh beside each connectivity probe with bounded contexts. Preserve run concurrency limits and insert connectivity history regardless of multiplier outcome.

- [ ] **Step 4: Verify and commit**

Run all account-monitor and multiplier service tests, then commit:

```bash
git add upstream/sub2api/backend/internal/service/account_monitor_service.go \
  upstream/sub2api/backend/internal/service/account_monitor_service_test.go
git commit -m "feat: refresh monitored account multipliers"
```

### Task 5: Add Admin Multiplier States And Group Filter

**Files:**
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.vue`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`

**Interfaces:**
- Consumes schema v2 `multiplier: {value,source,status,observed_at}`
- Produces filter model `groupID: '' | string`
- Group options preserve `{id,name}` association and sort by name then ID

- [ ] **Step 1: Write failing card tests**

Cover `0.07x 声明`, `0.25x 测算`, and labels for stale, unsupported, failed, and unavailable. Confirm `1.00x` is not rendered when no trusted value exists.

- [ ] **Step 2: Verify RED**

```bash
cd upstream/sub2api/frontend
pnpm exec vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts
```

- [ ] **Step 3: Implement card states**

Render one current multiplier, a compact source label, and localized state text. Keep existing card actions and dimensions stable.

- [ ] **Step 4: Write failing filter/view tests**

Assert all-groups plus unique current-projection groups, name/ID ordering, multi-group membership, and composition with search/platform/status.

- [ ] **Step 5: Verify RED and implement filter**

Add a group select to `AccountMonitorFilters`, bind it in the view, and filter with `account.group_ids.includes(Number(groupID))`.

- [ ] **Step 6: Verify and commit**

Run the three focused Vitest files and frontend typecheck, then commit only intentional source/test/i18n files:

```bash
git commit -m "feat: filter account monitor by group"
```

### Task 6: Make Relay-Ops Consume Schema V2 Fail-Closed

**Files:**
- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`
- Modify: `relay-ops-service/internal/accountrecommendation/service.go`
- Modify: `relay-ops-service/internal/accountrecommendation/service_test.go`

**Interfaces:**
- Consumes only `schema_version=2`
- Uses multiplier only when `status=ok`, source is `declared` or `measured`, value is positive/finite, and observed time is within policy
- Missing multiplier evidence cannot create a “lower multiplier” reason or an unsupported recommendation

- [ ] **Step 1: Write failing parser and recommendation tests**

Cover schema v2 parsing, schema v1 rejection, fresh valid multipliers, stale/missing/failed values, and absence of false “倍率更低” reasons.

- [ ] **Step 2: Verify RED**

```bash
cd relay-ops-service
go test ./internal/sub2api ./internal/accountrecommendation -count=1
```

- [ ] **Step 3: Implement schema v2 consumption**

Replace numeric comparisons with an explicit helper returning `(float64, bool)`. Keep recommendation mode read-only and command mode dry-run.

- [ ] **Step 4: Verify and commit**

Run `go test ./...`, then commit:

```bash
git add relay-ops-service/internal/sub2api/types.go \
  relay-ops-service/internal/sub2api/client_test.go \
  relay-ops-service/internal/accountrecommendation/service.go \
  relay-ops-service/internal/accountrecommendation/service_test.go
git commit -m "feat: consume trusted account multipliers"
```

### Task 7: Full Verification, Production Deployment, And Main Merge

**Files:**
- Create: `docs/superpowers/reports/2026-07-26-account-monitor-multiplier-verification.md`
- Modify only existing server deployment metadata required by the repository workflow.

- [ ] **Step 1: Run full verification**

```bash
(cd upstream/sub2api/backend && go test ./...)
(cd upstream/sub2api/frontend && pnpm exec vitest run && pnpm type-check && pnpm build)
(cd relay-ops-service && go test ./...)
(cd sub2api-updater && go test ./...)
(cd internal-test-service && go test ./...)
```

- [ ] **Step 2: Review the diff and secret surface**

Check `git diff main...HEAD`, search monitor JSON fixtures and production API output for credential/Base URL fields, and confirm unrelated dirty files remain untouched.

- [ ] **Step 3: Build and deploy the server image**

Build for `linux/amd64`, transfer/deploy through the repository's existing production workflow, preserve current env values, and verify container health before replacing the prior image tag.

- [ ] **Step 4: Run production acceptance**

Verify:

```text
12 expected active+schedulable account IDs only
native declarations show declared values
New API accounts show measured values only after valid three-sample runs
single-account refresh forces multiplier refresh
refresh-all reuses values younger than 24 hours
group filtering works in the admin browser
relay-ops remains read_only and Feishu command mode remains dry_run
no route/account/group mutation occurs
```

- [ ] **Step 5: Record evidence and commit**

Document image tag, commit SHA, account results, rejected measurements, browser/API checks, and test output in the verification report.

- [ ] **Step 6: Merge and push main**

Fetch `origin`, update local `main` without discarding unrelated work, merge the completed feature branch, rerun critical tests on `main`, then:

```bash
git push origin main
git rev-parse main
git rev-parse origin/main
git ls-remote origin refs/heads/main
```

All three SHAs must match before completion is reported.

## Acceptance Criteria

- The admin monitor never presents local `accounts.rate_multiplier` as the upstream multiplier.
- Trusted native declarations and valid New API measurements are visibly distinguished.
- Measurement charging is bounded to missing/expired snapshots or an explicit single-account refresh.
- Group filtering composes correctly with all existing filters.
- relay-ops recommendations fail closed when multiplier evidence is absent and never mutate routing.
- Production runs the verified image and remote `main` exactly matches the merged local `main`.
