# T27 READY_FOR_ROOT_REVIEW Handoff

## Candidate

- Refreshed baseline: `origin/main@73369873e402133695e0a3c3f5567536b405991d`
- Branch: `codex/t27-procurement-oauth-position-fix`
- State: `READY_FOR_ROOT_REVIEW`
- Merge, push, deployment, and cleanup: not performed

## Delivered

- `cost_pending` procurement versions use nullable scans; NULL prior cost/quota do not enter old-consumption allocation, while the existing transaction, idempotency, version closure, actor audit, and account projection remain intact.
- Self-purchased reporting, legacy projection, aggregation, and settlement require literal `accounts.type='oauth'` plus existing procurement ledger/projection ownership.
- Profitability UI uses primary USD/CNY views with shared today/24h/7d/31d selection, current-view-only loading/refresh, isolated errors, stale-range reload protection, USD exclusion wording, and a seven-metric CNY summary.
- Self-purchased API accepts compatible `range`; handler applies AccountFinancial-equivalent Beijing boundaries while preserving explicit date and legacy no-parameter behavior.
- The 390px page remains contained; the CNY long table owns its horizontal scrolling.

## Verification

- `go test ./internal/service -run 'Test(SelfPurchasedReport|UpdateProcurementConfig|SettleProcurement)'`: passed.
- `go test ./internal/handler/admin -run 'TestDashboardHandler(SelfPurchasedRange|AccountProfitability|Procurement)'`: passed.
- `pnpm exec vitest run src/api/admin/__tests__/selfPurchasedProfitability.spec.ts src/views/admin/__tests__/AccountProfitabilityView.spec.ts`: 2 files, 23 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm run build`: passed; existing pnpm-field, Browserslist, and Vite dynamic/static import warnings remain.
- `go build ./cmd/server`: passed.
- `git diff --check`: passed.

## Release Notes

- Migrations: none.
- Configuration: none.
- Historical backfill or production data writes: none.
- Expected `downtime_required=false`; root release preflight remains authoritative.
- Rollback: revert the T27 candidate commits; no data rollback is required.

## Not Verified And Residual Risk

- No production deployment, authenticated browser acceptance, or live database sample mutation was performed in this worktree.
- Time-window correctness is covered with fixed-time handler tests; production clock/timezone configuration remains a release-time observation.
- Frontend range/view state is covered by Vitest, including today -> USD -> 7d -> CNY stale-data prevention; final rendered behavior still requires root online acceptance after deployment.
