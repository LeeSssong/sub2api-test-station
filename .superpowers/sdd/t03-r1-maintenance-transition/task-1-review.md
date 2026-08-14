# T03-R1 Task 1 Independent Scoped Review

## Review scope

- Baseline: `e3449fe38`
- Candidate: `f2145eb9ae275dd0a76d3ce44f1644cebe8131fd`
- Reviewed commits: `d2fc90632ff4276c7e4491220a028e931485f964`, `f2145eb9ae275dd0a76d3ce44f1644cebe8131fd`
- Scope: exact maintenance migration transition only. This review made no production, upstream, push, or deployment calls.

## Verdicts

- **Spec Compliance: REJECT**
- **Code Quality: REJECT**

The executor change itself correctly adds only the specified complete 64-hex-pair comparison and the focused regression proves both the allowed path and fail-closed behavior for a one-character mutation and an unknown target. However, the changed maintenance runbook is not accurate about the hard unavailable-time bound, which is a required part of this task's operator contract. The candidate must not be authorized for maintenance deployment until that documentation is corrected and independently re-reviewed.

## Open findings

### P1 — Runbook states a 600-second maximum although the executor hard-limits the outage to 300 seconds

`docs/runbooks/sub2api-blue-green-production-deployment.md:93` says the host executor has a “default 300-second, maximum 600-second” unavailable window. The actual executor rejects `MAINTENANCE_UNAVAILABLE_SECONDS` above `300` with `MAINTENANCE_UNAVAILABLE_SECONDS must be an integer between 1 and 300` (`ops/deploy-sub2api-blue-green-host.sh:1351-1353`, covered by `test_maintenance_window_hard_maximum`). The `+600 seconds` controller/recovery deadline is not an authorization to extend the API/worker outage. This conflicts with the required accurate 300-second maintenance-path documentation and can mislead the operator about the authorized interruption bound.

Required resolution: state an exact maximum 300-second unavailable window, and distinguish it from the separate controller/recovery deadline if that deadline remains documented.

No other open findings.

## Evidence checked

1. The only new executor logic is `MAINTENANCE_7_OLD_MIGRATIONS_HASH` equal to `f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc`, `MAINTENANCE_7_NEW_MIGRATIONS_HASH` equal to `6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`, and one full-equality disjunct in `approved_maintenance_transition`. There is no wildcard, prefix comparison, environment override, or gate bypass in the candidate diff.
2. The migration-difference gate invokes `approved_maintenance_transition "$state_migrations_hash" "$migrations_hash"` before `maintenance_stopped=true` and before the `docker compose stop` of API/worker services. The focused test asserts `migration_set_changed` and no Docker/Caddy mutation for both a one-character-mutated target and an unknown 64-hex target.
3. Recomputed the target migration-set hash with the controller's normalized SQL hashing algorithm: `6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71`. Independently computed migration 222 SHA-256: `47f786d6b2b020d0211a17d4ccd2bc6bb3774a315f483fdc0ac45657c9ee738e`. Its SQL is expand-only: it creates independent evidence/review/account-day/settings tables and indexes; it neither alters `usage_logs` nor includes a historical backfill.
4. The candidate scope is limited to the five task-package files: host executor, host-executor test, runbook, project progress update, and task report. It does not change application code, migrations, production configuration, GitHub Actions, or other worktrees.
5. The executor retains the existing bounded maintenance path that stops only API/worker and does not stop or recreate PostgreSQL, Redis, or Caddy. The runbook correctly describes forward-only application rollback for migration 222; the P1 finding is limited to its contradictory outage maximum.

## Fresh local verification

All commands below were run against candidate `f2145eb9ae275dd0a76d3ce44f1644cebe8131fd` and exited `0`:

```bash
ONLY_TEST=maintenance-t03-r1-transition bash tests/operations/deploy_sub2api_blue_green_host_test.sh
ONLY_TEST=maintenance bash tests/operations/deploy_sub2api_blue_green_host_test.sh
bash tests/operations/release_sub2api_blue_green_test.sh
bash -n ops/deploy-sub2api-blue-green-host.sh
git diff --check e3449fe38..f2145eb9ae275dd0a76d3ce44f1644cebe8131fd
```

The focused test includes a successful exact-pair maintenance traversal plus mutated/unknown-target `migration_set_changed` failures before service mutation. The full maintenance suite also exercises the 300-second maximum, rollback, deadline partitioning, and shared-service protections. The controller suite verifies maintenance-flag/hash forwarding. Test success does not cure the P1 operator-documentation mismatch above.
