# Native Account Monitor and Feishu Upstream Status Design

**Date:** 2026-07-25 (Asia/Shanghai)  
**Status:** Approved for specification review; implementation not started

## Goal

Add an administrator-only account monitor to the server-side Sub2API
application. The monitor must perform real upstream probes for every current
`active + schedulable` account, show each account's current multiplier,
request usage, official usage windows, and recent quality, and expose the same
structured result to relay-ops and Feishu.

Also correct the homepage pricing text from:

`官方价格的0.1——0.3折`

to the exact user-requested text:

`官方价格的0.1——0。3倍`

The full-width Chinese stop in `0。3` is intentional for this approved
specification and must not be normalized automatically during implementation.

## Scope And Constraints

### In scope

- The root `homepage` application and its tests for the copy change.
- The server-side Sub2API source under `upstream/sub2api`.
- The server-side relay-ops source under `relay-ops-service`.
- Shared operational scripts, deployment contracts, and tests under `ops`,
  `infra`, and `tests` when required by the feature.
- Administrator-only UI, API, persistence, scheduling, reporting, and
  deterministic recommendation analysis.

### Out of scope

- Any local `sub` runtime, local account data, credentials, database rows, or
  live deployment state.
- Automatic changes to routes, groups, account scheduling, prices, keys,
  multipliers, or balances.
- A second independent probe implementation in relay-ops.
- Exposing upstream keys, cookies, authorization headers, Base URLs, raw
  responses, or generated content to the browser, relay-ops, or Feishu.

## Approved Decisions

1. Probes are real upstream calls, not cache refreshes.
2. Only accounts satisfying `status=active && schedulable=true` are monitored.
3. Refresh cadence is one global interval for the whole account pool.
4. A card-level settings button opens the same global interval setting.
5. A card-level immediate action probes only that account.
6. Sub2API is the authority for account identity, credentials, model mapping,
   test execution, multiplier, and monitoring results.
7. relay-ops and Feishu read the same native monitor result; they do not probe
   accounts independently.
8. Feishu produces read-only recommendations only. It never switches an
   account, group, route, price, multiplier, key, or balance.
9. The first recommendation score uses stability/success rate 40%, TTFT and
   total latency 25%, account multiplier 20%, and usage-window/recent-load
   headroom 15%.
10. Recommendations require fresh evidence, sufficient samples, a compatible
    model, current schedulability, and a minimum improvement margin.

## Architecture

### Native Sub2API monitor

Add an account-monitor service to the Sub2API backend. It reuses the existing
`AccountTestService` platform adapters, account authentication, proxy, TLS
profile, model mapping, and upstream safety validation. The reusable probe
core returns a structured result instead of only writing browser SSE events.

Each probe:

1. Takes an account snapshot from the current active/schedulable pool.
2. Selects one deterministic compatible text model.
3. Sends the existing account-test style request through the account's normal
   server-side path.
4. Records the time to the first valid content event as TTFT.
5. Records total request latency and a sanitized result classification.
6. Closes the response after the first valid content event when the platform
   permits it.
7. Discards response content and never stores generated text.

A failed probe is an account-level result. It must not cancel the remaining
accounts in the same run. The scheduler and manual endpoints share a global
run lock and per-account in-flight protection.

### Persistence

Add a singleton global settings record for the account-monitor interval,
updated timestamp, and updating administrator. Validation and UI presets
follow the existing channel-monitor interval behavior.

Add bounded account-monitor history with at least:

- account ID;
- selected model ID;
- result status and stable error classification;
- HTTP status when safe to expose internally;
- TTFT and total latency;
- checked timestamp;
- run identifier.

Results are retained for a bounded rolling window, defaulting to seven days
for aggregation. Account configuration remains separate from monitor history;
monitor results must not be stored in the account `extra` JSON.

### Administrator API

Implement administrator-only endpoints equivalent to:

- `GET /api/v1/admin/account-monitors`
  - returns active/schedulable account cards;
  - includes global settings and latest aggregate values;
  - includes redacted account metadata only.
- `PUT /api/v1/admin/account-monitors/settings`
  - updates only the global interval.
- `POST /api/v1/admin/account-monitors/run`
  - probes the current account pool.
- `POST /api/v1/admin/account-monitors/:account_id/run`
  - probes one current account.
- `GET /api/v1/admin/account-monitors/:account_id/history`
  - returns bounded sanitized history for the card detail view.

The aggregate endpoint must use batch account statistics where available and
must not introduce an unbounded N+1 query pattern.

## Administrator UI

Add an administrator-only route, recommended as
`/admin/accounts/monitor`, and an administrator navigation item named
“账号监控”. Ordinary users must not see the navigation item and must receive
the existing unauthorized behavior when they request the route or API.

### Page header

The page shows:

- number of monitored accounts;
- last completed run;
- whole-pool status;
- global refresh interval control;
- immediate refresh-all action.

The interval control is one setting for all cards. Card settings buttons open
the same control and do not create per-account schedules.

### Account card

Each card shows:

- account name, ID, platform, type, and group membership;
- latest probe status and checked time;
- selected test model;
- total latency, TTFT, and recent-window success rate;
- current account multiplier;
- current invocation information, reusing the account-management today-stats
  projection;
- official upstream usage windows, reusing the existing `AccountUsageCell`
  behavior and active-query support;
- a settings action for the global interval;
- an immediate-refresh action for the current account.

