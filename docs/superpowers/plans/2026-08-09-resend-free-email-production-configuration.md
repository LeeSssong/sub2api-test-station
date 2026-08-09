# Resend Free Email Production Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Use the existing Resend free-tier account and verified `xingqiaolab.top` domain to activate Sub2API registration verification and password-reset email safely in production.

**Architecture:** Keep Tencent Enterprise Mail as the inbound MX provider and Resend as the outbound transactional SMTP provider. Change only persisted Sub2API settings through the authenticated admin API, retain invitation-code gating throughout the rollout, and verify delivery with the existing administrator mailbox before considering public registration.

**Tech Stack:** Resend dashboard/API, Cloudflare DNS, Sub2API admin/public APIs, PostgreSQL settings, SMTP over TLS, Docker Compose production deployment.

## Global Constraints

- Use the existing Resend account only; the target plan is Free, with no paid subscription, card attachment, plan upgrade, dedicated IP, or new provider purchase.
- Never print, persist, or copy SMTP passwords, Resend API keys, admin API keys, JWTs, TOTP codes, or full email addresses into reports.
- Keep `registration_enabled=true` and `invitation_code_enabled=true`; do not disable invitation-code gating in this plan.
- Set `frontend_url` exactly to `https://api.xingqiaolab.top`.
- Set `email_verify_enabled=true` and `password_reset_enabled=true` only after confirming the saved Resend SMTP configuration and domain state.
- Preserve the current SMTP host, port, username, password, From address, From name, TLS setting, registration whitelist, all CAPTCHA settings, OAuth settings, billing settings, and unrelated application settings.
- Do not change the root-domain Tencent Enterprise Mail MX/SPF records or the existing Resend DKIM and `send` return-path DNS records unless Resend explicitly reports a verification failure and the exact change is independently reviewed first.
- Send test mail only to the existing production administrator mailbox discovered from production; do not send to arbitrary users.
- Do not create a production test user, delete users, change user balances, or inspect verification codes in Redis.
- Record the exact pre-change values and provide a rollback payload before the production settings update.
- A successful SMTP connection is not sufficient: require a successful Sub2API test-email API response and user-visible receipt confirmation where the available mailbox/browser session permits it.
- If CAPTCHA remains disabled, report that fully public registration is still blocked; this plan does not enable or configure CAPTCHA.
- No GitHub Actions may be added or used.
- Do not modify, clean, merge, or delete other worktrees, release evidence, or protected “新建运营界面” / “优化账号卡片” work.

---

### Task 1: Qualify the existing Resend Free account and sending domain

**Files:**
- Create: `docs/superpowers/reports/2026-08-09-resend-free-account-qualification.md`

**Interfaces:**
- Consumes: Existing Resend dashboard session or existing production Resend credentials; public DNS for `xingqiaolab.top`.
- Produces: A redacted qualification decision that Task 2 can use to authorize or block activation.

- [ ] **Step 1: Capture the production baseline without secrets**

Run read-only checks for public settings, sanitized SMTP settings, authoritative NS, root MX/SPF/DMARC, Resend DKIM, and `send` MX/SPF. Record only booleans, provider hostnames, port, TLS, masked From domain, and configuration presence.

- [ ] **Step 2: Inspect the Resend account and domain**

Open the Resend dashboard using an existing signed-in session. Verify the active plan is Free, no paid add-on or dedicated IP is active, `xingqiaolab.top` is verified for sending, and the dashboard DNS records match the public records. Do not add a card or change the plan.

- [ ] **Step 3: Verify current quota and account health**

Record the dashboard-displayed monthly/daily limit, current usage, domain status, and any bounce/complaint warning without recording recipient addresses or message content. If the plan or domain cannot be confirmed, stop with `BLOCKED`.

- [ ] **Step 4: Write and commit the qualification report**

The report must contain Scope, Baseline, Resend Account, Domain/DNS, Decision, Risks, and Not Verified sections. Include screenshot evidence paths only if screenshots contain no secrets or recipient addresses.

Run:

```bash
git diff --check
git add docs/superpowers/reports/2026-08-09-resend-free-account-qualification.md
git commit -m "docs: qualify Resend free email account"
```

Expected: one documentation commit, no production changes.

### Task 2: Activate Sub2API email verification and password reset

**Files:**
- Create: `docs/superpowers/reports/2026-08-09-sub2api-production-email-activation.md`

**Interfaces:**
- Consumes: Task 1 qualification decision and current Sub2API production settings.
- Produces: Persisted `frontend_url`, email verification, and password reset settings with an exact rollback record.

- [ ] **Step 1: Re-read the complete current settings and build a minimal payload**

Use the authenticated admin API to obtain current settings. Construct a payload containing only:

```json
{
  "frontend_url": "https://api.xingqiaolab.top",
  "email_verify_enabled": true,
  "password_reset_enabled": true
}
```

