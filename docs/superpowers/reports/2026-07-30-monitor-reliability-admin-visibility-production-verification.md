# Monitor Reliability And Administrator Visibility Production Verification

**Date:** 2026-07-30 (Asia/Shanghai)
**Status:** deployed and verified

## Scope

This release fixes two production monitor issues:

- account multiplier measurement could report `测算失败` when the upstream
  quota counter updated asynchronously;
- an administrator could see only public groups on the service monitor page,
  instead of every enabled group.

No routing, price, balance, Key, account binding, or user data was changed by
the release.

## Implementation

Multiplier measurement now reads the initial quota once, runs three probes,
aggregates their official reported cost, and then polls for the aggregate quota
change for up to approximately 78 seconds. A missing variance sample is no
longer represented as a false zero.

`GET /api/v1/monitor-v2` now resolves an administrator request with the admin
scope, which includes every enabled group. Ordinary users continue to receive
only public groups. Channel monitor configuration also has an explicit
`group_id` association instead of relying on group-name matching.

Implementation commits:

```text
d7391ab2c fix: harden monitor reliability and group visibility
cd1940a58 fix: extend quota counter polling window
e0752a27b fix: aggregate delayed quota measurements
```

## Local Verification

The deployed source passed:

```text
go test ./... -count=1
go vet ./...
pnpm@10.28.1 install --frozen-lockfile
pnpm test
pnpm build
```

The frontend suite completed with 212 test files and 1428 passing tests.

## Qualified Image

```text
tag             xingqiao-sub2api:monitor-e0752a27b
image ID        sha256:d68bf3fac446433450d7c2b7ee0410f1b6e12151bf19517ac98745b36e139bb9
platform        linux/amd64
version         0.1.168
source label    e0752a27b333501d36eda8bcdd68ff7e31cef33f
qualified       true
```

An isolated `/app/sub2api --version` run succeeded before promotion.

## Production Promotion

The protected host updater completed operation:

```text
monitor-e0752a27b-20260730t0850z
```

Its durable release record is:

```text
release-records/host-updater/20260730T084829Z-monitor-e0752a27b-20260730t0850z.json
```

The record reports state `promoted`. Storage identity, backup, health, and
smoke checks are all true. The verified backup is:

```text
/opt/sub2api/production/backups/release/20260730T084830Z-monitor-e0752a27b-20260730t0850z
```

The previous image remains available through rollback tag:

```text
sub2api-host-updater:rollback-monitor-e0752a27b-20260730t0850z
```

## Production UI Acceptance

The service monitor was opened with the existing administrator session. It
identified the user as `Admin` and displayed all six enabled groups:

```text
GPT-Pro
GPT-Plus
GPT特惠
GPT PRO 20x
Claude Code Max
GPT-PLUS-内测
```

The account monitor displayed seven monitored accounts and no
`测算失败`. Account `Pro20x-SHUAI-0.2 #23` produced a fresh measured result:

```text
0.2512x
额度测得
```

Accounts whose upstream does not support quota probing show `暂无倍率探测`;
accounts using declared values show `上游声明`.

## Fresh Production Health

The final read-only check found:

```text
Sub2API       4c4b5495e71b / healthy / restart 0 / OOM false
Caddy        b4145ae48fbf / running / restart 0 / OOM false
PostgreSQL   2db52788ad73 / healthy / restart 0 / OOM false
Redis        c45202c0d9e6 / healthy / restart 0 / OOM false
relay-ops    d4a6802a09d7 / healthy / restart 0 / OOM false
```

Caddy, PostgreSQL, Redis, and relay-ops retained their pre-deployment
container IDs. Since promotion, Sub2API access logs contained zero HTTP 5xx
responses and zero fatal or panic entries. Fresh `/api/v1/monitor-v2` requests
returned HTTP 200, including observed latencies of 287 ms, 294 ms, and 235 ms.

## Completion

Both reported symptoms are resolved in production. The release is traceable,
has a verified backup and rollback image, and has not replaced the protected
supporting containers.
