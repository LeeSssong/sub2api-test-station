# Account Monitor V3 final fix wave report

Date: 2026-08-04

## Scope

This wave implements only the three fixes in `final-fix-brief.md`:

1. Native group-tab ordering uses `rate_multiplier` descending, then `native_order` ascending, then group ID ascending.
2. Existing-account single and bulk priority updates reject values below `1` before invoking the service layer.
3. Native upstream multiplier cost remains eligible when same-window real usage cost is zero; procurement-mode zero-base cost remains ineligible.

No push, deployment, production access, or production-state mutation was performed. `docs/project/project-progress.md` remains **进行中**.

## Root-cause evidence

- `AccountMonitorView.vue` previously sorted only by native order and ID, so a lower-multiplier group could precede a higher-multiplier group.
- Service-level lower-bound checks existed, but the HTTP update and bulk-update handlers accepted `0` and negative priorities and forwarded them to the service. Boundary validation is now explicit in both handlers.
- Multiplier-mode cost calculation previously treated `baseCost <= 0` as ineligible. That incorrectly discarded native multiplier evidence when usage was zero. The procurement-mode guard remains unchanged.

## Changes

- `upstream/sub2api/frontend/src/views/admin/AccountMonitorView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/AccountMonitorView.spec.ts`
- `upstream/sub2api/backend/internal/handler/admin/account_handler.go`
- `upstream/sub2api/backend/internal/handler/admin/account_handler_procurement_cost_test.go`
- `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- `docs/project/project-progress.md`
- `.superpowers/sdd/2026-08-04-account-monitor-card-production-implementation-plan/progress.md`

## TDD evidence

Recorded RED checks from the investigation:

- Before the frontend comparator fix, the focused group-tab test received low/native-order-first ordering instead of multiplier-first ordering.
- Before handler guards, update and bulk-update requests with priorities `0` and `-1` returned HTTP 200 instead of HTTP 400 and reached the service stub.
- Reintroducing the historical multiplier `baseCost <= 0` guard caused the native-multiplier/no-window-cost test to fail because the effective multiplier was discarded.

## GREEN verification

Fresh verification on implementation commit `741f6206f`:

```text
cd upstream/sub2api/frontend
pnpm vitest run src/views/admin/__tests__/AccountMonitorView.spec.ts
15 tests passed

pnpm typecheck
vue-tsc --noEmit passed

cd upstream/sub2api/backend
go test ./internal/service ./internal/handler/admin
both packages passed

git diff --check
passed
```

The focused backend tests for handler priority boundaries and account-monitor cost/ranking semantics also passed during the fix wave, as did the broader affected-package test selections.

## Commits

- `e5fdbed68f0f4401db5d8b9ae1cf8e23c26c9689` — backend evidence, priority service guards, and multiplier-mode cost eligibility correction.
- `741f6206f` — frontend ordering, HTTP boundary regression tests, adjusted ranking expectation, progress ledger, and project-progress update.

## Concerns and remaining gates

- No push, deployment, production access, or online validation was performed; production acceptance remains outstanding.
- Existing non-failing warnings remain: Browserslist data age, pnpm package-field warning, and Node localStorage experimental warning.
- Overall project status must remain **进行中** until the required push, deployment, and online verification gates are completed.
