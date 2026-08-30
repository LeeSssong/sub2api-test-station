# Plan: Monitor V4 分组缓存 P95

> For agentic workers: execute task-by-task and verify before completion.

## Source

- Spec: `docs/superpowers/specs/2026-08-31-monitor-v4-group-cache-p95-design.md`
- Baseline: `main@e9db36d4b5cf789ac85bbabdfb82aa2c4beb7479`

## Scope

- In: native Monitor V4 projection, service/handler contract, existing group card, focused tests.
- Out: success-rate logic, probe scheduling, database migrations, other monitor versions.

## Acceptance

- [ ] Successful real-request `cache_read_tokens` P95 is returned per group, including zero values.
- [ ] Probe/failure rows are excluded and null/zero sample behavior is explicit.
- [ ] Existing card renders the new field without layout regression.

## Tasks

- [ ] 1. Add focused backend and frontend contract tests.
- [ ] 2. Extend projection/domain/handler and card/i18n with cache P95.
- [ ] 3. Run focused tests, typecheck/build, gofmt and diff check.
- [ ] 4. Record verification and handoff; root controls merge/deploy.

## Verification Commands

- `go test ./internal/repository ./internal/service ./internal/handler/...`
- `pnpm vitest run src/features/monitor-v4`
- `pnpm typecheck`
- `pnpm build`
- `gofmt -w ... && git diff --check`

## Risks

- P95 over a large raw request set may add query cost; measure 7d/30d endpoint latency after deployment.
