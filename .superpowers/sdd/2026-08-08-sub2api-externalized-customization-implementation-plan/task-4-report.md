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

## Fix Round 2

### RED / GREEN

- **Trusted-proxy session binding:** RED command
  `cd relay-ops-service && go test ./internal/adminauth -run 'TestRequireAdmin(UsesOriginal|IgnoresForwarded)' -count=1 -v`
  failed with `ClientIP:172.20.0.4`, proving the relay socket peer was sent
  instead of the browser IP. GREEN now trusts `X-Forwarded-For`/`X-Real-IP`
  only when the immediate peer is loopback/private, selects the first valid
  forwarded address, and retains the peer for untrusted callers. The real
  `adminauth.RequireAdmin -> sub2api.HTTPReader.VerifyAdminSession ->
  /api/v1/auth/me` boundary test asserts all three IP headers equal
  `198.51.100.23`; focused result: `PASS`.
- **Empty read-model metadata:** RED was the existing focused assertion showing
  empty accounts/profitability as `unknown` and `0` (the accounting assertion
  covered only one endpoint). GREEN uses the four projection calculation
  version constants and API empty convention `freshness_seconds=-1`; the
  injected-clock test covers all four empty endpoints and verifies a non-empty
  account row still recomputes to `90` seconds. Focused result: `PASS`.
- **Refresh error durability:** RED added tests for a failed official dispatch
  plus failed completion and for replaying a durable `pending` result. Before
  the fix, the former returned only the dispatch error and the latter returned
  success without dispatch. GREEN joins both errors with `errors.Join` and
  returns `ErrExternalizationCommandPending` for pending/processing/unknown
  replays, while accepted/failed replays remain idempotent. Focused result:
  `PASS`.

### Commands / Results

```text
cd relay-ops-service
go test ./internal/adminauth -run 'TestRequireAdmin(UsesOriginal|IgnoresForwarded)' -count=1 -v   # PASS
go test ./internal/sub2api -run '^TestAdminAuthClientBoundarySendsOriginalBrowserIPToAuthMe$' -count=1 -v   # PASS
go test ./internal/controlplane -run 'Test(StoreReaderRecomputes|Refresh(ReturnsCombined|DoesNotAccept))' -count=1 -v   # PASS
go test ./internal/controlplane ./internal/adminauth ./internal/http ./internal/sub2api -count=1   # PASS
go test ./internal/controlplane ./internal/adminauth ./internal/http -run 'Test(Auth|ReadModel|Refresh|Xingqiao)' -count=1 -v   # PASS
go test ./...   # PASS
go test -race ./internal/controlplane ./internal/adminauth ./internal/http   # PASS
go vet ./...   # PASS
cd .. && bash tests/infra/validate-sub2api-update-routing.sh   # PASS
git diff --check   # PASS
```

### Residual Concerns

Forwarded-IP trust is intentionally limited to private/loopback immediate
peers; deployments using a public-address Caddy-to-relay hop must configure a
private network or the relay will fail closed to the proxy peer. A completion
write failure after an official failure can still leave the durable row
pending when storage is unavailable; subsequent identical requests now fail
closed instead of being reported accepted, and operator/storage repair is
required before retrying.
