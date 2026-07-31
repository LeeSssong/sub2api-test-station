# Task 1 report — synchronization policy and pure logic

STATUS: DONE

## Scope

Added a backward-compatible `accounts.extra` policy key and pure decision
logic for native billing multiplier synchronization. Missing policy values
default to `upstream_managed`; explicit `manual_override` prevents writes;
malformed policy values are rejected. The decision function validates only
`effective_rate_multiplier`, distinguishes a missing field from a malformed
present field, and requires a finite positive value within the
current `decimal(10,4)` account-column bound, and suppresses idempotent writes.

No repository, audit, cache, scheduler, lifecycle, or production deployment
code was changed in this task.

## Changed files

- `upstream/sub2api/backend/internal/service/upstream_billing_rate_multiplier_sync.go`
- `upstream/sub2api/backend/internal/service/upstream_billing_rate_multiplier_sync_test.go`
- `docs/project/project-progress.md`

## Verification evidence

RED (before implementation):

```text
go test ./internal/service -run 'Test(DecideUpstreamBillingRateMultiplierSync|UpstreamBillingRateMultiplierPolicyFromExtra)$' -count=1
...
undefined: UpstreamBillingRateMultiplierPolicyManaged
undefined: UpstreamBillingRateMultiplierDecisionReasonUpdated
FAIL
```

GREEN (targeted tests):

```text
go test ./internal/service -run 'Test(DecideUpstreamBillingRateMultiplierSync|UpstreamBillingRateMultiplierPolicyFromExtra)$' -count=1
ok   github.com/Wei-Shaw/sub2api/internal/service  2.198s
```

GREEN (service package):

```text
go test ./internal/service -count=1
ok   github.com/Wei-Shaw/sub2api/internal/service  109.555s
```

`git diff --check` also passed.

## Self-review

The implementation is pure and idempotent, uses the existing numeric Extra
decoder and multiplier equality helper, does not infer manual override from a
normal rate edit, and does not fall back from a missing effective field to
group/resolved fields. Tests cover valid, invalid, unchanged, manual override,
missing-field, default policy, and malformed policy cases.

## Remaining concerns

The overall feature remains **进行中** in the project ledger. Later tasks must
wire this decision into repository transactions, audit events, cache/scheduler
refresh, lifecycle and periodic probe paths, then push, deploy, and verify in
production.
