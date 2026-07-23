# Decision: D04 Internal Test Policy And Neko Capacity

## Status

Accepted by the user on 2026-07-19. This decision overlays the production D03 route; it does not change the public production `GPT` group's `0.15x` multiplier.

The `GPT-内测` Sub2API group was created and verified on 2026-07-19 with OpenAI platform, `1.0x` multiplier, exclusive visibility, the Neko production account as its sole bound account, and per-user RPM `3`. A same-day read-only console recheck confirmed one total/one available account in this group; Aliu is not bound to it and remains paused. The production Neko account remains at concurrency `3`.

On 2026-07-19 the user explicitly rejected daily manual administration. This is a side business with little daytime operator availability, so D04 must automate the registration cap, cumulative check-in grants, fixed referral rewards, budget reconciliation, reports, and alerts through an independent service and Sub2API Admin API. Manual actions are reserved for the initial invitation bootstrap, final closure, and exception handling.

## Internal test policy

- Maximum registered internal-test users: `15`.
- Internal-test group multiplier: `1.0x`.
- Each Shanghai calendar day, a checked-in user receives a fixed `$20` internal balance allowance.
- Check-in allowances accumulate and do not expire at Shanghai midnight; they remain available until internal-test closure.
- A successful referred user's first billable request grants the referrer a fixed `$5` reward.
- Each referred user can trigger that `$5` reward once; one referrer can receive multiple `$5` rewards for different referred users, with no personal count limit.
- Check-in and referral grants enter the same cumulative Sub2API balance. Their source is recorded for audit and idempotency, but consumption is not split by source and neither grant type expires during the internal test.
- Signup alone does not grant referral credit; duplicate, self-referred, and obviously repeated identities do not qualify.

The grant is an allowance, not an immediate upstream cost. D04 uses one total budget rather than a daily budget. Actual Neko cost is driven by successful usage:

```text
actual provider cost = actual internal-test debit × 0.07
budget occupancy = actual provider cost + current D04 balances × 0.07 + pending referral commitments × $5 × 0.07
```

The service reserves `$5 × 0.07` when a referred registration succeeds. At the configured total budget it stops new check-ins, invitation generation, and registrations, while existing balances remain valid and already-reserved referral rewards can still be paid after first successful usage. If one referrer has `n` different referred users who each make a first successful request, that referrer receives `n × $5`.

## Capacity evidence

The dated capacity report records a temporary `$0.50` Neko key bound to the Pro pool. The key was deleted after testing and no production key or route was changed.

- Synchronous concurrency: 1 through 50 concurrent short requests all returned HTTP 200.
- SSE concurrency: 3, 5, and 10 concurrent streams all returned HTTP 200 and terminated with `[DONE]`.
- RPM windows: 60, 120, and 180 RPM for one minute all returned 200; 240 RPM returned 239/240 successes and one timeout, with no 429.
- Conclusion: observed stable evidence is at least 50 synchronous concurrent requests, 10 SSE concurrent requests, and 180 RPM. These are observations, not provider hard limits or an SLA.

## Production configuration policy

- Keep the current production Neko account at concurrency `3` until the internal group and real users exist; this is already well below the observed capacity.
- Keep account concurrency `3` for the first internal-test users, with per-user concurrency `1` and group RPM `3` per user. The Sub2API group RPM field is per-user, not a whole-group aggregate.
- Heavy users receive a separate group or user override only after measured usage and error/latency review. Start at concurrency `2` and RPM `6` for that user; do not raise the shared Neko account above `6` without a new capacity sample.
- Review capacity after 3 consecutive days where the rolling P95 latency and error rate remain stable. Only then raise in steps (`3` to `6`, then `8`, then `10`) and keep each new value below the latest stable SSE observation.
- Do not advertise RPM or concurrency externally.
- Reuse Sub2API's native one-time invitation codes, affiliate relationship binding, user JWT validation, usage/balance reads, and idempotent Admin balance endpoint. After the initial bootstrap, every authenticated internal-test user can generate invitation codes. Multiple unused codes may coexist; the registration gateway atomically enforces `registered_count < N` and returns `409 INTERNAL_TEST_FULL` at capacity. Invitation codes have no time expiry while D04 is open; closing D04 disables invitation registration and invalidates unused codes. Its native percentage-of-recharge affiliate rebate must remain disabled because D04 uses fixed `$5` rewards after first billable usage.
- Do not modify the Sub2API database directly or maintain a private Sub2API fork for D04. The independent automation service owns its own state and stops writes on reconciliation failure.

## Sub2API v0.1.161 limit dimensions

Request-count RPM is resolved in this order:

1. A group-specific RPM override for one user (`专属 RPM`).
2. The group's default RPM, applied independently to each user in that group.
3. The user's RPM field, used only when the selected group has no RPM configured.

The system-level default user RPM only initializes newly created users; it is not a runtime site-wide limit. A value of `0` means unlimited at that layer.

Other controls are separate:

- User concurrency limits the user's simultaneous in-flight requests across their Keys.
- Account concurrency protects one upstream account/Key and is shared by every group bound to that account.
- API Key `速率限制` is a spending-window control (5-hour, daily, and 7-day USD limits), not request RPM. The Key form has no separate request-count RPM or concurrency setting.
- Upstream supplier RPM/TPM remains an external capacity fact. Sub2API v0.1.161 does not expose a native upstream-account RPM field, so supplier protection uses account concurrency, conservative user/group RPM, monitoring, and provider-side limits.

## Review triggers

- Any 429, repeated timeout, SSE termination failure, or material P95 latency increase.
- Internal-test traffic reaching 10 active users or total-budget occupancy crossing its configured review threshold.
- Neko model, price, multiplier, or account policy change.
- A new production account or second upstream becomes available.

## Commercial scaling overlay

When traffic grows, add a second Neko Key as a second Sub2API account object and bind it to the same Neko/GPT logical group. Keep the same `0.07` account cost multiplier and do not assume capacity doubles until a new isolated sync/SSE/RPM probe passes. Different suppliers should remain separate internal channel sets with separate cost evidence. See `docs/superpowers/reports/2026-07-19-gateway-scaling-practices.md`.
## Implementation status (2026-07-19)

The policy is now represented by the local `internal-test-service` baseline. Go/SQLite ledger, Sub2API idempotent client, registration gate, cumulative credits, budget projection, JWT routes, scheduler, reports, alerts, Compose and Caddy contracts are implemented and tested. The default deployment remains `D04_MODE=read_only`; real credentials, production balance writes, domain/TLS cutover, and isolated low-credit acceptance remain gated and are not implied by the local test results.
