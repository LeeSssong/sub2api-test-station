# Final fix round 1 report

Base commit: `bee5fe2e3`

Status: local implementation and validation complete. The project remains `进行中` because this dispatch did not authorize push, deployment, production data access, or production verification, and the final independent re-review remains outside this sole-implementer task.

## Finding 1 — explicit manual override semantics

- Added the typed caller intent `rate_multiplier_policy` with allowed values `upstream_managed` and `manual_override` to shared service inputs and relevant HTTP/import request types.
- New accounts persist `upstream_managed` by default even when `rate_multiplier` is supplied. Mere multiplier presence no longer implies an override.
- Create, update, and bulk update reject unknown policy values. `manual_override` additionally requires a supplied multiplier so policy and value are persisted atomically.
- Omitted policy on edit preserves the current explicit or legacy implicit policy. Explicit managed/manual selections replace it atomically with the rest of the account update.
- The policy key is stripped from arbitrary `extra` input on create, update, and bulk update, preventing callers from bypassing typed validation. Bulk sanitization clones the caller map first.
- Duplicate explicitly carries a valid source policy. Generic create/update/batch/bulk, Codex PAT create, Codex session create/update import, Grok SSO import, and data export/import propagate the typed field.
- The standard OpenAI API-key create/edit UI exposes a managed/manual selector, disables multiplier editing while managed, defaults legacy accounts to managed, and submits explicit intent. English and Chinese strings and frontend request types were updated.
- Regression coverage proves default managed create, multiplier-without-policy remaining managed, explicit manual override, explicit return to managed, unrelated legacy edit preservation, invalid intent rejection, and `extra` smuggling prevention.

## Finding 2 — commit-order-safe Redis fencing

- Normal account edits now calculate the next durable version while holding the account row lock:
  `GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')`.
- The locked database value is assigned to the service account and explicitly persisted by the Ent update, so the committed row, direct cache refresh, and outbox/retry re-read observe the same version.
- Raw account-row mutations and CAS paths now use the same monotonic database expression across the main account repository, Ollama group writes, proxy-driven account invalidation/reassignment, and transactional account quota usage.
- Existing Redis stale-write and tombstone comparisons remain unchanged.
- Repository SQL tests assert the lock-derived version and monotonic expressions for normal edits, bulk updates, probe snapshot updates, and multiplier CAS updates.
- Added an integration regression in `account_rate_multiplier_sync_integration_test.go` that starts a probe transaction first, commits a normal edit, then performs and commits the probe mutation. It asserts that the probe row version is strictly newer and that the Redis cache accepts the final database snapshot.

## Finding 3 — mode-safe probe coalescing

- Introduced explicit `manual`, `scheduled`, and `lifecycle` probe modes.
- Singleflight keys are now `<accountID>:<mode>`, so calls with different semantic guards or audit attribution never share a leader.
- Manual remains forced and does not require enabled/active/due state.
- Scheduled continues to recheck active, enabled, and due state after acquiring the shared concurrency slot.
- Lifecycle remains forced while rechecking active state immediately before HTTP.
- Added the explicit `manual` synchronization trigger; persistence now receives the trigger for the mode that actually executed the network request.
- Deterministic blocked-upstream tests prove three concurrent cross-mode requests make three upstream calls with manual/scheduled/lifecycle triggers, while duplicate manual requests still make one upstream call.

## TDD and validation evidence

- Finding 1 RED: new frontend policy tests failed 4 cases while the pre-fix payload inferred/omitted the wrong intent. GREEN: focused Vitest completed with 51/51 passing.
- Finding 2 RED: strengthened SQL expectations failed for normal locked edit, bulk update, probe snapshot update, and multiplier CAS while they still used transaction-start timestamps. GREEN: repository tests pass with the monotonic database expression and integration-tag compilation succeeds.
- Finding 3 RED: the cross-mode test timed out waiting for three upstream entries because the account-ID-only key admitted only one; the same-mode audit assertion observed `scheduled` instead of `manual`. GREEN: focused mode/guard/trigger tests pass.
- Required backend validation:
  `go test ./internal/service ./internal/handler/admin ./internal/repository ./internal/server/routes ./cmd/server -count=1`
  passed all five packages.
- Repository validation:
  `go test ./internal/repository -count=1`
  passed.
- Integration compilation:
  `go test -tags=integration -c ./internal/repository -o /tmp/sub2api-repository-integration.test`
  passed.
- Frontend validation:
  `./node_modules/.bin/vitest run src/components/account/__tests__/CreateAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts --reporter=dot`
  passed 51 tests; `./node_modules/.bin/vue-tsc --noEmit` passed.
- `git diff --check` passed.

## Integration limitation and remaining work

The targeted real PostgreSQL/Redis execution was attempted with:

`go test -tags=integration ./internal/repository -run '^TestProbeTransactionStartedBeforeNormalEditPublishesStrictlyNewerCacheVersion$' -count=1 -timeout=2m`

The harness panicked before test execution with the exact environment error `rootless Docker not found`. The integration test compiles, but it must be executed in a Docker-capable environment before deployment.

No code was pushed, deployed, or exercised against production. Required remaining steps are scoped independent re-review, service-side push, deployment, and production verification before either progress ledger may mark this feature complete.
