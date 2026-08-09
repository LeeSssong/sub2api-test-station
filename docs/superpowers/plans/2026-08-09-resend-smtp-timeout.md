# Resend SMTP Timeout Reproduction And Conditional Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce one unambiguous Sub2API/Resend test-email result and, only if the 20-second SMTP deadline defect reproduces, replace the connection-lifetime deadline with a bounded sliding per-operation deadline.

**Architecture:** First capture a redacted production and Resend baseline, send exactly one additional test email to the existing administrator mailbox, and classify the result through an explicit decision gate. A reproducible deadline failure authorizes a narrow `net.Conn` wrapper in the existing SMTP service; a successful test closes the task without code changes. Any code change must pass TDD, independent review, merged-main validation, the approved host deployment chain, and one explicitly authorized post-deployment verification.

**Tech Stack:** Go `net`, `net/smtp`, TLS, Sub2API Admin API, Docker Compose production runtime, Resend dashboard, Git worktrees, shell/Go tests.

## Global Constraints

- Send exactly one pre-fix controlled test email to the same existing active administrator mailbox; retain only a recipient SHA-256 and masked domain.
- Do not create users, read verification codes, inspect or consume reset tokens, or send to another recipient.
- Keep `registration_enabled=true`, `invitation_code_enabled=true`, all CAPTCHA settings, the three-entry registration whitelist, SMTP settings, DNS, Resend Free plan, OAuth, billing, notifications, and protected container identities unchanged.
- Use the authenticated Admin API with `X-API-Key`; never print or persist the key and never bypass the Admin API with direct database writes.
- Do not change SMTP port, TLS semantics, credentials, provider, handler payloads, or email templates.
- A code fix is authorized only when Task 1 records HTTP 400 at approximately 20 seconds and either no Resend event or a conflicting accepted/delivered event.
- If Task 1 returns HTTP 200 and Resend `delivered`, classify the prior failure as transient and skip Tasks 2 and 3.
- Any result outside the decision gate stops fail-closed with no code or production mutation.
- Do not use GitHub Actions. Release and deployment must use the reviewed local/host script chain.
- Do not modify, merge, clean, or delete other active/protected worktrees or their uncommitted content.
- Do not mark the task complete until provider activity, production health, and any deployed fix are verified online.

---

### Task 1: Controlled Production Reproduction

**Files:**
- Create: `docs/superpowers/reports/2026-08-09-resend-smtp-controlled-reproduction.md`
- Modify: `docs/project/project-progress.md`

**Interfaces:**
- Consumes: existing Resend authenticated browser session; production SSH alias `sub2api-prod`; `/opt/sub2api/production/secrets/sub2api-admin-api-key`; saved Sub2API SMTP settings.
- Produces: decision value `TRANSIENT_SUCCESS`, `DEADLINE_NO_EVENT`, `DEADLINE_AMBIGUOUS_SUCCESS`, or `BLOCKED_OTHER`, plus a redacted reproduction report.

- [ ] **Step 1: Capture the pre-request Resend baseline**

Read `https://resend.com/emails`, `https://resend.com/settings/usage`, and `https://resend.com/metrics` in the authenticated browser. Record only:

```text
latest_event_id_or_none=<redacted UUID or none>
latest_subject_class=<test|password_reset|other|none>
latest_status=<delivered|sent|bounced|complained|none>
transactional_monthly_used=<integer>
transactional_monthly_limit=3000
transactional_daily_used=<integer>
transactional_daily_limit=100
bounce_rate=<percentage>
complaint_rate=<percentage>
```

Do not retain a full recipient address, message body, reset link, or headers.

- [ ] **Step 2: Capture the protected production baseline**

From `sub2api-prod`, record the active API slot; API, worker, PostgreSQL, Redis, and Caddy container IDs/restart counts; `/healthz`; sanitized Admin/public settings; and a SHA-256 of all Admin settings excluding no fields. Discover the first active administrator email into a shell variable and emit only:

```bash
printf '%s' "$ADMIN_EMAIL" | sha256sum
printf '%s\n' "$ADMIN_EMAIL" | sed -E 's/^(.{2}).*(@[^.]+\..*)$/\1***\2/'
```

Do not echo the source query or variable value.

- [ ] **Step 3: Send exactly one controlled test email**

On the production host, read the Admin API key into a shell variable, build the JSON payload with `jq -n --arg email "$ADMIN_EMAIL" '{email:$email}'`, and call:

```text
POST https://api.xingqiaolab.top/api/v1/admin/settings/send-test-email
X-API-Key: <production key from file>
Content-Type: application/json
```

Capture start/end UTC timestamps, HTTP status, total duration, request ID, and the redacted error class. Do not print the request headers, payload, response body containing configuration, email address, or key.

- [ ] **Step 4: Correlate the provider event and quota**

After the request completes, re-read Resend Emails, Usage, and Metrics. Match only events created after the Step 3 timestamp and classify the subject as `test`. Record status and quota delta without retaining the recipient.

- [ ] **Step 5: Apply the decision gate**

