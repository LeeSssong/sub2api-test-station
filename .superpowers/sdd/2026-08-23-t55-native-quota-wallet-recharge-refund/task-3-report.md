# Task 3 report

Implemented coordinator routing for native balance writers. Usage, redeem, promo, administrator balance updates, gateway legacy billing, and payment refund rollback now accept and use `QuotaWalletService` when wired; legacy repository fallback remains for isolated compatibility tests. Wire wiring creates one coordinator instance and injects it into relevant services.

Directly related validation:

- `go test ./internal/service ./cmd/server ./internal/repository -run 'QuotaWallet|UpdateBalance|UsageService|Redeem|Promo|Payment' -count=1`
- `git diff --check`

Known limitation: the primary SQL usage-billing repository path still owns its existing transaction and was not rewritten to invoke the Ent coordinator inside the same SQL transaction; the coordinator is used by the service fallback path and all native service writers listed above.

Review fix `cff753b8acdd6606f6ffc83af8959cf72207e6cb` routes production usage billing balance deduction, affiliate quota transfer credit, and OAuth first-bind default balance through the coordinator. Batch image hold reserve/capture/release still preserve total `balance` plus `frozen_balance` SQL semantics and require a follow-up transaction-aware wallet hold API before replacement.
