# Ops Monitoring and Feishu Dual-Domain Verification

**Date:** 2026-07-23 (Asia/Shanghai)
**Result:** `PASS` (deployed and accepted in production read-only mode)
**Scope:** implementation, production deployment, one systemd-driven account-quality run, authenticated `/ops` acceptance, and zero-scope-change verification for the approved Sub2API-native monitoring projection.

## Verified Design Boundary

- `/monitor` remains the Sub2API-native monitoring page; relay-ops only links to it.
- `/ops` remains a hidden-admin, read-only projection. It now separates native site runtime from scheduled account-quality evidence; the daily digest and 15-minute alert/recovery evaluator use the same projection and existing Interactive Card sender.
- Current upstream membership is discovered only from Sub2API accounts that are `active && schedulable`; no provider, account name, URL, Key, or model response is hard-coded.
- The approved deployment recreated only `relay-ops` and ran the existing account-quality systemd task once. It did not send a synthetic Feishu event, delete deduplication state, create a candidate, enable a paid probe, or change a Sub2API route, account, multiplier, price, balance, Key, D04 setting, or business database row.

## Final Data-Integrity Review

- Account-quality evidence uses an optional multiplier: a numeric value, including a legitimate zero, is known evidence; `null` is unavailable, displays as `未提供`, and cannot establish or change a multiplier-alert baseline.
- The Go reader recomputes the account-quality file's canonical sorted-ID JSON hash, including the empty `[]` case, and rejects inconsistent metadata. `/ops`, the daily digest, and the alert evaluator additionally require that hash to match the current native `active && schedulable` set.
- Native rows with zero requests display error rate and SLA as `未知`; rows with zero successful requests display TTFT as `无成功样本`. Sample-insufficient observations remain visible but cannot become fabricated `0%` or `0ms` metrics.
- Final independent review found no remaining implementation issue. The production acceptance below verifies the same pinned image and collector.

## Local Gates

All local gates completed serially against the final current worktree:

```text
GOMAXPROCS=1 go test ./... -race -count=1   PASS (34 packages)
GOMAXPROCS=2 go vet ./...                    PASS
ruby -Itest tests/operations/collect_account_quality_pulse_test.rb PASS (13 runs, 52 assertions)
node --check internal/http/static/ops.js     PASS
node --check internal/http/static/ops-admin.js PASS
bash tests/relay_ops/validate_relay_ops_contract.sh PASS
git diff --check                             PASS
```

The Go image's login shell removes the Go binary from `PATH`, so the required Go container used `bash -c` with persistent module/build caches. This is an execution-environment detail, not a compiler or test failure.

The contract initially failed because it still asserted the retired `账号池质量` label and the prior asset cache version. The contract now asserts the approved labels `站内运行`、`公开分组`、`当前调度账号`、`上游账号质量` and the `20260723-site-runtime-1` asset version. It then passed without changing runtime behavior.

## Security Review

The changed-file scan covered `api_key`, `base_url`, `Authorization`, `response_text`, and `Cookie` identifiers. Only source-code identifiers in existing client/redaction paths matched. No changed secret/credential file was present, and the tracked diff did not match recognized token-literal patterns. Scan output deliberately suppressed content and values.

## Public Read-Only Check

Fresh unauthenticated HTTPS checks returned:

```text
/healthz  200
/readyz   200
/pricing  200
/ops      200
/monitor  200
/relay-ops/api/ops-view (anonymous) 404
```

The post-deployment native `/monitor` response had this content fingerprint, recorded without retaining its body:

```text
45e85809e0a48d13dce257c16da363c280ec94be933200a25ea1648c1385a765
```

## Server Read-Only Pre-Deployment Check

The first bounded SSH attempt ended during key exchange. A later single retry succeeded, and the following no-write evidence was collected without printing secrets or full Admin API responses:

