# Task 3 Report: Keep Billing Probes Observational

## Scope

Task 3 only. This change removes billing probe authority to recover generic
account scheduling state. It does not change the probe request, snapshot
persistence, failure/unsupported scheduling, or optional billing rate sync.

## Changes

- Removed `recoveryMu`, `recoveryStreak`, constructor initialization, and the
  `recordSuccessfulProbeRecovery` callback from
  `internal/service/upstream_billing_probe.go`.
- Removed the failure-path recovery-streak cleanup.
- Kept `SetAccountRuntimeBlocker` as a no-op compatibility setter because the
  existing service wiring still calls it; probes no longer clear runtime or
  persisted scheduling blocks.
- Changed the lifecycle test so one and two successful probes both preserve a
  future generic `TempUnschedulableUntil` and make zero clear calls.

## RED/GREEN Evidence

- RED before implementation: the updated lifecycle test failed after the
  second successful probe because the old code called `ClearTempUnschedulable`.
- GREEN after implementation:
  `go test ./internal/service -run '^TestUpstreamBillingProbeSuccessfulProbesDoNotClearGenericTempUnschedulable$' -count=1`
  passed.

## Direct Tests

- Passed:
  `go test ./internal/service -run '^TestUpstreamBillingProbe' -count=1`
- Passed:
  `go test ./internal/repository -run 'Test.*UpstreamBillingProbe|Test.*TempUnschedulable' -count=1`
- `git diff --check` passed.
- The broader planned service filter failed in two unrelated scheduler tests:
  `TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceTopKFallback`
  and
  `TestOpenAIGatewayService_SelectAccountWithScheduler_LoadBalanceDistributesAcrossSessions`.
  No unrelated files were changed.

## Residual Check

No `recoveryMu`, `recoveryStreak`, `recordSuccessfulProbeRecovery`, or probe
recovery log references remain in the Task 3 service/test files. CN-provider
owned recovery code was not modified.

## Commit

Commit SHA is reported in the task completion message.
