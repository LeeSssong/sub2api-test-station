# Resend SMTP Controlled Reproduction

**Date:** 2026-08-09
**Decision:** `BLOCKED_OTHER`
**Status:** Paused, fail-closed. No code, SMTP, DNS, Resend, or production
setting change was made.

## Scope And Irreversible State

This task was authorized for one test-email request to the first active
administrator mailbox only. The mailbox was represented only by SHA-256
`d764a63fa2680407b78319aa7f9e311aa2e9d21b2c64bca1b0a38c1de0382119`
and the masked domain `***@qq.com`.

The request client was canceled in flight after approximately 11 seconds when
the production-policy drift was discovered during the request/audit, not as a
pre-request gate. The exact client-captured request start timestamp is
unavailable. Read-only audit evidence later proved that the server had
nevertheless completed that one request at `2026-08-09T09:19:52Z`: HTTP `400`,
latency `20,191 ms`. The client captured no final response, request ID, or
error class; no value is inferred for any of those fields. The request was not
retried.

The one-email authorization is consumed. No retry is permitted.

## Pre-Request Provider Baseline

Captured from the existing authenticated Resend Chrome session before the
request:

- `latest_event_id_or_none=67326c02...d110e14277c6`
- `latest_subject_class=password_reset`
- `latest_status=delivered`
- `transactional_monthly_used=2`
- `transactional_monthly_limit=3000`
- `transactional_daily_used=1`
- `transactional_daily_limit=100`
- `bounce_rate=0%`
- `complaint_rate=0%`
- `resend_sending_domain_status=verified`

## Protected Production Baseline

The active API slot was `blue`; `/healthz` returned HTTP `200`. The complete
Admin settings object SHA-256 was
`b7d74b9cb38dcbd431d0304b74a67fdb58cb3bfdf3d1c02e942cf7bd313ac88e`.

| Component | Container ID | Restart count |
| --- | --- | --- |
| API (blue) | `1075b35d5abd93e2140f94f555beeb20f495f1ec84efeec6ff72901876ec7aa2` | 0 |
| Worker | `1a42bb8d4a589fe5df7675164ee93d7f95f99dc90fe92c7f1bdab9c75bd2ceec` | 0 |
| PostgreSQL | `2db52788ad733522b3398f3ba9c0ff4c45a418c360a57424a9e115feb43d4db6` | 0 |
| Redis | `c45202c0d9e64f27d21191e87681c3ccb70e927555b74a4b9a47eb701afaa475` | 0 |
| Caddy | `ace4a23b965086852470350fc1e9de1232c793d0e01b8128eb2f5f8e4da1bd73` | 0 |

Sanitized Admin/public settings agreed on: `frontend_url` was the expected
production URL; email verification and password reset were enabled;
`registration_enabled=true`; all three CAPTCHA flags were `false`; the
registration-suffix whitelist had three entries; and SMTP was configured for
`smtp.resend.com:465` with TLS and configured credentials/from fields.

The baseline also exposed a policy drift: `invitation_code_enabled=false` while
all CAPTCHA flags were disabled. This was not modified.

## Provider Reconciliation

After the audit-proven request, Resend Emails showed no event created after the
request that could be classified as `test`. The latest visible event remained
the redacted password-reset event above. Transactional usage remained monthly
`2/3000` and daily `1/100`, so the delta was zero. Metrics still showed one
email in the last 15 days, 100% deliverability, 0% bounce rate, and 0%
complaint rate. The dashboard notes that metrics are refreshed periodically;
the Emails and Usage checks are the direct no-event/no-quota evidence.

## Decision Gate

`BLOCKED_OTHER` is required rather than a deadline-fix authorization:

1. The application audit proves an approximately 20-second HTTP 400, but the
   canceled client did not retain the response body, request ID, or a timeout
   error class required by the `DEADLINE_NO_EVENT` gate.
2. No provider acceptance was observed in Resend Emails or Usage (no matching
   event and zero quota delta), but this does not repair the missing client-side
   request ID or error classification.
3. The baseline policy drift (`invitation_code_enabled=false` with CAPTCHA
   disabled) remains a separate unresolved gate condition; this task did not
   change the intentional invitation policy.

## Documentation Fix (2026-08-09)

The original report overstated client evidence and chronology. This correction
records the exact request start timestamp, client request ID, and client error
class as unavailable; retains only the server completion UTC and audited
duration; states that invitation drift was discovered during the in-flight
request/audit; records that no retry occurred; and softens provider language to
the direct observation that no provider acceptance appeared in Resend Emails or
Usage. The `invitation_code_enabled=false` value is intentional user policy and
was not changed. No production or email retry action was taken.

Tasks 2 and 3 are not authorized. No code or production configuration change
was made.

## Post-Request Invariants

Read-only post-checks found `/healthz=200`, the same complete Admin-settings
SHA-256, the same sanitized settings values, and identical protected container
IDs/restart counts. The only observed state associated with the one request is
the audit row described above; Resend events and quota did not change.

## Required Follow-Up

Do not send another email. Resolve and explicitly authorize handling of the
invitation/CAPTCHA policy drift before any future email test. If a subsequent,
separately authorized reproduction is needed, it must capture the client
response error class and request ID without retaining sensitive payload data.
