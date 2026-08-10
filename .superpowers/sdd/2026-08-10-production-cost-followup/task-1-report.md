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

Reviewed the minimal diff against the historical contract and confirmed the two account-quota consumers call the shared resolver, both recording call sites propagate `UsageLog.AccountCost`, and no customer-cost assignment changed.

## Concerns

None identified. This is a source-only restoration; deployment and online verification remain owned by the parent release workflow.
