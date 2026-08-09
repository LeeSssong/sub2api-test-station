# Task 2 Implementer Report

## Status

IMPLEMENTED - INDEPENDENTLY REVIEWED; Task 3 authorized

## Production Result

- Applied exactly one authenticated Admin API partial update at
  `2026-08-09T07:27:05Z`; it returned HTTP 200.
- Persisted `frontend_url=https://api.xingqiaolab.top`,
  `email_verify_enabled=true`, and `password_reset_enabled=true`.
- Admin API and sanitized PostgreSQL read both confirm all three values.
- Public settings confirm both enabled flags and preserved registration and
  invitation gating. `frontend_url` is not a field in the public-settings DTO;
  its value was verified through the Admin API and PostgreSQL.
- Health stayed HTTP 200. API/worker/PostgreSQL/Redis/Caddy IDs and restart
  counts remained unchanged; no restart or deployment occurred.

## Scope Controls

- Handler and service omission semantics were checked before the write.
- The settings hash excluding the three target keys was identical before and
  after, covering SMTP, whitelist, CAPTCHA, OAuth, billing, notifications,
  and other unrequested configuration.
- No direct database write, mail send, user creation, account mutation, DNS,
  Resend plan, or billing change was performed.

## Evidence

The committed activation report is
`docs/superpowers/reports/2026-08-09-sub2api-production-email-activation.md`.
It contains redacted baseline, request, rollback, persistence, and runtime
evidence without secrets or full email addresses.

## Concern

The public settings contract omits `frontend_url`; Task 3 should use its
normal public flag checks plus authenticated Admin API/database checks for the
reset-link base URL.

## Independent Review

**Decision: APPROVED - Task 3 authorized.**

- Reviewed commit `721f1295a` and the activation report against the Task 2
  brief. The recorded request and rollback payloads contain exactly the three
  approved fields and the report identifies one authenticated Admin API PUT,
  with no direct database write.
- The baseline and post-change evidence covers the Admin API, public settings,
  sanitized PostgreSQL values, health, runtime identities/restart counts,
  invitation gating, CAPTCHA flags, whitelist, and sanitized SMTP settings.
  The protected-settings hash is unchanged outside the three target keys.
- Source inspection confirms the handler computes omitted fields from the raw
  JSON payload and calls the omission-aware service update path. It also
  confirms that the public-settings DTO intentionally excludes `frontend_url`,
  so the Admin API and PostgreSQL checks are the correct evidence for that
  field; this is a verification-path limitation, not a Task 2 failure.
- The committed artifacts contain no credentials, tokens, passwords, full
  email addresses, or message content. Full container IDs are included only as
  the explicitly requested runtime identity evidence.

Task 3 may proceed. It must retain invitation-code gating, avoid creating a
production test user, and use authenticated Admin API/PostgreSQL checks for
the reset-link base URL in addition to the normal public flag checks.
