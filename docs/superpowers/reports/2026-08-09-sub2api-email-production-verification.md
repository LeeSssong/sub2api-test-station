# Sub2API Email Production Verification

**Date:** 2026-08-09
**Decision:** Active for existing invitation-gated use, with a provider-event verification blocker. Fully public registration is not approved.

## Scope And Safety Controls

This verification used the first active administrator selected on the production host without printing its address. Only its SHA-256 recipient hash `d764a63fa2680407b78319aa7f9e311aa2e9d21b2c64bca1b0a38c1de0382119` and masked domain `qq***.com` are retained. It sent exactly one Sub2API test email to that mailbox and submitted one password-reset request for the same mailbox. No user was created, no verification code or reset token was read, no reset link was opened, and no SMTP/API/admin credential was recorded.

## Feature And Gate State

Authenticated Admin API and read-only PostgreSQL agree that:

- `frontend_url=https://api.xingqiaolab.top`
- `email_verify_enabled=true`
- `password_reset_enabled=true`
- `registration_enabled=true`
- `invitation_code_enabled=true`

The public settings API independently confirms the two mail flags and the two registration flags. PostgreSQL also confirms that the registration suffix whitelist still contains three entries. Turnstile, Tencent, and Aliyun CAPTCHA flags are all `false`. Invitation-code gating therefore remains required; this verification does not approve unrestricted public registration.

The configured frontend URL is the password-reset link base. The application's reset-link construction appends `/reset-password`, so the verified origin is `https://api.xingqiaolab.top/reset-password`; no token-bearing link was exposed or consumed.

## Delivery Results

| Evidence layer | Test email | Password reset |
| --- | --- | --- |
| Sub2API endpoint | One authenticated Admin test-email call reached Sub2API and returned HTTP 400 because the SMTP connection timed out. | The public endpoint returned HTTP 200 with its normal generic anti-enumeration response. |
| SMTP authentication | Not verified; the connection-timeout response does not establish that SMTP authentication completed. | Not independently observable from the retained redacted application logs. |
| Application logs | The access log records the test endpoint HTTP 400. | Redacted logs record the request, enqueue, and a worker success event; no server error occurred, but this application event does not prove provider acceptance or mailbox delivery. |
| Provider evidence | Direct event acceptance is unverified. | Direct event acceptance is unverified. |
| Mailbox evidence | The user explicitly confirmed receipt by the administrator mailbox. Mailbox UI metadata and message content were not inspected or retained. | Not verified; the message and reset link were intentionally not opened. |

The HTTP 400 SMTP timeout and the user-confirmed test-mail receipt are intentionally reported as separate, conflicting layers. Receipt cannot be used to claim a clean test endpoint success, and the timeout cannot negate the user's reported mailbox observation.

## Resend Evidence

The authenticated Resend domain page remains verified for `xingqiaolab.top`. Task 1's independently reviewed same-day evidence established the Free plan, no paid add-on, no dedicated IP, and healthy aggregate metrics before this task. A direct post-event activity/quota query was attempted but timed out twice before data could be read. Current event count, quota impact, bounce state, and complaint state for the test and reset events are therefore unverified; no conclusion is inferred from the pre-event aggregate metrics.

## Runtime Health

`/healthz` returned HTTP 200 after the two requests. The API, worker, PostgreSQL, Redis, and Caddy containers match the Task 2 identities, are all running, and each has restart count zero. No deployment, restart, DNS change, SMTP configuration change, or Resend billing/plan change was made.

## Conclusion

The configuration is acceptable to remain active only for the current invitation-gated production flow. The user-confirmed test receipt plus the password-reset enqueue/worker-success logs show useful layered evidence, but SMTP authentication is not established by the timed-out test request and the missing direct provider activity/quota records remain blockers to a fully verified provider-delivery claim. Reconcile the SMTP timeout and obtain redacted Resend event/quota evidence before relaxing any registration control.
