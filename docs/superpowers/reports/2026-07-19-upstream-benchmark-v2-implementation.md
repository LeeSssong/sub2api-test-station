# Upstream Benchmark V2 Implementation

**Date:** 2026-07-19  
**Scope:** Deterministic evaluator, pricing advisor, proposal output, and approval-gated Skill workflow

## Implemented

- Added `ops/upstream-benchmark-v2.rb` while preserving the V1 CLI and ledger format.
- Added `mvp-text-v2` with bounded concurrency `1,2,3,5,8,10`, RPM `6,12,20,30`, a 10-second RPM window, and short output limits.
- Added full model catalog classification for text, image, audio, realtime, and unknown models.
- Added one synchronous and one SSE test per discovered text model, including usage, TTFT, total duration, terminal-event, and error summaries.
- Added bounded concurrency/RPM probing, rate-limit stop conditions, and latency-based queueing detection.
- Added price evidence validation, unknown-price blocking, actual multiplier calculation, internal multiplier `1.0`, and 50% fully-loaded margin commercial advice.
- Added secret-free JSON/Markdown/YAML proposal generation with `requested` billing, restricted models, model mapping, prices, capacity recommendations, and SHA-256 proposal hash.
- Added V2 CLI commands `validate`, `run`, `advise`, and `proposal`; dry-run never sends network traffic.
- Upgraded the personal `benchmark-upstream-channel` Skill with browser credential boundaries, V2 workflow, explicit `采纳` approval, snapshot/rollback, production verification, and cleanup.

## Verification Scope

The implementation tests use local scripted clients and example YAML only. They do not contact Neko, Aliu, Sub2API production, or any supplier. A real V2 evaluation still requires a temporary supplier Key created in the signed-in supplier page and a separate user approval before configuration changes.

## Remaining Runtime Unknowns

- Supplier browser pages may not expose a safe direct API credential bridge; in that case direct evidence remains `unknown` and isolated gateway evidence is kept separate.
- Price, actual charge, billed Token, RPM/TPM, resale terms, mainland network evidence, and long-duration reliability remain provider-specific runtime evidence.
- V2 does not automatically test image, audio, realtime, or unknown model protocols.

## Verification Evidence

- Full Ruby suite: `115 runs / 451 assertions / 0 failures / 0 errors / 0 skips`.
- V1 registry validation: `2 channels / 14 runs / 5 decisions`.
- V2 example validation: passed.
- V2 dry-run: `network_sent: false`, bounded concurrency/RPM estimate emitted.
- Ruby syntax checks for V1 and V2: both `Syntax OK`.
- Skill metadata validation with the bundled Python runtime: `Skill is valid!`.
- V2-scoped credential-pattern scan: no Bearer or `sk-` shaped values found.
