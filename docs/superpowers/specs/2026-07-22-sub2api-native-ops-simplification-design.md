# Sub2API Native Ops Simplification Design

**Date:** 2026-07-22 (Asia/Shanghai)  
**Status:** Approved

## Goal

Turn `/ops` into a small, read-only administrator status page that projects Sub2API's current state instead of maintaining a second upstream-management system. The page must explain whether controlled user testing can open, update automatically, and be undiscoverable to missing, invalid, or non-admin sessions.

## Product Decisions

- Sub2API owns upstream accounts, Base URLs, Keys, group membership, scheduling, native registration, users, sessions, API Keys, usage, and balances.
- The active-upstream set is exactly the Sub2API accounts where `status == "active" && schedulable == true`; no provider name or fixed account ID is allowed in membership logic.
- `/ops` is read-only. Remove production-source intake, billing-session configuration, candidate Base URL/Key intake, candidate controls, synthetic alert controls, and their browser-accessible mutation routes.
- The existing relay-ops database records remain historical evidence. They do not define current upstreams and are not deleted by this change.
- The generic minimum provider-balance gate is USD 5.00. Quality freshness and hard quality thresholds remain fail-closed and provider-neutral.
- D04 remains `read_only` with registration closed until a fresh readiness result passes for the same live account set and opening is explicitly approved.

## Reuse Audit

### Direct reuse

- `GET /api/v1/admin/accounts` for active/schedulable account discovery.
- `GET /api/v1/admin/groups/all` for group names.
- `GET /api/v1/auth/me` for administrator-token verification.
- Sub2API native account and settings UI for Base URL, Key, scheduling, and registration management.
- Existing D04 authentication proxy, 15-user cap, daily USD 20 grant, idempotency, budget, and reconciliation behavior.

### Adapted reuse

- Relay-ops joins the live Sub2API account set with the latest secret-free D04 readiness result only when their canonical account-set hashes match.
- The browser re-fetches the authenticated read-only projection every 30 seconds. Account changes therefore appear without rebuilding relay-ops or manually entering a source.
- The existing Sub2API browser token is verified before rendering operator data. Unauthorized and non-admin outcomes are masked as not found.

### New only where absent

- A concise Chinese readiness projection named `内测开放状态`.
- A hidden-admin middleware variant returning HTTP 404 for missing, invalid, inactive, or non-admin identities.
- A read-only auto-refresh controller for `/ops`.

## User Interface

The page contains only:

1. Header with links to Sub2API native monitoring and model pricing.
2. `内测开放状态`: `可以开放` or `暂不可开放`, last refresh time, current live upstream accounts, balance/quality evidence, and plain-language blockers.
3. Collapsed `技术详情`: snapshot ID, account-set hash, evaluation time, and stable blocker codes.
4. Existing read-only incidents, Agent summaries, daily/quality reports where data exists.

The page does not contain Base URL, Key, Cookie, Bearer Token, secret-file path, candidate creation, source creation, disable buttons, acceptance buttons, or route controls.

If the readiness result references an old account set, the page displays the current Sub2API accounts and `活动上游已变化，等待新门禁检查`; it never displays the old accounts as current truth.

## Authentication

- The initial `/ops` document contains no operational data.
- The bootstrap reads the existing Sub2API `auth_token` from browser local storage only to call the same-origin authenticated projection endpoint.
- All `/ops` data APIs return HTTP 404 for missing, invalid, inactive, and non-admin sessions.
- The browser navigates to the application's not-found route on any authentication failure and never redirects a non-admin to an administrator login flow that confirms `/ops` exists.
- Tokens are not placed in query strings, cookies created by relay-ops, HTML, logs, or persisted relay-ops storage.

Because Sub2API stores its token in local storage, the first static bootstrap response cannot itself be server-authenticated without modifying Sub2API's login/session architecture. The accepted lightweight boundary is a data-free bootstrap followed by authenticated retrieval; unauthorized users receive no operations HTML or data and end on the native 404 page.

## Opening And Closing Controlled Testing

- This UI cleanup does not open registration.
- Normal upstream management remains in Sub2API's account UI.
- The existing server-side D04 mode is the safety arm. A controlled launch requires `D04_MODE=write`, a qualified cost policy, an explicit budget, and a fresh same-account-set readiness `GO`.
- The effective public-registration switch remains the conjunction of Sub2API native registration and D04's server-side registration gate. Closing either side closes registration.
- No web toggle is added in this increment because dynamically changing D04 write authority would create a second privileged control plane.

## Failure States

- Sub2API account read failure: show no stale account list, return a read-only unavailable state, and keep launch `NO-GO`.
- Empty active account set: show `没有已启用调度的上游` and keep launch `NO-GO`.
- Account-set mismatch or stale evidence: show the live accounts, explain that the gate is waiting for a new check, and keep launch `NO-GO`.
- Auto-refresh failure: keep the last rendered page, show a non-sensitive refresh warning, and retry after 30 seconds.

## Acceptance Criteria

- Accounts `10/11` appear automatically when they are the current `active + schedulable` Sub2API set; old `7/8` readiness evidence cannot appear as current accounts.
- Changing Sub2API scheduling changes the next read-only projection without any relay-ops intake action.
- No input, select, textarea, mutation button, Base URL, Key, billing-session, candidate-intake, or production-intake element remains on `/ops`.
- Browser-accessible candidate/upstream/billing/acceptance mutation routes return 404.
- Missing, invalid, user-role, and inactive-admin sessions receive 404 from the authenticated projection.
- The browser automatically refreshes the authenticated projection every 30 seconds without exposing the token.
- The v3 balance policy passes at USD 5.00 and blocks at USD 4.99.
- Relay-ops remains `read_only + dry_run`; D04 remains `read_only` with registration closed; no production route, account, scheduling, price, multiplier, balance, Key, candidate, probe, user, or database row is mutated during deployment acceptance.

