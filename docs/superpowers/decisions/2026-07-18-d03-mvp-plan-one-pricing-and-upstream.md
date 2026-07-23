# Decision: D03 MVP Plan One Pricing and Upstream

## Status

Accepted by the user on 2026-07-18 and amended on 2026-07-20. The production customer surface is split into `GPT-Pro` and `GPT-Plus`; Aliu remains unscheduled and retained as a shared rollback candidate. Wawazz Plus balance, Python/Node compatibility, SSE, and `0.05x` billing verification passed after applying an account-level `User-Agent: node` override. Neko Pro has historical `0.10x` gateway/billing evidence, but its current native monitor is again blocked by `INSUFFICIENT_BALANCE`.

The older `0.15x` single-group and Neko `0.07x` values below are historical evidence for the prior production phase. The 2026-07-20 amendment supersedes them for the current routing.

## Context

The MVP is intended to start quickly with a small invite-only user group. Neko completed the standardized direct, gateway, SSE, and three-party billing checks, then passed a production-route smoke after the user explicitly approved the switch. Long-duration reliability, mainland-user network evidence, and commercial resale authorization remain unknown.

Competitors commonly credit CNY recharge amounts into numerically equal USD-denominated internal quota and then apply a visible model/group multiplier. The previous fixed CNY price proposal is therefore not suitable for direct market comparison.

For capacity terminology:

- RPM means requests per minute. It limits request count, not Token count.
- TPM means tokens per minute. It limits total input and output volume.
- Concurrency means requests simultaneously in progress. A long streaming request occupies one concurrency slot for its full duration.

## Decision

### 2026-07-20 amendment: two public customer groups

1. `GPT-Pro` is a public OpenAI group at customer multiplier `1.0x`, bound only to `neko-production-primary`. The account cost multiplier is `0.10x`, concurrency remains `3`.
2. `GPT-Plus` is a public OpenAI group at customer multiplier `1.0x`, bound only to `wawazz-production-primary`. The account cost multiplier is `0.05x`, concurrency is `1`.
3. Both groups expose only the six priced text models already verified in the production pricing channel. The shared pricing channel uses requested-model billing; images, audio, realtime, and internal probe models remain excluded.
4. There is no automatic cross-pool failover. Wawazz passed isolated/formal Plus and post-fix Python sync/SSE checks. GPT-Pro previously passed sync, SSE, and billing reconciliation, but current Neko monitor samples are blocked by insufficient balance and must recover before first-user launch.
5. The temporary `wawazz-test` group, temporary user, and temporary downstream Keys used for this change were deleted after verification.

Use the following as MVP Plan One:

1. NekoAPI Pro is the only scheduled production upstream for invite-only user traffic. The production Sub2API account is `neko-production-primary`, with cost multiplier `0.07` and concurrency `3`.
2. Aliu/UP01 scheduling is disabled. Its account is retained as a paused rollback candidate and does not receive user traffic.
3. The public user group multiplier is `0.15x`. Model base prices use explicit official/upstream standard prices; the customer debit is base price multiplied by `0.15`.
4. Manual recharge uses the market-comparison convention of CNY 1 paid for 1 unit of USD-denominated internal quota. Automatic payment remains disabled.
5. There is no automatic cross-supplier failover. Aliu may be restored only through a separately reviewed rollback decision.
6. Plan One has no active cross-supplier failover. If Neko is unavailable, stop or reject new requests and enter maintenance rather than route traffic to a paused supplier without a separate rollback decision.
7. The public production group remains invite-only and must not advertise an RPM value. The separate D04 internal-test overlay allows up to 15 registered users at `1.0x`, with daily check-in allowances and no production pricing change. Neko account concurrency `3` remains the current configured boundary, not an SLA.
8. Only models with explicit prices and successful controlled verification may be sold. Audio, realtime, and image models require their own billing-class prices and must not inherit a generic text-token price.

Text-model base prices and customer prices are per 1M Token in internal quota:

