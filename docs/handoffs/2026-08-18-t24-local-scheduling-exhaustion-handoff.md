# T24 handoff — 特惠分组本地调度耗尽 503 纠正

- Worktree: `/Users/gongtengxinwen/.codex/worktrees/e7bc/sub2api搭建`
- Branch: `codex/t24-local-scheduling-exhaustion`
- Baseline main: `3b6fda9ba4dd2a65b8621a9d1b9324332e98699e`
- Candidate commits: `7ca0d774b`, `1a7008a31`
- State: `READY_FOR_ROOT_REVIEW` (not merged/pushed/deployed)

## Delivered

- 本地账号调度耗尽保持 HTTP 503，稳定用户错误码为 `local_capacity_exhausted`，中文消息为“当前服务资源暂时不可用，请稍后重试”。
- Responses 与 Chat Completions 的非流式、流未开始 JSON 和流已开始 SSE 共享同一错误语义。
- 管理员诊断投影为 `local_capacity_exhausted / LOCAL_CAPACITY_EXHAUSTED / routing / platform / upstream_account_selected=false`，用户含义为“当前分组暂无可用服务资源”。
- 已选择账号后的真实上游 503 仍使用既有透传、失败切换和上游诊断路径，不会被投影为本地容量耗尽。
- 永久模型不支持继续返回既有 404 `model_not_found`；未改账号、分组、计费、重试或外部控制面。
- 正式规格和计划位于 `docs/superpowers/specs/2026-08-18-t24-local-scheduling-exhaustion-design.md` 与 `docs/superpowers/plans/2026-08-18-t24-local-scheduling-exhaustion.md`。

## Changed files

- `upstream/sub2api/backend/internal/handler/gateway_handler.go`
- `upstream/sub2api/backend/internal/handler/gateway_handler_chat_completions.go`
- `upstream/sub2api/backend/internal/handler/gateway_handler_responses.go`
- `upstream/sub2api/backend/internal/handler/no_account_error.go`
- `upstream/sub2api/backend/internal/handler/no_account_error_test.go`
- `upstream/sub2api/backend/internal/handler/openai_gateway_handler.go`
- `upstream/sub2api/backend/internal/handler/ops_error_logger_test.go`
- `upstream/sub2api/backend/internal/service/native_error_diagnostics.go`
- `upstream/sub2api/backend/internal/service/native_error_diagnostics_test.go`
- `docs/superpowers/specs/2026-08-18-t24-local-scheduling-exhaustion-design.md`
- `docs/superpowers/plans/2026-08-18-t24-local-scheduling-exhaustion.md`
- `docs/handoffs/2026-08-18-t24-local-scheduling-exhaustion-handoff.md`

## Direct verification

- `go test ./internal/handler -tags unit -run 'Test(ClassifyNoAccountError|ClassifyOpenAICompatibleNoAccountError|LocalCapacityExhaustedProtocolContract|ClassifyOps|OpenAIHandleStreamingAwareError|GatewayHandleStreamingAwareError|OpenAIResponses_APIKeyPassthroughPool5xxRetriesThenExhaustsMaxSwitches)' -count=1` — pass.
- `go test ./internal/service -run 'Test(ProjectNativeErrorDiagnosis|MatchRule_RealWorldScenario_CustomErrorMessage|OpenAIGatewayService_APIKeyPassthrough_Transient5xxTriggersFailover)' -count=1` — pass.
- `go test ./internal/handler ./internal/service -run '^$' -count=1` — pass.
- `go build ./cmd/server` — pass.
- `gofmt` — pass.
- `git diff --check` — pass.

## Migration/config/downtime

- No database migration, production-data change, dependency, or configuration change.
- Expected `downtime_required=false`; root preflight after integration remains authoritative.

## Remaining risks / unverified

- No deployment or online verification was performed in this candidate worktree.
- Earlier full handler/service package attempts exposed existing scheduler/retry assertion failures in untouched code (`TestOpenAIGatewayHandlerResponses_FailoverContinuesForConnectedClient`, `TestOpenAIGatewayService_SelectAccountWithScheduler_Enabled_EmbeddingsSkipsChatOnlyStickyBindings`, and `TestOpenAIGatewayService_SelectAccountWithScheduler_ClearsStickyAccountOutsideGroup`). They are outside T24's changed paths; direct T24 tests, affected-package compile-only, and server build pass. Per current scope, the broad package suites were not investigated or rerun.
- Root integration should retain the candidate's stable client code/message and local-versus-upstream diagnostic boundary when resolving any later conflicts.

## Rollback

Revert candidate implementation commit `1a7008a31` (and documentation commits if desired), then rebuild through the existing release chain. There is no schema, data, or configuration rollback.
