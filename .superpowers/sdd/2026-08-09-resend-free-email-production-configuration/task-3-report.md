# Task 3 Implementer Report

## Status

COMPLETED WITH DOCUMENTED PROVIDER-OBSERVABILITY BLOCKER - configuration stays active only for the existing invitation-gated production flow.

## Production Checks

- Selected the first active administrator by stable production user ID without printing its address. The recipient evidence hash is `d764a63fa2680407b78319aa7f9e311aa2e9d21b2c64bca1b0a38c1de0382119`; its domain was recorded only as `qq***.com`.
- Performed exactly one authenticated `POST /api/v1/admin/settings/send-test-email` with that recipient and the saved SMTP configuration. It returned HTTP 400 after an SMTP connection timeout.
- The user explicitly confirmed that the administrator mailbox received the test email. This is recorded as mailbox-receipt evidence, but it does not erase the conflicting API-layer timeout. No mailbox message, header, recipient, authentication result, or message content was inspected or recorded.
- Called `POST /api/v1/auth/forgot-password` for the same mailbox without consuming a reset token. It returned HTTP 200 with the normal generic anti-enumeration response. Redacted API logs record enqueueing the password reset work and then a worker send event; no server error was observed.
- Authenticated Admin API and read-only PostgreSQL both confirm `frontend_url=https://api.xingqiaolab.top`, `email_verify_enabled=true`, and `password_reset_enabled=true`. The public settings API confirms the two mail flags plus `registration_enabled=true` and `invitation_code_enabled=true`. All three CAPTCHA integrations remain disabled; the registration whitelist remains at three entries.
- `/healthz` remained HTTP 200. API, worker, PostgreSQL, Redis, and Caddy identities and restart counts match the Task 2 baseline (all are running, with restart count zero).

## Delivery Evidence By Layer

| Layer | Test email | Password reset |
| --- | --- | --- |
| SMTP/API invocation | Authenticated request reached Sub2API; HTTP 400 due to SMTP connection timeout. | Public request returned HTTP 200. |
| Application processing | The access log records the HTTP 400. | Redacted logs record enqueue and worker send, with no server error. |
| Provider acceptance/activity | Not directly verified. Resend activity navigation timed out; no event record was inferred. | Not directly verified for the same reason. |
| Mailbox receipt | User explicitly confirmed receipt. Sender display, From domain, subject, rendering, folder, and authentication indicators were not independently inspected. | Not verified; no message or reset link was opened. |
| Reset-link origin | Not applicable. | Configured base URL verified by Admin API and PostgreSQL as `https://api.xingqiaolab.top`; source construction therefore targets `https://api.xingqiaolab.top/reset-password`. No token or link was exposed or consumed. |

## Resend And Registration Gate

- The authenticated Resend domain view still showed `xingqiaolab.top` as a verified sending domain. Task 1's independently reviewed same-day baseline remains Free, with no paid add-on or dedicated IP enabled.
- A direct post-event Resend activity/quota read could not be completed: the activity view timed out twice. Therefore event counts, current quota impact, bounce state, and complaint state for these two events are **unverified**. The prior Task 1 aggregate baseline must not be treated as current-event proof.
- Invitation-code gating remains enabled. CAPTCHA remains disabled. This task neither created a user nor tested a new unregistered inbox, so registration verification E2E is unverified. Fully public registration is not approved.

## Decision And Concern

The email configuration remains active for the existing invitation-gated production flow. The confirmed test-mail receipt and password-reset queue/send logs are positive evidence, but the SMTP API timeout and missing direct Resend event/quota evidence prevent a clean end-to-end provider-verification claim. Investigate the SMTP timeout and re-check the two provider events/quota before expanding the registration posture. Do not disable invitation gating while CAPTCHA is disabled.

## Verification

- `git diff --check` and a focused secret/full-email scan are required before commit.
- No SMTP/API/admin key, JWT, reset token, full email address, or message content is included in this report.
