# Unified Quality Priority Weight Design

## Status

The behavior design was approved by the user on 2026-09-12. This specification is ready for user review before implementation planning. No runtime code, production configuration, database, or deployment is changed by this document.

## Problem Evidence

The Plus group currently routes ordinary OpenAI text requests through the `unified_quality` scheduler. In that path, API-key account priority is only a bounded cold-start signal and becomes zero after the quality confidence reaches the maturity threshold. The legacy scheduler weight override for `priority` is marked ignored for this path.

Production evidence from the Plus group showed that the target account entered the candidate pool but remained behind a higher-quality account. This confirms that the issue is ranking semantics, not only account eligibility.

## Goals

- Give a newly eligible API-key account with a numerically lower priority a strong initial opportunity to receive traffic.
- Keep the Sub-native priority field in the daily `unified_quality` score after quality data matures.
- Preserve quality-based selection as a soft preference: quality may still win when its advantage is materially larger.
- Use the group-scoped `account_groups.priority` value when a group membership exists.
- Make priority contributions observable in scheduler decision logs.

## Non-Goals

- Do not make priority a hard routing rule.
- Do not bypass account eligibility, rate-limit, cooldown, runtime-block, proxy-quarantine, transport, privacy, or concurrency gates.
- Do not change the traditional load/advanced scheduler formula.
- Do not alter account quality measurement, billing, usage logs, or upstream credentials.
- Do not deploy or change production settings as part of the implementation task.

## Approved Behavior

The unified-quality ranking score for API-key accounts will include two priority contributions:

```text
ranking_score = quality_score + cold_start_priority_signal + daily_priority_signal
```

The approved defaults are:

- Cold-start maximum: `50` points.
- Daily maximum: `20` points.
- Priority values are clamped to the supported range `1..100`.
- Lower numeric priority produces a larger signal.
- A priority of `50` produces a neutral signal under the default mapping.

The cold-start signal decays with quality confidence and reaches zero at the existing maturity threshold. The daily signal remains active after maturity, but is bounded so that it remains a soft preference.

For a linear mapping, the maximum cold-start contribution is:

```text
((50 - priority) / 49) * 50 * cold_start_strength
```

The daily contribution is:

```text
((50 - priority) / 49) * 20
```

Both signals are non-negative for priorities `1..50` and non-positive for priorities above `50`, preserving the existing meaning of `50` as neutral. The implementation must preserve finite-value guards and avoid NaN/Inf propagation.

## Data and Control Flow

1. The scheduler loads schedulable accounts for the requested group.
2. Existing eligibility and runtime-block checks remove unavailable accounts.
3. The scheduler builds quality breakdowns and live load information.
4. The scheduler resolves priority using the matching group membership first, falling back to global account priority only when no group row exists.
5. The scheduler calculates cold-start and daily priority contributions.
6. The scheduler orders candidates using the combined score, then existing success-rate, TTFT, and account-ID tie breakers.
7. Existing account-slot acquisition and fresh-account validation remain unchanged.
8. The decision log records the selected account, candidate order, priority used, and priority contributions without storing credentials or request bodies.

## Configuration Contract

The implementation will expose the two caps through the existing scheduler configuration/settings mechanism rather than requiring a code-only constant change:

- `unified_quality_priority_cold_start_max`
- `unified_quality_priority_daily_max`

Defaults are `50` and `20`. Values must be finite and non-negative. The existing group override mechanism will carry group-specific values, with the global values used as fallback for groups without an override.

The legacy `openai_advanced_scheduler_weight_priority` setting must not be silently reinterpreted for unified quality. Its existing legacy-ignore semantics remain documented and unchanged unless a separate compatibility decision is made.

## Observability Contract

For `openai.scheduler_selection`, extend the decision payload with additive fields for:

- `priority_used` or equivalent selected-priority context;
- `cold_start_priority_signal`;
- `daily_priority_signal`;
- optionally, the configured caps when already included in scheduler diagnostics.

Existing fields and algorithm version handling remain backward compatible. The candidate list must continue to contain account IDs only; no account credentials, raw API keys, or request payloads may be logged.

## Failure and Safety Semantics

- An unavailable account cannot become selectable solely because it has priority `1`.
- A fresh-account revalidation failure releases its acquired slot and continues with the existing fallback behavior.
- Invalid configuration fails closed through the existing scheduler/config validation path.
- If the quality snapshot is stale, existing stale-snapshot semantics remain in force; priority does not suppress the stale indicator.
- If a candidate has no quality history, it receives the full cold-start strength but still remains subject to the daily cap and all gates.

## Compatibility and Migration

- No database migration is required for the algorithm itself.
- Existing `accounts.priority` and `account_groups.priority` data remains valid.
- Existing group membership priority remains authoritative for group-scoped requests.
- Existing traditional scheduler behavior is unchanged.
- Existing scheduler logs remain readable; new decision fields are additive.
- A configuration migration is only needed if the existing settings store requires explicit persisted defaults. Otherwise defaults are applied by the runtime config layer.

## Acceptance Matrix

| Scenario | Expected result |
|---|---|
| New API-key account, priority `1`, no quality history | Receives up to `+50` cold-start points and can rank first when otherwise eligible. |
| New API-key account, priority `50`, no quality history | Receives neutral priority contribution. |
| Mature API-key account, priority `1` | Still receives the daily priority contribution, up to `+20`. |
| Mature API-key account with materially better quality | May outrank priority `1` under soft-priority semantics. |
| Group priority differs from global priority | Matching `account_groups.priority` is used. |
| Priority `1` account in cooldown/runtime block | Is excluded and cannot be selected. |
| Traditional scheduler request | Existing priority formula and behavior remain unchanged. |
| Two candidates tie after all signals | Existing stable tie breakers remain deterministic. |
| Invalid cap configuration | Existing validation rejects or safely defaults according to the established settings contract; no invalid score is emitted. |

## Test Strategy

- Unit-test cold-start cap and confidence decay, including the exact `+50` maximum.
- Unit-test daily priority participation after maturity, including priority `1`, `50`, and an above-neutral value.
- Unit-test group-priority precedence over global priority.
- Unit-test quality-over-priority soft behavior and priority-over-quality cold-start behavior.
- Unit-test eligibility gates remain stronger than priority.
- Unit-test additive scheduler decision fields and finite numeric values.
- Run only the directly related Go service/repository/config tests plus the required build and diff checks under the project’s minimal-validation policy.

## Release, Verification, and Rollback

Implementation must occur in an independent task worktree derived from the latest clean `main`, with the task registered in the project ledger by the release controller. No production setting or deployment is changed during implementation.

Before any deployment, the candidate must be merged into a clean, pushed root `main` and pass the direct validation gates. Production deployment requires one of the explicitly authorized release phrases in the acceptance-station constraints. Online verification must confirm the Plus scheduler decision payload, candidate ordering, and actual usage distribution.

Rollback is a forward Git revert or restoration of the previous verified blue-green slot through the existing release chain. If the new priority behavior causes excessive concentration, revert the code/config change; do not manually copy server files or alter production data.

## Implementation Notes

- Confirm the exact existing settings key/handler boundary for the two new caps while preparing the implementation plan; this is an integration detail, not a product decision.
- Capture a baseline scheduler distribution for comparison during online verification.
