# Account Monitor Freshness Recovery Design

## Problem

The native account monitor serializes full and single-account runs with one
process-local mutex. Scheduled runs use a background context, while account
connection probes inherit that context without adding a deadline. If an
upstream stream never finishes, the scheduled run can hold the mutex
indefinitely. Later manual refreshes return HTTP 409 with
`account monitor run already in progress`, and persisted observations age past
the configured freshness window.

The stale marker is correct evidence that collection has failed. Removing or
weakening it would hide the monitoring outage rather than repair it.

## Required Behavior

1. Every account connection probe has a fixed upper time bound, including
   probes started by the background runner.
2. A timed-out probe is persisted as a fresh failed observation with error code
   `timeout`; one bad upstream cannot prevent other accounts or future runs
   from completing.
3. Concurrent full refresh callers join the in-flight full run and receive its
   result instead of receiving a conflict.
4. A waiting HTTP request may stop waiting when its own context is cancelled,
   without cancelling a background run that it joined.
5. Single-account refresh remains exclusive with a full run. It waits for the
   in-flight run to finish, then performs the requested probe with the same
   bounded-probe guarantee.
6. The existing stale calculation remains unchanged: a row is stale when its
   latest persisted observation is older than twice the configured interval.

## Design

`AccountMonitorService` owns one in-flight run record guarded by a short-lived
state mutex. The record contains a completion channel and the completed count
and error. The first full-refresh caller becomes the leader and performs the
run. Later full-refresh callers wait on the same completion channel and return
the leader's result. The state mutex is never held during network or database
work.

Single-account refresh waits for the current full run to finish and then claims
exclusive ownership before probing. This preserves the existing no-overlap
contract while eliminating the user-visible conflict.

`probeAccount` derives a timeout context for `ProbeAccountConnection`. Timeout
is classified through the existing `context.DeadlineExceeded` path and the
result is inserted normally. Persistence and multiplier refresh retain their
current behavior.

## Failure Handling

- Probe deadline: persist a failed result with `error_code=timeout`.
- Waiting caller cancellation: return the caller context error; do not mutate
  or cancel the leader.
- Run setup or persistence failure: publish the same partial completed count
  and error to every joined caller, release the in-flight state, and allow the
  next scheduled or manual run to proceed.
- Process restart: clears process-local in-flight state and starts a new
  bounded run.

## Verification

- A blocking account probe reaches the deadline, records `timeout`, and releases
  the run for a subsequent call.
- Two overlapping full refresh calls execute one physical probe batch and both
  receive the same completed count.
- A cancelled joining caller returns promptly while the leader continues.
- A single-account refresh waits behind a full refresh instead of returning
  `already in progress`.
- Existing account monitor service, handler, runner, and probe tests remain
  green.

