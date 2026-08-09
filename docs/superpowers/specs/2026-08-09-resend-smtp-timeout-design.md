# Resend SMTP Controlled Reproduction And Deadline Design

**Date:** 2026-08-09
**Status:** Design approved in conversation; pending written-spec review.

## Problem

Sub2API's authenticated `send-test-email` endpoint returned HTTP 400 after
about 20.2 seconds, while the SMTP client uses a fixed 20-second connection
deadline. The Resend activity page now shows the password-reset message as
`delivered`, but it does not show the test message. The earlier mailbox receipt
confirmation therefore cannot be attributed to the test message with enough
confidence.

The next step must distinguish a transient SMTP/network delay from a
repeatable client deadline defect before changing code or production
configuration.

## Scope

The task may:

- capture a redacted Resend event and Free-quota baseline;
- send exactly one additional test email to the same existing administrator
  mailbox through the authenticated Sub2API endpoint;
- record the endpoint status, duration, redacted error stage, Sub2API logs,
  and the resulting Resend event and quota state;
- implement a narrow SMTP deadline fix only if the controlled reproduction
  meets the decision gate below.

The task must not change the recipient, create a user, inspect verification
codes or reset tokens, change SMTP settings/DNS/Resend plan, disable invitation
gating, enable or disable CAPTCHA, or relax the registration whitelist.

## Controlled Reproduction

1. Record the current Resend Sending table, transactional monthly/daily usage,
   domain status, bounce rate, and complaint rate without retaining a full
   email address or message body.
2. Discover the existing active administrator mailbox on the production host
   without printing it. Reuse the existing production SMTP configuration.
3. Call `POST /api/v1/admin/settings/send-test-email` exactly once using the
   `X-API-Key` path. Record only HTTP status, duration, error class, recipient
   hash/masked domain, and request timestamp.
4. Re-read the Resend Sending table and quota state. Match the new event by
   timestamp and redacted subject class, not by exposing the recipient.
5. Confirm `/healthz`, protected container identities/restart counts, and all
   registration/email settings remain unchanged.

## Decision Gate

- **HTTP 200 and Resend `delivered`:** classify the earlier event as a
  transient failure. Do not change code. Close with production evidence.
- **HTTP 400 at approximately 20 seconds and no Resend event:** reproduce a
  client-side SMTP deadline failure. Proceed to the TDD fix below.
- **HTTP 400 at approximately 20 seconds but Resend records acceptance or
  delivery:** reproduce an ambiguous-success defect. Proceed to the same TDD
  fix and require the handler to return success only after the SMTP DATA
  acknowledgement is read.
- **Any other result:** stop fail-closed and report the new evidence. Do not
  guess or widen the change.

## Conditional Code Fix

If the decision gate authorizes a fix, keep `smtpDialTimeout` as the bounded
connection-establishment timeout and replace the single connection-lifetime
I/O deadline with a deadline-aware `net.Conn` wrapper that refreshes the
20-second deadline before each `Read` and `Write`. This creates a sliding
per-operation timeout while preserving a bounded failure for stalled SMTP
operations.

The wrapper is injected before `smtp.NewClient`, so the standard library SMTP
client automatically receives a fresh deadline for the server greeting,
EHLO/AUTH, MAIL, RCPT, DATA, message transfer, DATA acknowledgement, and QUIT.
No handler contract, SMTP port, TLS behavior, credential handling, or provider
integration changes.

## Error Handling

- Dial, TLS, authentication, envelope, DATA, write, and final DATA
  acknowledgement errors remain distinguishable in the existing wrapped error
  messages.
- A successful message requires `Data()` writer `Close()` to receive the
  provider's final acknowledgement. QUIT remains best-effort after success.
- Reproduction or verification failures leave the current production
  configuration active behind invitation gating and keep the task status
  `in progress`.

## Testing

Conditional implementation follows strict TDD:

1. Add a fake SMTP server test whose successful command/response sequence
   lasts longer than one I/O timeout in aggregate but less than one timeout per
   individual operation. The current fixed absolute deadline must fail.
2. Add a stalled-operation test proving a single read or write still times out.
3. Make the minimum connection-wrapper change and run the focused SMTP unit
   suite, the complete email-service unit suite, `go test ./...` where
   practical, `go vet`, and `git diff --check`.
4. Merge the reviewed candidate into `main`, run merged-main checks, deploy
   through the approved local/host chain, and repeat exactly one production
   test email only if the implementation plan explicitly authorizes that
   post-deployment verification.

## Completion Criteria

The task may be marked complete only when the controlled reproduction has an
unambiguous Sub2API/Resend outcome, any authorized code fix is independently
reviewed and deployed from verified `main`, provider activity is recorded,
production health is unchanged, and the online result is verified. Otherwise
the task remains in progress with its exact blocker.
