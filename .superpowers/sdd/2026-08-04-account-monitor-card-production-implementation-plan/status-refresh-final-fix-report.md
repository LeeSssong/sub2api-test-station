# Status Refresh Final Fix Report

## Scope

Final fix wave 1/1 addresses only the two reviewed selected-window request-evidence findings:

1. Threshold-qualified real-request evidence now determines account service state immediately after the paused-management check, independent of latest-probe freshness or presence.
2. One or two real requests retain the latest-probe gate and cannot become a `real_requests` state override merely because the aggregate probe query returned no row.

No frontend behavior, score/cost formulas, schemas, deployment files, or production state changed. The existing coordinator-owned `progress.md` entry remains unmodified.

## TDD Evidence

Test guidance read before test edits:

- `superpowers:test-driven-development`
- `superpowers:test-driven-development/writing-good-tests.md`

The regressions name the protected breaks and assert state, eligibility, and ranking through real `ListWindow` projections. Expected values are literal contract outcomes rather than values calculated by production helpers.

### RED 1: Threshold Overrides Probe Gate

Added `TestAccountMonitorWindowServiceStateUsesOnlyThresholdQualifiedRealRequestsBeforeProbeGate` and `TestAccountMonitorWindowThresholdQualifiedRealRequestsIgnoreAbsentLatestInGlobalAndGroupProjections`.

Command:

```sh
go test ./internal/service -count=1 -run '^TestAccountMonitorWindowServiceStateUsesOnlyThresholdQualifiedRealRequestsBeforeProbeGate$'
```

Observed RED:

```text
threshold-qualified real requests with absent latest probe = "pending", want available
FAIL github.com/Wei-Shaw/sub2api/internal/service
```

Projection RED command:

```sh
go test ./internal/service -count=1 -run '^TestAccountMonitorWindowThresholdQualifiedRealRequestsIgnoreAbsentLatestInGlobalAndGroupProjections$'
```

Observed RED:

```text
global threshold success ... ServiceState:"pending" ... want available and ranked without latest probe
FAIL github.com/Wei-Shaw/sub2api/internal/service
```

### RED 2: Subthreshold Requests Retain Latest-Probe Gate

Added `TestAccountMonitorWindowSubthresholdRealRequestsKeepLatestProbeGateInGlobalAndGroupProjections`.

Command:

```sh
go test ./internal/service -count=1 -run '^TestAccountMonitorWindowSubthresholdRealRequestsKeepLatestProbeGateInGlobalAndGroupProjections$'
```

Observed RED:

```text
global subthreshold success ... LatestStatus:"failed" ... ServiceState:"available" ... want failed latest probe to make it unavailable and unranked
FAIL github.com/Wei-Shaw/sub2api/internal/service
```

## Implementation

`accountMonitorWindowServiceState` now checks a small explicit threshold predicate immediately after the paused-management check. When `Source == "real_requests"` and `SampleCount >= AccountMonitorGroupEvidenceMinSamples`, raw `SuccessSampleCount` decides `available` versus `unavailable`. All other evidence, including the one-to-two-request fallback, continues through the existing stale/latest-probe gate.

This preserves probe facts and timestamps: `LatestStatus`, `Latest`, `Timeline`, `checked_at`, and `evidence.observed_at` were not changed.

## GREEN Verification

Focused command:

```sh
go test ./internal/service -count=1 -run '^(TestAccountMonitorWindowServiceStateUsesOnlyThresholdQualifiedRealRequestsBeforeProbeGate|TestAccountMonitorWindowThresholdQualifiedRealRequestsIgnoreAbsentLatestInGlobalAndGroupProjections|TestAccountMonitorWindowSubthresholdRealRequestsKeepLatestProbeGateInGlobalAndGroupProjections)$'
```

Output:

```text
ok github.com/Wei-Shaw/sub2api/internal/service 0.693s
```

Required package command:

```sh
go test ./internal/service -count=1
```

Output: exit 0.

Diff validation:

```sh
git diff --check
```

Output: exit 0.

## Files Changed

- `upstream/sub2api/backend/internal/service/account_monitor_service.go`
- `upstream/sub2api/backend/internal/service/account_monitor_service_test.go`
- `.superpowers/sdd/2026-08-04-account-monitor-card-production-implementation-plan/status-refresh-final-fix-report.md`

## Self-Review

- Paused management remains the first state decision.
- Threshold is explicit and uses the existing minimum-sample constant.
- Raw success count, rather than success rate, drives threshold-qualified state.
- Regression coverage exercises both global and group projections, as well as the direct state boundary.
- The below-threshold regression protects the aggregate/latest-query race: a fresh failed latest probe remains authoritative.
- No deployment, push, production-state, or coordinator-ledger change was made.
