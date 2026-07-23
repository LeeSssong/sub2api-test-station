# D04 Authentication Client Identity Forwarding

**Date:** 2026-07-23
**Scope:** `internal-test-service` only

## Problem

Production routes native registration, password login, and login 2FA through
the D04 internal-test service. The service forwards a small allowlist of HTTP
headers to Sub2API but omits the proxy-derived client identity headers.

When Sub2API session binding is enabled, the login token is therefore bound to
the D04 container address. Subsequent browser requests go directly from Caddy
to Sub2API and carry the real client address. Sub2API correctly treats the
different session fingerprint as a mismatch, revokes the refresh-token family,
and returns 401 immediately after successful 2FA.

## Boundaries

This repair must not modify or recreate Sub2API, PostgreSQL, Redis, Caddy,
accounts, balances, API keys, routes, or settings. D04 remains in read-only
mode with registration closed. Session binding remains enabled.

## Design

Extend the D04 native-auth forwarder's explicit header allowlist with the
standard proxy identity headers produced by Caddy:

- `X-Forwarded-For`
- `X-Forwarded-Proto`
- `X-Forwarded-Host`
- `X-Real-IP`

The service does not accept public traffic directly and has no host port.
Caddy is the only public ingress and constructs the forwarded client identity.
Sub2API independently applies its trusted-proxy configuration before using the
forwarded address. The D04 service continues to reject unknown auth endpoints,
cross-origin writes, redirects, and oversized bodies.

The response path is unchanged: status, body, `Content-Type`, `Set-Cookie`, and
`Authorization` continue to pass through unchanged. Daily-login grant behavior
is unchanged.

## Alternatives Rejected

Disabling Sub2API session binding would restore login but weaken an unrelated
security control. Bypassing D04 for login would restore the native path but
break D04's future daily-login accounting. Modifying Sub2API to special-case
D04 would violate service ownership and create a private platform behavior.

## Validation

Add a focused forwarder test that supplies the four proxy identity headers and
asserts that the native Sub2API request receives them unchanged. First run the
test against the current implementation and confirm it fails, then implement
the allowlist change and rerun the focused test plus the full
`internal-test-service` test, race, and vet suites.

For production, build a new immutable D04 image and recreate only
`internal-test-service` with the existing read-only Compose overlay. Verify:

1. Sub2API, PostgreSQL, Redis, Caddy, and relay-ops container identities and
   restart counts are unchanged.
2. D04 remains healthy, read-only, and registration remains closed.
3. A harmless invalid auth request reaching Sub2API through D04 records the
   public client address instead of the D04 container address.
4. The operator can complete password plus 2FA login without an immediate
   session-binding rejection.

## Rollback

If D04 health or the auth proxy contract fails, restore the previous immutable
D04 image and recreate only `internal-test-service`. No database rollback is
required because the change has no schema or persistent-state writes.
