# Whole-Site Accounting Ledger: Task 7 Local Verification

**Date:** 2026-07-31
**Status:** local implementation and isolated PostgreSQL E2E verification complete; not deployed
**Implementation commits:** Task 7 test evidence and corrective disabled-route fix
**Branch:** merged from `codex/accounting-ledger` into local `main`

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

The initial Task 7 run did not have `RELAY_OPS_TEST_DATABASE_URL`, so its
application-construction checks used an in-memory repository. During the
final merged-tree verification, a temporary local PostgreSQL 18 container was
created with a dedicated `relay_ops_test` database. The destructive test
helper operated only on that disposable database, and the container was
removed immediately after the tests. No production or staging database URL
was read or used.

The in-memory application checks passed:

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
RELAY_OPS_TEST_DATABASE_URL=<temporary-local-postgres> go test ./internal/store -count=1
RELAY_OPS_TEST_DATABASE_URL=<temporary-local-postgres> go test ./internal/nativeopssilence -count=1
RELAY_OPS_TEST_DATABASE_URL=<temporary-local-postgres> go test ./internal/app -count=1
bash tests/operations/reset_accounting_baseline_test.sh
git diff --check
```

The complete Go suite reported `ok` for every test package, including
`internal/accounting`, `internal/app`, `internal/http`, `internal/scheduler`,
and `internal/store`; `cmd/relay-ops` correctly reports no test files. The
database-backed store, native silence, and app packages also passed against
the disposable PostgreSQL instance. The accounting fixture now creates the
minimal Sub2API public-table schema when the isolated database is otherwise
empty, while remaining a no-op when those production-owned tables already
exist.

The new runbook verification record also documents this same boundary. It
states that no account credential was supplied to the tests and that no
credential appeared in the application test payload, page/API assertions, or
Task 7 source diff. Existing page HTML and internal-error assertions cover
credential-leak boundaries; the successful cash-event JSON response test
asserts its expected response shape without claiming a separate forbidden-key
assertion.

## Not verified

- Staging reset, staging activation, or a first post-reset snapshot.
- Production deployment and observed scheduled settlement after 00:10
  Asia/Shanghai.

These remain required before the ledger can be described as deployed or
activated.
