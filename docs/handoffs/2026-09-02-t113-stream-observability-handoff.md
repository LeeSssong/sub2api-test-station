# T113 Stream Observability Handoff

## Status

`READY_FOR_ROOT_REVIEW`

## Scope

T113 adds native lifecycle observation for OpenAI streaming paths, exact-ID read-only administrator diagnostics, admin Usage detail presentation, and production/acceptance Caddy/Compose identity contracts. It does not change scheduling, retry budgets, billing, account state, upstream protocol behavior, database schema, production data, or deployment authorization.

## Git

- Baseline: `main@5bff30023`
- Candidate branch: `codex/t113-stream-observability`
- Worktree: `/Users/gongtengxinwen/Documents/sub2api搭建/.worktrees/t113-stream-observability`
- Candidate commits: `8c8f77d34`, `0da38a325`, `f274c4083`, `b03db0ae4`, `51bf627a6`, plus final candidate commit below

## Changed areas

- `backend/internal/service/stream_observability.go`: lifecycle snapshot, transport classification, redaction, root-cause evidence projection.
- `backend/internal/service/stream_observability_runtime.go`: request-scoped lifecycle hooks and structured logging.
- `backend/internal/service/openai_gateway_chat_completions.go` and `_raw.go`: native Responses/Chat SSE hooks.
- `backend/internal/service/ops_upstream_context.go`: embeds sanitized snapshot in existing `upstream_errors` JSON.
- `backend/internal/service/ops_models.go`, `ops_repo.go`, `ops_service.go`, `handler/admin/ops_handler.go`, `server/routes/admin.go`: exact request/logical-request diagnostic query.
- `frontend/src/api/admin/usage.ts`, `types/index.ts`, `components/usage/UsageDetailDialog.vue`, admin locales: admin-only diagnostic display.
- `infra/Caddyfile`, `infra/Caddyfile.acceptance`, `infra/compose.yaml`, `infra/compose.acceptance.yaml`: environment/commit/slot identity and JSON log fields, existing `20m x 5` retention preserved.
- `tests/operations/stream_observability_caddy_contract_test.sh`, `tests/admin_lab/stream_observability_acceptance_contract_test.sh`: infrastructure contracts.

## Verification

- Stream/domain focused Go tests: passed.
- Ops diagnostic focused Go tests: passed.
- Admin Usage/diagnostic Vitest: `35/35` passed.
- Frontend `pnpm typecheck`: passed.
- Frontend `pnpm build`: passed.
- Backend `go build ./cmd/server`: passed before the final infrastructure-only changes; a broader `go test ./cmd/server` remains blocked by the pre-existing generated `cmd/server/wire_gen_test.go` argument drift.
- `bash -n ops/deploy-sub2api-blue-green-host.sh`: passed.
- Caddy/Compose contracts: passed.
- `git diff --check`: passed.

## Known limitations

- Runtime instrumentation currently covers the main OpenAI Responses-to-Chat and raw Chat SSE paths; native Anthropic conversion and WebSocket paths are not expanded in this candidate.
- `deployment_commit` and container identity are populated when the existing release environment supplies them; absent values remain degraded rather than guessed.
- No live production or acceptance deployment/verification was performed.

## Release boundary

- Migrations: none.
- Production data writes: none.
- Configuration schema changes: none; only optional Compose environment propagation.
- `downtime_required`: unverified until root preflight.
- Deployment authorization: not granted by this task.
- Rollback: restore the previous validated blue-green slot/image using the existing root release chain.