The UI must represent loading, running, stale, no-history, balance-exhausted,
failure, and account-pool-changed states explicitly. It must support filtering
by platform, group, status, and account name for larger pools.

The first load fetches the monitor aggregate. Usage-window details remain
compatible with the existing component's lazy and cached behavior. Account
cards that fail do not hide successful cards.

## Shared Result Contract

The native response must provide a versioned, secret-free projection suitable
for both the admin page and relay-ops. The projection includes:

- account ID and display name;
- active/schedulable state;
- current group IDs and names;
- selected model;
- latest status, error class, and checked timestamp;
- sample count, success count/rate, TTFT P50/P95, total-latency P95;
- account multiplier;
- recent invocation count and error count;
- official usage-window summaries when available;
- evidence timestamp and stale flag.

The contract must reject or omit fields matching credential, token, cookie,
authorization, password, secret, Base URL, request header, or raw response
patterns. Raw upstream messages must be reduced to stable classifications.

## Feishu Integration

Extend the existing relay-ops Sub2API read client to consume the native
account-monitor projection. Remove the need for a second account-quality probe
for this reporting path. Existing fallback and stale-result behavior remains
fail-closed.

### Interactive card

Use Feishu's native `interactive card` format and extend the existing renderer
instead of introducing custom HTML or an unsupported visual format.

Card structure:

1. Blue header: `上游账号情况 · 更新时间`.
2. Overall status block: monitored count, healthy count, failed count, and
   evidence freshness.
3. Group blocks showing the current account and eligible alternatives.
4. Compact account metric rows containing success rate, TTFT, multiplier,
   invocation count, and usage-window summary.
5. Green recommendation block only when a candidate clears all evidence gates
   and exceeds the minimum improvement margin.
6. Orange/red evidence warnings for stale, insufficient, or failed results.
7. Native action button linking to the administrator account-monitor page.

The renderer must preserve the existing 30 KiB card limit, escaping rules,
abnormal-row priority, and truncation behavior. When the pool is too large,
retain all abnormal and recommended rows and link to the full administrator
view for the rest.

## Deterministic Recommendation Script

Add a versioned script under `ops/` that consumes the native structured
projection and emits both recommendation JSON and concise Chinese text.

For each group:

1. Resolve the current scheduled account from the native group/account
   projection.
2. Consider only accounts currently bound to that group, still
   `active + schedulable`, and compatible with the group's required model.
3. Reject candidates with stale evidence, insufficient samples, missing key
   metrics, recent consecutive failures, or an account-set mismatch.
4. Score eligible candidates with:
   - stability/success rate: 40%;
   - TTFT and total latency: 25%;
   - account multiplier: 20%;
   - usage-window and recent-load headroom: 15%.
5. Emit “B 账号综合更佳” only when the candidate's score exceeds the current
   account by the configured minimum margin.
6. Otherwise emit either “当前账号暂不需要更换” or “证据不足，暂不建议”.

The script is advisory. Its output is consumed by relay-ops for the Feishu
card and is never connected to a route or account mutation API.

## Error And Safety Semantics

- Discovery failure fails the run without guessing the account pool.
- One-account timeout, malformed stream, HTTP error, explicit balance
  exhaustion, or missing model is persisted as that account's result and does
  not stop later accounts.
- A run is not published as current until all discovered accounts have a
  valid result record.
- A stale or account-set-mismatched result is never presented as a current
  recommendation.
- Admin APIs remain protected by native administrator authorization.
- relay-ops uses a read-only server-side credential and receives only the
  sanitized projection.
- No feature in this change writes routing, pricing, account, credential,
  balance, or Feishu command state.

## Validation

### Backend tests

Cover:

- active/schedulable filtering;
- deterministic model selection;
- first-content TTFT measurement and response discard;
- platform-specific probe result normalization;
- timeout, malformed stream, balance exhaustion, and one-account failure
  isolation;
- global interval validation;
- scheduler and per-account in-flight locks;
- history aggregation and seven-day retention;
- batch query behavior;
- admin authorization and secret-field rejection.

### Frontend tests

Cover:

- admin-only route and navigation;
- global interval changes from header and card settings;
- refresh-all and single-card refresh;
- card rendering for success, stale, running, failure, and no-history states;
- multiplier, invocation stats, and usage-window rendering;
- filtering and pool changes.

### relay-ops and script tests

Cover:

- native projection decoding and stale behavior;
- current-group/candidate matching;
- score calculation and minimum-margin gate;
- insufficient evidence suppression;
- secret and raw-response rejection;
- Feishu interactive-card layout, escaping, truncation, and size limit;
- guarantee that recommendation generation has no write-client dependency.

### Regression and acceptance

Run the homepage test, Sub2API backend/frontend tests relevant to accounts and
channel monitoring, relay-ops tests, script contract tests, and the existing
build/typecheck commands. Verify that no local `sub` data or live deployment
state is touched.

## Additional Considerations

- Real probes consume upstream capacity and may consume provider quota. The
  implementation should keep one short probe per account per interval, use a
  bounded worker count, and expose the last-run cost-risk state in the admin
  page when the provider reports it.
- The first version should keep the recommendation score deterministic and
  explainable. LLM analysis can be added later as a commentary layer, never as
  the authority for a routing change.
- The native account-monitor projection should become the future source for
  other Feishu upstream-monitor functions so later functions do not create
  parallel account-quality pipelines.
