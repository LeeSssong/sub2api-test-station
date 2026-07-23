# Lightweight Account-Pool Quality Monitor Design

**Date:** 2026-07-23 (Asia/Shanghai)  
**Status:** Approved design; implementation not started

## Goal

Replace the model-release and whole-pool launch-gate focus with a lightweight,
scheduled quality monitor for the live Sub2API account pool. The monitor must
show each enabled account's stability, TTFT, account rate multiplier, and
explicit balance-exhaustion state without one account preventing the others
from being tested or reported.

## Reuse Decision

Sub2API remains the sole authority for account identity, account credentials,
models, scheduling, rate multiplier, and account-specific test execution.

The monitor directly reuses these native Admin APIs:

1. `GET /api/v1/admin/accounts` to discover the account pool. Membership is
   exactly `status=active && schedulable=true`.
2. `GET /api/v1/admin/accounts/:id` and `GET /api/v1/admin/accounts/:id/models`
   to obtain the account's existing metadata, multiplier, and compatible model
   list.
3. `POST /api/v1/admin/accounts/:id/test` to make an account-bound native test
   request. The monitor never reads, copies, or transmits an upstream Base URL
   or Key.

Sub2API's built-in scheduled-test plans are not the executor for this monitor:
their persisted result has total latency only and retains response text, while
the required quality record is first-token timing without model output.

The existing benchmark runner is not the executor: its catalog, compatibility,
SSE-completion, and capacity stages exceed the recurring health-monitor scope.

## Scheduled Behavior

One host-local systemd timer runs every 15 minutes, with a short randomized
delay and persistent catch-up. It launches a constrained, short-lived worker
on the internal Docker network.

For each discovered account, in deterministic account-ID order, the worker:

1. Reads current account metadata and selects one existing compatible text
   model with a deterministic generic policy. It prefers an available GPT text
   model with the highest parsed numeric family, then the lexicographically
   first available text model. It never uses a provider or account-name rule.
2. Calls Sub2API's native account-test SSE endpoint with the short fixed prompt
   `Reply with OK only.`.
3. Measures elapsed time to the first non-empty native `content` event as
   TTFT, then closes the test response. It stores no generated content.
4. Records the configured account rate multiplier with the sampled result.
5. Continues to the next account regardless of timeout, HTTP error, malformed
   SSE, missing compatible model, or explicit insufficient-balance response.

The task has no model-directory publication, model-promotion, route, group,
priority, multiplier, price, balance, credential, D04, or Feishu write action.
It does not run full compatibility, terminal-SSE, capacity, or paid-probe
campaigns.

## Results And Reporting

The worker writes an atomically replaced, secret-free evidence file. Each
account result contains only account ID, selected model ID, multiplier,
timestamp, status code, result class, TTFT when available, and a stable
redacted error code. Generated text, request headers, Base URLs, Keys, raw
responses, and user traffic are excluded.

`balance_exhausted` is emitted only for an explicit upstream insufficient
balance/quota response. An unreadable balance or any other failure remains its
own non-passing result class; the monitor must not infer either a passing or a
depleted balance.

The existing `/ops` and Feishu reporting path consumes the latest rolling
window. It preserves existing report ordering and presentation, adding the
account-level stability summary, TTFT P50/P95, multiplier, and last result.
No account can suppress another account's record. A run with zero successful
accounts remains a visible failed run, not a silent success.

## Resource And Cost Boundaries

The worker has a read-only root filesystem, no Linux capabilities,
`no-new-privileges`, a bounded temporary filesystem, CPU/memory/PID limits,
and only the existing internal Sub2API Admin-Key mount. It logs stable result
codes and hashes only.

Each account receives at most one short first-token pulse per 15-minute
interval. The worker stops after the first native content event, avoids model
output persistence, and never retries an account within the same run. A daily
run limit is enforced from the timer cadence; the monitor does not claim an
exact USD spend because native account-test requests bypass customer billing
and external-provider pricing is not reliably machine-readable.

## Failure Semantics

Account discovery failure fails the run without reusing a guessed pool. A
per-account failure is persisted and reported while all remaining discovered
accounts continue. Atomic output replacement occurs only after the full run
has produced a valid document. Failed runs retain the last valid evidence;
the report marks it stale rather than presenting it as current.

## Validation

Tests must prove:

1. only active and schedulable accounts are selected;
2. selected model choice is deterministic and provider-neutral;
3. TTFT is measured at the first content event and response text is discarded;
4. explicit balance exhaustion is isolated to one account;
5. timeout, malformed stream, and one-account failures do not stop later
   accounts;
6. forbidden secrets and response payloads cannot enter evidence or logs;
7. timer, worker limits, atomic publishing, report ordering, and stale-result
   behavior match the existing operational contract.

Production acceptance must verify only the task installation, execution mode,
result-file visibility, health, and unchanged routes/configuration. The LLM
does not issue account tests during acceptance; the installed task performs
the recurring tests after deployment.
