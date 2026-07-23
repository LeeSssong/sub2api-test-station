# Verification: Manual Ledger Simulation

## Scope

Verify the append-only event chain, semantic controls, payment-to-balance reconciliation, usage summary, no-send Sub2API request preview, simulation examples, local file protections, and regressions against pricing, upstream, and infrastructure baselines.

## Commands and Results

| Check | Result |
|---|---|
| `ruby -c ops/manual-ledger.rb` | PASS: syntax OK |
| `ruby -w tests/ledger/manual_ledger_test.rb` | PASS: 8 runs, 33 assertions, 0 failures/errors/skips, no warnings |
| Pricing calculator regression | PASS: 8 runs, 28 assertions |
| Upstream validator regression | PASS: 13 runs, 30 assertions |
| Example ledger verification | PASS: 4 events, continuous SHA-256 chain |
| Example JSON summary parse | PASS: simulation mode, CNY 72 payment, USD 10 expected credit, zero variance, no unreconciled order |
| Request preview parse | PASS: correct POST path, add 10 USD, no auth/API header |
| Temporary `init` command | PASS: simulation ledger created with mode 0600 and verified |
| Local ledger/event/ops-data ignore checks | PASS |
| `bash tests/infra/validate-baseline.sh` | PASS |
| Controlled artifact secret scan | PASS |
| Markdown fence scan | PASS |
| Project Compose container check | PASS: no active project containers |

The verification ran locally on 2026-07-15. No payment, user balance change, Admin API request, upstream request, purchase, or account login occurred.

## TDD Evidence

- Initial test run failed with `LoadError` because `ops/manual-ledger.rb` did not exist.
- The first implementation run exposed Ruby 2.6 incompatibility with `Array#filter_map`; the summary code was changed to a Ruby 2.6-compatible accumulation and all tests then passed.
- Tests cover chain creation, tamper detection, zero and nonzero variance, duplicate idempotency rejection, simulation-status enforcement, request preview, and credential-field/value rejection.

## Example Reconciliation

- `payment_received`: CNY 72 at `0.1388888889 USD/CNY` = USD 10 expected balance.
- `balance_adjustment`: simulated `add` USD 10 with one idempotency key.
- `usage_snapshot`: fictional USD 1.5 site usage, CNY 8 upstream cost, 12 requests.
- Payment-credit variance: USD 0; unreconciled orders: none.

These are simulation fixtures and do not prove money or balance movement.

## Review Findings

- Payment and balance adjustment are separate events, matching Sub2API's recommended payment-success/recharge-success separation.
- The preview matches `POST /api/v1/admin/users/:id/balance` with a positive amount and `add/subtract`; routine `set` is unavailable.
- The preview omits Admin API authentication by design and has no send capability.
- A simulation ledger cannot contain a `succeeded` adjustment.
- Existing events cannot be silently corrected without breaking sequence/hash verification; corrections require new events.
- Real write paths are ignored, `init` refuses overwrite, and files are created mode 0600.
- D13 remains intact.

## Not Verified

- An external payment, settlement reference, payer identity, chargeback, or refund.
- A real Sub2API Admin API request, idempotency replay, user balance, balance history, or usage deduction.
- Real UP01 cost, daily usage, three-way reconciliation, or profit.
- D05 manual recharge rules, D12 commercial-use judgment, customer disclosures, or legal refund obligations.

## Follow-up

Prepare the L1-6 subscription-account procurement comparison, safe ownership checklist, Sub2API-supported authorization mapping, and one-account live acceptance plan. Do not log in to a seller, buy an account, or handle credentials.
