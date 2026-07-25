# Account Monitor Multiplier Production Verification

**Date:** 2026-07-26 (Asia/Shanghai)
**Application release source:** `fdefe539d8e27acfc34c486aced4aa64732ffbb9`
**Result:** deployed and accepted; the post-deploy SSH configuration recheck is pending because the production host closes SSH sessions before key exchange.

## Scope

- The admin-only native Sub2API account-monitor view now projects only accounts that are both `active` and schedulable.
- Every card shows the existing probe-quality and usage-window data, a group filter, global/per-account refresh controls, and a multiplier state.
- A multiplier is either an upstream-declared value, a successful three-sample New API measurement, an explicit measurement failure, or unavailable. Failed and unavailable states never become a numeric multiplier.
- Relay-ops and Feishu consume the same schema-v2 native projection. They remain read-only and do not select, switch, or mutate account routes.
- The homepage copy is `官方价格的0.1——0。3倍`.

## Production Release

The production compose file was backed up before replacement:

```text
/opt/sub2api/production/compose.yaml.pre-account-monitor-multiplier-20260726-v1
```

The source export was created from the application release source above:

```text
/opt/sub2api/releases/20260726-account-monitor-multiplier-v1
archive SHA-256: a543c4cdcf0b307a66b4d7b18cd11a69f6b7f3efd52a425a5a7f89cddc45d1e0
```

Both production services were rebuilt and recreated together. They reached the Docker health state after recreation:

```text
xingqiao-sub2api:account-monitor-multiplier-20260726-v1
sha256:b2f8d7b5c3612a650c9e84f56f5d49262223d7416eabd0a629cfc4c50534140c

sub2api-relay-ops:account-monitor-multiplier-20260726-v1
sha256:7a7c592322eefc95d01dd3744b8ca9d9ed1c8137d01162e99e90970fa7d8ada5
```

The first server build required `FRONTEND_NODE_MAX_OLD_SPACE_SIZE=3072`; the initial 1536 MiB Node heap limit was insufficient for the frontend build. No route, account eligibility, Key, balance, price, or Feishu command mode was changed to complete the release.

## Native Projection Acceptance

The native monitor projection reported `schema_version: 2`, a 300-second global interval, and a non-stale response.

It contained exactly these 11 currently `active + schedulable` accounts:

```text
20, 21, 22, 23, 24, 25, 26, 27, 29, 31, 32
```

Account 28 is paused/non-schedulable and was correctly excluded. Filtering to `GPT-Pro` returned only accounts `21`, `24`, and `25`.

The authenticated production admin page subsequently showed:

- `监控中 11 个账号` and a global interval of `5 分钟`;
- platform, group, and status filters;
- card-level `监控设置`, `立即刷新此账号`, and usage-window data;
- account 21 and 23 as `测算失败`, never as a numeric multiplier;
- account 32 as `暂无倍率探测`;
- the navigation entry `账号监控`, visible in the administrator surface.

One explicit card refresh for account 21 executed a real native probe. Its check timestamp advanced to `2026-07-25T18:22:40.911933Z`, connectivity remained successful, and its multiplier remained `measured/failed`. The failure detail is intentionally not persisted in the public projection.

## Multiplier Outcomes

The safe production values at acceptance were:

| Account | Multiplier state | Displayed value |
| --- | --- | --- |
| 20 | upstream declared | `0.05x` |
| 21 | measured, failed | no numeric value |
| 22 | upstream declared | `0.25x` |
| 23 | measured, failed | no numeric value |
| 24 | upstream declared | `0.16x` |
| 25 | upstream declared | `0.15x` |
| 26 | upstream declared | `0.25x` |
| 27 | upstream declared | `0.25x` |
| 29 | upstream declared | `0.10x` |
| 31 | upstream declared | `0.05x` |
| 32 | unavailable | no numeric value |

Fresh upstream declarations superseded the prior historical `0.07x` display for accounts 20 and 31. The new `0.05x` values are upstream-declared values, not a local estimate.

## Read-Only Boundary

The deployed relay-ops design is fail-closed for untrusted or stale multiplier projection data. Its Feishu recommendation can report that another account is operationally preferable, but it cannot automatically change a route or account selection.

Production acceptance before and during the deployment confirmed the running relay-ops mode as `read_only` and the Feishu command mode as `dry_run`. A later bounded SSH recheck was attempted with the configured production identity; the host accepted TCP but closed the SSH session before key exchange:

```text
Connection closed by 43.133.75.82 port 22
```

That transient access limitation prevented a second direct container/environment inspection. The authenticated `/ops` page still renders `只读状态`, and the native account-monitor page remains accessible with the accepted projection above.

## Local Verification

The following fresh verification completed against the clean exported release tree at `fdefe53`:

```text
upstream/sub2api/backend: go test ./...                        PASS
relay-ops-service: go test ./...                               PASS in the prior release gate
sub2api-updater: go test ./...                                 PASS
internal-test-service: go test ./...                           PASS in the prior release gate
ruby -Itest tests/operations/analyze_account_monitor_test.rb   PASS (3 runs, 10 assertions)
bash tests/relay_ops/validate_relay_ops_contract.sh             PASS
```

The clean frontend export could not perform `pnpm install --frozen-lockfile`: the pre-existing pnpm overrides configuration does not match its lockfile under pnpm 11. The working frontend dependency tree was then used for test execution. A concurrent, uncommitted edit to `UserErrorDetailModal.vue` and its test landed while the first full Vitest run was in progress, producing one mixed-source assertion failure. Re-running that specific test after the edit completed passed 9/9. This report commit does not include those unrelated worktree edits.

## Follow-up

Retry the read-only SSH verification after the host-side connection throttle clears. The expected check is limited to the two deployed image IDs, health/restart state, `RELAY_OPS_MODE=read_only`, `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`, and multiplier-related warning logs; it must not mutate production state.
