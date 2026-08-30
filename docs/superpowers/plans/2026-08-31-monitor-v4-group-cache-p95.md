# Plan: Monitor V4 分组缓存 P95

> For agentic workers: execute task-by-task and verify before completion.

## Source

- Spec: `docs/superpowers/specs/2026-08-31-monitor-v4-group-cache-p95-design.md`
- Baseline: `main@e9db36d4b5cf789ac85bbabdfb82aa2c4beb7479`
- Refreshed baseline: `main@5d77271b32990076b8b0344a3f1909c62192abc6`

## Scope

- In: native Monitor V4 projection, service/handler contract, existing group card, focused tests.
- Out: success-rate logic, probe scheduling, database migrations, other monitor versions.

## Acceptance

- [x] Successful real-request `cache_read_tokens` P95 is returned per group, including zero values.
- [x] Probe/failure rows are excluded and null/zero sample behavior is explicit.
- [x] Existing card renders the new field without layout regression.

## Tasks

- [x] 1. Add focused backend and frontend contract tests.
- [x] 2. Extend projection/domain/handler and card/i18n with cache P95.
- [x] 3. Run focused tests, typecheck/build, gofmt and diff check.
- [x] 4. Record verification and handoff; root controls merge/deploy.

## Verification Commands

- Go 1.27.0 container, repository: `go test -vet=off -p 1 -run '^TestAccountMonitorRepositoryProjectMonitorV4' -count=1 ./internal/repository`
- Go 1.27.0 container, service/handler: compile all non-test package sources and run only the self-contained Monitor V4 test files (the full packages contain unrelated baseline test compile failures).
- `./node_modules/.bin/vitest run src/features/monitor-v4/__tests__/HybridPerformanceGroupCard.spec.ts src/features/monitor-v4/__tests__/api.spec.ts`
- `./node_modules/.bin/vue-tsc --noEmit`
- `./node_modules/.bin/vue-tsc -b && ./node_modules/.bin/vite build`
- `gofmt -w ... && git diff --check`

## Risks

- P95 over a large raw request set may add query cost; measure 7d/30d endpoint latency after deployment.
