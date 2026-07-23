# Spec: MVP Pricing Simulation

## Problem

The project needs a repeatable way to convert upstream costs into a launch price, but UP01 prices are not available yet and Sub2API v0.1.155 bills token prices in USD per token while likely customer settlement is CNY.

## Goal

Build a deterministic offline calculator that accepts a validated upstream document plus a scenario, allocates fixed costs, includes failure compensation and payment fees, enforces at least 20% target fully loaded margin, and emits both public CNY-per-million-token prices and exact Sub2API USD-per-token channel fields.

## Non-goals

- Do not invent or fetch UP01 prices.
- Do not modify a live Sub2API instance, user balance, channel, group, payment setting, or upstream account.
- Do not treat fictional scenario output as D03 approval or a public price commitment.
- Do not model subscription-account capacity in this API-upstream calculator.

## Formula

For each supported token category:

```text
fixed allocation per 1M tokens = monthly fixed costs / forecast monthly million tokens
loaded category cost = upstream category cost x (1 + compensation rate) + fixed allocation
minimum public price = loaded category cost / (1 - payment fee rate - target margin rate)
recommended public price = minimum public price rounded upward to the configured increment
Sub2API USD/token price = public CNY/1M price / CNY-per-USD FX / 1,000,000
```

The forecast summary recomputes revenue, upstream cost, compensation, payment fee, fixed cost, and fully loaded margin using rounded prices. Rounding must not reduce margin below the target.

## Input Contract

- Upstream document must pass `UpstreamConfigValidator` and use `per_1m_tokens` prices in CNY or USD.
- Scenario contains a source upstream ID, manual CNY-per-USD rate, target margin, payment fee, compensation rate, fixed-cost items, rounding increment, and per-model monthly input/output/cache token forecast.
- Every forecast model must be an enabled upstream model.
- Nonzero forecast for a category whose upstream price is unknown is rejected.
- `target_fully_loaded_margin_rate` must be at least 0.20, and margin plus payment fee must be below 1.

## Output Contract

- Public CNY prices per 1M tokens for input, output, cache read, and cache write.
- Sub2API channel pricing using `billing_mode: token` and USD-per-token fields.
- Recommended `balance_recharge_multiplier_usd_per_cny = 1 / cny_per_usd` for future CNY payment configuration.
- Group and account cost multipliers remain 1.0 when exact channel pricing is used.
- Forecast profit summary and an explicit `fictional`/`draft` status carried from the scenario.

## Acceptance Criteria

- [ ] The worked example yields CNY 1.54 input and CNY 5.74 output per 1M tokens from the documented assumptions.
- [ ] Rounded forecast fully loaded margin is at least the configured target.
- [ ] USD upstream prices are converted to CNY before margin calculation.
- [ ] Unknown models, insufficient target margin, invalid rate combinations, and unpriced nonzero cache usage fail.
- [ ] Output field names and units match Sub2API v0.1.155 channel and payment settings.
- [ ] No real price, balance, payment, or production setting is changed.

