# Feishu Production Dry-Run Verification

**Date:** 2026-07-20 (Asia/Shanghai)  
**Result:** `PASS`, production stopped at `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`  
**Scope:** real Feishu group callback, deterministic command worker, read-only Sub2API route validation

The five dry-run command records were present in the current production queue at verification time (19:39:00-19:46:46 Asia/Shanghai). No duplicate command messages were sent while this verification was resumed.

## Safety Boundary

- No `enabled` mode was entered.
- No public group, account binding, schedulable flag, concurrency, multiplier, model list, user, Key, balance, payment or registration was changed during the dry-run.
- No Sub2API, PostgreSQL, Redis or Caddy recreation was performed during this verification. The running `relay-ops` instance was already in `dry_run` when the verification resumed, so it was not needlessly recreated.
- No production secret, API Key, Cookie, JWT, App Secret, verification token or Encrypt Key is recorded here.

## Route Baseline

| Public group | Primary | Backup | Group rate | State during test |
|---|---:|---:|---:|---|
| `GPT-Pro` (`2`) | Neko `7` | Aliu `2` | `1.0x` | `primary` |
| `GPT-Plus` (`6`) | Wawazz `8` | Neko copy `9` | `1.0x` | `primary` |

Neko shared-key capacity remained bounded at configured concurrency `2 + 1 = 3`; account `7` remained schedulable at `2`, and account `9` remained unbound/unschedulable at `1`. Aliu `2` remained unschedulable. Both public groups retained their existing model and pricing configuration.

## Disabled Acceptance

The formal-domain callback `https://api.xingqiaolab.top/relay-ops/api/feishu/events` accepted a real structured `bot` mention. The group command `查询当前分组状态` received the fixed reply `命令功能未启用。结果：rejected` and was recorded with `command_disabled`. The earlier temporary `sslip.io` callback was not used for this acceptance because its DNS/TLS path returned a Feishu delivery error.

## Dry-Run Commands

Each message was sent separately in the real Feishu group with only the rendered leading bot mention.

| Command | Observed result | Meaning |
|---|---|---|
| `切换 GPT-Pro 到灾备` | `succeeded` | Predicted `primary -> backup`; no write |
| `切换 GPT-Plus 到灾备` | `succeeded` | Predicted `primary -> backup`; no write |
| `恢复 GPT-Pro 主分组` | `no_op` | Real state remained `primary` |
| `恢复 GPT-Plus 主分组` | `no_op` | Real state remained `primary` |
| `查询当前分组状态` | `succeeded` | Both groups reported `primary` |

Every reply contained a short audit reference. One deliberate group message with extra text was rejected as `unknown_command` and its fixed help response was delivered. Private-chat events are outside the installed group-mention permission; the automated event tests cover the private-chat rejection boundary, and no private-chat permission was added merely for production testing.

## Zero-Write Evidence

The same redacted Sub2API snapshot shape was used before and after the dry-run. Both the stored evidence files and their normalized canonical JSON were identical:

```text
stored file before/after: 3a3f2abd72e64fd088d31b20971794762152e4bff814ba23e08847975571f8ef
canonical before/after:   225777ef5a2f73b9bcbe276a43a52129a335c894c37dfb269d26c64fec5f18ff
```

The snapshot covers both public groups, all four route accounts, binding/schedulable state, concurrency, account cost multipliers, credential-presence indicators, runtime block fields and sorted model IDs. The redacted snapshots and hashes remain on the server under `/opt/sub2api/production/evidence/feishu-dry-run-20260720/` with restrictive permissions.

Recent relay-ops audit summary: two dry-run switch successes, two restore `no_op` results, two successful status queries in the dry-run window (three including the preceding status query), one unknown-command rejection, all replies delivered on the first attempt, and zero `partial` or route-write errors.

## Runtime Health

- Image: `sub2api-relay-ops:feishu-command-bot-mention-20260720-v1`
- AMD64 image ID: `sha256:5ea34afe054842c8ff32a34358f3315e2c7cc08e822839a372efe36c4852a9f2`
- Final relay-ops container: healthy, restart count `0`
- Final container IDs (short): `sub2api=5fd8adccdb9e`, `postgres=2db52788ad73`, `redis=c45202c0d9e6`, `caddy=7c28088cd9fe`, `relay-ops=20f06294c37e`
- Final modes: `RELAY_OPS_MODE=read_only`, `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`
- `https://api.xingqiaolab.top/healthz`: `200`
- `https://api.xingqiaolab.top/readyz`: `200`
- `https://api.xingqiaolab.top/pricing`: `200`
- `https://api.xingqiaolab.top/ops`: `200`
- Base container IDs for Sub2API, PostgreSQL, Redis and Caddy matched the pre-transition snapshot.

Wawazz's known upstream balance condition remains separate: its native monitor can remain `DEGRADED` with `INSUFFICIENT_BALANCE`; this dry-run neither masked nor changed that condition.

## Remaining Gate

Production remains at `dry_run`. A real route failover or recovery requires a later, separate approval naming the target group and authorizing `enabled`; no such action is included in this verification.
