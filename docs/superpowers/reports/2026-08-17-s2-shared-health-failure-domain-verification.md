# Verification: S2 共享健康、故障域与抗故障重试

## Scope

- 基线：`main@1bc052d8e`
- 最终刷新目标：`main@a533d84b0`
- 已验证实现 tip：`0e52328a9`
- 范围：Redis 共享 account-model health/EWMA/cooldown、failure-domain 投影、fencing half-open lease、本地 30 秒可信快照、Responses/Messages/Chat Completions 请求级重试硬上限、`Retry-After` 与有界退避。

## Commands / Results

- `go test ./internal/repository -run '^TestOpenAISharedHealth' -count=1`: PASS.
- `go test ./internal/service -run 'Test(OpenAIAccountModelTransient|OpenAIAccountScheduler).*Shared|Test.*HalfOpen.*Fence|TestOpenAIModelTransient|TestRecordOpenAIIncompleteStreamFailure|TestRecordOpenAIAccountModelFailure|TestOpenAI.*RecordUsage|TestOpenAI.*Resilience|^TestProvideUsageCostEvidenceRegistrarPreservesNewAPIRateRefreshInjection$' -count=1`: PASS.
- `go test ./internal/handler -run 'Test(OpenAI.*Failover|OpenAI.*Retry|Gateway.*Failover|OpenAIRetryBudget|OpenAIResponses_.*NeverReplays|OpenAIMessages_.*NeverReplays)' -count=1`: PASS.
- `go test ./internal/config -run 'Test.*SharedHealth|Test.*Retry' -count=1`: PASS.
- `go test ./cmd/server -run '^$'`: PASS, compile-only.
- `go build -o /dev/null ./cmd/server`: PASS.
- `git diff --check`: PASS after S2 spec/plan whitespace cleanup.
- Affected Go files `gofmt -l`: no output.

## Evidence

- Shared cooldown written by one service instance blocks another instance.
- Redis read failure uses a 10-second trusted snapshot; a 31-second snapshot becomes unknown.
- Redis failure does not clear the S1 native schedulability veto.
- Concurrent shared half-open acquisition has one winner; completion requires the held fencing lease.
- A logical request permits at most 4 real attempts, 3 account switches, 2 ordinary failure domains, and 5 seconds total retry time.
- Unknown domains collapse to one domain.
- 429 supports delta-seconds and HTTP-date `Retry-After`; waits beyond the remaining request budget are rejected.
- 5xx/connection retry delay starts at 120ms and is capped at 2 seconds.
- Existing tools, `function_call_output`, `tool_result`, semantic-output, usage/resilience and NewAPI rate injection regressions remain green.

## Not Verified

- Real Redis integration test did not compile because of a pre-existing unrelated integration harness collision: `user_profile_identity_repo_contract_test.go:577 stringPtr` conflicts with `usage_log_repo_stats.go:203 stringPtr`. S2 did not modify either file and did not widen scope to repair it.
- No full-repository, pressure, mutation, soak, race, or unrelated browser matrix was run.
- No release preflight, deployment, production Redis inspection, production account mutation, or online functional acceptance was run because deployment remains frozen.

## Review

- No database migration, GitHub Actions workflow, pricing, multiplier, billing formula, Top-K ordering, sticky contract, external control plane, T15, T16, or S3 change is present.
- Redis keys hash untrusted model/domain/event material; warning logs contain account ID and model hash but no credential, full request body, raw domain ID, or API key.
- Redis writes are best-effort and do not block the local transient update. Redis reads share a 75ms selection deadline and are capped at 128 candidates; stale snapshots fail to unknown.
- S1 database/native veto checks remain before shared health filtering.

## Follow-ups

- Root release controller may review and merge only after setting the global S2 state to `READY_FOR_ROOT_REVIEW`.
- Release preflight and blue-green deployment must wait for a new explicit user deployment instruction.
- S3 remains blocked until S2 production deployment and online acceptance complete.
