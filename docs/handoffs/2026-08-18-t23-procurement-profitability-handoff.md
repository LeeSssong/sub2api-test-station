# T23 handoff — self-purchased procurement profitability

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t23-procurement-profitability`
- Branch: `codex/t23-procurement-profitability`
- Baseline main: `7b8367942ca9e6efccb706ca623c186103cd1c13`
- State: `READY_FOR_ROOT_REVIEW` (not merged/pushed/deployed)

## Delivered
- Expand-only migration `226_account_procurement_cost_versions.sql` with version, effective/ended/settled timestamps, loss, actor/request id, active/request uniqueness.
- Independent CNY report service/API: `GET /admin/operations/self-purchased-profitability`.
- Atomic procurement config endpoint: `PUT /admin/operations/accounts/:id/procurement`; first version effective at account.created_at, later versions at update time; projection update in same transaction; audit row appended.
- Idempotent settlement endpoint: `POST /admin/operations/accounts/:id/procurement/settle`; settlement computes loss_cny capped by standard `usage_logs.total_cost` (never account_cost) and appends audit.
- Frontend self-purchased CNY panel with cost pending state, metrics, table, and confirmation settlement action; channel USD view unchanged.
- Formal spec/plan under `docs/superpowers/specs/2026-08-18-t23-procurement-profitability-design.md` and `docs/superpowers/plans/2026-08-18-t23-procurement-profitability-plan.md`.

## Verification
- `go test ./internal/service ./internal/handler/admin ./migrations -run 'Procurement|SelfPurchased|AccountProfitability' -count=1` — pass.
- `pnpm vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts` — 16/16 pass.
- `pnpm run typecheck` — pass.
- `pnpm run build` — pass.
- `git diff --check` — pass.

## Migration/config/downtime
- Migration-only schema addition; no historical backfill or usage_logs mutation.
- No config changes. Expected `downtime_required=false`; root preflight remains authoritative.

## Remaining risks / unverified
- No fresh PostgreSQL concurrent integration test in this candidate; SQL transactions/unique indexes are covered structurally and by sqlmock-free unit paths only.
- Existing legacy account update endpoint does not call the new procurement endpoint automatically; UI can use the dedicated endpoint when wired by root integration.
- Browser screenshot/390px visual smoke not run; layout uses responsive grid and overflow wrapper.
- Production deployment/online verification intentionally not performed.

## Rollback
Revert candidate commit and omit migration 226 from release; no destructive migration or data rewrite.
