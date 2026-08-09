# Resend Free Account Qualification

**Date:** 2026-08-09
**Decision:** **QUALIFIED - Task 2 may perform its separate, gated settings
activation check.**

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

- Authenticated, read-only dashboard evidence confirms the active
  Transactional plan is **Free**.
- Usage displayed: monthly `1 / 3,000`; daily `0 / 100`.
- Domains entitlement displayed: `1 / 1`.
- Pay-as-you-go sending switches are disabled. Dedicated IP is shown only as
  pricing information and is not enabled.
- Last-15-day metrics display `0` emails, `0%` bounce rate, and `0%` complaint
  rate. No recipient, message, or event detail was inspected.

## Domain/DNS

- `resend._domainkey.xingqiaolab.top`: public DKIM TXT record present. Its
  content was intentionally not copied into this report.
- `send.xingqiaolab.top`: public SPF TXT record and MX record present. The MX
  target is an Amazon SES feedback hostname; its priority and record values are
  intentionally omitted.
- Root MX/SPF/DMARC and the sending-subdomain records coexist, which is
  consistent with keeping Tencent Enterprise Mail for inbound delivery and a
  separate transactional return path.
- Authenticated dashboard evidence confirms `xingqiaolab.top` is verified for
  sending in the Tokyo (`ap-northeast-1`) region.
- The dashboard verification state is consistent with the public DKIM and
  sending-subdomain records observed above. A line-by-line dashboard DNS table
  comparison was not separately retained.

## Decision

Task 1 is qualified: the active plan is Free, pay-as-you-go and dedicated IP
are not enabled, `xingqiaolab.top` is verified for sending, and the current
quota and health indicators are acceptable for the proposed controlled use.

This decision authorizes only Task 2's separate authenticated baseline and
settings update procedure. It does not itself enable `email_verify_enabled` or
`password_reset_enabled`, change DNS or SMTP, remove invitation gating, or
approve unrestricted public registration.

## Risks

- The public-settings endpoint was not reachable in the earlier read-only
  check, so the current
  application flags and persisted SMTP configuration need a separate
  authenticated, sanitized check before any update.
- DNS record presence was not used as the sole proof of plan tier,
  paid-product state, quota, account health, or provider-domain verification.
- Full public registration remains outside this qualification; invitation
  gating must remain enabled and CAPTCHA state must be assessed separately.

## Not Verified

- A line-by-line dashboard DNS-row comparison against public DNS was not
  separately captured.
- Current public Sub2API email, registration, invitation, whitelist, and
  CAPTCHA flags.

## Evidence

- Read-only DNS queries executed on 2026-08-09 for NS, root MX/SPF/DMARC,
  `resend._domainkey`, and `send` SPF/MX.
- Read-only Chrome fallback attempted on 2026-08-09; navigation timed out
  before a visible Resend page loaded.
- Subsequent authenticated, read-only Chrome dashboard inspection confirmed
  the plan, domain status, quota, paid-option state, and aggregate health
  indicators recorded above. No screenshot was retained.
- No production mutation was performed.
