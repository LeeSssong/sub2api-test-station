# T110 账号监控真实请求与调度排名统一交接

## Candidate

- Branch: `codex/t110-account-monitor-scheduler-alignment`
- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t110-account-monitor-scheduler-alignment`
- Baseline: current root `main` at worktree creation, then fast-forwarded to `main@cfebbf272` inside this candidate
- Status: implementation complete, not merged, not pushed, not deployed

## Delivered

- Added persisted lifetime real-request count `lifetime_real_request_count` using native `usage_logs`/`ops_error_logs` deduplication and excluding `usage_completeness='unknown'`.
- Account cards now show selected-window real requests and lifetime real requests together.
- Account timeline reads native probe results as fallback when a 5-minute source bucket has no real request; real and probe counts/source are exposed separately and the card keeps exactly 24 display bars.
- Group and global monitor ordering now consume scheduler-derived rank only. Global rows project the best group scheduler rank and group name; quality rank/score weights no longer drive primary monitor ordering.
- Removed score-weight editing controls from the account-monitor page while retaining backend compatibility APIs/storage.

## Verification

Passed:

- `go test ./internal/repository -run 'TestAccountMonitorRepository(ProjectMonitorV4|RealRequestTimeline|ListsLifetimeRealRequestCounts)' -count=1`
- `go test ./internal/service -run 'TestAccountMonitorListWindow(IgnoresPersistedGlobalScoreWeightsForPrimaryOrder|KeepsQualityEvidenceAndSchedulerRanksGroupScoped)' -count=1`
- `go build ./cmd/server`
- `pnpm vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/views/admin/__tests__/AccountMonitorView.spec.ts` — 45 passed
- `pnpm typecheck`
- `pnpm build`
- `gofmt` and `git diff --check`

The frontend install briefly rewrote `pnpm-lock.yaml`; the generated lockfile-only diff was reverted and is not part of the candidate.

## Scope and release boundary

- No migration, configuration, billing, scheduler-selection, retry, account-state, or production-data changes.
- No active probe is counted as a real request.
- Root `main` was not modified by T110 after the candidate was created; no push, merge, deployment, or online verification was performed per user instruction.
