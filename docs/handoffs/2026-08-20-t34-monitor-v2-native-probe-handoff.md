# T34 Handoff

Status: READY_FOR_ROOT_REVIEW
Branch: `codex/t34-native-probe-monitor-v2`

## Delivered

1. Added the read-only `AccountMonitorService.ProjectMonitorV2Groups` boundary with current schedulable-account qualification, deduplication, freshness bounds, and native projection types.
2. Added one batched `account_monitor_results` SQL projection for fixed buckets, latest status, TTFT P50, and average latency; retained 30-day raw results independently from the 7-day legacy aggregates.
3. Rebuilt Monitor V2 service/handler as contract v7 and removed the old performance repository and providers from the Monitor V2 path.
4. Updated the existing frontend API contract test, types, parser, cards, locale labels, and fixtures. Group multiplier is adjacent to the name; flagship/TPS/P95/legacy latency fields are absent.

## Verification

See `docs/superpowers/reports/2026-08-20-t34-monitor-v2-native-probe-verification.md` for commands and results. Backend targeted tests, frontend Monitor V2 tests, typecheck, build, and diff-check passed.

## Correction Pass

- TTFT now renders as `首字速度：` / `First token speed: `.
- Metric states are strictly `available` or `insufficient_data`; `not_provided` and its locale/test fixtures are removed.
- Unused legacy locale keys `ttftP95`, `tps`, `latency`, `latencyP95`, and `baseRate` are removed after consumer search.
- Revalidation passed the Monitor V2 Vitest suite (8 files, 35 tests), `pnpm typecheck`, `pnpm build`, and `git diff --check`.

## Root Action

Please review this candidate and issue `AUTHORIZE_MERGE_TO_MAIN` only after root-level review. This worktree has not been merged, pushed, deployed, or used for production verification.