Before sending it, verify from the current handler contract that omitted fields are preserved. Record the rollback payload:

```json
{
  "frontend_url": "",
  "email_verify_enabled": false,
  "password_reset_enabled": false
}
```

- [ ] **Step 2: Capture protected runtime identities and settings baseline**

Record the active Sub2API slot, API/worker/PostgreSQL/Redis/Caddy container IDs, restart counts, public health, registration/invitation/CAPTCHA flags, registration whitelist, and sanitized SMTP settings.

- [ ] **Step 3: Apply the minimal authenticated settings update**

Send the three-field payload once. Do not change registration, invitation, whitelist, CAPTCHA, SMTP, OAuth, billing, or notification settings. If the API requires a human TOTP step-up and the existing browser session cannot satisfy it, stop with `BLOCKED` rather than updating PostgreSQL directly.

- [ ] **Step 4: Verify persistence and runtime stability**

Verify through both the public settings API and a sanitized PostgreSQL query that `frontend_url`, `email_verify_enabled`, and `password_reset_enabled` have the target values. Confirm invitation gating remains enabled, CAPTCHA remains unchanged, SMTP fields are unchanged, health is 200, and all protected container IDs/restart counts are unchanged.

- [ ] **Step 5: Write and commit the activation report**

Record timestamps, redacted request shape, response status, pre/post values, rollback payload, container/health evidence, and remaining risks.

Run:

```bash
git diff --check
git add docs/superpowers/reports/2026-08-09-sub2api-production-email-activation.md
git commit -m "docs: record Sub2API email activation"
```

Expected: settings are active without restarting or redeploying any container.

### Task 3: Verify test delivery and gated authentication flows

**Files:**
- Create: `docs/superpowers/reports/2026-08-09-sub2api-email-production-verification.md`

**Interfaces:**
- Consumes: Task 2 activated settings and the existing administrator mailbox.
- Produces: Delivery evidence, password-reset request evidence, and a final decision on whether email configuration is production-ready while invitation gating remains enabled.

- [ ] **Step 1: Send one Sub2API test email to the administrator mailbox**

Discover the first active administrator email from production without printing it. Call `/api/v1/admin/settings/send-test-email` using the saved SMTP settings and that address. Record only a recipient hash or masked domain and the API result.

- [ ] **Step 2: Confirm receipt when accessible**

Use the existing signed-in mailbox/browser session if available. Confirm sender display name, From domain, subject, body rendering, delivery folder, and authentication results shown by the provider. Do not expose message content containing addresses or headers with identifiers. If the mailbox is not accessible, report receipt as unverified rather than assuming success.

- [ ] **Step 3: Exercise the password-reset request path**

Call the public forgot-password endpoint for the administrator email. Verify the public anti-enumeration response, the absence of server errors, and—if mailbox access permits—the reset email receipt and that its link begins with `https://api.xingqiaolab.top/reset-password`. Do not open or consume the reset token unless required to verify the URL origin.

- [ ] **Step 4: Verify the registration email UI/API gate without creating a user**

Confirm public settings expose email verification and password reset as enabled, the invitation flag remains enabled, and the registration whitelist remains unchanged. Do not create a production test user or extract a verification code. Record that a new-inbox registration remains a separate acceptance item if no controlled unregistered inbox is available.

- [ ] **Step 5: Check logs and Resend activity**

Inspect redacted Sub2API worker/API logs and Resend activity for the two test events. Record success/failure counts, bounce/complaint state, and quota impact without recording recipient addresses.

- [ ] **Step 6: Write and commit the final verification report**

The report must clearly distinguish SMTP authentication, API acceptance, provider acceptance, actual mailbox receipt, and unverified registration E2E. It must state that fully public registration is not approved while invitation gating is on and CAPTCHA is off.

Run:

```bash
git diff --check
git add docs/superpowers/reports/2026-08-09-sub2api-email-production-verification.md
git commit -m "docs: verify production email delivery"
```

Expected: the email configuration is either verified for gated production use or left active with a precise, documented blocker; invitation gating remains enabled.

## Final Verification

- [ ] `git diff --check` is clean for the complete branch.
- [ ] All three task reports contain no secrets or full email addresses.
- [ ] Resend remains on Free with no paid add-on.
- [ ] `frontend_url=https://api.xingqiaolab.top`.
- [ ] `email_verify_enabled=true` and `password_reset_enabled=true`.
- [ ] `registration_enabled=true` and `invitation_code_enabled=true` remain unchanged.
- [ ] CAPTCHA and registration whitelist remain unchanged.
- [ ] SMTP configuration and DNS remain unchanged.
- [ ] Protected containers remain healthy with unchanged identity/restart counts.
- [ ] Test-email and password-reset outcomes are recorded at each delivery layer.
- [ ] The production progress ledger is updated, but status is not marked completed unless configuration, delivery, and online verification all succeed.
