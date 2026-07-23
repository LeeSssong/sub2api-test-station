# D04 Lightweight Launch Gate Verification

**Date:** 2026-07-22 (Asia/Shanghai)
**Result:** `PASS` for lightweight-gate implementation and read-only production preparation
**Opening decision:** `NO-GO` (approval recorded; current evidence is still blocked)
**Policy:** `D04-LIGHTWEIGHT-LAUNCH-v2`

## Scope

This verification replaces the provider-specific v1 opening workflow with one small, report-only gate for the current active upstream. V1 policy files, tests, and dated reports remain historical evidence only.

V2 requires:

- one explicit launch approval;
- current active-upstream balance of at least USD 10 with financial evidence no older than 20 minutes;
- at least 20 natural-production samples from the current quality window, with fresh success, error, TTFT P95, and total-latency P95 evidence;
- a verified server-local backup containing the complete Sub2API PostgreSQL database and a consistent D04 SQLite snapshot;
- healthy services, matching D04 launch values, assigned operations ownership, and a validated read-only rollback.

V2 does not require provider names, balance-runway days, spend-rate estimates, encrypted off-site backup, retention by days, or recurring restore drills. The evaluator only returns a decision artifact and never opens registration or contacts an external system.

## Implemented Artifacts

- `config/operations/D04-lightweight-launch-readiness-v2.yaml`
- `config/operations/d04-lightweight-launch-snapshot.example.yaml`
- `ops/evaluate-d04-lightweight-launch-readiness.rb`
- `ops/backup-d04-account-data.sh`
- `internal-test-service backup-sqlite SOURCE DESTINATION`
- `infra/compose.d04-launch.yaml`
- `docs/runbooks/operations-and-incident-response.md`
- `docs/superpowers/checklists/2026-07-22-d04-controlled-launch-readiness.md`

The live non-sensitive snapshot is stored only in the Git-ignored local file `config/operations/d04-lightweight-launch-snapshot.local.yaml`.

## Production Preparation Evidence

A pinned AMD64 image was built on the production host but was not deployed:

```text
image=sub2api-internal-test:d04-lightweight-launch-20260722-v2
image_id=sha256:89bd28421f8002091f7d5411ae6da92d058f767db625e77bc65ce958c759a290
architecture=amd64
```

The running D04 container was not recreated. It remained healthy with restart count `0` and retained:

```text
D04_MODE=read_only
D04_REGISTRATION_OPEN=false
RELAY_OPS_MODE=read_only
RELAY_OPS_FEISHU_COMMAND_MODE=dry_run
```

The backup command was installed at `/opt/sub2api/production/ops/backup-d04-account-data.sh`. One verified server-local set was created at:

```text
/opt/sub2api/production/backups/d04-account-data/20260722T015202Z
```

For the approved opening attempt, the same server-local backup command produced a newer verified set at `/opt/sub2api/production/backups/d04-account-data/20260722T033408Z`; no off-site copy or additional retention gate was introduced.

Backup evidence:

| Item | Evidence |
|---|---|
| Directory/file permissions | `0700` / `0600` |
| `sub2api.dump` | 1,411,585 bytes; SHA-256 verified; `pg_restore --list` passed |
| `d04.sqlite` | 90,112 bytes; SHA-256 verified; SQLite `integrity_check=ok` |
| D04 account aggregates | 1 user, 1 grant, 0 usage records |
| Reconciliation | D04 grant USD 20, provider balance USD 20, drift USD 0 |

Production safety checks remained unchanged:

```text
same-origin registration=403 D04_REGISTRATION_CLOSED
healthz=200
readyz=200
ops=200
candidate_count=0
probe_run_count=0
quality_report_count=0
```

Notification count was `4`; the latest row was a natural daily report at `01:00:40Z`, before this task started. No model request, paid probe, synthetic Feishu event, candidate, route write, multiplier change, price change, balance change, Key change, account-binding change, or database mutation was used to satisfy this gate.