Set exactly one decision:

```text
TRANSIENT_SUCCESS
  HTTP 200, duration below 20 seconds, and matching Resend event delivered.

DEADLINE_NO_EVENT
  HTTP 400, duration 19-22 seconds, timeout error, and no matching Resend event.

DEADLINE_AMBIGUOUS_SUCCESS
  HTTP 400, duration 19-22 seconds, timeout error, and matching Resend event accepted/sent/delivered.

BLOCKED_OTHER
  Any other combination.
```

For `TRANSIENT_SUCCESS`, record that Tasks 2 and 3 are skipped. For either `DEADLINE_*`, authorize Task 2. For `BLOCKED_OTHER`, stop without code changes.

- [ ] **Step 6: Verify invariants and commit the report**

Re-read health, settings, and container identities/restarts. Require all baselines unchanged except Resend usage/event state caused by the single authorized email. Run:

```bash
git diff --check
rg -n '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' \
  docs/superpowers/reports/2026-08-09-resend-smtp-controlled-reproduction.md \
  docs/project/project-progress.md
```

Expected: `git diff --check` passes and the email scan has no match. Commit:

```bash
git add docs/project/project-progress.md \
  docs/superpowers/reports/2026-08-09-resend-smtp-controlled-reproduction.md
git commit -m "docs: reproduce Resend SMTP timeout"
```

### Task 2: Conditional Sliding SMTP I/O Deadline Fix

**Gate:** Run only for `DEADLINE_NO_EVENT` or `DEADLINE_AMBIGUOUS_SUCCESS` from Task 1.

**Files:**
- Modify: `upstream/sub2api/backend/internal/service/email_service.go`
- Modify: `upstream/sub2api/backend/internal/service/email_service_smtp_test.go`
- Create: `docs/superpowers/reports/2026-08-09-resend-smtp-deadline-fix.md`

**Interfaces:**
- Consumes: `smtpIOTimeout`; `newSMTPClient(conn net.Conn, host string) (*smtp.Client, error)`.
- Produces: `deadlineConn` with `Read([]byte) (int, error)` and `Write([]byte) (int, error)` that refresh a per-operation deadline before delegating.

- [ ] **Step 1: Make the SMTP timeout injectable for unit tests**

Change the timeout declarations to package variables while preserving production defaults:

```go
var smtpDialTimeout = 10 * time.Second
var smtpIOTimeout = 20 * time.Second
```

Each test that changes `smtpIOTimeout` must restore it with `t.Cleanup`.

- [ ] **Step 2: Write the failing aggregate-duration test**

Extend the fake SMTP server with an optional per-response delay. Add a test that sets `smtpIOTimeout` to `100 * time.Millisecond`, delays each SMTP response by `40 * time.Millisecond`, and completes multiple successful operations whose aggregate duration exceeds 100 ms while no individual operation does:

```go
func TestSendEmailWithConfigRefreshesDeadlinePerSMTPIO(t *testing.T) {
    previous := smtpIOTimeout
    smtpIOTimeout = 100 * time.Millisecond
    t.Cleanup(func() { smtpIOTimeout = previous })

    srv, port := startDelayedFakeSMTPServer(t, true, false, 40*time.Millisecond)
    svc := &EmailService{}

    err := svc.SendEmailWithConfig(
        smtpTestConfig(port, true),
        "rcpt@example.com",
        "subject",
        "<p>body</p>",
    )
    if err != nil {
        t.Fatalf("expected aggregate duration above one timeout to succeed, got: %v", err)
    }
    if !srv.sawCommand("DATA") {
        t.Fatal("expected send path to reach DATA")
    }
}
```

- [ ] **Step 3: Run the aggregate-duration test and verify RED**

Run:

```bash
cd upstream/sub2api/backend
go test -tags=unit ./internal/service -run TestSendEmailWithConfigRefreshesDeadlinePerSMTPIO -count=1 -v
```

Expected: FAIL with an I/O timeout under the fixed connection-lifetime deadline.

- [ ] **Step 4: Write the stalled-operation regression test**

Add a fake-server mode that delays the final DATA acknowledgement by `250 * time.Millisecond`. With `smtpIOTimeout=100*time.Millisecond`, assert:

```go
func TestSendEmailWithConfigTimesOutStalledSMTPIO(t *testing.T) {
    previous := smtpIOTimeout
    smtpIOTimeout = 100 * time.Millisecond
    t.Cleanup(func() { smtpIOTimeout = previous })

    _, port := startFinalAckDelayedSMTPServer(t, 250*time.Millisecond)
    svc := &EmailService{}

    err := svc.SendEmailWithConfig(
        smtpTestConfig(port, true),
        "rcpt@example.com",
        "subject",
        "<p>body</p>",
    )
    if err == nil || !strings.Contains(err.Error(), "close writer") {
        t.Fatalf("expected bounded final DATA acknowledgement timeout, got: %v", err)
    }
}
```

- [ ] **Step 5: Implement the minimal deadline wrapper**

Add beside `newSMTPClient`:

