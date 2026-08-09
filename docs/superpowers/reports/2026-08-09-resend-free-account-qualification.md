# Resend Free Account Qualification

**Date:** 2026-08-09
**Decision:** **BLOCKED - do not activate email verification or password reset.**

## Scope

This was a read-only qualification of the existing transactional-mail setup for
`xingqiaolab.top`. It did not change Resend, DNS, billing, cards, dedicated IPs,
SMTP settings, Sub2API settings, or production data. No SMTP/API credential,
full email address, message content, recipient, billing amount, or screenshot
was recorded.

## Baseline

- Task-scoped production baseline: SMTP host `smtp.resend.com`, port `465`, TLS
  enabled, credentials configured, and a configured From address under the
  masked domain `*@xingqiaolab.top`. Credentials were neither read nor tested.
- Public-settings check: not verified. The public Sub2API settings endpoint
  reset the connection during this read-only run, so no feature-flag or saved
  SMTP value was inferred from it.
- Authoritative nameservers resolve to Cloudflare hostnames.
- Root inbound mail remains on Tencent Enterprise Mail: public root MX is a
  Tencent hostname; root SPF and DMARC records are present.

## Resend Account

- Dashboard session: unavailable. The in-app browser had no existing Resend
  tab, and a new read-only navigation to the Resend overview did not leave
  `about:blank` before the bounded timeout.
- Active plan: **not verified**.
- Paid add-ons/card attachment/dedicated IP: **not verified**.
- Monthly and daily limits, current usage, bounce/complaint warnings, and
  account health: **not verified**.

## Domain/DNS

- `resend._domainkey.xingqiaolab.top`: public DKIM TXT record present. Its
  content was intentionally not copied into this report.
- `send.xingqiaolab.top`: public SPF TXT record and MX record present. The MX
  target is an Amazon SES feedback hostname; its priority and record values are
  intentionally omitted.
- Root MX/SPF/DMARC and the sending-subdomain records coexist, which is
  consistent with keeping Tencent Enterprise Mail for inbound delivery and a
  separate transactional return path.
- Resend dashboard domain status and dashboard-to-DNS record match: **not
  verified**. Public DNS presence alone is not proof that Resend has verified
  the sending domain.

## Decision

Task 2 is not authorized. The required evidence that the active Resend plan is
Free, no paid add-on or dedicated IP is active, and `xingqiaolab.top` is
verified for sending is absent. Keep `email_verify_enabled` and
`password_reset_enabled` unchanged until an authenticated, read-only Resend
dashboard check captures those facts and current quota/health status.

## Risks

- Activating from SMTP connectivity or public DNS alone could consume an
  unqualified account or send through an unverified domain.
- The public-settings endpoint was not reachable in this run, so the current
  application flags and persisted SMTP configuration need a separate
  authenticated, sanitized check before any update.
- DNS record presence cannot establish the plan tier, paid-product state,
  quota, account health, or provider-domain verification.

## Not Verified

- Resend active plan is Free.
- No paid add-on, payment method, or dedicated IP is active.
- `xingqiaolab.top` is Resend-verified for sending.
- Dashboard DNS rows match the authoritative public records.
- Dashboard monthly/daily quota, current usage, bounce/complaint warnings, and
  account health.
- Current public Sub2API email, registration, invitation, whitelist, and
  CAPTCHA flags.

## Evidence

- Read-only DNS queries executed on 2026-08-09 for NS, root MX/SPF/DMARC,
  `resend._domainkey`, and `send` SPF/MX.
- No screenshot was retained because the dashboard could not be reached.
- No production mutation was performed.
