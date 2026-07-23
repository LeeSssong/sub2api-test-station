# Decision: D03 Provisional GPT-5.4 Mini Pricing

## Status

Superseded on 2026-07-18 by `2026-07-18-d03-mvp-plan-one-pricing-and-upstream.md`. The fixed CNY prices below were never applied to production.

## Context

The first production downstream loop now has three small `gpt-5.4-mini` samples: one administrator account test and two downstream API-key requests. The two downstream records reconcile exactly across request logs, API-key quota, user balance, and upstream records.

The token-cost breakdown confirms the supplier's standard billing rates:

| Category | Standard USD / 1M Token |
|---|---:|
| Input | 0.75 |
| Output | 4.50 |
| Cache read | 0.075 |
| Cache write | Unverified |

The stable Plus group charged about 4% of standard cost for all three samples. This discount is not contractual and is excluded from the public pricing cost base.

## Decision

Use a provisional invite-only price based on standard cost, not the current 4% discounted charge:

| Category | Recommended CNY / 1M Token |
|---|---:|
| Input | 8.70 |
| Output | 48.30 |
| Cache read | 1.60 |
| Cache write | Not offered |

Baseline assumptions:

- CNY 7.20 per USD operating rate.
- 30M Token/month: 20% input, 5% output, 75% cache read.
- CNY 16.5833/month fixed server cost; domain and payment fee remain zero until incurred.
- 10% failure/compensation reserve.
- 25% fully loaded target margin.
- Exact channel prices, group multiplier 1.0, account multiplier 1.0.

Forecast at the baseline mix:

| Metric | Result |
|---|---:|
| Monthly revenue | CNY 160.65 |
| Standard upstream cost | CNY 93.15 |
| Failure reserve | CNY 9.315 |
| Fixed cost | CNY 16.5833 |
| Fully loaded profit | CNY 41.6017 |
| Fully loaded margin | 25.90% |

Using the 30M baseline prices without changing them as volume moves, the service reaches break-even at about 8.55M Token/month, 20% fully loaded margin at about 19.09M, and 25% at about 27.60M. Below 19.09M, keep the service invite-only and treat the server as validation spend rather than claiming a 20% business margin.

If prices are recalculated independently for each volume tier, the same formula yields:

| Monthly Token | Input CNY / 1M | Output CNY / 1M | Cache read CNY / 1M | Fully loaded margin |
|---:|---:|---:|---:|---:|
| 10M | 10.20 | 49.80 | 3.10 | 25.98% |
| 30M | 8.70 | 48.30 | 1.60 | 25.90% |
| 100M | 8.20 | 47.80 | 1.10 | 26.23% |

## Consequences

- If the stable Plus discount remains near 4%, the realized variable margin will be materially higher than this conservative forecast.
- If the supplier removes the discount but keeps standard rates, the 30M baseline still meets the 25% target.
- Public access remains blocked because resale permission, refund policy, supplier capacity, and cache-write pricing are unknown.
- Do not enable models without explicit pricing. Do not expose cache-write workloads until their billing behavior is verified.
- Any domain, payment, support, or additional infrastructure cost must be added before the price is applied.

## Review Trigger

Recalculate immediately if any of the following occurs:

- Standard upstream price changes or actual/standard ratio exceeds 10%.
- CNY/USD operating rate leaves 7.0-7.4.
- Monthly volume stays below 19.09M after the validation period.
- Observed token mix differs by more than 15 percentage points from the baseline.
- Domain or payment cost becomes nonzero.
- A retry, duplicate charge, refund, or supplier interruption consumes more than the 10% reserve.
