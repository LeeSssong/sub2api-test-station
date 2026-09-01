# T98-R2 Feishu Balance Freshness Handoff

## Candidate

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t98-r2-feishu-balance-freshness`
- Branch: `codex/t98-r2-feishu-balance-freshness`
- Base: `main@eac62b247b5117dd8d97a8249f2341059131cf7e`
- Status: `READY_FOR_ROOT_REVIEW`
- No merge, push, deployment, or real Feishu delivery was performed.

## Implemented

- Successful balance snapshots now carry a SHA-256 fingerprint of the API Key used for collection; plaintext keys never enter the snapshot or logs.
- Balance notification evaluation rejects snapshots with a mismatched fingerprint, missing fingerprint, future timestamp, or age greater than 10 minutes.
- Active zero-balance BaseURL events request a scoped refresh before `RunDue` reads the notification projection.
- Scoped refresh enumerates active OpenAI API-Key accounts without requiring them to be schedulable, so recharge recovery is not blocked by the prior balance isolation.
- A recovered balance of at least USD 5 clears only the matching deterministic `balance_exhausted` temporary scheduling block, using a credentials-and-reason CAS update that emits the existing scheduler outbox event and refreshes the account cache.
- Existing BaseURL aggregation, thresholds, notification cadence, and native SchedulerRank projection remain unchanged.

## Verification

Passed from `upstream/sub2api/backend`:

```text
go test -count=1 ./internal/service -run 'Test(AccountMonitor.*Balance|RefreshBalance|ResolveBalance|BalanceFailure|DecodeBalance|BuildUpstreamBalance|ReadUpstreamBalance|EvaluateUpstream|UpstreamBalanceNotification|ProvideUpstreamBalance)'
go test -count=1 ./internal/repository -run 'Test(UpdateAccountMonitorBalance|ClearBalanceExhausted|SchedulableBalanceVeto|ListSchedulableCapacity)'
go test -count=1 ./internal/notify -run 'TestRenderUpstreamBalanceCard|TestLoadUpstreamBalanceSecrets|TestFeishuSender'
go test -count=1 ./migrations -run 'TestUpstreamBalance'
go build ./cmd/server
gofmt -l
git diff --check
```

All commands exited successfully. The broader repository suites were not used as acceptance evidence because the current main baseline contains unrelated failures outside this task.

## Root review notes

- Review the freshness threshold and account-level recovery CAS before any integration.
- The candidate does not include the unmerged T98-R3 Feishu silence-button worktree.
- Deployment, root `main` refresh, and online verification remain outside this candidate.
