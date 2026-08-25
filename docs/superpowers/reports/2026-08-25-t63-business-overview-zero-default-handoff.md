# T63 Business Overview Zero-Default Handoff

Status: `READY_FOR_ROOT_REVIEW`

## Scope

The admin business overview now uses native `usage_logs.actual_cost` for revenue and trend consumption, keeps the approved effective account-cost expression for upstream cost, excludes `usage_completeness='unknown'`, and converts missing/non-finite numeric values to zero. Profit and margin always calculate; zero revenue produces a `0.00%` margin. Missing current-period recharge/wallet data is projected as zero, while non-schema query failures remain errors.

The frontend no longer renders a pending banner or “待确认”/“口径待确认” text. Required monetary fields render `¥0.00` when absent, margins render `0.00%`, and only an absent optional preset margin renders `—`.

Historical recharge data is not backfilled and no second accounting source, migration, production-data write, configuration change, or GitHub Actions workflow was added.

## Candidate

- Baseline: `main@fbe32c725`
- Worktree: `.worktrees/t63-business-overview-zero-default`
- Branch: `codex/t63-business-overview-zero-default`
- Backend commit: `8a6ea762a` (`fix: default business overview values to zero`)
- Frontend commit: `7f194686f` (`fix: show business overview missing values as zero`)

## Changed files

- `upstream/sub2api/backend/internal/service/business_overview.go`
- `upstream/sub2api/backend/internal/service/business_overview_test.go`
- `upstream/sub2api/frontend/src/api/admin/businessOverview.ts`
- `upstream/sub2api/frontend/src/api/__tests__/admin.businessOverview.spec.ts`
- `upstream/sub2api/frontend/src/views/admin/BusinessOverviewView.vue`
- `upstream/sub2api/frontend/src/views/admin/__tests__/BusinessOverviewView.spec.ts`

## Verification

Passed:

- `go test ./internal/service -run 'TestBusinessOverview' -count=1`
- `go build ./cmd/server`
- `pnpm vitest run src/views/admin/__tests__/BusinessOverviewView.spec.ts src/api/__tests__/admin.businessOverview.spec.ts` — 4 tests passed
- `pnpm typecheck`
- `pnpm build`
- `gofmt` and `git diff --check`

The broader `go test ./internal/service -count=1` was not used as a task gate: it ran unrelated service tests for over 100 seconds and emitted unrelated existing warning/error logs before failing outside the T63-focused suite. The directly related T63 suite and build are green.

## Release and rollback

- Expected `downtime_required=false`.
- Candidate has not been merged, pushed, deployed, or production-verified.
- Root controller must perform the normal merged-main release gates and online verification.
- Rollback is the previously verified blue/green slot or the pre-T63 main revision.

## Scope audit

No migrations, queue/progress ledger changes, production writes, configuration changes, `.github/workflows` changes, or external accounting data were modified in this candidate.
