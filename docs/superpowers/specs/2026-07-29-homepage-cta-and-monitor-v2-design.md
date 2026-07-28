# Homepage CTA and Monitor V2 Design

**Date:** 2026-07-29  
**Status:** Proposed for written review  
**Scope:** Homepage CTA copy and authenticated `/monitor` experience only

## 1. Approved product decisions

1. The homepage primary CTA always reads `立即开始`.
2. Its destination remains session-aware and continues to enter `/dashboard`;
   this change does not alter authentication or navigation behavior.
3. The homepage pricing section remains unchanged. It continues to show:
   - `官方价格的 0.1—0.3 倍`
   - `1 元 = 1 美元额度`
4. No real-call example, normalized official-price comparison, calculator, or
   pricing page is added in this increment.
5. `/monitor` retains the current login requirement.
6. `/monitor` becomes an isolated Xingqiao Monitor V2 experience. The existing
   native Sub2API monitor remains intact as a fallback and is not deeply
   rewritten.

## 2. Goals

- Remove the inconsistent authenticated homepage label `进入控制台`.
- Present monitoring by the user-facing group that customers select.
- Match the useful information hierarchy of the reference product without
  copying its brand or exposing Xingqiao's upstream implementation.
- Show real operational and usage evidence with explicit metric definitions.
- Keep the Sub2API customization surface small enough that future upstream
  releases usually require qualification rather than a monitor rewrite.

## 3. Non-goals

- Making `/monitor` public or accessible without authentication.
- Changing the pricing card, pricing policy, balance conversion, affiliate
  policy, support flow, or ticketing.
- Replacing Sub2API authentication, layout, group ownership, usage logging, or
  native channel-monitor storage.
- Showing upstream account names, suppliers, account cost multipliers, balances,
  credentials, API keys, user identities, or internal account IDs.
- Deleting or repurposing the native `ChannelStatusView`.
- Adding write actions, route switching, probes, or account controls to the
  user-facing monitor.

## 4. Homepage CTA

`HeroSection` continues to consume the session-derived CTA destination. The
visible label is normalized to `立即开始` for both guest and authenticated
sessions.

The secondary `查看文档` behavior remains unchanged. No additional CTA is
introduced.

## 5. Monitor V2 user experience

### 5.1 Page structure

The authenticated page keeps the standard Sub2API application shell and
contains:

1. Page title and overall service state.
2. Time-window selector: `24 小时`, `7 天`, `30 天`.
3. Responsive user-group card grid.
4. A model-detail interaction for each group.
5. Metric-definition and privacy notes.

### 5.2 Group card

Each card represents one user-visible group, not an upstream account or
monitor target.

The default card contains:

- public group name;
- user-effective group multiplier;
- current state: `运行中`, `服务波动`, `不可用`, or `样本不足`;
- TTFT P50;
- output TPS P50;
- total latency P50;
- successful call count, total call count, and availability percentage;
- bucketed availability timeline for the selected window;
- request-level cache-hit rate;
- model count and `查看模型` interaction.

The detail view shows TTFT P95, total-latency P95, per-model status, sample
counts, and metric definitions. It must not expose upstream account identity or
cost data.

### 5.3 Metric definitions

- **Group multiplier:** the effective multiplier presented to users for the
  selected group. It is not an upstream procurement multiplier.
- **Current state:** primarily derived from bounded active health checks so a
  low-traffic group can still report a current state.
- **Availability:** successful customer calls divided by all customer calls
  observed by the gateway in the selected window.
- **TTFT:** P50 of `first_token_ms` from successful streaming requests. Detail
  shows P95.
- **TPS:** P50 of per-request output-token throughput after the first token.
  Requests without sufficient timing or output-token evidence are excluded.
- **Total latency:** P50 of complete successful-request duration. Detail shows
  P95.
- **Cache-hit rate:** requests with a positive cache-read token count divided by
  requests for which cache-read evidence is available.

The API returns metric values together with their sample counts. The UI shows
`样本不足` instead of fabricated zeroes or percentages when the minimum sample
gate is not met.

### 5.4 Empty, partial, and error states

- No traffic does not mean `100%` availability.
- No streaming samples does not mean `0ms` TTFT.
- No cache evidence does not mean `0%` cache hits.
- A group with a healthy active probe but insufficient usage evidence shows
  `运行中` while individual historical metrics show `样本不足`.
- A Monitor V2 contract or service failure falls back to the preserved native
  monitor with a restrained notice. It does not expose the failed response,
  internal endpoint, or stack trace.