```text
relay-ops image: sub2api-relay-ops:account-quality-monitor-v1
relay-ops state: running / healthy / restart 0
relay-ops modes: read_only / dry_run
D04 state: running / healthy / restart 0
D04 modes: read_only / registration=false
account-quality timer: enabled / active
retired model-release timer: disabled / inactive
live active+schedulable IDs: [10,11,12,13]
live account-set SHA-256: f6b733f89e799048c92d90dc0d404ce1f96300bf1f2964184cc681bdcc2457e7
quality-result account-set SHA-256: f6b733f89e799048c92d90dc0d404ce1f96300bf1f2964184cc681bdcc2457e7
```

The account IDs were projected inside the server from the Sub2API native Admin account list; the mounted Admin key value and full response were never output.

## Production Deployment and Acceptance

The approved deployment installed:

```text
relay-ops image: sub2api-relay-ops:ops-dual-domain-20260723-v1
image manifest: sha256:9228ee881b257fb21404e951a8c0516efcffa3eb8b5584358308dfbf27e22b10
relay-ops container: 84ad983a6be4
state: running / healthy / restart 0 / OOM false
modes: read_only / dry_run
collector SHA-256: 20452b9218af79fb6ba19895d7f250b6754011a828eaf3e3f77f69faf8e90ea9
backup suffix: bak-ops-dual-domain-20260723-v1-20260723T152445Z
```

The account-quality timer was stopped before replacing the runner, and its one-shot service was confirmed inactive. The collector, Compose image reference, and runner image reference were installed atomically; `docker compose config --quiet` passed. Only `relay-ops` was recreated with `--no-deps --force-recreate`. The timer was then restored to `enabled / active`.

One explicit systemd-runner acceptance completed from `23:27:04` to `23:27:36` CST with `Result=success` and `ExecMainStatus=0`. The new result is `0600`, fresh, schema-valid, and self-consistent:

```text
snapshot: ACCOUNT-QUALITY-20260723T152705Z
result SHA-256: 4677925dbbcee85f4881c155c513bdb8f7fee2460bb851420b545fc3bd181536
active+schedulable IDs: [10,11,12,13]
account-set SHA-256: f6b733f89e799048c92d90dc0d404ce1f96300bf1f2964184cc681bdcc2457e7
account 10: 27/45, TTFT P95 12267.608 ms, multiplier 1.0x, last account_test_error
account 11: 12/45, TTFT P95 11915.447 ms, multiplier 1.0x, last account_test_error
account 12: 0/45, no successful TTFT sample, multiplier 1.0x, last account_test_error
account 13: 30/45, TTFT P95 14277.635 ms, multiplier 1.0x, last account_test_error
```

All four current observations were ordinary `account_test_error`; none was misclassified as `balance_exhausted`, and each account retained its independent history. These results are operational concerns for D04 readiness, not a deployment failure.

Authenticated Chrome acceptance showed the production `/ops` page with `站内运行`, `公开分组`, `当前调度账号`, and `上游账号质量`. It displayed the three public groups and exactly the four live scheduled accounts, explicit zero-sample labels, multiplier, historical success rate, TTFT P50/P95, and the latest result. `/monitor` remained a link to the native page. No form or write action was used.

After deployment, `/healthz`, `/readyz`, `/pricing`, `/ops`, and `/monitor` returned HTTP `200` from both the workstation and server; anonymous `/relay-ops/api/ops-view` returned `404`. Caddy, D04, PostgreSQL, Redis, and Sub2API retained their pre-deployment container IDs. D04 remained healthy at `read_only / registration=false`. The retired model-release timer remains `disabled / inactive`; its already-failed one-shot service result is a pre-existing non-blocking discrepancy and was not restarted.

## Next Mainline

Allow the systemd task to accumulate natural evidence, then proceed to D04 controlled opening preparation using fresh minimum-balance and quality evidence for the dynamic active account set. The current `0/45` account and high TTFT/error observations must be reviewed before any registration-opening action. No further parallel monitoring system or Feishu feature expansion is needed.
