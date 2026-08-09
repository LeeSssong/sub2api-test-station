# Resend Free Account Qualification

**Date:** 2026-08-09
**Decision:** **QUALIFIED - Task 2 may perform its separate, gated settings
activation check.**

## Scope

This was a read-only qualification of the existing transactional-mail setup for
`xingqiaolab.top`. It did not change Resend, DNS, billing, cards, dedicated IPs,
SMTP settings, Sub2API settings, or production data. No SMTP/API credential,
full email address, message content, recipient, payment method, invoice
identifier, or paid-plan purchase was recorded.

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
- Billing lists the Transactional and Marketing subscriptions at `$0 / mo`,
  reports no payment methods, and reports no invoices.
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
- Independent review compared every dashboard sending record with public DNS:
  the DKIM TXT value, `send` MX target and priority, and `send` SPF TXT value
  match, and all three dashboard rows report `verified`. Record contents remain
  intentionally omitted from this report.

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

- No screenshot artifact was retained. The independent reviewer re-queried the
  authenticated dashboard rather than relying solely on the implementer's
  written summary.
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
- Independent authenticated review confirmed the Free quotas, `$0 / mo`
  subscriptions, absence of payment methods and invoices, disabled
  pay-as-you-go controls, absence of a dedicated-IP subscription, verified
  domain/record state, and aggregate health metrics. Public DNS was queried
  again for the record comparison.
- No production mutation was performed.
