# Upstream SSE Capacity and Topology Nonfunctional Verification

**Date:** 2026-07-21 (Asia/Shanghai)  
**Implementation result:** `PASS / OFFLINE ONLY`  
**Target topology result:** `NOT_READY / NONFUNCTIONAL_EVIDENCE_INCOMPLETE`

## Scope

This verification covers the vendor-neutral nonfunctional benchmark implementation only. It did not send a live upstream request, create or read a Key, create a candidate, enable a paid probe, deploy a service, change a route, or write production data.

## Implemented Contracts

- Version 3 profiles validate bounded output, timeout, independent sync/SSE concurrency ladders, RPM windows, waves, nearest-rank P50/P95, safe paths, protocol terminal events, and secret-free content.
- Request budgets use the exact formula `D + 2M + W*Csync + W*Csse + R + K` for HTTP and exclude `D` for generation.
- Normalized samples bind channel, role, group, account evidence reference, model, profile ID/hash, measurement location, non-secret run ID, and ISO-8601 recording time while dropping response content and provider error text.
- Sync and SSE capacity use separate barrier-started ladders, actual overlap measurement, fail-fast stop reasons, stable lower bounds, and conservative recommendations. HTTP 200 without an SSE terminal event fails as `stream_incomplete`.
- RPM is a separate paced ladder scheduled against target start times rather than request-completion sleeps, and stops after its first qualifying error.
- Topology scenarios reject reused primary accounts and require repeated backup account references to be declared as one generic `shared_capacity_pool`.
- Shared-pool evaluation binds the complete scenario role identity, requires sync and SSE in isolated/equal-demand/approved-mix phases, proves combined overlap from sample intervals, and reports aggregate/per-member quality and fairness.
- Observation evaluation requires every scenario role in every contiguous approved hourly window and applies success, 429, 5xx, TTFT and SSE-interruption gates. Drill evaluation additionally requires canonical read-after-write route evidence, role-matched backup/primary sync+SSE samples and the approved primary recovery window.
- Explicit execution budgets enforce request, Token, currency and wall-clock ceilings; unknown billing, latency and queue thresholds stop the run. Summaries include error categories, first/last timestamps and non-secret estimated/actual cost totals.
- `capacity-dry-run` and `topology-dry-run` are zero-network commands. No live capacity or route-mutation command was added.

## Deterministic Dry-run Evidence

For the default profile with `M=3`, discovery enabled, and `K=4`:

```text
profile_hash=03cb79b0fc91b70f2dba01953db42f7b50245bf946dd15d002e7db5c86ba0390
discovery=1
compatibility=6
sync_capacity=29
sse_capacity=29
rpm_capacity=12
topology_verification=4
maximum_http_requests=81
maximum_generation_requests=80
requests_sent=0
network_sent=false
```

The default per-channel v3 bound is therefore HTTP `2M+71+K` and generation `2M+70+K`. Existing V2 `2M+42/2M+41` figures remain historical bounds for its sync-only capacity path and must not be used for v3 approval.

## Automated Verification

```text
upstream_benchmark_nonfunctional_test.rb: 31 runs / 142 assertions
upstream_benchmark_v2_test.rb:            32 runs / 194 assertions
upstream_benchmark_protocols_test.rb:     10 runs / 44 assertions
upstream_benchmark_test.rb:               18 runs / 63 assertions
ruby syntax:                              PASS
V2 validate:                              PASS
D04 go vet:                               PASS
D04 go test -race:                        PASS (serial rerun)
D04/infra contract scripts:               PASS
```

The first concurrent D04 race/vet verification attempt exhausted the local Docker compiler memory while compiling `modernc.org/sqlite/lib` (`signal: killed`). Vet completed successfully. Running the identical race command alone then completed all packages with exit `0`, confirming an execution-environment memory collision rather than a code failure.

## Production Read-only Recheck

- `sub2api-relay-ops:candidate-admin-intake-20260721-v2`: running, healthy, restart `0`.
- `sub2api-internal-test:d04-public-registration-20260721-v1`: running, healthy, restart `0`.
- Modes: relay-ops `read_only`, Feishu command `dry_run`; D04 `read_only`, registration `false`.
- `/healthz`, `/readyz`, `/pricing`, `/ops`, and `/monitor`: HTTP `200`.
- Fresh selected Admin API reads of public groups `2/6` and accounts/models `2/7/8/9` produced allowlisted canonical SHA-256 `b2a2a6ce01bc6135e996eacba4e3739052bb2a70720439782e6d4b96bc3aaf82`, exactly equal to the normalized saved baseline. The routing-file SHA-256 remained `3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e`.
- Sub2API was healthy with restart count `1` and OOM `false`; the other five containers had restart count `0` and OOM `false`. This is recorded as current runtime state, not hidden or normalized away.
- This recheck made only authenticated GETs through an in-memory secret boundary. It did not output a credential or raw response, rebuild a service, send a synthetic/model event, or change a route/database record.

## Remaining Gates

- D04 still requires separately authorized single-user write acceptance and immediate restoration to `read_only + registration closed`.
- XM Plus/Pro still require separately authorized temporary Keys and one authenticated `/models` request each.
- The actual per-channel `M` values are still unknown, so no paid qualification budget is approved.
- Sync, SSE, billing, capacity, terms, gateway roles, Wawazz shared-pool fairness, 24–72 hour observation, failover, and failback evidence are still missing.
- No secret-free topology proposal exists. Production routing remains unchanged and `relay-ops` remains `read_only + dry_run`.
