# Feishu Shared Aliu Dry-Run Verification

**Date:** 2026-07-20 (Asia/Shanghai)  
**Result:** `PASS` for code, production configuration and zero-write deployment  
**Deferred by design:** group invitation and post-change Feishu message acceptance

## Scope

Aliu account `2` is now the configured backup for both public groups:

| Public group | Current primary | Configured backup | Current state |
| --- | ---: | ---: | --- |
| `GPT-Pro` (`2`) | Neko `7` | Aliu `2` | `primary` |
| `GPT-Plus` (`6`) | Wawazz `8` | Aliu `2` | `primary` |

Neko copy account `9` remains present, unbound, unschedulable and at concurrency `1`. It was not deleted or repurposed. Aliu remains unbound, unschedulable and at total concurrency `1`; both routes reference its existing six required models.

## Implementation

- Configuration now permits an account to be reused only in backup roles.
- Primary account IDs remain unique and cannot appear in any backup role.
- Each switch locks namespaced resources for the public group, primary account and backup account. Keys are sorted and deduplicated before PostgreSQL transaction advisory locks are acquired.
- The existing additive/removal binding helpers preserve a shared account's other `group_ids`.
- Sub2API access remains limited to the `v0.1.161` native Admin API. No Sub2API PostgreSQL write or `confirm_mixed_channel_risk` request was used.

## Local Verification

- Go `1.24.13` full suite: `go test ./... -race -count=1`, all packages passed.
- PostgreSQL-backed `TestFeishuRouteLockSerializesSharedAccount`: passed without being skipped.
- `go vet ./...`: passed.
- `bash tests/relay_ops/validate_relay_ops_contract.sh`: passed.
- TDD RED evidence included the old config rejection and the absent route-lock interface; the new tests then passed after the minimal implementation.

## Production Deployment

- Final modes: `RELAY_OPS_MODE=read_only`, `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`.
- New image: `sub2api-relay-ops:feishu-shared-aliu-20260720-v1`.
- AMD64 image ID: `sha256:70e6fdcddb6bc2be67bae0a7ae011153a643e4ce58f247494ae3026c47c530b4`.
- Final relay-ops container: `1e7194b56bb8`, healthy, restart count `0`.
- Unchanged base container IDs: `sub2api=5fd8adccdb9e`, `postgres=2db52788ad73`, `redis=c45202c0d9e6`, `caddy=7c28088cd9fe`.
- `https://api.xingqiaolab.top/healthz`, `/readyz`, `/pricing` and `/ops`: HTTP `200`.
- Compose and routing backups were stored beside their production files with suffix `.bak-shared-aliu-20260720-v1`.

Only relay-ops was rebuilt and recreated. Sub2API, PostgreSQL, Redis and Caddy were not recreated.

## Zero-Write Evidence

The same sorted, redacted production snapshot was captured before and after deployment. It includes both public groups, accounts `2/7/8/9`, schedulable/binding/runtime state and sorted model IDs.

```text
before: bb12c7da55fbee4d05746bd2e8ed5d10e56c5b8b85e226e3579f7c25689e6275
after:  bb12c7da55fbee4d05746bd2e8ed5d10e56c5b8b85e226e3579f7c25689e6275
```

Evidence is stored under `/opt/sub2api/production/evidence/feishu-shared-aliu-20260720/` with restrictive permissions. Aliu's successful native account test was established in the immediately preceding production review and was not repeated, avoiding an unnecessary upstream request.

## Deferred Acceptance

The `星桥AI运维验收` group currently contains only the owner; the robot is present only as an application card. No invitation was sent and no message draft remained. Post-change group commands are intentionally deferred until the owner finishes the remaining bot configuration and formally adds the robot.

After invitation, keep `dry_run` and send the two exact switch commands plus the status query. A real switch still requires a separate approval naming the group and authorizing `enabled`.

