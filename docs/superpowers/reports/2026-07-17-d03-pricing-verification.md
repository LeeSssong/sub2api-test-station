# Verification: D03 Provisional GPT-5.4 Mini Pricing

## Scope

Verify the real non-sensitive UP01 pricing input, the 30M Token/month D03 scenario, volume sensitivity, fixed-cost recovery, fully loaded margin, ignored local-file boundaries, and the absence of production pricing changes.

## Evidence

- Three `gpt-5.4-mini` upstream records were inspected: one administrator test and two downstream E2E requests.
- The two downstream Sub2API records split costs exactly into input, output, and cache read, confirming standard rates of `$0.75/M`, `$4.50/M`, and `$0.075/M` respectively.
- The upstream detail page showed actual stable Plus charges of `$0.000030` for each downstream request, about 4% of the Sub2API standard charges. D03 intentionally uses standard rates.
- Server fixed cost is the paid CNY 199 annual amount amortized over 12 months. No domain or payment fee is included because neither has been incurred.

## Commands And Results

| Check | Result |
|---|---|
| `ruby ops/validate-upstream.rb config/upstreams/UP01.local.yaml` | PASS |
| D03 Markdown calculator output | PASS: 8.70 input, 48.30 output, 1.60 cache read CNY/M |
| 30M forecast | PASS: CNY 160.65 revenue, CNY 41.6017 profit, 25.90% fully loaded margin |
| 10M sensitivity | PASS: 10.20 / 49.80 / 3.10, 25.98% margin |
| 100M sensitivity | PASS: 8.20 / 47.80 / 1.10, 26.23% margin |
| `ruby -w tests/pricing/pricing_calculator_test.rb` | PASS: 8 runs, 28 assertions |
| Local file ignore check | PASS: both real non-sensitive YAML files are ignored |

## Production Mapping After Approval

| Field | Value |
|---|---:|
| `input_price` | 0.0000012083333333 USD/Token |
| `output_price` | 0.0000067083333333 USD/Token |
| `cache_read_price` | 0.0000002222222222 USD/Token |
| `cache_write_price` | Unset; workload not offered |
| `billing_model_source` | `requested` |
| Channel model restriction | Only `gpt-5.4-mini` |
| Group multiplier | 1.0 |
| Account multiplier | 1.0 |

These values have not been applied to production.

## Not Verified

- Supplier resale permission, refund policy, contractual price duration, RPM, TPM, and maximum concurrency.
- Cache-write billing behavior.
- Real customer demand or the assumed 20% input / 5% output / 75% cache-read mix.
- Domain and future payment costs.
- Production deduction after applying the proposed exact channel prices.

## Follow-Up

Obtain user confirmation of the proposed D03 price. After confirmation, configure only `gpt-5.4-mini`, run non-streaming and streaming billing checks, reject an unpriced model, and disable the new test key.
