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
