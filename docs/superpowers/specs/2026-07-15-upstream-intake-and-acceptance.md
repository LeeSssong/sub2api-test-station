# Spec: Upstream Intake and Acceptance

## Problem

UP01 exists, but its non-sensitive connection facts, model costs, limits, and commercial terms are not recorded in a form that can be checked before entering credentials into Sub2API. A free-form note would allow missing fields and accidental secret storage.

## Goal

Create a versioned YAML intake format, a local validator, a safe example, and a live-environment acceptance checklist for the first upstream API.

## Non-goals

- Do not store or validate a real API key.
- Do not log in to an upstream, recharge a balance, buy an account, or send a live request.
- Do not import YAML directly into Sub2API.
- Do not claim that supplier limits are enforced by Sub2API when they are only recorded as operating facts.

## Data Boundary

The YAML separates:

1. `connection`: protocol, HTTPS Base URL, exact allowlist host, auth scheme, and a symbolic `secret_ref`.
2. `sub2api`: fields that map to Sub2API v0.1.155 account configuration: platform, account type, account name, group name, priority, concurrency, rate multiplier, and retry settings.
3. `models`: the explicit model whitelist, upstream mapping, capabilities, and per-model cost.
4. `limits`, `rate_limit`, `balance`, and `commercial`: supplier facts used for operations and purchasing decisions.
5. `evidence`: when and how the non-sensitive facts were checked.

Real files use `config/upstreams/*.local.yaml` and stay outside Git. Credentials go only into the Sub2API admin UI or an approved secret store; YAML contains only `secret_ref`.

## Validation Contract

- Parse YAML with aliases disabled.
- Require the complete top-level structure and reject unknown root keys.
- Require an HTTPS Base URL without user information, query string, or fragment.
- Require `connection.allowlist_host` to exactly match the normalized Base URL host.
- Reject credential-shaped keys and likely secret values anywhere in the document, except the symbolic `secret_ref` key.
- Require at least one enabled model, unique public model names, explicit upstream model names, capabilities, and complete pricing keys.
- Require supplier concurrency, RPM, TPM, timeout, daily cost cap, 429 behavior, balance query, minimum top-up, refund, support, and resale-status fields.
- In `--live-ready` mode, additionally require `readiness: ready_for_live_test` and numeric values for the fields needed to bound a real test.

## Acceptance Criteria

- [ ] The repository example passes normal validation and intentionally fails `--live-ready` while marked `draft`.
- [ ] A complete `ready_for_live_test` document passes strict validation.
- [ ] Unsafe URLs, host mismatches, forbidden credential keys, suspected secret values, missing fields, and duplicate models fail with actionable field paths.
- [ ] `*.local.yaml` files are ignored by Git.
- [ ] The human guide distinguishes Sub2API-native fields from supplier facts.
- [ ] The live checklist covers protocol, streaming, error behavior, cancellation, logs, and billing without embedding secrets.