## 6. Isolation architecture

### 6.1 Frontend feature island

Create a new Xingqiao Monitor V2 feature directory rather than modifying the
native monitor components in place.

A thin route entry at `/monitor` selects the V2 view. The current native
`ChannelStatusView` and its components remain unchanged and importable as the
fallback. This keeps the recurring frontend conflict surface to the route entry,
the V2 feature directory, and shared public types.

### 6.2 Read-only projection

The group projection belongs to `relay-ops`, which already serves read-only
operational projections and reads Sub2API state. It adds a bounded,
authenticated Monitor V2 endpoint.

The projection:

- verifies the same active Sub2API user bearer used by the page;
- reads group identity and effective user-facing multiplier from Sub2API;
- aggregates usage timing, token, cache, success, and error evidence by group;
- combines active health-check state without exposing its upstream target;
- returns a versioned, allowlisted response;
- performs no account, group, price, route, balance, key, probe, or registration
  write.

The response contains only group-level aggregates. It must pass an explicit
secret and personal-data scan in tests.

### 6.3 Versioned contract

Use a Monitor V2 contract version so the frontend can reject incompatible
responses and use the native fallback. Unknown fields are ignored; missing
required fields, invalid ranges, or an unsupported contract version cause
fallback rather than partial rendering.

## 7. Upgrade and maintenance cost

Xingqiao's host updater already promotes only a qualified Xingqiao image tied to
an exact upstream Sub2API release. Monitor V2 therefore adds qualification work
but does not create a separate untracked deployment mechanism.

Every selected upstream upgrade must:

1. merge or rebase the exact upstream release;
2. rebuild the qualified Xingqiao image;
3. run the Monitor V2 contract, authentication, privacy, UI, and fallback tests;
4. rehearse the image before production promotion.

An upgrade does **not** necessarily require Monitor V2 code changes. Manual
adaptation is expected only when upstream changes the router/application shell,
the group or usage contracts used by the projection, relevant database
semantics, or the few integration files touched by the feature island.

This is lower recurring cost than editing the native monitor components and
aggregator directly. A deep native rewrite is explicitly out of scope.

## 8. Security and privacy

- `/monitor` and its V2 API remain authenticated.
- The API returns only data for public user-visible groups.
- No response contains user IDs, API-key IDs, account IDs, account names,
  supplier names, credentials, balances, request IDs, IP addresses, user
  agents, prompts, or raw errors.
- Aggregate queries are bounded by supported windows and group-count limits.
- Responses use `Cache-Control: no-store`.
- Invalid authentication and authorization preserve the existing Sub2API
  behavior.

## 9. Accessibility and responsive behavior

- Status is communicated with text and icons, not color alone.
- Cards and model details are keyboard accessible.
- Focus order follows the visual order.
- Metrics retain labels at 200% zoom.
- Desktop uses a two-column grid when space permits; narrow screens use one
  column without horizontal scrolling.
- Reduced-motion mode disables decorative transitions while retaining the
  timeline and all status information.

## 10. Verification

### Homepage

- Guest and authenticated sessions both render `立即开始`.
- CTA destinations remain unchanged.
- Pricing copy is byte-for-byte unchanged.

### Projection

- Formula tests cover success counts, availability, TTFT, TPS, latency, cache
  hits, percentiles, and sample gates.
- Authentication tests reject anonymous, expired, disabled, and malformed
  sessions.
- Contract tests reject invalid versions, missing fields, invalid ranges, and
  oversized responses.
- Serialized-response tests reject every forbidden identity, credential, and
  raw-request field.
- Query tests prove supported-window and group-count bounds.

### UI

- Cards render complete, partial, empty, degraded, and unavailable states.
- Contract failure renders the native fallback.
- Window switching, model details, keyboard operation, reduced motion, and
  narrow-screen layouts are covered.
- A visual comparison confirms the intended information hierarchy without
  copying the reference product's branding.

### Release

- The qualified image passes existing Sub2API, homepage, relay-ops, Caddy, and
  updater regressions.
- Rehearsal confirms `/monitor` login enforcement, V2 rendering, fallback, and
  zero operational writes before production promotion.

## 11. Done when

- Both homepage session states display `立即开始`.
- Existing pricing content has not changed.
- Authenticated `/monitor` displays one safe, accurate card per visible group.
- The native monitor remains available as an automatic fallback.
- Required metrics show explicit sample-aware states.
- No forbidden data is exposed and no operational write occurs.
- The upgrade runbook includes the Monitor V2 qualification checks.
