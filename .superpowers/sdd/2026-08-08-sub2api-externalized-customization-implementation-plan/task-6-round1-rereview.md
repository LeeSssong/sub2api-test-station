### Finding Verdicts

- C1 Critical: ADDRESSED. `relay-ops-service/internal/app/app.go:418` obtains billing sources inside the scheduled collection closure; `:428` gives every collection a 20-second context, and `:429` constructs a `BalanceCollector` with `FreshFor: 10 * time.Minute`. `:457` wires the authenticated control-plane server to the official adapter and durable store. The collector remains in that background closure, not the routing or charging path.
- C2 Critical: ADDRESSED. `relay-ops-service/internal/billing/externalization.go:342` canonicalizes and hashes the validated allowed-field map; `relay-ops-service/internal/store/postgres.go:735` persists and compares command ID, actor, account, command name, contract version, and payload hash for the idempotency key. `relay-ops-service/internal/sub2api/client.go:874` calls the versioned external-command endpoint, and `upstream/sub2api/backend/internal/handler/admin/account_handler.go:1036` invokes the official `executeAdminIdempotent` boundary with account, command ID, and fields. The diff adds direct official-endpoint replay/conflict coverage in `upstream/sub2api/backend/internal/handler/admin/account_external_command_test.go:949`. Limitation: the PostgreSQL durable-identity test in `relay-ops-service/internal/store/postgres_test.go:811` was skipped without `RELAY_OPS_TEST_DATABASE_URL`; that skipped integration case is not counted as passing evidence.
- I1 Important: ADDRESSED. `relay-ops-service/internal/billing/externalization.go:392` maps a stored `failed` result to the stable `ErrAccountUpdateFailed`, with first-failure/replay coverage at `relay-ops-service/internal/billing/externalization_test.go:423`.
- I2 Important: ADDRESSED. `relay-ops-service/internal/controlplane/server.go:586` requires an active admin identity, and `:606-608` overwrites body-supplied actor, account, and idempotency key with authenticated/request-bound values. `relay-ops-service/internal/controlplane/server_test.go:655` covers both unauthorized and forbidden cases and verifies that the body actor cannot reach the writer.
- I3 Important: ADDRESSED. `relay-ops-service/internal/billing/http_adapter.go:529` creates a bounded context for every adapter request even when the HTTP client was injected. `relay-ops-service/internal/billing/adapter_test.go:188` proves a delayed injected client fails and writes no fact.
- I4 Important: ADDRESSED. `relay-ops-service/internal/billing/externalization.go:296` accepts a scheduled observation time and derives both fact timestamps from it; `relay-ops-service/internal/app/app.go:417` shares one timestamp across the scheduled pass. `relay-ops-service/internal/billing/adapter_test.go:206` covers retry after an ambiguous append with the same observation identity.

### New Breakage in the Fix Diff

None.

### Out-of-Scope Observations

The intentionally deferred future-dated-fact Minor item was not re-reviewed in this round.

### Verdict

All findings addressed, no new Critical/Important breakage.
