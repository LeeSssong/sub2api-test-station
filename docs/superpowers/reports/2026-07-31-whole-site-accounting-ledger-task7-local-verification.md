# Whole-Site Accounting Ledger: Task 7 Local Verification

**Date:** 2026-07-31
**Status:** local implementation verification complete; PostgreSQL E2E skipped; not deployed
**Implementation commits:** Task 7 test evidence and corrective disabled-route fix
**Branch:** `codex/accounting-ledger`

## Scope

Task 7 adds application-construction checks for the accounting ledger:

- accounting routes are absent when accounting is disabled; and
- an enabled accounting service can materialize an empty daily snapshot before
  any post-baseline usage exists.

The test builds the same App root route composition as production but uses an
in-memory accounting repository. It does not add an unauthenticated route,
database-credential override, or production bypass. During this work, the
test exposed and corrected a typed-nil interface bug that otherwise mounted
the protected accounting route when accounting was disabled.

## Date and environment boundary

This verification occurred on **2026-07-31**. The date **2026-08-02** is in
the future and is used only as a deterministic empty-baseline test date. It is
not a staging activation date, an applied reset date, or evidence of any
completed deployment.

`RELAY_OPS_TEST_DATABASE_URL` was not set in this environment. PostgreSQL E2E
tests therefore did not run; no test falls back to or connects to another
database. The Task 7 app-construction tests no longer need a database and
passed against their in-memory repository:

```text
=== RUN   TestAccountingIsNotMountedWhenDisabled
--- PASS: TestAccountingIsNotMountedWhenDisabled
=== RUN   TestAccountingEnabledBuildsZeroBaselineUntilNewUsageArrives
--- PASS: TestAccountingEnabledBuildsZeroBaselineUntilNewUsageArrives
PASS
```

No production database, staging database, reset `--apply`, deployment,
service restart, image publication, push, or production HTTP endpoint was
used.

## Verification performed

Fresh local commands completed with exit code 0:

```text
cd relay-ops-service && go test ./... -count=1
cd relay-ops-service && go vet ./...
bash tests/operations/reset_accounting_baseline_test.sh
git diff --check
```

The complete Go suite reported `ok` for every test package, including
`internal/accounting`, `internal/app`, `internal/http`, `internal/scheduler`,
and `internal/store`; `cmd/relay-ops` correctly reports no test files. The
focused app command passed without a database connection.

The new runbook verification record also documents this same boundary. It
states that no account credential was supplied to the tests and that no
credential appeared in the application test payload, page/API assertions, or
Task 7 source diff. Existing page HTML and internal-error assertions cover
credential-leak boundaries; the successful cash-event JSON response test
asserts its expected response shape without claiming a separate forbidden-key
assertion.

## Not verified

- PostgreSQL E2E execution: `RELAY_OPS_TEST_DATABASE_URL` was absent and no
  database target was permitted for this local task.
- Staging reset, staging activation, or a first post-reset snapshot.
- Production deployment and observed scheduled settlement after 00:10
  Asia/Shanghai.

These remain required before the ledger can be described as deployed or
activated.
