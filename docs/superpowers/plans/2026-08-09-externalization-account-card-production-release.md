# Sub2API 外置层与账号卡片生产发布计划

> For agentic workers: execute each task sequentially, record evidence, and stop on any failed gate.

## Source

- Externalization plan: `docs/superpowers/plans/2026-08-08-sub2api-externalized-customization-implementation-plan.md`
- Account card candidate: `8562ca848774a28969793a9135fc9155aad3c94f`
- Production runbook: `docs/runbooks/sub2api-blue-green-production-deployment.md`

## Scope

- Include current `main` externalization commits and the final account-card application-tooltip commit.
- Exclude every commit and uncommitted change from `/Users/gongtengxinwen/Documents/sub2api-upstream-resilience-spec` (`codex/upstream-resilience-implementation`).
- Preserve existing untracked release evidence.
- Use only the reviewed local/host release chain; do not use GitHub Actions.
- If downtime occurs, complete promotion or rollback within five minutes.

## Task 1: Integrate the account-card candidate

- [ ] Cherry-pick only `8562ca848` onto `main`, resolve progress-ledger context without importing its old base, run focused account-card tests, and obtain an independent task review.

## Task 2: Verify the merged main branch

- [ ] Run externalization contracts, backend/relay-ops/updater tests and vet, frontend tests/typecheck/build, migration classification, release preflight, and final diff review on merged `main`.

## Task 3: Push and stage the immutable candidate

- [ ] Push the exact verified `main`, prepare an immutable Linux AMD64 candidate, record image/source/tree/migration identities, and stage it without changing traffic.

## Task 4: Promote with the existing blue-green executor

- [ ] Promote through the existing blue-green host executor, enforce the five-minute unavailable-window budget, and roll back immediately if health/readiness fails.

## Task 5: Complete production acceptance

- [ ] Verify public health/readiness/models, administrator authentication/routes, account monitor tooltip behavior, externalized control-plane freshness/degradation behavior, worker health, and protected PostgreSQL/Redis/Caddy identities.

## Task 6: Record and review the release

- [ ] Update the project ledger and production verification report, then run an independent whole-branch review.

## Acceptance

- `main` contains `8562ca848` patch-equivalent account-card changes and no scheduling-resilience commits.
- The verified `main` is pushed, deployed, and online-verified.
- Production `release-state` identifies the exact source commit/tree, migration hash, active slot/upstream, and immutable image.
- Any unavailable interval is at most five minutes; normal blue-green success should report no downtime.
- Existing release evidence and the protected scheduling worktree remain untouched.
