# Neko Capacity Verification

**Date:** 2026-07-19 Asia/Shanghai  
**Scope:** bounded direct capacity probe for the Neko Pro pool  
**Production route:** unchanged

## Test boundary

A temporary Neko API key with a `$0.50` maximum quota was created in the Pro pool. Requests used `gpt-5.6-sol`, a short prompt, and a maximum output of two tokens. The key was deleted after the probe; the recorded key value, authorization header, and model content were not retained.

## Results

| Probe | Result |
|---|---|
| Sync concurrency 1, 2, 3, 4, 5, 6, 8, 10, 12, 15, 20, 25, 30, 40, 50 | Every request HTTP 200; no 429 or provider rejection |
| SSE concurrency 3, 5, 10 | Every stream HTTP 200, content observed, `[DONE]` received |
| 60 RPM for 60 seconds | 60/60 HTTP 200; P50 1.73s, P95 3.90s |
| 120 RPM for 60 seconds | 120/120 HTTP 200; P50 1.96s, P95 4.73s |
| 180 RPM for 60 seconds | 180/180 HTTP 200; P50 1.72s, P95 3.93s |
| 240 RPM for 60 seconds | 239/240 HTTP 200; one client-side timeout; no 429 |

## Interpretation

The test establishes an observed stable lower bound, not a maximum or an SLA. The first unstable signal appeared at 240 RPM. The production account's current concurrency `3` is conservative relative to the direct result. A future internal group should start at account concurrency `6`, per-user concurrency `1`, and per-user group RPM `3`; heavy users should be isolated with a `6 RPM` and `2` concurrency override only after review.

## Cleanup and verification

- Temporary Neko key deleted.
- Clipboard cleared.
- Production Neko key, Sub2API account, group, pricing, and routing unchanged.
- `ruby ops/upstream-benchmark.rb validate` passed before ledger update.
