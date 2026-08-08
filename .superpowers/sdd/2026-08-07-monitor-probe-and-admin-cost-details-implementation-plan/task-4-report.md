# Task 4 implementation report

Status: DONE

Implemented:
- Preserved the existing `UsageDetailDialog` sections, wide dialog, two-column detail grids, and visual vocabulary while replacing the administrator summary with site charge, counted cost, and gross margin/pending values.
- Added administrator-only local/upstream request ID rows. `upstream_request_id` is typed only on `AdminUsageLog`; `UserUsageDetail` remains free of upstream, account, cost-evidence, and profit fields.
- Added an isolated relay-ops request-cost API call using the exact local request ID and `skipSessionRecovery: true`. Missing/404/unavailable evidence does not fail the usage-log detail; it stays pending reconciliation.
- Added native cost rows and localized evidence badges for confirmed native ledger cost, upstream price-table estimates, owned-account allocation, and pending reconciliation.
- Added explicit evidence projection rules: confirmed uses `upstream_actual_cost`; estimated uses `upstream_standard_cost`; pending uses neither. Confirmed and estimated gross margin are `site actual charge - counted upstream cost`, with estimated margin explicitly labeled. The obsolete site-price-derived account-cost display/helper was removed so it cannot replace native actual evidence.

Files:
- `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`
- `upstream/sub2api/frontend/src/components/usage/usageDetail.ts`
- `upstream/sub2api/frontend/src/components/usage/__tests__/UsageDetailDialog.spec.ts`
- `upstream/sub2api/frontend/src/components/usage/__tests__/usageDetail.spec.ts`
- `upstream/sub2api/frontend/src/api/admin/usage.ts`
- `upstream/sub2api/frontend/src/api/__tests__/admin.usage.spec.ts`
- `upstream/sub2api/frontend/src/types/index.ts`
- `upstream/sub2api/frontend/src/i18n/locales/en/admin/index.ts`
- `upstream/sub2api/frontend/src/i18n/locales/zh/admin/index.ts`

TDD evidence:
- RED: `npm run test:run -- src/components/usage/__tests__/UsageDetailDialog.spec.ts` failed 4 new cases because request-cost evidence was never queried and the confirmed/estimated/pending/request-ID rows did not exist.
- RED: `npm run test:run -- src/components/usage/__tests__/usageDetail.spec.ts` failed 3 new projection cases because the evidence-state, counted-cost, actual-cost, and gross-margin helpers did not exist.
- GREEN/final focused run: `npm run test:run -- src/api/__tests__/admin.usage.spec.ts src/components/usage/__tests__/usageDetail.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts` passed 33/33 tests.

Verification:
- Focused ESLint on all changed frontend source/test files: passed.
- `npm run typecheck`: passed.
- `npm run build`: passed; Vite emitted the repository's existing browserslist, mixed dynamic/static import, and chunk-size warnings.
- `git diff --check`: passed before report creation.

Concerns:
- No production visual or live relay-ops API verification was performed in this task; those remain Task 5 gates. The dialog intentionally treats request-cost 404/auth/service failures as `待对账` instead of fabricating a cost or failing the base usage detail.
