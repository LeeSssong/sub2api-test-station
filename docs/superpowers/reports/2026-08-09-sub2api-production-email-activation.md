# Sub2API Production Email Activation

**Date:** 2026-08-09
**Status:** Implemented; pending independent review and Task 3 delivery verification.

## Scope

Task 1 was independently approved at `049d93988`. This task changed exactly
three persisted Sub2API settings through the authenticated Admin API. It did
not change SMTP, DNS, registration policy, invitation gating, CAPTCHA, OAuth,
billing, notifications, container images, or runtime topology.

## Handler Contract

Before the write, the current handler was reviewed. It reads the original JSON
field set, calculates omitted setting keys, and calls
`UpdateSettingsWithAuthSourceDefaultsOmitting`. The service removes omitted
keys from the database update map and refreshes its cache from stored settings
after a partial update. This is the explicit partial-payload safety contract;
the request was not an empty or whole-document update.

## Baseline

Captured at `2026-08-09T07:26:08Z` from the production host:

- Admin settings API: HTTP 200; public settings API: HTTP 200; health: HTTP
  200.
- `frontend_url` was empty; `email_verify_enabled` and
  `password_reset_enabled` were both `false`.
- `registration_enabled=true` and `invitation_code_enabled=true`.
- Turnstile, Tencent, and Aliyun CAPTCHA flags were all `false`.
- Registration whitelist retained three entries (sanitized only).
- SMTP remained configured: `smtp.resend.com`, port `465`, TLS enabled,
  username/password present, a configured From name, and a masked From domain
  ending in `xingqiaolab.top`.
- Full baseline settings SHA-256:
  `c7a8abc6bbcae954676a41771e82d689c8d081470f91d6cc556648716afcd6f8`.
- Baseline SHA-256 excluding the three requested fields:
  `f97ed75bb3de26d21ed2e0f6425f2153bf9fa72725c5039fe7385ee1aadb718e`.

The active API slot was blue. The following protected containers were running
with restart count zero:

| Component | Container ID | Restart count |
| --- | --- | --- |
| API (blue) | `1075b35d5abd93e2140f94f555beeb20f495f1ec84efeec6ff72901876ec7aa2` | 0 |
| Worker | `1a42bb8d4a589fe5df7675164ee93d7f95f99dc90fe92c7f1bdab9c75bd2ceec` | 0 |
| PostgreSQL | `2db52788ad733522b3398f3ba9c0ff4c45a418c360a57424a9e115feb43d4db6` | 0 |
| Redis | `c45202c0d9e64f27d21191e87681c3ccb70e927555b74a4b9a47eb701afaa475` | 0 |
| Caddy | `ace4a23b965086852470350fc1e9de1232c793d0e01b8128eb2f5f8e4da1bd73` | 0 |

## Authenticated Update

At `2026-08-09T07:27:05Z`, one authenticated `PUT
/api/v1/admin/settings` using the `X-API-Key` header returned HTTP 200. The
redacted request shape was exactly:

```json
{
  "frontend_url": "https://api.xingqiaolab.top",
  "email_verify_enabled": true,
  "password_reset_enabled": true
}
```

No TOTP step-up was required for the Admin API key path. No direct database
write was performed.

## Persistence And Runtime Verification

Final evidence was captured at `2026-08-09T07:30:22Z` from the production
host:

- Admin settings API, public settings API, and `/healthz` each returned HTTP
  200.
- Authenticated Admin API reported
  `frontend_url=https://api.xingqiaolab.top`,
  `email_verify_enabled=true`, and `password_reset_enabled=true`.
- Sanitized PostgreSQL read returned the same three persisted values.
- Public settings API reported `email_verify_enabled=true`,
  `password_reset_enabled=true`, `registration_enabled=true`, and
  `invitation_code_enabled=true`.
- The public-settings DTO intentionally has no `frontend_url` field. Its
  absence from that response is therefore a contract limitation, not a failed
  persistence check; the authenticated Admin API and PostgreSQL query are the
  evidence for the URL.
- The protected-settings SHA-256 excluding the three target fields remained
  `f97ed75bb3de26d21ed2e0f6425f2153bf9fa72725c5039fe7385ee1aadb718e`.
  This proves all other Admin settings, including the whitelist, SMTP, OAuth,
  billing, and notification configuration, matched the baseline response.
- Invitation gating stayed enabled; CAPTCHA flags and sanitized SMTP values
  matched the baseline.
- All five protected container IDs and restart counts matched the baseline;
  all remained running. No restart or deployment occurred.

## Rollback Payload

If rollback is separately approved, submit exactly this minimal authenticated
Admin API payload; do not modify PostgreSQL directly:

```json
{
  "frontend_url": "",
  "email_verify_enabled": false,
  "password_reset_enabled": false
}
```

## Remaining Risks

- Task 3 must still prove test-email API acceptance, provider acceptance,
  mailbox receipt where available, and the password-reset flow. This task did
  not send mail or inspect user data.
- Invitation-code gating remains enabled and every CAPTCHA integration remains
  disabled. This activation does not authorize unrestricted public
  registration.
- The public settings API cannot expose `frontend_url` by its current DTO;
  future checks must use the authenticated Admin API and a sanitized database
  read for that field.
