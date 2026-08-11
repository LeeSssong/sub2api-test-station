# Task 1 — Restore Resolved Account-Cost Billing Contract

Status: ready for parent integration review (not deployed).

## Scope

Restored the reviewed `ce8b96a95685db43302bd191883050ead203cae4` account-cost billing contract against the current candidate without altering customer balance, API-key quota/rate-limit, or subscription billing semantics.

## RED evidence

Before the restoration, `go test -tags=unit ./internal/service -run 'TestBuildUsageBillingCommand_UsesResolvedAccountCostForAccountQuota' -count=1` failed to compile because `postUsageBillingParams` had no `AccountCost` or `AccountCostSet` fields. The focused regression test already covered both the explicit and legacy fallback cases.

## Implementation

- Restored `AccountCost`, `AccountCostSet`, and `accountCostForBilling`.
- Account quota writes now use the explicit resolved value when present and otherwise retain `TotalCost * AccountRateMultiplier`.
- Restored resolved account-cost propagation from both Gateway and OpenAI usage-recording call sites.
- Customer-facing billing continues to use `ActualCost` for balance, API-key, and subscription paths.

## Verification

- PASS: focused resolved-account-cost regression test.
- PASS: `go test -tags=unit ./internal/service -count=1`.
- PASS: `git diff --check`.

## Independent review

The first review found that `notifyAccountQuota` was the third account-quota consumer and still reconstructed `TotalCost * AccountRateMultiplier`, so the initial implementation was incomplete. The wording above is superseded by the fix round below.

## Fix round 1

Files changed:

- `upstream/sub2api/backend/internal/service/gateway_usage_billing.go`: `notifyAccountQuota` now calls `accountCostForBilling`.
- `upstream/sub2api/backend/internal/service/gateway_service_subscription_billing_test.go`: added notification-consumer regression coverage and marked billing fixtures as complete usage so `Normalize` does not intentionally zero their audit-only costs.

TDD RED before the production change:

```text
$ go test -tags=unit ./internal/service -run '^TestNotifyAccountQuota_UsesResolvedAccountCost$' -count=1 -v
--- FAIL: TestNotifyAccountQuota_UsesResolvedAccountCost
gateway_service_subscription_billing_test.go:144: notifyAccountQuota log = ... account_cost=0.45 ..., want resolved account_cost=0.24
FAIL
```

Verification after the change:

- PASS: `go test -tags=unit ./internal/service -run '^(TestBuildUsageBillingCommand|TestNotifyAccountQuota_UsesResolvedAccountCost)$' -count=1` (`ok`, exit 0).
- PASS: `git diff --check` (exit 0).
- ATTEMPTED: `go test -tags=unit ./internal/service -count=1`; it still fails on unrelated pre-existing request-ID expectations in `gateway_record_usage_test.go` (`client:...` vs `billing:...`) and account test fixtures, outside this contract.

Self-review confirms all three account-quota consumers (legacy write, billing command, notification) share the resolver, both usage-recording call sites propagate `UsageLog.AccountCost`, customer billing remains on `ActualCost`, and the fix is one production line plus focused coverage.

## Concerns

The full service package remains blocked by unrelated baseline test failures noted above; deployment and online verification remain owned by the parent release workflow.
