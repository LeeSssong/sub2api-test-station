# Account Upstream Rate Multiplier Synchronization Design

## Problem

The native upstream billing probe already resolves the account's effective
rate multiplier (`effective_rate_multiplier=0.07` for the affected account),
but it only stores the probe snapshot under `accounts.extra`. Runtime usage
accounting still reads `accounts.rate_multiplier`, so the probe result and the
charged multiplier can drift.

## Goals

1. Persist a valid upstream `effective_rate_multiplier` into
   `accounts.rate_multiplier` when the account is configured for upstream-
   managed pricing.
2. Make subsequent request logs, usage records, and scheduler snapshots use
   the synchronized value without probing the billing endpoint on every
   request.
3. Preserve an explicit manual override mode.
4. Keep a durable audit trail and never overwrite a valid value with an
   invalid or failed probe result.

## Non-goals

- Do not change production data or force a production refresh in this worktree.
- Do not infer billing from ordinary `/v1/responses` calls.
- Do not automatically convert New API quota measurements into real billing
  multipliers when the upstream has no native billing endpoint.

## Source of truth and policy

The native `GET /v1/sub2api/billing` response is the authoritative source for
automatic synchronization. The selected value is `effective_rate_multiplier`,
falling back only when the upstream explicitly provides an equivalent resolved
field according to the existing probe contract. `group_rate_multiplier` alone
is not sufficient when account-level resolution differs.

Accounts have two policies:

- `upstream_managed` (default for accounts using native billing probe): valid
  probe results may update `accounts.rate_multiplier`.
- `manual_override`: the configured `accounts.rate_multiplier` is retained;
  probes remain observable and auditable but never replace the value.

The policy must be represented by an existing account setting if one already
exists; otherwise add a small explicit field in the account's persisted
configuration/extra schema with a backward-compatible default. A manual edit
of the multiplier switches the account to `manual_override` only when the
caller explicitly requests override mode; it must not be inferred silently
from an ordinary save.

## Data flow

```text
account create/edit/enable OR scheduled probe
        -> GET /v1/sub2api/billing
        -> validate effective_rate_multiplier
        -> write probe snapshot + audit event
        -> atomically update accounts.rate_multiplier (managed only)
        -> invalidate account cache / sync scheduler snapshot
        -> later request usage reads the new account multiplier
```

## Validation and failure handling

- Accept finite, positive values within the existing multiplier safety bounds.
- Treat missing, zero, negative, NaN/Inf, malformed, or errored responses as
  probe failures; preserve the previous multiplier.
- Store the raw snapshot and error metadata independently of synchronization.
- Treat a no-op update as successful but do not emit a misleading change event.
- Use an atomic repository transaction for the account row, snapshot, and
  audit record where practical; otherwise guarantee the account update and
  scheduler refresh are retried/idempotent.

## Lifecycle and refresh behavior

- Account creation/edit/enable triggers one forced billing probe when the
  account is eligible and has native billing support.
- The existing periodic probe remains the normal refresh mechanism.
- A successful periodic probe synchronizes the managed multiplier.
- A real request never performs a billing probe before forwarding upstream.
- After synchronization, invalidate the account cache and call the existing
  scheduler account snapshot synchronization path so routing and accounting
  observe the new value immediately.

## Audit contract

Record account id, old multiplier, new multiplier, source (`native_billing`),
probe timestamp, trigger (`lifecycle` or `scheduled`), policy, and actor
(`system` or administrator identity). Audit failures must not cause a valid
probe to be discarded, but must be surfaced and retried according to existing
repository error handling.

## Verification

- Valid `0.07` probe updates a managed account from `1.0` to `0.07` and the
  scheduler snapshot/cache is refreshed.
- Manual override remains unchanged while the probe snapshot is recorded.
- Invalid and failed probes preserve the previous multiplier.
- Repeated identical probes are idempotent and do not duplicate change events.
- Lifecycle and scheduled paths both use the same synchronization function.
- Usage logging reads the updated account multiplier after synchronization.
