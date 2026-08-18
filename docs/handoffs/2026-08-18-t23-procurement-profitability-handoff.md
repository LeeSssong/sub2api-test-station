# T23 handoff — self-purchased procurement profitability

- Worktree: `/Users/gongtengxinwen/.codex/worktrees/1dc8/sub2api搭建`
- Branch: `codex/t23-procurement-profitability-refresh`
- Baseline main: `ff1f3434d422321a1bb3d140a9ee2f2696d66de7`
- Refreshed commits: `85832074d`, `6cdad5080`, `ea02997d2`, `eed016632`, `78a29c1f0`
- State: `READY_FOR_ROOT_REVIEW` (not merged/pushed/deployed)

## Delivered
- Expand-only migration `226_account_procurement_cost_versions.sql` with version, effective/ended/settled timestamps, loss, actor/request id, active/request uniqueness.
- Independent CNY report service/API: `GET /admin/operations/self-purchased-profitability`.
- Procurement ownership comes only from the ledger/current procurement projection; ordinary OAuth accounts are not inferred as self-purchased.
- Atomic procurement config endpoint: `PUT /admin/operations/accounts/:id/procurement`; first version effective at `account.created_at`, later versions store remaining cost/quota at update time; projection and audit update in the same SQL transaction. The existing account-monitor edit route delegates procurement changes to this service with auth actor and idempotency key.
- Idempotent settlement endpoint: `POST /admin/operations/accounts/:id/procurement/settle`; only disabled/error/expired accounts can settle, the operation persists fixed `loss_cny`, serializes on the latest version, and appends actor audit.
- Report SQL scans timestamps into native time values, applies each version's own effective interval and cap, reads settled loss from the ledger, and limits standard cost to complete successful positive-token rows while excluding media, per-request and Cyber rows.
- Legacy projection fallback treats either populated procurement field as explicit ownership; a partial legacy projection is returned as `cost_pending` instead of being silently omitted.
- Frontend self-purchased CNY panel renders every required field separately, preserves zero pending cost, contains wide tables within their scroll wrapper, and requires explicit confirmation before settlement; channel USD view unchanged.
- Formal spec/plan under `docs/superpowers/specs/2026-08-18-t23-procurement-profitability-design.md` and `docs/superpowers/plans/2026-08-18-t23-procurement-profitability-plan.md`.

## Verification
- `go test ./internal/service ./internal/handler/admin ./internal/server/routes ./migrations -run 'Procurement|SelfPurchased|AccountProfitability' -count=1` — pass.
- `go test ./internal/handler ./cmd/server -run '^$' -count=1` — pass.
- `pnpm vitest run src/views/admin/__tests__/AccountProfitabilityView.spec.ts` — 18/18 pass, including desktop/390px layout contract assertions.
- `pnpm run typecheck` — pass.
- `pnpm run build` — pass.
- `git diff --check 22cf4981d...HEAD` — pass.
- Migration 226 expand-only guard — pass; no existing migration changed.

## Migration/config/downtime
- Migration-only schema addition; no historical backfill or usage_logs mutation.
- No config changes. Expected `downtime_required=false`; root preflight remains authoritative.

## Remaining risks / unverified
- No fresh PostgreSQL concurrent integration test in this candidate; SQL transactions/unique indexes are covered structurally and by sqlmock-free unit paths only.
- Browser screenshot/390px visual smoke not run because this isolated worktree has no authenticated backend session; jsdom layout contract coverage is present, but it is not a substitute for a real screenshot.
- Production deployment/online verification intentionally not performed.

## Rollback
Revert candidate commit and omit migration 226 from release; no destructive migration or data rewrite.
