# T98-R3 Feishu Notification Silence Handoff

## Candidate

- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t98-r3-feishu-notification-snooze`
- Branch: `codex/t98-r3-feishu-notification-snooze`
- Base: `main@50040c48483d005f5696a9e1f0dbde47de047362`
- Status: `READY_FOR_ROOT_REVIEW`

## Implemented

- Feishu upstream-balance cards include current wallet balance and optional `1h`, `6h`, and `24h` silence buttons.
- Silence is scoped to the active normalized BaseURL event and stops future claims until the selected expiry.
- Action tokens are opaque, generated per claimed notification, and persisted only as SHA-256 hashes.
- Callback authorization checks the protected callback token and configured Feishu recipient OpenID.
- Callback supports URL verification and common Feishu card-action payload shapes.
- Added additive migration `233_upstream_balance_notification_silence.sql` with `silenced_until` and `action_token_hash`.
- Callback token file is optional for compatibility; buttons are omitted unless it is configured and protected-file validation succeeds.

## Verification

Passed from `upstream/sub2api/backend`:

```text
go test -count=1 ./internal/service -run 'TestUpstreamBalanceNotification|TestProvideUpstreamBalanceNotification'
go test -count=1 ./internal/handler -run 'TestFeishuUpstreamBalanceCallback'
go test -count=1 ./internal/notify -run 'TestRenderUpstreamBalanceCard|TestLoadUpstreamBalanceSecrets|TestFeishuSender'
go test -count=1 ./internal/repository -run 'TestUpstreamBalanceEventRepository'
go test -count=1 ./migrations -run 'TestUpstreamBalanceNotificationSilenceMigration'
go build ./cmd/server
git diff --check
```

The broader pre-existing `internal/handler` and `internal/server/routes` suites contain unrelated baseline failures; they were not used as evidence for this candidate.

## Root review notes

- Review generated wiring and route registration before merge.
- Configure `SUB2API_UPSTREAM_BALANCE_FEISHU_CALLBACK_TOKEN_FILE` as a `0600` file under a `0700` parent before enabling interactive silence.
- Do not send production Feishu messages or callbacks from this candidate. Merge, push, deployment, and online verification remain root-controller work.
