# T7 Monitor Terminal Governance Handoff

## Candidate

- Task: T7 monitor terminal governance
- Spec: `docs/superpowers/specs/2026-09-01-t7-monitor-terminal-governance-design.md`
- Plan: `docs/superpowers/plans/2026-09-01-t7-monitor-terminal-governance.md`
- Branch: `codex/t7-monitor-terminal-governance`
- Baseline: `main@50040c48483d005f5696a9e1f0dbde47de047362`
- Candidate HEAD: `8aef50503`

## Delivered

- Monitor V4 real-request projection now chooses one final event per group/logical request, allowing cross-account failover recovery to count once and preventing intermediate attempt errors from inflating failures.
- Missing probe terminals remain an explicit `missing_probe_terminal_count` integrity signal and no longer create a synthetic service failure or failure-denominator sample.
- Administrator request details now use a read-time usage/error projection with logical request correlation, attempt/failover/upstream-error counts, final status/protocol, terminal kind/reason, usage completeness, and safe-replay flags.
- Existing admin UI displays terminal lifecycle and attempt diagnostics in both desktop and narrow layouts with localized labels.

## Scope and safety

- No database migrations, new fact tables, historical backfill, production data writes, configuration changes, retry behavior changes, account state changes, or concurrency-control changes.
- User-facing error responses remain outside this projection and continue using existing redaction behavior.
- No credentials, API keys, authorization headers, full request bodies, full upstream responses, or model output are added to the admin contract.

## Verification

Passed:

- `go test ./internal/repository -run 'TestAccountMonitorRepositoryProjectMonitorV4|TestOpsRepositoryListRequestDetails' -count=1`
- `go test ./internal/service -run 'TestMonitorV4|TestOpsRequest' -count=1`
- `go build ./cmd/server`
- `pnpm vitest run src/features/monitor-v4/__tests__/api.spec.ts src/views/admin/ops/components/__tests__/OpsRequestDetailsModal.lifecycle.spec.ts`
- `pnpm typecheck`
- `pnpm build`
- `git diff --check`
- Scope guard: no migration, global queue, or global progress ledger file changed.

The full `go test ./internal/repository ./internal/service ./internal/handler -count=1` run is not clean because unrelated pre-existing tests fail in Codex fingerprint seed lifecycle, channel monitor endpoint validation, CodexRadar freshness, account monitor probe scheduling, service-tier/stream validation, and usage-record worker fixtures. Those failures were not changed by this candidate.

## Release boundary

- Not merged to root `main`.
- Not pushed, deployed, or online-verified.
- `downtime_required`: unexecuted; no migration or release preflight was run in this worktree.
- Rollback: discard this candidate or restore the previous root `main` release slot after root review; no data rollback is required.
- Required next step: root review and explicit `AUTHORIZE_MERGE_TO_MAIN`; root total controller owns merge, push, deployment, and online verification.