Container identities and production configuration hashes remained unchanged:

```text
compose.yaml  66c705745df8ba7488f85b3ddcf4ded749dfef68286e476d6ae96975d94a2be8
Caddyfile     668b274207f7265affa03f4ecc22725db34b30e9d9ae0cc1b7d39b483250b292
D04 compose   ad79d2c5a260c3ac13b417dc7f007545c8c8a2f38e898e553045c60244ac67b0
```

## Fresh Local Verification

The final worktree was verified with:

```text
ruby tests/operations/evaluate_d04_lightweight_launch_readiness_test.rb
  9 runs, 91 assertions, 0 failures
ruby tests/operations/evaluate_d04_launch_readiness_test.rb
  11 runs, 91 assertions, 0 failures
bash tests/operations/backup_d04_account_data_test.sh
  PASS
bash tests/internal_test/validate_internal_test_contract.sh
  PASS
docker compose -f infra/compose.d04-read-only.yaml -f infra/compose.d04-launch.yaml config --quiet
  exit 0
docker run --network none ... golang:1.24.13-bookworm go test ./... -p 1 -race -count=1
  all 14 packages passed
docker run --network none ... golang:1.24.13-bookworm go vet ./...
  exit 0
rg -n -i 'wawazz|neko|xm|aliu' <active-v2-policy-snapshot-evaluator-overlay>
  no matches
git diff --check
  exit 0
```

The first containerized race attempt encountered `unexpected EOF` while downloading `modernc.org/sqlite` from the Go proxy and was not counted as a code result. The complete race and vet gates were rerun successfully with the existing verified module cache, `GOPROXY=off`, and `--network none`.

## Current Decision

At the live-snapshot capture, the quality record was still within the 20-minute freshness window. The decision was `no_go` with four blocking reasons:

```text
launch_not_approved
upstream_balance_below_minimum
upstream_financial_evidence_stale
upstream_samples_insufficient
```

At `2026-07-22T02:28:04Z`, a fresh offline re-evaluation correctly added `upstream_quality_metrics_stale` because the unchanged natural-quality evidence had aged beyond 20 minutes:

```json
{
  "decision": "no_go",
  "blocking_reasons": [
    "launch_not_approved",
    "upstream_balance_below_minimum",
    "upstream_financial_evidence_stale",
    "upstream_quality_metrics_stale",
    "upstream_samples_insufficient"
  ],
  "real_action_executed": false,
  "external_system_contacted": false
}
```

Derived ages at that re-evaluation were `939.08` minutes for financial evidence, `30.84` minutes for quality evidence, and `0.60` hours for the local account backup. The natural 15-minute window contained `0` eligible samples. No traffic was manufactured to change that result.

## Decision And Next Action

The lightweight gate itself is implemented and production preparation is complete. The user approved the actual controlled opening, and that approval is recorded as the single `approvals.launch_approved: true` input in the fresh snapshot. The current evidence still does not authorize opening, so registration remains closed.

## Latest Approved Opening Attempt

At `2026-07-22T03:53:01Z`, a new Git-ignored snapshot recorded the current read-only production state and the user's approval:

```text
snapshot_id=D04-LIGHTWEIGHT-LAUNCH-20260722T035301Z
launch_approved=true
active_upstream.balance_usd=-0.01
active_upstream.financial_recorded_at=2026-07-22T03:53:01Z
active_upstream.quality_recorded_at=2026-07-22T03:52:11Z
active_upstream.sample_count=0
account_backup.archive_created_at=2026-07-22T03:34:08Z
```

The same report-only evaluator returned:

```json
{
  "decision": "no_go",
  "blocking_reasons": [
    "upstream_balance_below_minimum",
    "upstream_samples_insufficient"
  ],
  "real_action_executed": false,
  "external_system_contacted": false
}
```

