# Quality Report Feishu Production Verification

**Date:** 2026-07-22 (Asia/Shanghai)  
**Result:** `PASS`  
**Runtime:** `RELAY_OPS_MODE=read_only`, `RELAY_OPS_FEISHU_COMMAND_MODE=dry_run`

## Scope

The deployment installs the verified quality-report execution path:

```text
fast result -> stored quality report -> stable incident -> deduplicated Interactive Card
```

It does not enable candidate fast jobs. No candidate, paid probe, synthetic event, Feishu test message, route write, price, multiplier, balance, Key, account binding, or deduplication row was created or changed during acceptance.

## Build And Deployment

The restricted non-secret build context was prepared at:

```text
/opt/sub2api/production/builds/quality-report-read-only-20260722-v1
file count: 97
context SHA-256: 919958dfda7a836c3a36909a4dee6b35b794f9f374ac958e4c2685b9b34112e6
```

The production host built the native AMD64 image and confirmed that `quality-first-fast-v1.yaml` is packaged:

```text
image: sub2api-relay-ops:quality-report-read-only-20260722-v1
image ID: sha256:b7977f9cb850d020dba66443a920c186772649edecd12d80023825552dd84b8e
architecture: linux/amd64
```

The old Compose file is retained as `compose.yaml.bak-quality-report-read-only-20260722-v1`. Its SHA-256 matches the pre-state:

```text
before: bd4762fc2846e7f0e12ab1dba7b312ba5db2d772fe7504008bbe92174ab4c776
after:  66c705745df8ba7488f85b3ddcf4ded749dfef68286e476d6ae96975d94a2be8
```

Compose validation passed before the atomic replacement. Only `relay-ops` was recreated with `--no-deps --force-recreate`.

## Read-only Acceptance

The new container is healthy, restart count `0`, and OOM false. `/healthz`, `/readyz`, `/pricing`, `/ops`, and `/monitor` all returned HTTP `200`.

Database state after the startup migration:

```text
candidate upstreams: 0
probe_runs: 0
notification_deliveries: 3
incidents: 4
scheduler_jobs: 2
quality_reports: 0
scheduler keys: daily-report, production-collection
```

The `quality_reports` migration is the expected relay-ops-only schema change. The absence of `candidate-fast:*` jobs proves that the deployed feature did not claim paid quality runs in `read_only` mode. Existing notification and incident counts did not change, so no card or synthetic event was generated.

The Feishu routing file SHA-256 remains:

```text
3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e
```

Sub2API, PostgreSQL, Redis, Caddy, and D04 container IDs exactly match pre-state. D04 remains healthy with restart count `0`, OOM false, `D04_MODE=read_only`, and `D04_REGISTRATION_OPEN=false`. Same-origin registration returned `403 D04_REGISTRATION_CLOSED`. Its local state remains one internal user, one credit grant, and zero usage rows.

## Local Verification

Before deployment, the current source passed:

```text
relay-ops go test ./... -p 1 -race -count=1 against temporary real PostgreSQL
relay-ops go vet ./...
ops.js and ops-admin.js syntax checks
relay-ops deployment contract
```

The deployment contract specifically requires `quality-first-fast-v1.yaml` in the image. The temporary test PostgreSQL was removed after verification.

## Evidence And Rollback

Restricted server evidence:

```text
/opt/sub2api/production/evidence/quality-report-read-only-20260722-v1/pre-state.txt
/opt/sub2api/production/evidence/quality-report-read-only-20260722-v1/post-state.txt
post-state SHA-256: afb8ae8bb695d5ea8a27babf6fa9a6fdea9713d8713f66e9852cf701d756fe7d
```

Rollback restores the retained Compose backup and recreates only `relay-ops`. The old image remains installed. The new empty table is intentionally retained because rollback does not require or justify destructive database cleanup.

The quality-report monitoring and Feishu notification increment is therefore deployed and accepted without enabling any paid or write-capable workflow.
