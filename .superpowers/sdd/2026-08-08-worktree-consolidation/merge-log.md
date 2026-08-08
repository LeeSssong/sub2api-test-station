# Task 2 merge log — non-main worktree consolidation

Date: 2026-08-08
Candidate branch: `codex/worktree-consolidation-20260808`
Base before Task 2: `2fc6dc1bd9f029d1fb63da7008eaca906211b5ae` (with Task 1 scope clarification commit `8f35c6ccb`)

## Source classification

- `codex/fix-relay-admin-auth` (`fca5d454a`): **runtime-compatible, merge**. This is the latest production-relevant implementation for probe-only account monitoring, administrator usage cost details, upstream request IDs, and relay-ops reconciliation evidence.
- `codex/account` (`500320a2e` plus committed ancestors): **runtime candidate, selective recovery**. Group-aware score-weight persistence and group ranking are retained only where not superseded by the probe-only implementation. Its dirty working-tree patch is preserved in Task 1 recovery snapshots and will be reviewed separately; no source worktree is modified.
- `fix/sub2api-0171-release` (`b4da0abc5`): **release-chain candidate, selective merge**. Only local/host blue-green executor fixes absent from the candidate will be carried forward; GitHub Actions remains excluded.
- Other registered non-main worktrees: **documentation/generated/already-merged/stale evidence** unless a later review identifies a unique runtime delta. Their manifests and recovery archives remain authoritative; obsolete runtime code is not deployed.

## `codex/fix-relay-admin-auth` merge

Merge command: `git merge --no-ff --no-edit codex/fix-relay-admin-auth`
Result: merge conflicts in seven files; all resolved manually and staged.

### Conflict decisions

1. `docs/project/project-progress.md`: kept the consolidation registration and active-thread protection entry from the candidate; retained the pre-existing historical ledger entries from the source branch. The duplicate date heading from the source side was removed.
2. `upstream/sub2api/backend/internal/service/account_monitor_service.go`: used the source branch's explicit `probe := probes[...]` and `projectAccountMonitorProbe` availability/score gate, then retained candidate `EvidenceSource`, score-breakdown payload, stale-evidence timestamp check, and abnormal-score cap. This preserves probe-only truth while keeping the active UI's score composition.
3. `.../account_monitor_service_test.go`: used the source branch's newer probe-only/fatal-error regression suite; re-added the candidate's score-breakdown sum assertion as a non-duplicated test. Real-request aggregates remain disclosure-only.
4. `.../AccountMonitorCard.vue`: used probe_* metric fields and score-status eligibility from the source branch; retained candidate homepage link, cost-source warning, equivalent-site-multiplier-only presentation, and score-composition tooltip. The source branch's older detailed cost-guard panel is intentionally not restored because the candidate already contains the newer simplified cost contract.
5. `.../AccountMonitorCard.spec.ts`: retained candidate homepage/cost-warning tests and added the source branch's unavailable-account probe evidence test.
6. `i18n/locales/{en,zh}/admin/index.ts`: retained both `accountProfitability` and source `usageCostDetail` namespaces as separate object keys.

No unresolved conflict entries remain (`git ls-files -u` is empty after staging).

## Verification log

Commands and exact results will be appended below as each selective source is integrated.

### Fix-relay focused backend verification

- Command: `cd upstream/sub2api/backend && go test ./internal/service ./internal/handler/admin ./internal/repository ./internal/handler/dto`
- Result: PASS — `internal/service` 102.803s; `internal/handler/admin` 2.723s; `internal/repository` 3.344s; `internal/handler/dto` 1.352s.

## `codex/account` selective recovery review

- Commit range reviewed: `HEAD..codex/account` (`500320a2e` plus ancestors), including the account-monitor service/types, repository, admin handler, routes, and migration `188_account_monitor_group_score_weights.sql`.
- Runtime result: **already merged / stale source**. The candidate already contains the group score-weight table and migration, group score-weight load/save/reset handlers and protected routes, group score projections, group-aware quality scoring, eligibility, and group ranking. The account branch removes newer probe-only fields and real-request disclosure boundaries, so merging it wholesale would regress the candidate.
- Dirty working-tree result: **documentation/generated metadata only**. The preserved patch changes the project ledger/spec wording, removes the committed frontend pnpm override block, and adds an untracked design plan/workspace marker; no unique runtime behavior is present. Recovery evidence remains under `recovery/project-worktrees-account/` and was not applied.
- Decision: no source patch or merge commit from `codex/account`; retain the current candidate implementation and the recovery snapshot as audit evidence.

## `fix/sub2api-0171-release` selective recovery review

- `875a66d23` updater diff: **superseded**. The candidate already contains the newer `11a68b22b` updater/executor contract; its staged-archive cleanup intentionally removes the archive only after the executor returns, while the older branch keeps it after completion. No older updater behavior was restored.
- `3803723d9` deploy diff: **superseded by later candidate release gates**. The source branch authorizes the historical `e95b… -> 3372… -> ac8b…` transition. The candidate has the later native-multiplier gate `ac8b… -> 0204…` and explicitly rejects retired migration sets in its acceptance harness; widening it with the older transition would weaken the current release contract.
- Documentation changes on `fix/sub2api-0171-release` were not copied because they describe the superseded 0.1.171 release state. No GitHub Actions or remote deployment workflow was introduced.
- Decision: no release-script source patch was required; candidate scripts remain unchanged.

## Selective verification

- Command: `cd upstream/sub2api/backend && go test ./internal/service -run 'Test(AccountMonitor|CalculateAccountMonitor)'`
- Result: PASS — `github.com/Wei-Shaw/sub2api/internal/service` (7.554s).
- Command: `cd upstream/sub2api/backend && go test ./internal/repository ./internal/handler/admin ./internal/handler/dto -run 'Test(AccountMonitor|Account|Usage)'`
- Result: PASS — repository (5.888s), admin handler (6.307s), DTO (6.721s).
- Command: `cd relay-ops-service && go test ./...`
- Result: PASS — all relay-ops packages.
- Command: `bash tests/operations/update_sub2api_host_test.sh`
- Result: PASS — `Sub2API updater delegates strictly to blue-green production`.
- Command: `bash tests/operations/deploy_sub2api_blue_green_host_test.sh`
- Result: PASS — fail-closed validation, preloaded archive, downtime gates, authorized maintenance/Caddy route, success ordering, two-slot cycles, rollback/recovery, probe-image policy, worker health, lock/recovery safety, final-review host safety, and immutable release-tag/recovery-ID suites.
- Command: `cd upstream/sub2api/frontend && pnpm exec vitest run src/components/admin/account-monitor/AccountMonitorCard.spec.ts src/components/usage/__tests__/UsageDetailDialog.spec.ts src/components/usage/__tests__/usageDetail.spec.ts src/api/__tests__/admin.usage.spec.ts`
- Result: PASS — 4 files, 60 tests. The account card assertion was aligned with the current probe-only contract (`72 次探测样本，1 次失败`) after the pre-fix run exposed the stale `71 / 72 次主动探测成功` expectation.
- Command: `cd upstream/sub2api/frontend && pnpm typecheck`
- Result: PASS — `vue-tsc --noEmit`. The unused legacy `probeMetricDetail` computed property left by the conflict fusion was removed.