```go
type deadlineConn struct {
    net.Conn
    timeout time.Duration
}

func (c *deadlineConn) Read(p []byte) (int, error) {
    if err := c.Conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
        return 0, err
    }
    return c.Conn.Read(p)
}

func (c *deadlineConn) Write(p []byte) (int, error) {
    if err := c.Conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
        return 0, err
    }
    return c.Conn.Write(p)
}
```

Update `newSMTPClient` to wrap the established connection and remove the single absolute `SetDeadline` call:

```go
func newSMTPClient(conn net.Conn, host string) (*smtp.Client, error) {
    timedConn := &deadlineConn{Conn: conn, timeout: smtpIOTimeout}
    client, err := smtp.NewClient(timedConn, host)
    if err != nil {
        _ = conn.Close()
        return nil, fmt.Errorf("new smtp client: %w", err)
    }
    return client, nil
}
```

- [ ] **Step 6: Run focused and broad verification**

Run:

```bash
cd upstream/sub2api/backend
go test -tags=unit ./internal/service -run 'Test(SendEmailWithConfig|SMTPConnection)' -count=1 -v
go test -tags=unit ./internal/service -count=1
go test ./...
go vet ./...
cd ../../..
git diff --check
```

Expected: all commands pass. If the environment prevents a broad command, record the exact command/error and keep the task in progress.

- [ ] **Step 7: Write and commit the conditional fix report**

The report must include the Task 1 decision, RED output, GREEN output, exact source diff, security invariants, tests, and deployment requirement. Scan for secrets/full emails and commit:

```bash
git add upstream/sub2api/backend/internal/service/email_service.go \
  upstream/sub2api/backend/internal/service/email_service_smtp_test.go \
  docs/superpowers/reports/2026-08-09-resend-smtp-deadline-fix.md \
  docs/project/project-progress.md
git commit -m "fix: refresh SMTP I/O deadlines"
```

### Task 3: Conditional Merge, Deployment, And Online Verification

**Gate:** Run only when Task 2 produced an independently approved code commit.

**Files:**
- Modify: `docs/project/project-progress.md`
- Create: `docs/superpowers/reports/2026-08-09-resend-smtp-deadline-production-verification.md`

**Interfaces:**
- Consumes: independently approved Task 2 commit; approved local/host release scripts; production Admin API and Resend dashboard.
- Produces: merged/pushed/deployed/online-verified result or an exact preserved blocker.

- [ ] **Step 1: Reconcile worktrees and merge the candidate to `main`**

Re-scan all worktrees and preserve protected/active exceptions. Merge the reviewed candidate into `main` without discarding unrelated root changes. Record candidate commit, merge commit, tree hash, and retained recovery evidence.

- [ ] **Step 2: Run merged-main verification**

On merged `main`, rerun:

```bash
cd upstream/sub2api/backend
go test -tags=unit ./internal/service -run 'Test(SendEmailWithConfig|SMTPConnection)' -count=1 -v
go test -tags=unit ./internal/service -count=1
go test ./...
go vet ./...
cd ../../..
git diff --check
```

Require the repository's migration/release preflight and current production baseline checks before any push or deployment.

- [ ] **Step 3: Push and deploy through the approved chain**

Push verified `main`, publish/qualify the immutable candidate with the reviewed local scripts, deploy using the host blue-green chain, and preserve all release evidence. Do not use GitHub Actions. On any failure, preserve the candidate/worktree and continue remediation on the same candidate.

- [ ] **Step 4: Send one post-deployment verification email**

After health and container checks pass, send exactly one test email to the same administrator mailbox and capture the same Sub2API/Resend evidence contract as Task 1. Require HTTP 200 and a matching Resend `delivered` event. Do not send a password-reset message in this step.

- [ ] **Step 5: Close or retain the task truthfully**

Verify health, container identities/restarts, settings invariants, Resend quota delta, bounce/complaint state, and mailbox receipt. Write the production report and update the ledger. Mark complete only for pushed, deployed, and online-verified success; otherwise keep `in progress` and retain the candidate/worktree and evidence.

```bash
git add docs/project/project-progress.md \
  docs/superpowers/reports/2026-08-09-resend-smtp-deadline-production-verification.md
git commit -m "docs: verify Resend SMTP deadline fix"
```

## Final Verification

- [ ] Exactly one pre-fix controlled test email was sent.
- [ ] Task 1 decision matches the documented status/duration/provider event combination.
- [ ] Tasks 2 and 3 were skipped unless the deadline gate authorized them.
- [ ] Any code change has RED/GREEN evidence, independent task review, and whole-branch review.
- [ ] Invitation gating remains enabled and CAPTCHA/whitelist/SMTP/DNS/Resend plan remain unchanged.
- [ ] No secret, full email address, verification code, reset token, or message body exists in committed artifacts.
- [ ] A deployed fix, if any, was merged and verified on `main` through the local/host chain without GitHub Actions.
- [ ] Project status remains `in progress` unless push, deployment, provider event, health, and online verification all pass.
