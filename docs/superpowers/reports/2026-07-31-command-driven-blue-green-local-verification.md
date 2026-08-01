# Command-Driven Sub2API Blue-Green Local Verification

**Date:** 2026-07-31

**Status:** `准备完成（待生产部署与验收）`

**Scope:** local implementation, rendered Compose/Caddy validation, and fake-command release rehearsal only

## Result

The isolated rehearsal topology now uses Compose project `sub2api-blue-green-rehearsal`, disposable PostgreSQL/Redis/Sub2API bind storage, blue and green API-only slots, exactly one worker role, and only a localhost Caddy port. The host-executor harness completed blue-to-green and green-to-blue promotion and proved candidate failure isolation, Caddy reload recovery, public-acceptance cutback, unchanged shared-service identities, previous-slot retention, one worker update, and fixture elapsed time below 1800 seconds.

No production host was contacted. No image was built or pushed to a production registry. No branch was pushed, no production deployment was started, and no live production acceptance was performed.

## TDD Evidence

### Initial two-slot RED

Before changing `infra/compose.sub2api-rehearsal.yaml`:

| Command | Exit | Duration | Observed failure |
|---|---:|---:|---|
| `bash tests/operations/sub2api_blue_green_topology_test.sh` | 1 | 0.20s | `FAIL: rehearsal Compose project name must be isolated` |
| `bash tests/operations/deploy_sub2api_blue_green_host_test.sh` | 1 | 0.32s | `FAIL: isolated two-slot rehearsal topology is not ready` |

The first topology attempt initially stopped on the legacy `REHEARSAL_ROLLBACK_IMAGE` interpolation requirement. The test fixture was corrected without changing implementation, then rerun to produce the behavior-specific RED above.

### Caddy placeholder regression RED

The real Caddy adapter check exposed Compose consuming Caddy `{$VAR}` placeholders. A regression assertion was added first:

| Command | Exit | Duration | Observed failure |
|---|---:|---:|---|
| `bash tests/operations/sub2api_blue_green_topology_test.sh` | 1 | 0.26s | `FAIL: rehearsal Caddy must preserve its public-acceptance failure placeholder` |

Escaping Compose dollars preserved the Caddy runtime placeholders. The GREEN test passes both `REHEARSAL_FAIL_PUBLIC_ACCEPTANCE=false` and `true` through the pinned real Caddy adapter.

## Implementation Summary

- Replaced the legacy single-app rehearsal file with permanent blue, green, and worker roles while retaining isolated bind storage and `127.0.0.1:18080` as the only published endpoint.
- Added a rehearsal-only `REHEARSAL_FAIL_PUBLIC_ACCEPTANCE` hook. It is validated as `true|false`, defaults to `false`, and can force `/health` to return 503 without modifying production code.
- Extended the rendered-topology contract to reject production/non-fixture bind paths, non-localhost published ports, duplicate workers, legacy single-slot service names, or missing Caddy runtime placeholders.
- Extended the fake host rehearsal across both slot directions and the candidate-health, reload, and public-acceptance failure paths. The test checks no previous slot stop/down operation, one worker replacement, repeated PostgreSQL/Redis/Caddy identity resolution, and elapsed time under 1800 seconds.
- Published the production operator runbook, including authorization boundaries, first-topology bootstrap gate, all host-executor downtime reason codes, 30-minute timeline, cutback/recovery, shared identity proof, and root-owned state/record retention.
- Updated durable project state to `准备完成（待生产部署与验收）` and explicitly recorded that production transport and validation did not occur.

## Final Verification Matrix

The following commands were run against the final Task 6 tree. Independent commands ran concurrently; durations are observed wall-clock times and therefore include local resource contention.

| Command | Exit | Duration | Result |
|---|---:|---:|---|
| `go -C upstream/sub2api/backend test ./... -count=1` | 0 | 151.40s | PASS |
| `go -C upstream/sub2api/backend vet ./...` | 0 | 2.49s | PASS |
| `go -C sub2api-updater test ./... -count=1` | 0 | 7.08s | PASS |
| `go -C sub2api-updater vet ./...` | 0 | 0.44s | PASS |
| `go -C relay-ops-service test ./... -count=1` | 0 | 59.22s | PASS |
| `go -C relay-ops-service vet ./...` | 0 | 0.65s | PASS |
| `bash tests/operations/sub2api_blue_green_topology_test.sh` | 0 | 4.33s | PASS; isolated topology and real Caddy adapter checks passed |
| `bash tests/operations/deploy_sub2api_blue_green_host_test.sh` | 0 | 184.29s | PASS; two-slot fixture reported 15s, below 1800s |
| `bash tests/operations/release_sub2api_blue_green_test.sh` | 0 | 65.08s | PASS |
| `bash tests/operations/deploy_sub2api_release_test.sh` | 0 | 126.57s | PASS |
| `bash tests/operations/update_sub2api_host_test.sh` | 0 | 107.92s | PASS |
| `ruby -Itest tests/operations/sub2api_release_workflow_test.rb` | 0 | 0.37s | 9 runs, 135 assertions, 0 failures/errors |
| `ruby -Itest tests/operations/publish_sub2api_candidate_test.rb` | 0 | 50.24s | 5 runs, 30 assertions, 0 failures/errors |
| `bash upstream/sub2api/deploy/test-caddyfile-cache.sh` | 0 | 0.09s | PASS |
| `git diff --check` | 0 | 0.04s | PASS |

## Unverified Production Assumptions

- The production forced-command account, executor installation, canonical paths, file ownership/modes, SSH host key, and immutable network probe allowlist have not been inspected in this loop.
- Current production is believed to remain the legacy single-application topology. Therefore the first ordinary production attempt is expected to return `legacy_topology_bootstrap`; actual production topology/state was not queried.
- Production disk, memory, PostgreSQL connection headroom, Docker context/project/network identity, and PostgreSQL/Redis/Caddy container IDs were not measured.
- Registry authentication, Linux AMD64 push throughput, production SSH latency, public DNS/TLS behavior, real API keys, and the complete 1800-second live timeline remain unverified.
- Application/data compatibility across the first legacy-to-blue/green topology bootstrap remains a maintenance authorization gate. No downtime path is implemented or authorized by the ordinary command.

## Why This Is Still Pending Production Acceptance

Project completion requires all three conditions: pushed to the server, deployed to production, and verified effective through a real production release. This implementation loop intentionally satisfied none of those production conditions. It produced only local code, isolated/fake rehearsal evidence, documentation, and offline verification. The initiative must therefore remain `准备完成（待生产部署与验收）`, not `已完成`, until a later explicit `部署生产` instruction is executed and accepted; any `downtime_required=true` result additionally requires the separate phrase `允许停机部署` before a maintenance procedure can be considered.
