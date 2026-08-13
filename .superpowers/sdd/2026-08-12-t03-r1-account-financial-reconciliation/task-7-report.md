# T03-R1 Task 7 Report

- Status: local implementation and verification complete; pending independent task review. Not merged, pushed, deployed, or online-verified.
- Baseline: `4cc1d8bb825c5e2f266ffaec1f0bcedb6b74adb3`.
- Scope: administrator-only UsageView `cost-exceptions` tab; local evidence detail; exception filtering, pagination, selection, review actions, cutoff/count disclosure, route restoration, and exception CSV export.

## RED / GREEN evidence

- Restored-draft baseline command was run first:

  ```bash
  cd upstream/sub2api/frontend
  pnpm test:run src/api/__tests__/admin.usage.spec.ts src/components/admin/usage/__tests__/CostExceptionTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts
  ```

  It was already GREEN (4 files / 46 tests), so no honest RED claim is possible for the recovered draft.
- A new local-evidence/no-legacy-client contract and exception-export contract were added. Their focused RED run failed as expected because the legacy `getUpstreamCost` client still existed and the export control was absent.
- GREEN verification:

  ```bash
  pnpm test:run src/api/__tests__/admin.usage.spec.ts src/components/admin/usage/__tests__/CostExceptionTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/usageDetail.spec.ts
  pnpm typecheck
  pnpm build
  git diff --check
  ```

  Result: PASS — 5 files / 55 tests; typecheck and production build exit 0; diff check clean.

## Changed files

- `upstream/sub2api/frontend/src/api/admin/usage.ts` and its API test: local evidence and exception review/list contracts; removed retained legacy `getUpstreamCost` frontend client.
- `upstream/sub2api/frontend/src/components/admin/usage/CostExceptionTable.vue` and test: exception rows, filters, pagination, selection, one/selected/filtered review actions, server cutoff/count feedback, refetch, and paginated CSV export using the same exception filter.
- `upstream/sub2api/frontend/src/views/admin/UsageView.vue` and test: `cost-exceptions` tab, query restoration for `tab/range/account/evidence/review`, shared date/account filters, and existing admin detail opening.
- `upstream/sub2api/frontend/src/components/usage/UsageDetailDialog.vue`, types, helpers, locale modules, and tests: administrator detail reads only the local evidence/review projection; ordinary-user narrowing remains intact.
- `docs/project/project-progress.md`: Task 7 state recorded as locally verified but still in progress until review/release closure.

## Risks / concerns

- The reviewed backend accepts `max_usage_log_id: 0` and calculates/returns the server cutoff atomically; the UI displays the returned cutoff/counts and refetches afterwards. Independent review should confirm this is the intended server-side freeze contract.
- The retained backend path name remains `/admin/usage/:id/upstream-cost`, but this Task 7 frontend makes no upstream source request: it consumes the local persisted evidence projection through `getCostEvidence` and removes the legacy client/type/helpers.
- No backend, schema, migration, generated source, main, production, deploy, push, or GitHub Actions changes were made. `downtime_required` remains unassessed until root preflight.
