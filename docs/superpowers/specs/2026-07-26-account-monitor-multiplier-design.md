# Account Monitor Multiplier And Group Filter Design

**Date:** 2026-07-26 (Asia/Shanghai)
**Status:** Approved design; pending written-spec review

## Goal

Correct the administrator account-monitor multiplier so it represents the
upstream group cost of the monitored API key instead of the local
`accounts.rate_multiplier`. Add a group filter to the account-monitor page.

Keep the page simple: each card shows one current multiplier with a compact
source label, while Sub2API handles declaration and measurement details on the
server.

## Scope

- Change only the server-side Sub2API source, administrator UI, shared
  account-monitor projection, tests, and required deployment artifacts.
- Continue monitoring only non-deleted `active + schedulable` accounts.
- Do not change account groups, routes, scheduling state, prices, credentials,
  or upstream configuration.
- Do not infer a multiplier from local Sub2API usage costs. Those values use
  local billing rules and cannot prove the upstream deduction.

## Multiplier Sources

### Native declaration

For an OpenAI-compatible upstream that implements
`GET /v1/sub2api/billing`, reuse the existing native
`UpstreamBillingProbeService` result.

The authenticated declaration provides the key owner's upstream group rate,
an optional user override, peak-period configuration, and the currently
effective rate. A fresh, valid declaration is authoritative for the monitor.

### New API measurement fallback

When the declaration endpoint returns `404` or `405`, detect compatible New
API quota endpoints:

- `GET /api/usage/token/` for API-key-specific cumulative quota usage;
- `GET /api/status` for `quota_per_unit`.

Measure the effective upstream multiplier with a controlled, non-streaming
text request:

1. Read the key's cumulative quota usage.
2. Send a deterministic request through the account's normal Base URL,
   proxy, TLS profile, headers, and model mapping.
3. Read cumulative quota usage again.
4. Convert the positive quota delta using `quota_per_unit`.
5. Divide the upstream deduction by the official cost computed from the
   response's actual token usage.

Use one stable, supported text model and bounded input/output tokens. Take
three samples and publish the median only when every sample is valid and the
relative spread is within a conservative tolerance. Reject contaminated,
zero-delta, negative, non-finite, unsupported, or high-variance measurements.

This value is an observed effective multiplier relative to official model
pricing. It is the best available fallback for New API, but it must be labeled
as measured rather than declared.

## Refresh Policy

- Native billing declarations retain their existing Sub2API refresh policy.
- New API measurements are considered fresh for 24 hours.
- The normal seconds-level account-monitor scheduler reuses a fresh
  multiplier snapshot and must not perform charged measurement requests on
  every connectivity probe.
- A card-level manual refresh forces both the normal account connectivity
  probe and one multiplier refresh for that account.
- A refresh-all action refreshes connectivity for the pool but only measures
  multiplier snapshots that are absent or older than 24 hours.

Multiplier probing failures do not turn a successful connectivity probe into
an account failure.

## Projection

Replace the ambiguous numeric-only multiplier contract with a sanitized
projection containing:

- current multiplier when valid;
- source: `declared` or `measured`;
- status: `ok`, `stale`, `unsupported`, `failed`, or `unavailable`;
- observed timestamp.

Do not expose Base URLs, API keys, raw quota values, request bodies, response
bodies, or upstream error messages. Bump the shared projection schema version
and update relay-ops parsing so recommendations consume only fresh valid
multipliers.

## Administrator UI

The multiplier metric displays:

- `<value>x` plus `声明` for a fresh native declaration;
- `<value>x` plus `测算` for a valid New API measurement;
- `上游未声明`, `倍率已过期`, `测算失败`, or `暂无倍率探测` when no trustworthy
  value is available.

Add a single-select group filter beside the existing platform and status
filters:

- first option: all groups;
- remaining options: unique groups present in the current monitor projection,
  ordered consistently by group name and ID;
- an account with multiple groups matches when its `group_ids` contains the
  selected group;
- searching and other filters continue to compose with the group filter.

## Persistence

Store the New API measurement snapshot in the account's existing `extra`
document under a dedicated, versioned key. Persist only sanitized aggregate
results, timestamps, status, source, model, sample count, and variance.

Use the account repository's optimistic account snapshot update pattern so a
concurrent credential or Base URL change cannot attach a measurement to the
wrong account identity.

## Error Handling

- Unsupported declaration triggers New API capability detection, not a fake
  `1.0x` fallback.
- Missing quota endpoints, invalid quota units, missing response usage, or
  model pricing uncertainty produces an unavailable/failed snapshot.
- Concurrent traffic on the same key can inflate a quota delta; the
  three-sample variance gate rejects unstable measurements.
- A mid-probe credential, Base URL, proxy, or account-state change discards
  the result.
- No multiplier failure changes account scheduling or routing.

## Validation

Backend tests cover:

- declared multiplier precedence and freshness;
- New API endpoint capability detection;
- quota-unit conversion and official-cost calculation;
- three-sample median and variance rejection;
- identity-change rejection and secret-free projection;
- 24-hour automatic refresh and forced single-account refresh;
- multiplier failure isolation from connectivity status.

Frontend tests cover:

- declared, measured, stale, unsupported, failed, and unavailable states;
- removal of the incorrect local `rate_multiplier` display;
- group option derivation and multi-group membership filtering;
- composition with search, platform, and status filters;
- card/manual refresh behavior.

Relay-ops tests cover the schema bump, fresh multiplier consumption, and
fail-closed recommendation behavior when multiplier evidence is absent.

Production acceptance verifies the active/schedulable account set, existing
Sub2API declarations, controlled New API measurements, group filtering, no
credential exposure, and no automatic route or account mutation.
