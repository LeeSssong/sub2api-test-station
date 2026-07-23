# Spec: Manual Ledger Simulation

## Problem

The MVP defers automatic payment, but manual recharge still needs a traceable separation between payment confirmation, Sub2API balance adjustment, customer usage, upstream cost, correction, and refund. A spreadsheet row that can be edited or deleted cannot prove idempotency or reveal unmatched payments.

## Goal

Create a local append-only JSONL ledger with a SHA-256 event chain, semantic validation, reconciliation summary, and a no-send Sub2API balance-request preview. Provide a simulation-only example and operating runbook.

## Non-goals

- Do not accept, verify, or initiate a real payment.
- Do not call Sub2API, change a user balance, create a redeem code, or access an Admin API key.
- Do not replace Sub2API's own balance history or usage log.
- Do not store payer bank details, full API keys, passwords, tokens, cookies, full prompts, or payment signing secrets.

## Event Model

Every line is one canonical JSON event containing `schema_version`, monotonic `sequence`, unique `event_id`, UTC `occurred_at`, `operator_ref`, `previous_hash`, and `event_hash`.

Supported types:

- `ledger_opened`: first event; declares ledger ID, `simulation` or future `real` mode, payment currency CNY, and balance currency USD.
- `payment_received`: records an order reference, CNY amount, USD-per-CNY operating rate, derived expected USD balance, user reference, and non-sensitive external reference.
- `balance_adjustment`: records Sub2API user ID, `add` or `subtract`, positive USD amount, source event, idempotency key, and `simulated`, `pending`, `succeeded`, or `failed` state.
- `refund_recorded`: records a CNY refund against an earlier payment.
- `usage_snapshot`: records a period's site billed usage in USD, upstream cost in CNY, and request count.
- `incident_recorded`: records a non-sensitive operational incident reference.

## Invariants

- First event is `ledger_opened`; sequence and hash chain are continuous.
- IDs, payment order IDs, external references, and idempotency keys are unique where applicable.
- References point to earlier compatible events and the same user.
- `expected_balance_usd = amount_cny x usd_per_cny` within decimal tolerance.
- A simulation ledger only accepts `simulated` balance adjustments; it cannot masquerade as a successful real balance change.
- Corrections use new `balance_adjustment` or `refund_recorded` events; old lines are never deleted or rewritten.
- Obvious credential fields and secret-shaped values are rejected.

## Reconciliation

The summary reports total payment CNY, expected USD credit, applied add/subtract adjustments, payment-to-credit variance, refunds, site billed usage, upstream cost, pending/failed adjustment count, and order IDs that do not reconcile.

For a simulation ledger, `simulated` adjustments count as applied for scenario math. For a future real ledger, only `succeeded` adjustments count.

## Sub2API Request Preview

For one balance adjustment, emit but do not send:

- `POST /api/v1/admin/users/:id/balance`
- `Idempotency-Key` and `Content-Type` headers, never an auth header
- Body with positive `balance`, `operation: add|subtract`, and ledger event reference in `notes`

The live operator must separately obtain authorization, verify the user and current balance, submit once, and append the result.

## Acceptance Criteria

- [ ] Valid chains verify and any payload/hash/sequence alteration fails.
- [ ] Duplicate IDs/order references/idempotency keys and bad references fail.
- [ ] Simulation events reconcile payment to credit and expose a deliberate variance.
- [ ] Request preview matches Sub2API v0.1.155 without containing an Admin API key.
- [ ] Secret-shaped fields or values fail.
- [ ] Example ledger is simulation-only, self-verifying, and ignored local-ledger paths are configured.
- [ ] No HTTP request or real payment action occurs.

