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
| Sub2API endpoint | The authenticated Admin request reached Sub2API; it returned HTTP 400 due to an SMTP connection timeout. | The public request returned HTTP 200. |
| SMTP authentication | Not verified; the connection-timeout response does not establish that SMTP authentication completed. | Not independently observable from the retained redacted application logs. |
| Application processing | The access log records the HTTP 400. | Redacted logs record enqueue and a worker success event, with no server error; this does not by itself prove provider acceptance or mailbox delivery. |
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

## Independent Review

**Decision: APPROVED for the existing invitation-gated production flow; BLOCKED for unrestricted public registration and for any clean end-to-end provider-delivery claim.**

- Reviewed commit `51cb93dc7`, the Task 3 brief, the formal production verification report, and the frozen `59a769ae2..51cb93dc7` review package. The changed scope is documentation only and records one test-email request plus one forgot-password request to the same existing administrator mailbox; it records no new user, extra recipient, verification-code read, reset-token read, reset-link open, or token-consuming reset request.
- The supplied evidence is layered without substitution: Admin HTTP 400 and user-confirmed test receipt remain conflicting facts; the generic forgot-password HTTP 200 is anti-enumeration evidence; enqueue/worker success is application evidence only; direct Resend event acceptance, current quota effect, bounce state, and complaint state remain unverified after two activity-view timeouts.
- Source inspection confirms the configured `frontend_url` is read before the request, `/reset-password` is appended before enqueue, and token consumption occurs only on the separate reset-password path. Invitation-code gating is enforced before user creation. The forgot-password endpoint is rate-limited fail-closed, but CAPTCHA remains disabled, so the present approval depends on invitation gating remaining enabled.
- Focused scans found no full email address, credential, API key, SMTP password, JWT, reset token, or message content in the three task reports or production reports. The retained SHA-256 recipient hash and masked domain are within the brief's permitted redaction format.
- The configuration may remain active for the gated flow, but the SMTP timeout and unavailable Resend event/quota view must be reconciled before any claim of provider-verified delivery or any relaxation of registration controls.
