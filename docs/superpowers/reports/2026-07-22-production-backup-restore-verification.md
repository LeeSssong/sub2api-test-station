# Production Backup And Restore Verification

**Date:** 2026-07-22 (Asia/Shanghai)  
**Result:** `PASS` for server-local logical backup and isolated restore  
**Residual:** encrypted off-site copy and seven-day retention history are not configured

## Scope

The exercise backed up both application databases in the running PostgreSQL 18 cluster:

```text
sub2api: 82 user tables before backup
relay_ops: 17 user tables before backup
```

The production database stayed online. No production table, route, balance, Key, container, or database setting was changed.

## Backup Set

Server-only path:

```text
/opt/sub2api/production/backups/postgres/20260721T205644Z/
```

| Archive | Bytes | SHA-256 |
|---|---:|---|
| `sub2api.dump` | 1,351,660 | `3af32fdae564cedf75199ea95f3fdf8907bd23181a405e79ed6867e37de73ca7` |
| `relay_ops.dump` | 150,761 | `596f1b5afef43f653f201458f944a3657ef38a1990c3a3a03dbff9d80b7e2305` |

The set directory is `0700`; archives, checksums, lists, and result evidence are `0600`, owned by the production operator. Both `sha256sum -c` checks and both `pg_restore --list` reads passed.

## Isolated Restore

The restore used the pinned PostgreSQL 18 image on a newly created internal Docker network and named volume, with no published port and database names different from production. Both archives were restored with:

```text
pg_restore --exit-on-error --no-owner --no-privileges
```

The following production/restored aggregates matched:

```text
sub2api exact per-table row-count canonical hash: match
relay_ops exact per-table row-count canonical hash: match
active administrator count: 1
settings rows: 248
schema migration rows: 228
relay_ops schema tables: 17
```

The complete backup and restore cycle took 10 seconds. The temporary container, network, and volume were removed; the remaining-resource count was `0`.

## Health Recheck

After cleanup, PostgreSQL, Sub2API, Redis, relay-ops, and D04 were healthy. PostgreSQL, Redis, relay-ops, and D04 had restart count `0` and OOM false. Sub2API retained its pre-existing restart count `1` and OOM false; the exercise did not recreate it.

## Boundaries

- The dump files contain production application data and remain only in the restricted server directory.
- No archive or row content was copied to the repository, chat, or ordinary evidence files.
- No isolated Sub2API HTTP instance was started because the stronger database-level exact row-hash and aggregate checks passed and starting another application instance was unnecessary for the D04 opening gate.
- Server-local backup does not protect against complete host loss. Restic/R2 or B2 encrypted off-site retention remains unimplemented and must remain visible as residual risk.