The server-local backup was refreshed at `/opt/sub2api/production/backups/d04-account-data/20260722T033408Z`. The production operations dashboard refreshed at `03:52:11Z` and showed zero requests, zero Token, zero errors, and no latency/TTFT samples in the selected recent window. Production remained D04 `read_only` with registration closed, relay-ops `read_only + dry_run`, and `/healthz`, `/readyz`, `/ops` all HTTP `200`. No launch overlay was applied, no model traffic or Feishu event was manufactured, and no route, multiplier, price, balance, Key, candidate, probe, or database state was changed.

The evaluator now accepts a negative active-upstream balance as valid evidence and emits the explicit minimum-balance blocker instead of rejecting the snapshot format; this regression is covered by the focused test.

## Latest Read-only Recheck

At `2026-07-22T04:09:19Z`, the approved opening attempt was rechecked without changing production. The D04 container remained `healthy`, restart count `0`, OOM `false`, using `sub2api-internal-test:d04-public-registration-20260721-v1`; relay-ops remained `healthy`, restart count `0`, OOM `false`, using `sub2api-relay-ops:quality-report-read-only-20260722-v1`. Selected runtime modes were still `D04_MODE=read_only`, `D04_REGISTRATION_OPEN=false`, `RELAY_OPS_MODE=read_only`, and `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`. `/healthz`, `/readyz`, and `/ops` returned `200`, while same-origin empty registration returned `403`.

Running the report-only evaluator against the same Git-ignored snapshot again returned:

```json
{
  "decision": "no_go",
  "blocking_reasons": [
    "upstream_balance_below_minimum",
    "upstream_samples_insufficient"
  ],
  "real_action_executed": false,
  "external_system_contacted": false
}
```

The approval is therefore valid but not sufficient to open registration. No launch overlay was applied, and no model request, synthetic Feishu event, route write, multiplier, price, balance, Key, candidate, probe, or database change was made during this recheck.

At `2026-07-22T04:11:27Z`, a further read-only evaluator run against the same snapshot still returned `decision=no_go` with exactly `upstream_balance_below_minimum` and `upstream_samples_insufficient`. Production remained healthy and unchanged; the quality evidence was approaching the 20-minute freshness limit. No synthetic traffic or external action was used to refresh it.

## Final Fresh External-state Audit

At `2026-07-22T04:25:07Z`, authenticated read-only pages were used to refresh both external inputs without submitting any form or creating traffic. The active-upstream dashboard still showed a balance of `-$0.01`. The Sub2API operations dashboard refreshed at `2026-07-22 12:22:32` Asia/Shanghai and showed zero requests, zero Token, zero errors, and no TTFT or total-latency samples in the selected one-hour window.

The Git-ignored snapshot was refreshed as `D04-LIGHTWEIGHT-LAUNCH-20260722T042507Z`. At evaluation time, financial evidence age was approximately `1.43` minutes and quality evidence age was approximately `3.40` minutes. The evaluator returned:

```json
{
  "decision": "no_go",
  "blocking_reasons": [
    "upstream_balance_below_minimum",
    "upstream_samples_insufficient"
  ],
  "real_action_executed": false,
  "external_system_contacted": false
}
```

The same external blockers have now repeated for three consecutive goal turns. All safe work that does not recharge an upstream, manufacture model traffic, or change production has been completed. The opening goal is therefore paused as externally blocked and should resume only after the active-upstream balance and natural-production sample window actually change. Repeated polling is not useful evidence.

The next evaluation should occur only after:

1. the current active-upstream balance is at or above the configured USD 10 minimum and its evidence is fresh;
2. at least 20 natural samples provide fresh success rate, error rate, TTFT P95, and total-latency P95 evidence.

Then rerun the report-only v2 evaluator. Apply the launch overlay only when the same current snapshot returns `decision=go`. The built image is preparation evidence, not evidence that D04 was deployed or registration was opened.
