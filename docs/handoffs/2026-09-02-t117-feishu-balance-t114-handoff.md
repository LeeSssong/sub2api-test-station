# T117 飞书余额卡片静默按钮与 T114 调度排名对齐交接

## Candidate

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t117-feishu-balance-t114`
- Branch: `codex/t117-feishu-balance-t114`
- Base: `main@c0aadcee3`
- Root main was not modified by this implementation. It later advanced independently with documentation commits; root review must refresh this candidate to the latest clean main before integration.

## Implemented

- Extended the internal upstream-balance evaluation/card contract with ranking snapshot time, rank total, eligibility, and T114-enabled metadata.
- Projected existing account-monitor scheduler ranks and snapshot time into Feishu balance cards; no second ranking formula or external API was added.
- Cards show T114 ranking source/time and `第 X / N 名`, with explicit unavailable/non-T114 labels for new metadata-bearing inputs.
- Preserved legacy in-process card callers that provide only the old numeric rank shape.
- Preserved existing 1h/6h/24h silence action rendering and callback behavior.
- Added a non-sensitive startup log when silence actions are disabled because the callback token is unconfigured or the silence repository is unavailable; balance delivery remains enabled.

## Verification

Passed in the candidate worktree:

```text
go test ./internal/service -run 'Test(NormalizeNotificationBaseURL|EvaluateUpstreamBaseURLBalance|ReadUpstreamBalanceEvaluations|UpstreamBalanceNotificationService|BuildUpstreamBalanceEvaluations)' -count=1
go test ./internal/notify -run 'TestRenderUpstreamBalanceCard|TestLoadUpstreamBalanceSecrets|TestFeishuSender' -count=1
go test ./internal/handler -run 'TestFeishuUpstreamBalanceCallback' -count=1
go test ./internal/repository -run 'TestUpstreamBalanceEventRepository' -count=1
go build ./cmd/server
git diff --check
```

## Release boundary

- No migration, configuration, production-data mutation, real Feishu delivery, push, or deployment.
- Callback buttons still require the existing protected callback token file (`0600` file under a `0700` parent); missing/unsafe configuration intentionally produces a body-only alert with a structured non-sensitive log.
- Historical Feishu cards are not updated. New and repeated deliveries read the current monitor/scheduler projection available at send time.
- Root controller must refresh to latest `main`, rerun direct tests, review generated wiring, and decide release authorization.
