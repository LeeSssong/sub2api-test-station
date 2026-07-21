# D04 Public Registration and Daily Login Credit Design

**Date:** 2026-07-21
**Status:** Approved design
**Scope:** D04 launch-policy sidecar only; reuse Sub2API's native user experience.

## Goal

For the controlled launch, allow configurable public email/password registration until 15 successful launch users are enrolled. Each enrolled user's first successful authentication on a Shanghai calendar day, including the authenticated response returned by a successful registration, receives a single USD 20 site-balance grant.

The feature must use Sub2API's native `/register`, `/login`, two-factor completion, dashboard, API key, usage, profile, and later affiliate pages. It must not create a second user center.

## Explicit Non-Goals

- No referral reward, invitation link, affiliate enrollment, or manual check-in during the launch.
- No OAuth account creation or OAuth daily-credit eligibility change in this scope.
- No change to Sub2API routing, upstream selection, pricing, multipliers, balances outside the defined daily grant, Keys, candidates, probes, or relay-ops modes.
- No production activation in this design. Production remains `D04_MODE=read_only` until a separately approved low-budget write acceptance.

## Architecture

```text
Browser
  -> Caddy
  -> D04 transparent authentication proxy
  -> Sub2API native auth endpoints
       -> native user shell and dashboard

D04 local launch ledger
  -> Sub2API Admin API balance grant with idempotency key
```

The sidecar owns launch policy and the credit ledger. Sub2API remains authoritative for credentials, authentication, sessions, user pages, and user balances.

### Intercepted Endpoints

- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/login/2fa`

For each endpoint, Caddy forwards to D04. D04 forwards the request unchanged to the private Sub2API service and preserves the upstream response status, allowed authentication headers, body, and content type.

D04 only inspects a successful, bounded JSON response to read `user.id`. It never changes an authentication success into a login failure because a balance grant is unavailable.

## Registration Policy

The effective registration decision is the conjunction of all conditions:

```text
D04_MODE == write
AND D04_REGISTRATION_OPEN == true
AND successful_launch_user_count < D04_MAX_USERS
AND launch budget gate permits the daily-credit policy
```

- `D04_MAX_USERS` defaults to `15` and is a hard cap.
- `D04_REGISTRATION_OPEN` is the operator-controlled registration switch. It replaces invitation-based registration semantics.
- `D04_TOTAL_BUDGET_USD` remains the independent launch stop-loss. It must be explicitly sized and qualified before write-mode activation; the old USD 100 default must not be silently treated as sufficient for 15 users receiving USD 20 each day.
- Reopening the registration switch cannot bypass the hard cap; after user 15 succeeds, every later public registration receives a stable closed/full response.
- The roster write after a successful Sub2API registration is serialized and idempotent. A failed upstream registration does not consume a slot.
- A successful registration atomically records the launch user before the response is returned. Its authenticated response then uses the normal daily-credit path, which grants the same-day USD 20 once.

The old invitation requirement, invitation-code forwarding, referral reservation, and join-link page are removed from the public D04 surface. Existing SQLite data and secret files are not destructively deleted; schema migration retains historical rows for audit compatibility.

## Login Credit Policy

For a successful authenticated response, D04 checks whether `user.id` belongs to the launch roster. Non-launch users receive the native authentication response without a D04 credit action.

For a launch user, D04 calls the existing credit service with:

```text
grant kind: daily_login_credit
amount: USD 20
date: Asia/Shanghai calendar date
idempotency key: d04-login-<user-id>-<YYYY-MM-DD>
```

The underlying balance write uses the existing pending/succeeded/uncertain state machine and Sub2API Admin API idempotency record. Concurrent password and two-factor completions for the same user/day may yield at most one balance mutation.

If a network result is uncertain, D04 enters its existing read-only safety state and reconciles provider history before any retry. The already-successful authentication response remains successful. The system never credits based on a client-provided user ID, date, or amount.

## Configuration and Migration

| Setting | Meaning |
|---|---|
| `D04_MODE` | Global safety mode: `read_only`, `write`, or `closed`. |
| `D04_REGISTRATION_OPEN` | Explicit public-registration toggle; defaults to `false`. |
| `D04_MAX_USERS` | Launch-user hard cap; defaults to `15`. |
| `D04_DAILY_LOGIN_CREDIT_USD` | Fixed launch daily credit; defaults to `20`. |
| `D04_TOTAL_BUDGET_USD` | Independent launch stop-loss; must be explicitly qualified before `write` mode. |
| `D04_TIMEZONE` | Must remain `Asia/Shanghai`. |

Invitation-key configuration becomes optional in the new code path and will be removed from deployment configuration only after the migration and rollback window are verified. Referral constants, invitation-only API routes, manual check-in endpoint, and launch-specific affiliate UI are retired from the public surface.

The SQLite schema evolves `internal_users` into a launch roster without inviter or invitation dependencies. Existing unused invitation and referral tables remain readable but are no longer written. Credit uniqueness is changed from generic date uniqueness to an explicit `(user_id, grant_kind, grant_date)` policy so future commercial referral work cannot accidentally collide with daily login credits.

## Security and Reliability Boundaries

- Forward request and response bodies in memory only, maximum 1 MiB and fixed 20-second upstream deadline.
- Do not log bodies, `Authorization`, `Cookie`, `Set-Cookie`, passwords, tokens, temporary 2FA tokens, or TOTP values.
- Forward only the required request headers and authentication response headers; preserve content type and response status.
- Do not put authentication input, response data, or grant idempotency identifiers in Feishu messages.
- Reject malformed or oversized payloads before forwarding.
- Keep `Cache-Control: no-store`, content-type sniffing protection, same-origin write policy, and existing bearer-token protection for any remaining D04 operator endpoints.

## Validation

Unit and contract tests must prove:

1. Native successful and failed registration/login/2FA responses preserve status, body, and allowed headers.
2. Registration is rejected when the explicit switch is off, the global mode is non-write, or 15 users already succeeded.
3. Exactly the first 15 successful registrations enter the roster under concurrent requests.
4. Registration success triggers one immediate same-day USD 20 credit.
5. Password login and 2FA completion share one Shanghai-day idempotency key and cannot double grant under concurrency.
6. Non-roster users never receive a launch credit.
7. No credential, cookie, token, 2FA value, or complete response body is persisted, logged, returned by D04 APIs, or sent to Feishu.
8. Uncertain balance writes lock the credit path read-only and require reconciliation; authentication still succeeds.
9. Retired invitation, referral, and manual check-in public endpoints return a stable unavailable response rather than silently performing the old behavior.

Production acceptance is separate: it requires an explicitly approved and qualified launch budget, a low-budget write-mode approval, an isolated launch account, registration-switch proof, 15-user cap proof without creating 15 real accounts, same-day idempotency, next-Shanghai-day behavior, and provider/ledger/balance reconciliation.
