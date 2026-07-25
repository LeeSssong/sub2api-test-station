# Account Monitor Multiplier And Group Filter Implementation Plan

**Goal:** Replace the account monitor's local billing multiplier with a safe
upstream declaration or New API measurement, and add composable group
filtering to the administrator page.

**Branch:** `codex/account-monitor-multiplier`

**Workspace:** `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/account-monitor-multiplier`

**Constraints:**

- Work only in the isolated worktree.
- Do not merge, deploy, or mutate production data.
- Keep connectivity results independent from multiplier failures.
- Never expose credentials, Base URLs, raw quota values, request/response
  bodies, or upstream error text in the projection.
- Use tests before implementation for every behavioral change.

## Task 1: Define The Multiplier Snapshot And Projection Contract

**Files:**

- Modify: `upstream/sub2api/backend/internal/service/account_monitor_types.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/frontend/src/api/admin/accountMonitor.ts`

1. Add failing service tests for declared, measured, stale, unsupported,
   failed, and unavailable projection states.
2. Assert that the projection contains only value, source, status, and
   observed timestamp, and that the old local `BillingRateMultiplier` is not
   used.
3. Introduce `AccountMonitorMultiplier`, bump the projection schema version,
   and implement a resolver that prefers a fresh valid native declaration over
   a fresh valid measured snapshot.
4. Keep stale values non-authoritative: report the stale status without a
   usable multiplier value.
5. Update the TypeScript contract and run focused Go tests.

## Task 2: Implement New API Quota Parsing And Measurement Math

**Files:**

- Create: `upstream/sub2api/backend/internal/service/upstream_multiplier_measurement.go`
- Create: `upstream/sub2api/backend/internal/service/upstream_multiplier_measurement_test.go`
- Modify: `upstream/sub2api/backend/internal/service/billing_service.go` only if
  a small reusable official-cost helper is required

1. Add failing table tests for `/api/usage/token/` response variants and
   `/api/status` `quota_per_unit` parsing.
2. Add failing tests for positive quota conversion, official model cost,
   zero/negative/non-finite deltas, missing usage, and unknown pricing.
3. Add failing tests for three-sample median calculation and relative-spread
   rejection.
4. Implement strict bounded parsers and pure measurement math.
5. Reuse `BillingService.CalculateCost(..., 1)` for official cost and reject
   any request whose official cost is not positive and finite.
6. Run the focused measurement test suite.

## Task 3: Add Controlled New API Measurement And Safe Persistence

**Files:**

- Modify: `upstream/sub2api/backend/internal/service/upstream_multiplier_measurement.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_multiplier_measurement_test.go`
- Modify: `upstream/sub2api/backend/internal/repository/account.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_repo.go`
- Modify: `upstream/sub2api/backend/internal/repository/account_repo_test.go`
  or the closest focused repository test

1. Add failing HTTP tests proving the request sequence is quota, status,
   bounded non-streaming completion, quota for each sample.
2. Assert requests use the account's validated Base URL, API key, proxy, TLS
   profile, header overrides, and mapped text model without persisting any of
   those values.
3. Add failing tests for timeout, response-size limit, missing usage,
   contaminated samples, and unsupported capability.
4. Add a versioned measured snapshot under a dedicated account `extra` key.
5. Add an optimistic repository update that compares the loaded account
   identity before writing; return the established identity-changed error on a
   mismatch.
6. Persist only source, status, multiplier, model, sample count, relative
   spread, observed time, and fresh-until time.
7. Run focused service and repository tests.

## Task 4: Integrate Refresh Policy With Account Monitor Runs

**Files:**

- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- Modify: `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_billing_probe.go`
- Modify: `upstream/sub2api/backend/internal/service/upstream_billing_probe_test.go`
- Modify: `upstream/sub2api/backend/internal/service/wire.go`
- Modify/regenerate: `upstream/sub2api/backend/cmd/server/wire_gen.go`

1. Add failing tests that a fresh declaration prevents measurement.
2. Add failing tests that a declaration `404` or `405` enables New API
   measurement, while other declaration failures remain failed.
3. Add failing tests that scheduled refresh-all reuses a measured snapshot
   younger than 24 hours and refreshes an absent/stale snapshot.
4. Add failing tests that card-level `RunOne` forces declaration and fallback
   measurement refresh.
5. Add failing tests proving multiplier failures do not alter successful
   connectivity results or prevent their persistence.
6. Wire the multiplier refresher into `AccountMonitorService`; execute it
   independently from the connectivity probe.
7. Run focused service, handler, route, and wire compilation tests.

## Task 5: Bump And Harden The Relay-Ops Projection Consumer

**Files:**

- Modify: `relay-ops-service/internal/sub2api/types.go`
- Modify: `relay-ops-service/internal/sub2api/client.go`
- Modify: `relay-ops-service/internal/sub2api/client_test.go`
- Modify: account recommendation or daily-report tests that construct the
  monitor projection

1. Add failing tests for the new schema and multiplier object.
2. Reject old/unknown schema versions, non-finite/negative values, invalid
   source/status pairs, and stale/absent evidence.
3. Adapt recommendation input so only `status=ok` multiplier evidence is
   consumed.
4. Preserve fail-closed behavior when evidence is unavailable.
5. Run `go test ./internal/sub2api ./internal/accountrecommendation
   ./internal/dailyreport -count=1`.

## Task 6: Render Multiplier States In The Administrator Card

**Files:**

- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.vue`
- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorCard.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/accounts.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/accounts.ts`

1. Add failing component tests for declared, measured, stale, unsupported,
   failed, and unavailable states.
2. Assert the old local numeric multiplier is never rendered.
3. Render a stable multiplier metric with compact source text for valid values
   and explicit state text otherwise.
4. Keep the existing card dimensions and actions stable.
5. Run focused Vitest, ESLint, and TypeScript checks.

## Task 7: Add Composable Group Filtering

**Files:**

- Modify: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.vue`
- Create: `upstream/sub2api/frontend/src/components/admin/account-monitor/AccountMonitorFilters.spec.ts`
- Modify: `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- Modify: `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/zh/admin/accounts.ts`
- Modify: `upstream/sub2api/frontend/src/i18n/locales/en/admin/accounts.ts`

1. Add failing tests for unique stable group option derivation.
2. Add failing tests for multi-group membership and composition with search,
   platform, and status filters.
3. Add a single-select group control with an all-groups option.
4. Match by `group_ids`, using `group_names` only as display labels and a
   deterministic ID fallback when names are absent.
5. Run focused view/filter tests, ESLint, and TypeScript checks.

## Task 8: Verify, Review, And Commit

1. Run focused backend tests:

   ```bash
   cd upstream/sub2api/backend
   go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -count=1
   ```

2. Run relay-ops:

   ```bash
   cd relay-ops-service
   go test ./... -count=1
   ```

3. Run frontend verification:

   ```bash
   cd upstream/sub2api/frontend
   pnpm test:run
   pnpm typecheck
   pnpm lint:check
   pnpm build
   ```

4. Inspect the projection JSON tests for secret-free output and confirm no
   production endpoints, deployment manifests, or account data changed.
5. Review `git diff --check`, staged file scope, and branch history.
6. Remove the temporary untracked `frontend/node_modules` symlink.
7. Commit only the feature files on `codex/account-monitor-multiplier`.
8. Report the worktree path, branch, commit, and verification evidence. Do not
   merge, deploy, or push `main`.
