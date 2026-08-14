# Task 1 Report — T03-R1 maintenance migration transition

## Status

`READY_FOR_INDEPENDENT_REVIEW`. The local implementation is committed, but it
has not been merged, pushed, deployed, or validated on production.

## RED evidence

Before the executor change, the focused regression was run with:

```bash
ONLY_TEST=maintenance-t03-r1-transition bash tests/operations/deploy_sub2api_blue_green_host_test.sh
```

It exited `1` with the expected pre-mutation gate:

```text
FAIL: T03-R1 maintenance transition failed: {
  "schema_version": 1,
  "downtime_required": true,
  "reason_code": "migration_set_changed",
  "reason": "candidate migration set differs from the active release",
  "estimated_unavailable_seconds": 300,
  "rollback": [
    "keep current active slot",
    "do not start candidate",
    "prepare an authorized maintenance release"
  ]
}
```

## Implementation summary

- Added only the seventh exact maintenance allowlist pair: active
  `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc` to
  T03-R1 `6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`.
- Added an executor regression that proves the exact pair uses the bounded
  maintenance path and that a one-character-mutated or unknown candidate from
  the same active hash remains `migration_set_changed` with no mutation.
- Updated the maintenance runbook for migration
  `222_account_financial_reconciliation.sql` (file SHA-256
  `47f786d6b2b020d0211a17d4ccd2bc6bb3774a315f483fdc0ac45657c9ee738e`),
  including its expand-only, no-`usage_logs`-change, no-backfill semantics and
  forward-only application rollback.

## Changed files

- `ops/deploy-sub2api-blue-green-host.sh`
- `tests/operations/deploy_sub2api_blue_green_host_test.sh`
- `docs/runbooks/sub2api-blue-green-production-deployment.md`
- `docs/project/project-progress.md`
- `.superpowers/sdd/t03-r1-maintenance-transition/task-1-report.md`

## GREEN verification

All commands below exited `0`:

```bash
ONLY_TEST=maintenance-t03-r1-transition bash tests/operations/deploy_sub2api_blue_green_host_test.sh
ONLY_TEST=maintenance bash tests/operations/deploy_sub2api_blue_green_host_test.sh
bash tests/operations/release_sub2api_blue_green_test.sh
bash -n ops/deploy-sub2api-blue-green-host.sh
git diff --check
```

## Commit SHA

Implementation commit: `d2fc90632ff4276c7e4491220a028e931485f964`.

## Concerns

The change deliberately authorizes only the reviewed complete hash pair. It
does not authorize any future migration-set transition, and production still
requires independent review, root authorization, merge, push, the separate
maintenance invocation, and online acceptance.
