# Model Release Read-Only Monitor Design

**Date:** 2026-07-23 (Asia/Shanghai)
**Status:** Approved design, pending implementation plan

## Goal

Run the existing model-release discovery and readiness evaluation on the
production host without an LLM or manual command, so `/ops` always presents a
fresh, secret-free, read-only view of model candidates and their blockers.

## Reuse and Boundaries

Sub2API remains the authority for accounts, scheduling, priorities, concurrency
limits, group membership, model mappings, pricing, and native model discovery.
An account is relevant only when Sub2API reports it as `active` and
`schedulable`.

Sub2API's account `priority` is a scheduling order, not a traffic-share
weight: a lower number is preferred within the same group. A resilient native
configuration uses separate compatible account objects in the same group, with
the preferred account assigned the lower priority and the reserve account a
higher priority. Sub2API's native candidate selection excludes accounts that
are disabled, unschedulable, unbound from the group, incompatible with the
requested model, temporarily unavailable, expired, overloaded, or in a
rate-limit window. The monitor observes this state but never changes it.

The existing Feishu route controller remains `dry_run`; it is not an automatic
failover mechanism. This increment must not promise that every failed in-flight
request retries on a reserve account, because retry behavior is a separate
Sub2API policy and provider-error concern.

## Scheduled Work

A host-local systemd oneshot service will run every 15 minutes, with up to two
minutes of randomized delay and persistent catch-up after a reboot. A wrapper
will run these existing commands in order:

1. `collect-model-release-snapshot.rb collect`: read active/schedulable
   accounts, public groups, native account mappings, native upstream model
   directories, and native price completeness.
2. `evaluate-model-release-readiness.rb evaluate`: validate the new snapshot
   and write the current read-only result consumed by relay-ops.

Native model sync is an HTTP POST in Sub2API, but its defined operation is
directory discovery only. It does not create a model-generation request or
change account mappings, group catalogues, pricing, schedules, routes,
multipliers, balances, credentials, D04 mode, or Feishu mode.

The collector and evaluator already use atomic replacement. The wrapper must
invoke the evaluator only after collection succeeds, leaving the last valid
relay-ops result in place when either operation fails.

## Explicitly Excluded Work

The timer must never invoke a compatibility probe, synchronous model request,
SSE stream, capacity test, benchmark runner, promoter, route mutation,
price/multiplier mutation, balance mutation, Key operation, candidate creation,
D04 overlay, or Feishu action. Qualification remains a separately approved
paid activity.

Natural quality monitoring continues in the existing relay-ops scheduler.
Provider balance evidence remains fail-closed when no trustworthy machine-
readable source exists; the timer must record it as missing rather than infer a
passing balance.

## Files and Runtime Contract

The implementation will add a source-controlled wrapper and systemd unit
templates. Production installation will place the wrapper, policy, collector,
evaluator, and service-specific environment file under
`/opt/sub2api/production/ops/` and `/etc/sub2api/` as appropriate.

The environment file will contain only a local Sub2API URL and the path to the
existing restricted Admin-Key file. It will be mode `0600` and never printed.
The service will write only to the restricted existing model-release evidence
directory. Relay-ops mounts that directory read-only and reads the result
inside it, so the evaluator's atomic replacement is visible without a service
restart. The one-time migration from the former single-file mount requires only
one relay-ops recreation; no container rebuild is needed for each timer run.

Logs contain only start/finish status, timestamps, snapshot/proposal hashes,
and stable failure codes. They must not contain credentials, Base URLs, model
outputs, raw API responses, or request headers.

## Failure and Visibility

Systemd serializes runs. A failed run exits non-zero, leaves the prior valid
result unchanged, and is visible through `systemctl` and the journal. `/ops`
continues to fail closed when the retained result becomes stale or invalid.
The timer sends no new Feishu message; existing monitoring and notification
policy is unchanged.

## Validation

Automated tests will cover command order, permitted arguments, atomic
publication, failure retention, secret-free logs, timer cadence, service
hardening, and proof that prohibited commands cannot appear in the wrapper.
Production acceptance will install the unit, run one manual service invocation,
confirm a fresh `/ops` result, then verify timer enablement and the unchanged
Sub2API/relay-ops/D04 modes and configuration hashes. No model request or
production configuration mutation is part of acceptance.
