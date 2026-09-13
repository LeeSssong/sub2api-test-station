# Unified Quality Account Priority And Daily Cap 50

## Status

User confirmed 2026-09-14: implement A+C with daily cap 50. This specification records that approval and is the implementation contract.

## Problem Evidence

Pro-group ordinary OpenAI text uses `unified_quality`. The monitor card edits `accounts.priority`, labeled「Sub 原生优先级」. Live ranking currently:

1. Reads `account_groups.priority` when a membership row exists, falling back to `accounts.priority` only when it does not.
2. Native bind-group APIs rewrite `account_groups.priority` to membership order `1..N`, so the card value often does not enter ranking.
3. After quality matures, `priority=1` only adds `+20` daily points on a 0–100 quality scale. A healthier `priority=50` account can stay first.

`account_groups.priority` and `accounts.priority` are both Sub-native columns from `001_init.sql`. This task does not add a new fact source.

## Goals

- Keep native priority as a **soft** ranking bonus, not a hard first-key.
- Raise the default daily cap from 20 to **50**.
- Unified quality ranking must use **`accounts.priority`** (the card field).
- Preserve native hard gates, self-owned OAuth layer, cold-start cap 50, quality score, and the 1-hour W5/W55 window.
- Keep scheduler projection on the same ranking rule as live selection.

## Non-Goals

- Do not make `priority=1` a hard first rank (rejected option B).
- Do not change native non-unified selectors (images, Grok, load-balance, `gateway_scheduling.go`).
- Do not change `account_groups.priority` storage or bind-group APIs.
- Do not bypass cooldown, temp unschedulable, overload, quota, runtime block, proxy quarantine, profit gate, or concurrency slots.
- Do not change billing, migrations, production settings, or deploy in this task.
- Do not raise the daily cap to 100: that can let a 0% success account outrank a healthy one.

## Approved Behavior

```text
API-key ranking_score = quality_score
                      + cold_start_priority_signal   // cap 50, zero at confidence >= 0.75
                      + daily_priority_signal        // cap 50, always on
```

```text
signal = ((50 - accounts.priority) / 49) * cap
```

- `accounts.priority` is clamped to `1..100`.
- `priority=50` remains neutral.
- `priority=1` daily contribution is `+50`.
- Cold-start remains `((50 - priority) / 49) * 50 * strength`.
- Self-owned OAuth accounts still rank as an entire layer above all API keys. Inside that layer, sort by `accounts.priority` ASC, then load, then ID.
- Group overrides of `unified_quality_priority_daily_max` still win over the global default when valid.

### Failure and recovery

| State | Ranking |
|---|---|
| Native hard unavailable (cooldown, temp unschedulable, overload, quota, disabled) | Excluded. No `scheduler_rank`. Priority unused. |
| Still eligible, recent success 0% | Quality ~10–30. Daily `+50` cannot cover a ~55–80 gap versus a healthy account. Healthy `priority=50` stays ahead. |
| Short recovery | Account re-enters the pool immediately; 1-hour quality memory keeps it behind until failures roll off. |
| Empty 1-hour window after a long outage | Neutral quality ~50 plus full cold-start and daily bonus; one probe attempt is allowed. Another failure refreshes quality and/or native cooldown. |

## Data And Control Flow

1. Eligibility and runtime blocks unchanged.
2. Unified quality candidate `priority` is `account.Priority`, never the matching `account_groups` row.
3. Apply cold-start and daily signals with caps 50/50 unless a valid group override exists.
4. Sort self-owned, then API-key combined score.
5. Projection `Project()` uses the same `accounts.priority` and caps.

## Compatibility

- No migration.
- Existing `account_groups.priority` rows stay as membership data for native non-unified paths.
- Legacy `openai_advanced_scheduler_weight_priority` remains ignored on the unified quality path.
- Config key `gateway.openai_scheduler.unified_quality_priority_daily_max` default becomes 50.

## Acceptance

| Scenario | Expected |
|---|---|
| Mature API-key `accounts.priority=1` | Daily signal `+50`. |
| `accounts.priority=50` | Daily and cold-start 0. |
| Card `priority=1`, group row `priority=9` | Uses 1. |
| Card `priority=100`, group row `priority=1` | Uses 100. |
| Eligible 0% success `priority=1` vs healthy `priority=50` | Healthy wins. |
| Runtime-blocked `priority=1` | Excluded. |
| Image / non-OpenAI path | Unchanged native selector. |

## Test Strategy

Direct Go tests only: unified quality scheduler, projection, config default, malformed-cap fallback. `go build ./cmd/server`, gofmt, `git diff --check`. No extra regression, no production writes.

## Release

Implementation stays on `feat/unified-quality-account-priority`. No push, merge, or deploy until a later explicit authorization. Rollback is a forward revert after any future merge.

## User Approval

2026-09-14: user confirmed A+C with daily cap 50 after reviewing hard-unavailable, 0% success, and recovery behavior.
