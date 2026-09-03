# T127 OpenAI Observability, Capability Cache, And Cache Metric Handoff

- Baseline: `main@b7656bc6bd6a416df5f0b6158e629756ad1e2bf1`
- Candidate: `codex/t127-openai-observability-capability-cache`
- Status: `READY_FOR_ROOT_REVIEW`
- Scope: native OpenAI stream incomplete event semantics and Monitor V4 cache-token denominator contract. Existing T113 lifecycle logging and existing model-not-found/account-model cooldown behavior are reused; no new capability table or second fact source was added.

## Changes

- `StreamObservation.RecordFailure` emits `openai.stream_incomplete` only before a terminal event; terminal edge/client-disconnect behavior keeps `openai.stream.lifecycle` and terminal evidence.
- Monitor V4 projections and JSON responses expose `cache_read_tokens`, `cache_creation_tokens`, and `cache_hit_denominator`; SQL rate is `cache_read / (cache_creation + cache_read)` with zero denominator remaining nullable.
- Frontend Monitor V4 types accept the server-owned fields and do not derive cache rate from ordinary input tokens.
- Existing repository tests were updated for the expanded SQL result contract.

## Verification

- `go test ./internal/service ./internal/handler ./internal/repository -run 'Test(StreamObservation|MonitorV4|ClassifyOpenAINotFound|AccountMonitor)' -count=1`: PASS
- `go build ./cmd/server`: PASS
- `pnpm typecheck`: PASS
- `pnpm vitest run src/features/monitor-v4/__tests__`: PASS (18 tests)
- `bash tests/operations/deploy_sub2api_blue_green_host_test.sh`: PASS
- `bash tests/operations/release_sub2api_blue_green_test.sh`: PASS
- `git diff --check`: PASS

The combined topology command reached its final compose interpolation scenario and stopped because this local environment does not expose the protected `SUB2API_MODEL_DETECTOR_TOKEN`; no credential was read or changed. No deployment, push, migration, production data, configuration, or root `main` modification was performed.

## Release and rollback

- `downtime_required`: not evaluated; candidate is not authorized to deploy.
- Release source must be a clean, pushed root `main` after root authorization.
- Rollback remains the existing blue/green atomic rollback to the previous verified slot; no new rollback path was introduced.

## Residual risks

- Full runtime verification of model-detector topology requires the protected host credential and belongs to root release verification.
- The candidate does not claim production image/digest parity until the root release preflight and host checks run from `main`.
