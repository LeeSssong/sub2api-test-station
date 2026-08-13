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

## Source projection fix (post-review)

- Status: local fix verified; pending fresh scoped re-review. This fixes the single Important finding in `task-7-review.md` and does not start Task 8.
- Root cause: the exception API already projects persisted `source`, but `AdminUsageCostException` omitted it and `CostExceptionTable` inferred provenance from optional NewAPI quota fields. An `unavailable` NewAPI row is allowed to have both quota values empty, which the inference incorrectly rendered as Sub.
- RED: `CostExceptionTable.spec.ts` added a `source: 'newapi'`, `evidence_status: 'unavailable'` fixture with both quota fields empty, asserted the rendered provenance is `newapi`, then ran `pnpm test:run src/components/admin/usage/__tests__/CostExceptionTable.spec.ts`. It failed as expected: the received row text contained `Sub` and did not contain `NewAPI`/`newapi`.
- GREEN: `AdminUsageCostException.source` now accepts the persisted API value; table rendering and CSV export use `item.source` directly and remove quota-based inference. The regression test also asserts exported CSV content retains `newapi` with empty quota fields.
- Fresh verification:

  ```bash
  cd upstream/sub2api/frontend
  pnpm test:run src/api/__tests__/admin.usage.spec.ts src/components/admin/usage/__tests__/CostExceptionTable.spec.ts src/views/admin/__tests__/UsageView.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/usageDetail.spec.ts
  pnpm typecheck
  pnpm build
  git diff --check
  ```

  Result: PASS — 5 files / 55 tests; typecheck and production build exit 0; diff check clean. The production build retains pre-existing Browserslist, dynamic-import/chunk-size warnings only.
- Scope: only `frontend/src/types/index.ts`, `CostExceptionTable.vue`, and its test. No backend, schema, migration, local evidence detail guessing, upstream HTTP, main, production, deploy, push, or stash mutation.

## Local detail source follow-up

- The task contract also requires the persisted source to be consistent in local evidence detail. The backend local evidence DTO already contains `source`; no backend change was needed.
- RED: the administrator detail fixture was changed to `source: 'newapi'`, `evidence_status: 'unavailable'`, empty normalized cost, then the focused detail test was run. Vitest failed with `missing detail label: admin.usageCostDetail.costSource`, proving the field was not projected/rendered.
- GREEN: `UsageCostEvidenceDetail` now accepts optional `source`, and `UsageDetailDialog` renders that exact persisted value for administrators. It does not infer provenance from quota or cost fields.
- Verification after both source fixes: target RED test 1/1 passed; the five Task 7 focused files passed 55/55; `pnpm typecheck`, `pnpm build`, and `git diff --check` passed. Build output contained only the repository's pre-existing pnpm/localStorage/Browserslist/dynamic-import/chunk-size warnings.
- Commit: `aa5c1ed2b` (`fix: show persisted cost evidence source`). No user-scope rendering, backend, schema, migration, main, production, deploy, push, or Task 8 change.
