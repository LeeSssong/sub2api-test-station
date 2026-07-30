# Monitor V2 Long-Window Buckets Production Verification

Date: 2026-07-30

## Outcome

Monitor V2 long-window requests are fixed and promoted in production.

- `7d`: HTTP 200, one public group, 28 timeline points.
- `30d`: HTTP 200, one public group, 30 timeline points.
- Public `/health`: HTTP 200.
- Admin system version: `0.1.168`.
- Recent Monitor V2 HTTP 500 log count: 0.

## Source

- Repository bucket fix: `49473d8201bda0a8f11bf6cce035d2be93612ca4`
- Inclusive boundary fix: `0ae56f0c8c7b16fc2cb4f712c93d7b4a057fcb3c`
- Upstream version: `0.1.168`
- Upstream commit: `99c8e4bf7564823bafbab369acab6539e734c1bb`

The repository now accepts 21,600-second and 86,400-second trend buckets.
Snapshot assembly removes only a single inclusive oldest boundary bucket when
the merged result is exactly one point above the calculated window capacity.
Grossly oversized timelines still fail the existing 64-point contract guard.

## Qualification

The final source tree passed:

```text
go test ./... -count=1
go vet ./...
go test -count=1 -run TestMonitorV2SnapshotKeepsLatestExpectedTimelineBuckets -v ./internal/service
go test -count=1 -run TestMonitorV2SnapshotBoundsModelsTimelineAndStrings/timeline -v ./internal/service
go test -count=1 -run TestOpsRepositoryPreservesLongTrendBuckets -v ./internal/repository
git diff --check
```

The final qualified image passed Linux AMD64 platform, four-label, archive
checksum, and isolated read-only `--version` verification:

- Image ID:
  `sha256:8c94a6b9283250a5b3c9ac114ae0f14204dd9ba51e12f56fdc65e35b087bef9d`
- Source label:
  `0ae56f0c8c7b16fc2cb4f712c93d7b4a057fcb3c`
- Uncompressed archive SHA-256:
  `531ef8e580cc1a44a60eabe3d4b2e958ef9834b1d3a6fe2f96f2e355e89e10bd`
- Compressed transfer SHA-256:
  `8252e3e9ec6f679bfd9246504e3078d1789a85ae6aa7003afcf288bbe5c8db92`

Production independently verified the compressed SHA-256, image ID, platform,
labels, and isolated version before changing the running service.

## Production Promotion

The root host updater returned `result=promoted` for operation:

```text
monitor-v2-boundary-0ae56f0c8
```

Its retained release record is:

```text
/opt/sub2api/production/release-records/host-updater/20260730T053833Z-monitor-v2-boundary-0ae56f0c8.json
```

The record reports `storage_identity`, `backup`, `health`, and `smoke` as
`true`. The verified backup is retained at:

```text
/opt/sub2api/production/backups/release/20260730T053833Z-monitor-v2-boundary-0ae56f0c8
```

Only `sub2api` was recreated. The final application container is healthy with
restart count 0 and runs the qualified image above. PostgreSQL, Redis, Caddy,
and relay-ops retained their exact pre-release container IDs:

```text
postgres  2db52788ad733522b3398f3ba9c0ff4c45a418c360a57424a9e115feb43d4db6
redis     c45202c0d9e64f27d21191e87681c3ccb70e927555b74a4b9a47eb701afaa475
caddy     b4145ae48fbf079bf67e091e8e813db2da453e4ab98fd43ab876cb3b5ff53ca2
relay-ops d4a6802a09d728b805e292b4f3fd040943c36213591387c7a09cf61775db1ed8
```

The rollback tag is retained and resolves to the previous healthy image:

```text
sub2api-host-updater:rollback-monitor-v2-boundary-0ae56f0c8
sha256:4fa162b9a198e481b1fb200c1da82bc689be1ad49d19682d3b99334732281fab
```

## Iteration Note

The first promoted repository fix removed the production 500 and returned a
bounded timeline, but acceptance observed 29 and 31 points because PostgreSQL
includes both window endpoints. The release was immediately iterated through
the same test, image qualification, backup, host-updater, and acceptance gates.
The final deployment satisfies the planned 28-point and 30-point contracts.
