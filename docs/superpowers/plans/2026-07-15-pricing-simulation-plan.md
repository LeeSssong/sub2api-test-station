# MVP Pricing Simulation Implementation Plan

> **For agentic workers:** Execute inline under the existing user-approved primary-agent workflow. Use test-driven development and preserve D13.

**Goal:** Create a verified offline API pricing calculator and fictional worked example that later accepts real UP01 non-sensitive costs without changing production.

**Architecture:** Reuse the UP01 YAML validator, validate a separate scenario document, calculate with Ruby standard-library decimal arithmetic, and emit structured JSON or a concise Markdown decision report. Map exact recommended prices into Sub2API channel fields instead of approximating with group multipliers.

**Tech Stack:** Ruby 2.6 standard library, BigDecimal, YAML, JSON, Minitest, Markdown.

## Global Constraints

- Do not perform login, payment, recharge, purchase, account creation, API calls, or production configuration.
- All example model costs and forecasts must be labeled fictional.
- Internal Sub2API channel prices are USD per token; public scenario prices are CNY per 1M tokens.
- Target fully loaded margin must be at least 20%.
- Keep `group_rate_multiplier` and exact-pricing `account_rate_multiplier` at 1.0.

---

### Task 1: Define and test the calculator

**Files:**
- Create: `tests/pricing/pricing_calculator_test.rb`
- Create: `ops/calculate-pricing.rb`

**Interfaces:**
- `PricingCalculator.new(upstream:, scenario:).calculate` returns a JSON-serializable result.
- Invalid inputs raise `PricingCalculator::ValidationError` with field-qualified `errors`.
- CLI: `ruby ops/calculate-pricing.rb --upstream FILE --scenario FILE --format markdown|json`.

- [x] Write formula, conversion, margin, field-mapping, and invalid-input tests.
- [x] Run tests and confirm failure because the calculator does not exist.
- [x] Implement the minimum calculator and CLI.
- [x] Run tests in warning mode with zero failures or warnings.

### Task 2: Add fictional inputs and operator guidance

**Files:**
- Create: `config/pricing/MVP.example.yaml`
- Create: `docs/project/pricing-and-billing.md`

- [x] Add a fictional scenario matching the tested worked example.
- [x] Document every formula, currency boundary, Sub2API field, and D03 handoff.
- [x] Document how to replace fictional values after UP01 facts exist.
- [x] Generate Markdown and JSON output without writing production settings.

### Task 3: Persist state and verification

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/superpowers/plans/2026-07-15-commercial-ai-api-relay-implementation-plan.md`
- Create: `docs/superpowers/reports/2026-07-15-pricing-simulation-verification.md`

- [x] Record offline completion while leaving D03 and all live billing checks open.
- [x] Run calculator tests, both output formats, upstream tests, infrastructure regression, secret scan, Markdown scan, and project-container cleanup check.
- [x] Self-review units, formula, rounding, Sub2API mapping, D13, and fictional labeling.

## Verification Commands

- `ruby -w tests/pricing/pricing_calculator_test.rb`
- `ruby -w ops/calculate-pricing.rb --upstream config/upstreams/UP01.example.yaml --scenario config/pricing/MVP.example.yaml --format markdown`
- `ruby -w ops/calculate-pricing.rb --upstream config/upstreams/UP01.example.yaml --scenario config/pricing/MVP.example.yaml --format json`
- `ruby -w tests/upstreams/validate_upstream_test.rb`
- `bash tests/infra/validate-baseline.sh`

## Acceptance

- [x] Fictional output is reproducible and reaches at least 25% example fully loaded margin after rounding.
- [x] Output includes exact Sub2API USD/token channel prices and CNY-to-USD balance multiplier.
- [x] Real UP01 pricing and D03 remain unresolved rather than guessed.
- [x] No external action occurs.

## Risks

- Forecast mix changes fixed-cost allocation and realized margin; rerun after early usage data exists.
- FX changes alter recharge purchasing power; freeze a displayed operating rate and review it before real recharge.
- Sub2API configuration errors can still cause misbilling; live request-by-request reconciliation remains mandatory.