| Model | Base input | Base cache read | Base cache write | Base output | 0.15x input | 0.15x cache read | 0.15x cache write | 0.15x output |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| GPT-5.6 Sol / `gpt-5.6-sol` | 5.00 | 0.50 | 6.25 | 30.00 | 0.750 | 0.075 | 0.9375 | 4.500 |
| GPT-5.6 Terra | 2.50 | 0.25 | 3.125 | 15.00 | 0.375 | 0.0375 | 0.46875 | 2.250 |
| GPT-5.6 Luna | 1.00 | 0.10 | 1.25 | 6.00 | 0.150 | 0.015 | 0.1875 | 0.900 |
| GPT-5.5 | 5.00 | 0.50 | Unavailable | 30.00 | 0.750 | 0.075 | Unavailable | 4.500 |
| GPT-5.4 | 2.50 | 0.25 | Unavailable | 15.00 | 0.375 | 0.0375 | Unavailable | 2.250 |
| GPT-5.4 Mini | 0.75 | 0.075 | Unavailable | 4.50 | 0.1125 | 0.01125 | Unavailable | 0.675 |
| GPT-5.2 | 1.75 | 0.175 | Unavailable | 14.00 | 0.2625 | 0.02625 | Unavailable | 2.100 |
| GPT-5.2 Pro | 21.00 | Unavailable | Unavailable | 168.00 | 3.150 | Unavailable | Unavailable | 25.200 |

Dated aliases inherit the corresponding base model price only after their mapping is verified. `codex-auto-review` remains internal and is not a public model.

Neko's published GPT-5.6 long-context tier starts above 272,000 input tokens. The configured Neko channel uses `10 / 45 / 6.25 / 0.5` for Sol, `5 / 22.5 / 3.125 / 0.25` for Terra, and `2 / 9 / 1.25 / 0.1` for Luna in that tier (input / output / cache write / cache read, USD per 1M Token). The six production allowlisted models are `gpt-5.6-sol`, `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-5.5`, `gpt-5.4`, and `gpt-5.4-mini`; image and internal probe models are excluded.

Official pricing reference: <https://developers.openai.com/api/docs/pricing>. GPT-5.6 alias reference: <https://developers.openai.com/api/docs/guides/latest-model>.

## Consequences

- Plan One is the fastest path to an invite-only MVP and has a simple price users can compare.
- Neko's standardized billing evidence displayed an effective multiplier of about `0.06895x`, consistent with its published `0.07x` Pro pool multiplier. This is an observed cost basis, not a contractual guarantee for all future traffic.
- A `0.15x` sale price has substantial variable headroom while the observed cost persists, but fixed server cost, retries, compensation, and future payment fees still require measurement.
- A single upstream cannot provide high availability. The MVP may describe measured stability and latency, but must not promise uninterrupted service.
- Long-running requests are never moved between upstreams mid-request. Any later planned switch applies to new requests after notification and a drain window.

## Later Upstream Expansion

New suppliers are a separate follow-up loop and do not block Plan One. Prefer candidates at or below `0.10x`, but price alone is not a qualification result. Before serving users, a candidate must pass model, non-streaming, streaming, billing, latency, effective concurrency, rate-limit, and sustained-availability checks in an isolated test group.

After validation, decide whether to:

- keep the same `0.15x` customer group and switch only new requests;
- create a separate group when quality, capacity, or cost materially differs; or
- retain the supplier as cold fallback only.

## Review Trigger

Review this decision when any of the following occurs:

- Neko actual/standard cost ratio materially increases, the Pro pool changes, or the account balance approaches the operating floor;
- Neko availability, TTFT, long-stream behavior, or rate limits fail the invite-only target;
- a new supplier passes the isolated acceptance checks;
- invite traffic exceeds effective concurrency `1`;
- automatic payment, public registration, or non-text billing classes are enabled.

The 2026-07-19 internal-test policy and capacity evidence are recorded in `docs/superpowers/decisions/2026-07-19-d04-internal-test-policy-and-capacity.md` and `docs/superpowers/reports/2026-07-19-neko-capacity-verification.md`.
