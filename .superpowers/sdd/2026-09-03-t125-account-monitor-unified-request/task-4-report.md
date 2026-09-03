# T125 Task 4: Account Monitor Public JSON Contract

## Scope

- Baseline `main`: `b7656bc6bd6a416df5f0b6158e629756ad1e2bf1`
- Limited to the admin account-monitor handler's public JSON response and its handler regression test.

## Finding

`AccountMonitorHandler.List` previously returned the service page directly, so service-internal request-source split fields were serialized into the public response.

## Test-Driven Evidence

- RED: `go test ./internal/handler/admin -run 'TestAccountMonitorHandlerPublicJSONHidesProbeSourceAndPreservesSchedulerAndAccounting' -count=1` failed because `probe_success_count` was present in the JSON response.
- GREEN: the same regression test passes after the handler projects the serialized page through the public source-agnostic response boundary.
- Focused suite: `go test ./internal/handler/admin -run 'TestAccountMonitorHandler.*' -count=1` passes.

## Changes

- Added handler-level JSON projection that removes timeline/source-split fields, including probe counters and source labels.
- Added a regression test covering grouped and full-site rows, preserved `request_count`, preserved `scheduler_rank`, and zero probe-only profitability revenue/cost.
- No service files changed.

## Delivery State

- No migrations, configuration changes, deployment, or push.
- Not validated: full test suite and online acceptance.
- Candidate state: `READY_FOR_ROOT_REVIEW`.
