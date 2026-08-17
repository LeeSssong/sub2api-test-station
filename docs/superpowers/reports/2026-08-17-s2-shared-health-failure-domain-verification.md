# Verification: S2 共享健康、故障域与抗故障重试

## Scope

- 初始基线：`main@1bc052d8e`
- 最新刷新目标：`main@566fc52ba`（refresh merge `ce48bef0b`）
- 已验证运行时 tip：`e3d905412`
- 范围：Redis 共享 account-model health/EWMA/cooldown、failure-domain 投影、fencing half-open lease、本地 30 秒可信快照、Responses/Messages/Chat Completions 请求级重试硬上限、`Retry-After`、稳定 jitter 与明确 exhaustion reason。

## Commands / Results

- `go test ./internal/repository -run '^TestOpenAISharedHealth' -count=1`: PASS.
- `go test ./internal/service -run 'Test(OpenAIAccountModelTransient|OpenAIAccountScheduler).*Shared|Test.*HalfOpen.*Fence|TestOpenAIModelTransient|TestRecordOpenAIIncompleteStreamFailure|TestRecordOpenAIAccountModelFailure|TestOpenAI.*RecordUsage|TestOpenAI.*Resilience|^TestProvideUsageCostEvidenceRegistrarPreservesNewAPIRateRefreshInjection$' -count=1`: PASS.
- `go test ./internal/handler -run 'Test(OpenAI.*Failover|OpenAI.*Retry|Gateway.*Failover|OpenAIRetryBudget|OpenAIResponses_.*NeverReplays|OpenAIMessages_.*NeverReplays)' -count=1`: PASS.
- `go test ./internal/config -run 'Test.*SharedHealth|Test.*Retry' -count=1`: PASS.
- `go test ./cmd/server -run '^$'`: PASS, compile-only.
- `go build -o /dev/null ./cmd/server`: PASS.
- `git diff --check`: PASS.
- Affected Go files `gofmt -l`: no output.
- `go test -tags=integration ./internal/repository -run '^TestOpenAISharedHealthIntegrationConcurrentHalfOpenHasOneWinner$' -count=1 -v`: BLOCKED before test execution by the pre-existing unrelated `stringPtr` redeclaration described below.

## Evidence

- Shared cooldown written by one service instance blocks another instance.
- Ordinary success writes `Success=true`, clears remote failure streak/cooldown, and unblocks another service instance.
- Shared success write failure remains best-effort and still clears the local transient state.
- Redis read failure uses a 10-second trusted snapshot; a 31-second snapshot becomes unknown and marks only that logical request degraded.
- A successful Redis miss remains a known empty projection and does not mark the request degraded.
- Degraded shared health permits at most one additional account switch.
- Redis failure does not clear the S1 native schedulability veto.
- Concurrent shared half-open acquisition has one winner; completion requires the held fencing lease.
- Failed `provider_channel` / `quota_pool` domains are retained by the request budget and used to prefer the best remaining scheduler bucket without reordering that bucket.
- A logical request permits at most 4 real attempts, 3 account switches, 2 ordinary failure domains, and 5 seconds total retry time.
- Unknown domains collapse to one domain.
- Budget refusal exposes stable reasons for attempt, switch, domain, deadline, and unsafe-replay exhaustion.
- 429 supports delta-seconds and HTTP-date `Retry-After`; waits beyond the remaining request budget are rejected.
- 5xx/connection retry delay starts at 120ms, adds stable 0–20% jitter, and remains capped at 2 seconds.
- Responses, Messages and Chat Completions pass failure-domain preference/read tracking through selection; Chat failures now carry attempt metadata and shared failure events.
- Existing tools, `function_call_output`, `tool_result`, semantic-output, usage/resilience and NewAPI rate injection regressions remain green.

## Not Verified

- Real Redis integration test did not compile because of a pre-existing unrelated integration harness collision: `internal/repository/user_profile_identity_repo_contract_test.go:577:6 stringPtr redeclared`, conflicting with `internal/repository/usage_log_repo_stats.go:203:6`. S2 did not modify either file and did not widen scope to repair it.
- No full-repository, pressure, mutation, soak, race, or unrelated browser matrix was run.
- No deliberate production upstream failure, cooldown, half-open lease race, Redis outage, account mutation, or unsafe replay request was induced. Those destructive/synthetic cases remain covered by focused tests rather than production fault injection.

## Production Closure

- Final release source: pushed root `main@aab79007fc90c842eaf9971b6f5304f4ab7b6503`, tree `c33145f1fdac4bf4b28d4cdc516036d3d938f75e`.
- Canonical `0600` evidence: `/Users/gongtengxinwen/.codex/release-evidence/sub2api/2026-08-17-main-aab79007f-s2-shared-health-v1.json`; migration hash remained `aaebed88f7fb712e1f518e73cc89bd44eb214f365f3b49f003598c93883a4604`.
- The first controller invocation safely stopped before host execution because the operator profile omitted the required immutable network curl image variables. The fixed historical digest was then verified locally and read-only on the production host before retrying the unchanged controller.
- The reviewed preloaded blue-green chain returned `downtime_required=false`, `result=succeeded`, active slot `blue`; release record `/var/lib/sub2api/release-records/20260817T072052Z-production-1893124.json` is `succeeded/promoted` with `rolled_back=false`.
- Active API and worker use the same immutable image ID `sha256:52f7b7b9735b321cdb995bf79d6f52147b27b0f97683ef2880f911ec70ba0c89`, are healthy, and have restart count 0. PostgreSQL, Redis, and Caddy container IDs remained unchanged.
- Public `/healthz`, `/readyz`, and `/health` returned HTTP 200.
- Read-only Redis inspection after natural traffic found 8 account-model projections, 2 domain projections, and 13 event markers; all 8 account-model states were `healthy`, and Redis returned `PONG`.
- API and worker logs for the inspected 15-minute window contained zero `openai.shared_health` warnings and zero panic/fatal matches. No production account was changed and no upstream failure was manufactured.

## Review

- Primary release-controller audit found and closed the prior candidate gaps: normal success reset, degraded-budget narrowing, domain-aware preference, stable jitter/reasons, and Chat shared failure reporting.
- No database migration, GitHub Actions workflow, pricing, multiplier, billing formula, external control plane, T15, T16, or S3 change is present.
- Domain preference is applied after S1/native/shared eligibility filtering and before load scoring; it retains only the best failure-domain bucket while preserving order inside that bucket and leaving subsequent Top-K/sticky behavior intact.
- Redis keys hash untrusted model/domain/event material; warning logs contain account ID and model hash but no credential, full request body, raw domain ID, or API key.
- Redis writes are best-effort and do not block the local transient update. Redis reads share a 75ms selection deadline and are capped at 128 candidates; stale snapshots fail to unknown.
- S1 database/native veto checks remain before shared health filtering.
- Half-open completion remains the authoritative shared success path for half-open probes; normal success events are not duplicated for those probes.

## Follow-ups

- S2 is complete and the S3 dependency gate is now satisfied.
- Keep the pre-existing unrelated integration-tag `stringPtr` collision out of S2 scope; address it only under a separately queued task if needed.
- Preserve the previous green image, release state/record, canonical evidence, and recovery bundle through the normal rollback window.
