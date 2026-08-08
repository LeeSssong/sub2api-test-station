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
