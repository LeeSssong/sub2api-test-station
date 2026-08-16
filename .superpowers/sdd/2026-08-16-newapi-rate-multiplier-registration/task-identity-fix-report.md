# T13 Whole-Review P1 Identity Fix

- Service NewAPI ledger identity now accepts either an explicit NewAPI account
  monitor source or an `upstream_billing_probe=unsupported` snapshot.
- Repository Claim/Complete CAS predicates accept the same two identities while
  still rejecting successful native probe snapshots.
- OAuth, non-API-key, unknown identity, fuzzy matches, invalid ratios, and
  successful native declarations remain excluded.
- The existing exact-log registration regression now exercises the unsupported
  probe identity path; the ledger unit case records it as NewAPI.
- Validation passed:
  `go test ./internal/service ./internal/repository ./cmd/server -run '^$'`
  and `go test ./internal/repository -run TestCompleteNewAPIRateRefreshRemovesNestedClaimFields -count=1`.
- Frozen review package including all three untracked Task 2 implementation
  files: `/tmp/t13-whole-branch-final-with-untracked.diff`.
- Frontend Vitest remains blocked by missing dependencies and registry
  `ENOTFOUND`; no frontend pass is claimed.

## Scoped correction after re-review

- Restored `usageCostLedgerForAccount` so an unsupported probe alone remains
  excluded from the generic usage-cost evidence ledger.
- Added registration-only `newAPIRateRegistrationIdentity`: it accepts either
  an explicit NewAPI balance source or an unsupported native probe, while
  rejecting OAuth/non-API-key and explicit Sub balance identity.
- Both the pre-claim candidate gate and the post-lookup exact-record eligibility
  gate use this registration-only proof. Claim/Complete repository CAS parity
  remains unchanged.
- Reproduced the two reviewer failures before the fix, then passed the exact
  focused service selection, the focused repository SQL test, package compile,
  formatting, and `git diff --check` after the fix.

## Final narrow verification (2026-08-17)

- The user replaced the remaining review gates with direct functional
  verification. No further scoped or whole-branch review was requested or run.
- Generic usage-cost evidence still treats an unsupported-only probe as an
  unknown ledger. Registration uses its own identity proof and accepts either
  `account_monitor_balance.source=newapi` or
  `upstream_billing_probe.status=unsupported`.
- Registration still requires an API-key account, no successful native probe,
  an exact NewAPI request match, and a valid top-level `other.group_ratio`.
  Claim and Complete apply the same repository eligibility predicate.
- PASS:
  `go test ./internal/service -run '^(TestUsageCostEvidenceRegistrarRequiresKnownNativeLedger|TestNewAPIUsageRecordEligibilityRequiresExactSuccessfulNewAPIUsage|TestUsageCostEvidenceRegistrarReusesNewAPILogForRateRegistration)$' -count=1`.
- PASS:
  `go test ./internal/repository -run '^(TestNewAPIRateRefreshRepositoryContract|TestCompleteNewAPIRateRefreshRemovesNestedClaimFields)$' -count=1`.
- PASS:
  `go test ./internal/service ./internal/repository ./cmd/server -run '^$'`.
- PASS: `gofmt -d` produced no output and `git diff --check` exited 0.
- Frontend Vitest was not run: the worktree has no Vitest binary and the prior
  dependency installation attempt failed with `ENOTFOUND registry.npmjs.org`.
