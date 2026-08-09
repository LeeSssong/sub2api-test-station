# OpenAI Upstream Failure Recovery

## Customer Recovery

An OpenAI stream can end with the retryable `upstream_temporarily_unavailable` error. Preserve all content received before that terminal error.

Wait for the supplied `retry_after_seconds` value (and the HTTP `Retry-After` header when present), then use the client’s Continue action. Continue is a new logical request and excludes the account-model that failed in the prior response/session. Do not restart the client and do not rebuild an API key.

When `resume_supported` is true, submit the included `response_id` as the continuation reference. When it is false, do not fabricate a response ID: create a normal new continuation from the visible content. A post-output failure is never automatically replayed.

## Admin Recovery

Use `GET /api/v1/admin/account-monitors/runtime` to inspect the transient account-model state. The response reports the canonical scheduling model, failure streak, last error, cooldown deadline, and half-open lease state.

Use the account-monitor runtime actions only with an authenticated administrator session; every action is written through the existing administrative audit middleware.

- `POST /api/v1/admin/account-monitors/runtime/cooldown` with `account_id`, `canonical_scheduling_model`, and a bounded positive `cooldown_seconds` immediately places only that transient account-model in cooldown.
- `POST /api/v1/admin/account-monitors/runtime/restore` with `account_id` and `canonical_scheduling_model` removes only the transient record. It does not restore a persistently disabled account. Use the existing account restore workflow for hard-disable state.
- `POST /api/v1/admin/account-monitors/runtime/probe` with `account_id` and `canonical_scheduling_model` acquires one expired-cooldown half-open lease, runs exactly one real account-test probe for that canonical model, then releases the lease from the observed result. It returns the actual `success` value and never accepts an operator-supplied result.

## Safety Limits

Do not clear a transient record to bypass a persistent hard-disable, and do not release a pending half-open lease manually. Do not retry a request that has emitted content, a tool side effect, or unknown completion state. Treat `response_id` as optional and only resume when the recovery event explicitly supports it.

Watch the structured events `openai.stream_upstream_failure`, `openai.account_model_soft_failure`, `openai.account_model_cooldown_started`, `openai.account_model_cooldown_skipped_for_cache`, `openai.failover_after_stream_failure`, `openai.account_model_half_open_probe`, and `openai.retry_billing_reconciled`. The Ops alert evaluator accepts the matching account-model failure, cooldown saturation, stream/failover degradation, post-failure selection, and cache-hit/failover decline counters.

## Rollback

Rollback this application release through the reviewed local/host blue-green release chain. Do not use GitHub Actions. If rollback is not immediately available, leave transient cooldowns in place, restore only verified healthy account-model records through the admin endpoint, and retain the audit and structured-event evidence for follow-up.
