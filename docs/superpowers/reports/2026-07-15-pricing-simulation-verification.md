# Verification: MVP Pricing Simulation

## Scope

Verify the offline cost formula, CNY/USD conversion, upward rounding, fully loaded margin, Sub2API channel field mapping, fictional scenario, operator guide, and regressions against the existing UP01 and infrastructure baselines.

## Commands and Results

| Check | Result |
|---|---|
| `ruby -c ops/calculate-pricing.rb` | PASS: syntax OK |
| `ruby -w tests/pricing/pricing_calculator_test.rb` | PASS: 8 runs, 28 assertions, 0 failures/errors/skips, no warnings |
| `ruby -w tests/upstreams/validate_upstream_test.rb` | PASS: 13 runs, 30 assertions, 0 failures/errors/skips |
| Markdown CLI output | PASS: input 1.54, output 5.74, fully loaded margin 25.21% |
| JSON CLI output parse and assertions | PASS: fictional status, `requested` billing source, restricted models, 1.0 multipliers, margin floor, exact input price |
| `git check-ignore ... config/pricing/MVP.local.yaml` | PASS: real local scenario path ignored |
| `bash tests/infra/validate-baseline.sh` | PASS: infrastructure baseline contracts |
| Controlled artifact secret scan | PASS |
| Markdown fence scan | PASS |
| Project Compose container check | PASS: no active project containers |

The verification bundle ran locally on 2026-07-15. It did not call UP01, edit Sub2API, adjust balances, configure payment, buy anything, or receive money.

## Worked Example Evidence

Fictional inputs:

- CNY 1.00 input and CNY 4.00 output per million tokens.
- 80 million input and 20 million output tokens per month.
- CNY 10 monthly fixed cost, 5% compensation reserve, 0% payment fee, and 25% target margin.

Verified outputs:

- Fixed allocation: CNY 0.10 per million tokens.
- Public price: CNY 1.54 input and CNY 5.74 output per million tokens.
- Forecast revenue: CNY 238; upstream cost: CNY 160; reserve: CNY 8; fixed cost: CNY 10; profit: CNY 60.
- Fully loaded margin: 25.210084% after upward rounding.
- At 7.2 CNY/USD, recharge multiplier is `0.1388888889 USD/CNY`; channel prices are emitted as USD/token.

All values above are a formula fixture, not a price recommendation for any real model.

## Review Findings

- Payment fee is applied as a fraction of revenue in the denominator, not incorrectly added as a fraction of cost.
- Fixed cost is recovered across the complete forecast token volume and is not double-counted.
- USD upstream prices are converted to CNY before reserve, fixed allocation, and margin.
- Nonzero usage with an unknown category price is rejected.
- Channel pricing uses `billing_model_source: requested`, `restrict_models: true`, explicit public-to-upstream mapping, and exact model prices.
- Group and account multipliers remain 1.0 so exact prices are not multiplied twice.
- Scenario and output carry `fictional`; the guide keeps D03 open.

## Not Verified

- Real UP01 prices, currency, cache billing, usage mix, charge discrepancies, or resale permission.
- Actual server/domain cost, payment fee, CNY/USD operating rate, compensation rate, or customer demand.
- Sub2API channel creation, billing snapshots, user balance deductions, CNY recharge, or payment callbacks.
- D03 public price and target margin approval.

## Follow-up

Build the L1-5 artificial recharge ledger and request-level reconciliation simulation. Use USD as the Sub2API balance unit, keep customer payment in CNY metadata, and do not accept a real payment.
