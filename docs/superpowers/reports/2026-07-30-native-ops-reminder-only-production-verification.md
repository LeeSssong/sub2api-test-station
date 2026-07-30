# Native Ops Redirect and Reminder-Only Feishu Production Verification

**Date:** 2026-07-30 (Asia/Shanghai)
**Result:** `PASS / PRODUCTION DEPLOYMENT AND PUBLIC AUDIT COMPLETE`

## Scope

This release retires the custom operations backend at `https://api.xingqiaolab.top/ops`
and makes Sub2API's native `/admin/ops` the only operations UI. Feishu remains
an outbound reminder channel only; it does not accept commands, callbacks,
acknowledgements, or state-changing actions.

The production connection metadata used for this work is recorded in the
project-global [CLAUDE.md](/Users/gongtengxinwen/Documents/sub2api搭建/CLAUDE.md)
under "已验证的生产连接". That document is the required source for future
server operations. It records the SSH alias, host, port, user, identity-file
location, production root, and Compose project, but never private-key contents,
passwords, API keys, or `.env` values.

## Deployment Evidence

- Source revision: `785aca6f62557a540dd5b314ed958a36afac0adb`
- Relay image: `sub2api-relay-ops:native-ops-reminder-only-785aca6f6-20260730`
- Image ID: `sha256:70f5c412b9bced6f518f2fedcc0a74c017cf7f2c0093c1c3cc37992ee369c71a`
- Platform: `linux/amd64`
- Transferred archive SHA-256:
  `1f835fc0f87550214ae54f00a1302cb7a49d60012cbb9fae38afe3b24a7e9ce9`
- Release backup:
  `/opt/sub2api/production/backups/release/20260730T122951Z-native-ops-reminder-only-785aca6f6`

The backup contains the original Compose file, Caddyfile, PostgreSQL
custom-format dump, and a SHA-256 manifest. The rollout changed only the Relay
image and edge routing, removed inbound Feishu configuration and mounts,
recreated `relay-ops` and Caddy, and did not recreate Sub2API, PostgreSQL, or
Redis.

## Issues Found During Rollout

1. Replacing the host Caddyfile atomically did not update the file-level bind
   mount used by the running Caddy container. Recreating only Caddy remounted
   the validated file and resolved the stale configuration.
2. After the old Relay proxy routes were removed, retired Relay API paths fell
   through to the Sub2API SPA and returned `200`. An explicit edge `404` rule
   was added for:
   - `GET /relay-ops/api/ops-view`
   - `POST /relay-ops/api/incidents/ack`
   - `POST /relay-ops/api/feishu/events`

The edge rejection fix is in commit `a2617c4c0` and is included in the merged
production revision.

## Verification

The production SSH/Compose preflight was rerun successfully:

```text
SSH_AND_COMPOSE_PREFLIGHT_OK
```

The full public audit was rerun against `https://api.xingqiaolab.top`:

```text
PASS: public link audit
```

The audit confirmed:

- `/ops`, `/ops/`, and `/ops/incidents` return `302` to `/admin/ops`;
- all three retired Relay/Feishu API paths return unauthenticated `404`;
- `/healthz`, `/readyz`, `/pricing`, and `/admin/ops` are available;
- public docs, assets, storefront, and public settings remain valid;
- no retired relay hostname appears in audited responses.

Current protected service identities and state were checked read-only:

- Sub2API container prefix `4c4b5495e71b`, healthy, restart count `0`;
- PostgreSQL container prefix `2db52788ad73`, healthy, restart count `0`;
- Redis container prefix `c45202c0d9e6`, healthy, restart count `0`;
- Relay container prefix `0c632ed82fbe`, healthy, restart count `0`;
- Caddy container prefix `bf0a4547f935`, running, restart count `0`.

Relay inspection confirmed the expected revision and image ID, absence of
retired inbound Feishu environment keys, and no `panic` or `fatal` messages in
recent logs.

No synthetic Feishu alert, acknowledgement, or production write workflow was
triggered during this deployment verification.

## Local Gates

The merged branch passed the routing contract, public-audit static contract,
Relay outbound-only/native-ops contract, Compose structural validation, and
`git diff --check`. The production public audit and read-only server preflight
were rerun after deployment as recorded above.
