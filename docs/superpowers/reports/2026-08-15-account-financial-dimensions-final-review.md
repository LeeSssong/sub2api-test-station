# T11 Final Review Record

Date: 2026-08-15

Baseline: `bdfd05578`

Runtime candidate: `aa224a764`

## Verdict

`READY_FOR_ROOT_REVIEW_WITH_USER_WAIVER`

The planned fresh whole-branch independent review did not complete because the
desktop agent message channel delivered opaque encrypted payloads and both
installed fallback CLIs returned HTTP 401. After this was reported for three
consecutive attempts, the user explicitly instructed: `跳过这个审核`.

This record does not claim an independent review PASS. It records the user's
explicit process waiver and preserves every other release gate.

## Completed Independent Reviews

- Task 1 independent review: PASS.
- Task 2 initial review and first scoped re-review: findings were reported and
  fixed in `2e756fe60` and `aa224a764`.
- Task 3 scoped re-review: PASS at `2e756fe60`.

## Root Controller Review

The root controller checked the complete candidate diff against the approved
spec and plan and found no remaining P0-P2 issue in the following required
areas:

- persisted `usage_logs.group_id` is the group attribution source;
- whole-site totals remain single-counted while group rows use
  `(group_id, account_id)` projections;
- unassigned usage is explicit and account-level/OAuth adjustments are not
  guessed into groups;
- the repository uses one repeatable-read, read-only snapshot;
- the response is additive and existing top-level fields remain present;
- group views consume backend group rows, with editing restricted to the
  whole-site today view;
- exception navigation preserves account, pending review, and exact range,
  including rolling 24 hours;
- loading, data, empty, retryable error, explicit-all filters, and overlapping
  request ordering have focused regression coverage;
- Chinese and English production locale keys exist for new visible copy;
- no migration, dependency, billing/scheduling write, GitHub Actions,
  `/xingqiao/**`, external control plane, or second ledger change exists.

## Fresh Verification

```text
Backend AccountFinancial service/repository/handler tests: PASS
Backend Financial/CostException route selection: PASS (no matching route test)
Frontend focused matrix: 4 files, 42 tests PASS
pnpm typecheck: PASS
pnpm build: PASS
git diff --check: PASS
Migration diff: empty
Dependency diff: empty
.github/workflows diff: empty
Forbidden control-plane symbol scan: empty
```

## Visual Evidence Boundary

Controlled local browser QA covered desktop and 390x844 mobile layouts, group
selection, simultaneous group notices, financial error/retry, and exception
loading/empty/error states. It is not production acceptance evidence.

## Remaining Gates

- merge into current root `main`;
- rerun the focused matrix and release scope checks on merged `main`;
- create canonical final-tree release evidence;
- push `main`;
- run the reviewed local/host blue-green production chain;
- stop before production mutation if `downtime_required=true`;
- complete authenticated production acceptance and health checks.
