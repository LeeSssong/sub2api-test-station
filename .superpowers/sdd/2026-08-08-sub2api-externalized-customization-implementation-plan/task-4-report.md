# Task 4 Report: Same-Origin Control Plane

## Status

Local implementation and verification complete. No production deployment or
project-progress ledger update was performed by this task.

## RED / GREEN

RED was recorded before implementation with:

```text
go test ./internal/controlplane ./internal/sub2api \
  -run 'Test(RequireAdminTreatsInactiveSessionAsLocalUnauthorized|VerifyAdminSessionForwardsBearerAndRequiresAdmin)' -count=1 -v
```

The control-plane test failed because it passed `RemoteAddr` including the
port and returned `403` for an inactive session. The Sub2API test did not
compile because `Origin` was not part of the isolated authentication session.

GREEN added canonical client-IP parsing, Origin propagation, local `401` for
inactive sessions, and no `Set-Cookie` side effect. Follow-on RED/GREEN tests
cover the mounted `/api/v1/xingqiao/*` route, top-level freshness fields,
command audit facts, explicit core outbox configuration, and a nil persistent
consumer rejection.

## Specification Mapping

- `GET /api/v1/xingqiao/accounts/monitor`, profitability, accounting, and
  reconciliation are mounted through the existing HTTP server. `StoreReader`
  is limited to relay-owned `relay_ops` projection loaders; it has no core
  account/group/usage/billing access.
- Every read response has `generated_at`, `source_watermark`,
  `freshness_seconds`, `completeness`, and `calculation_version` at the top
  level and retains the structured `freshness` object.
- Same-origin bearer verification calls only `/api/v1/auth/me`. It forwards
  the canonical socket client IP and Origin, does not forward Cookie or API
  key, turns inactive/invalid sessions into local `401`, and returns `403`
  only for an active non-admin identity.
- Refresh requires `Idempotency-Key`, uses the official
  `/api/v1/admin/accounts/{id}/refresh` endpoint with the service key, and
  never reuses the browser bearer on a write. The relay audit records actor,
  account command, idempotency key, result, and contract version before and
  after dispatch.
- Normal non-closed startup requires `RELAY_OPS_CORE_DATABASE_URL_FILE` and
  builds one Task-3 `NewPersistentConsumer` from the same relay Store used as
  its journal and by all four projections. `cmd/relay-ops` supervises the
  loop and terminates on an unexpected consumer failure.

## Runtime Call Chain

```text
core externalization_outbox
  -> CoreOutbox.ClaimBatch (SKIP LOCKED lease)
  -> events.NewPersistentConsumer(relay Store, accounts/profitability/accounting/reconciliation)
  -> relay_ops projection transaction + watermark/dead-letter
  -> CoreOutbox.MarkPublished or MarkFailed
```

The cross-DB adapter contains a fixed SQL allowlist for only
`externalization_outbox` claim/published/retry state. It does not query or
modify any core business table. Deployment must provide a least-privilege core
database role restricted to that table's SELECT/UPDATE operations.

## Sensitive Fields

- Request tests prove only Bearer is supplied to `/api/v1/auth/me`; Cookie and
  `x-api-key` are absent.
- Control-plane `401` responses neither echo Bearer/Cookie/API-key text nor
  emit logout cookies.
- The official refresh request carries the relay service API key and the
  idempotency key; the administrator Bearer is not available to that path.

## Verification

```text
cd relay-ops-service
go test ./...
go test -race ./internal/controlplane ./internal/adminauth ./internal/http
go vet ./...
git diff --check
```

All commands exited `0`.

## Residual Risk

The repository has no local core+relay database integration fixture for the
cross-database lease cycle. Unit coverage validates explicit configuration and
the persistent-consumer precondition; the deployed core role and full
claim/ack path still need a staging database exercise before production.

## Commit

Pending at report creation.

## Fix Round 1

### RED / GREEN

- `TestLoadKeepsReadOnlyExternalizationOptIn` first failed to compile because
  `ExternalizationEnabled` did not exist. The configuration now defaults it to
  `false`; only an explicit `RELAY_OPS_EXTERNALIZATION_ENABLED=true` requires
  `RELAY_OPS_CORE_DATABASE_URL_FILE` and starts the core outbox loop.
- Routing test first returned `404` after switching the Xingqiao mount to the
  established admin-auth middleware. The test was then updated to exercise the
  real session-verifier boundary and proved Bearer, User-Agent, forwarded IP,
  real IP, and Origin reach `/auth/me` verification while Cookie is not
  forwarded by Caddy.
- The PostgreSQL lease test initially failed with `expected 2 arguments, got
  3`, exposing the stale claim SQL placeholder after adding the token. It now
  passes against a fresh PostgreSQL 18 container: an expired `processing`
  lease is reclaimed with a new random token, and stale publish plus stale
  failure acknowledgement are both rejected.

### Fix Mapping

- Added same-origin Caddy `/api/v1/xingqiao/*` route with relay proxying,
  explicit User-Agent/Origin/trusted IP preservation, and Cookie removal.
  `tests/infra/validate-sub2api-update-routing.sh` verifies the route contract.
- Core outbox claims now include expired `processing` rows, set a cryptographic
  claim token, and fence `MarkPublished`/`MarkFailed` on event ID, owner, and
  token; both acknowledgement paths require exactly one updated row.
- Refresh commands use Store-backed claim/result persistence. The first
  matching actor/command/account/idempotency key dispatches; concurrent or
  repeated matching requests replay stored state without another official
  refresh. Conflicting command identity returns the Store conflict error.
- `StoreReader.Now` now recomputes `freshness_seconds` on read. Empty
  accounting/reconciliation responses carry deterministic generated-at,
  `empty` completeness, and their calculation version.

### Commands / Results

```text
cd relay-ops-service
go test ./internal/config ./internal/controlplane ./internal/http -run 'Test(LoadKeeps|Xingqiao|ReadModel)' -count=1 -v
go test ./internal/sub2api -run '^TestCoreOutboxReclaimsExpiredLeaseAndRejectsStaleOwner$' -count=1 -v
go test ./...
go test -race ./internal/controlplane ./internal/adminauth ./internal/http
go vet ./...
cd .. && bash tests/infra/validate-sub2api-update-routing.sh
git diff --check
```

All recorded commands exited `0`. The lease/fencing test used a fresh local
PostgreSQL 18 container and `RELAY_OPS_TEST_CORE_DATABASE_URL`; the container
was stopped with `--rm` after completion.

### Files

- Config/startup: `internal/config/*`, `internal/app/app.go`,
  `cmd/relay-ops/main.go`
- Auth/routing: `internal/adminauth/*`, `internal/http/*`, `infra/Caddyfile`,
  `tests/infra/validate-sub2api-update-routing.sh`
- Command replay/freshness: `internal/controlplane/*`, `internal/store/*`
- Core transport/migration: `internal/sub2api/outbox*`,
  `upstream/sub2api/backend/migrations/200_externalization_outbox.sql`

### Commit

Pending fix-round commit.
