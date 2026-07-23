# Account-Pool Quality Monitor Verification

**Date:** 2026-07-23 (Asia/Shanghai)  
**Result:** `PASS`  
**Production modes:** relay-ops `read_only`, Feishu commands `dry_run`, D04 `read_only`, registration closed

## Scope

This replaces the active model-release scheduler with a lightweight,
account-isolated quality pulse. The installed timer, rather than an LLM,
will discover the native Sub2API account pool and run one bounded native
account test per `active + schedulable` account every 15 minutes.

The implementation records only account ID, selected model ID, configured
multiplier, stability metrics, TTFT, and stable result codes. It stops reading
after the first non-empty native content event and never persists model text,
Base URLs, credentials, headers, or raw responses. Explicit balance shortage
is the only path labelled `balance_exhausted`.

The production task is installed and active. Only the installed systemd
service invokes the native account-test endpoint; no ad hoc account request
was issued by the operator or LLM. The old model-release unit, scripts and
evidence remain as history, while its timer is disabled.

## Local Verification

The following gates passed against the current worktree:

```text
ruby tests/operations/collect_account_quality_pulse_test.rb
ruby tests/operations/account_quality_monitor_test.rb
bash tests/relay_ops/validate_relay_ops_contract.sh
docker ... go test ./internal/accountquality ./internal/config ./internal/http ./internal/app -count=1
git diff --check
```

The collector suite validates native account-list selection, deterministic
model selection, account isolation, explicit balance classification, timeout
and HTTP error mapping, malformed SSE handling, TTFT-only storage, forbidden
response text exclusion, 24-hour rolling metrics, private restricted files,
and native HTTP/SSE paths. The task suite verifies the exact hardened Docker
arguments, fixed 15-minute timer cadence, secret-free environment template,
and fail-closed wrapper logs.

The relay-ops contract now requires an entire read-only account-quality
evidence directory rather than a single result-file mount. This preserves
visibility of atomic `rename` publication.

## Production Installation

The server-native AMD64 image is
`sub2api-relay-ops:account-quality-monitor-v1`, image ID
`sha256:593e37c9d1a89c34b9ceacef0d3dedc06d619b8bc444faea6d321ad5188ffbd9`.
Only `sub2api-relay-ops-1` was recreated; it became healthy with restart count
`0` and OOM status `false`. Sub2API, PostgreSQL, Redis, Caddy and D04 retained
their pre-install container IDs.

The systemd service completed successfully. Starting the persistent timer
immediately produced a second timer-owned pulse. The next naturally scheduled
run at 12:13 CST produced a third sample per account. These are separate runs,
not in-run retries. After suppressing child-process output, the natural run's
journal contains only `status=started` and `status=succeeded`. The resulting
timer state is:

```text
sub2api-account-quality-monitor.timer enabled / active
sub2api-model-release-monitor.timer   disabled / inactive
```

The old unit, task directory and `model-release-20260722` evidence directory
remain installed and unchanged.

## First Production Evidence

Snapshot `ACCOUNT-QUALITY-20260723T041327Z` dynamically discovered four
active, schedulable accounts. Account `12` returned a generic native account
test error in all three samples. It was deliberately not labelled as exhausted
balance, and it did not prevent account `13` from running. The natural third
run also recorded a generic error for account `11` and a timeout for account
`13`, demonstrating that per-account failure isolation is active.

| Account | Model | Success | TTFT P95 | Multiplier | Latest result |
|---|---|---:|---:|---:|---|
| 10 | `gpt-5.6` | 2/3 | 1323.425 ms | 1.0x | `passed` |
| 11 | `gpt-5.6` | 2/3 | 2381.362 ms | 1.0x | `account_test_error` |
| 12 | `gpt-5.6` | 0/3 | unavailable | 1.0x | `account_test_error` |
| 13 | `gpt-5.6` | 2/3 | 1257.208 ms | 1.0x | `timeout` |

The account-set hash is
`f6b733f89e799048c92d90dc0d404ce1f96300bf1f2964184cc681bdcc2457e7`.
Both result and private history files are mode `0600`, owned by container UID
`10002`. Result SHA-256 is identical on the host and inside relay-ops:

```text
f9eef0c81e79109ba10a6009c0756275632d1858100bea9116fda0ec7ea8034c
```

Schema validation passed, and neither file contains response text, Base URL,
authorization data, API Key fields, headers or raw responses.

## Production Safety Recheck

- Relay-ops remains `read_only`; Feishu commands remain `dry_run`.
- D04 remains `read_only` and `D04_REGISTRATION_OPEN=false`.
- The account-quality directory is mounted read-only into relay-ops.
- Twelve native account-test calls match three runs across four discovered
  accounts; no account/group/model/routing `PUT`, `PATCH`, `DELETE`, bulk
  update, scheduling or multiplier write was found in the deployment window.
- `/healthz`, `/readyz`, `/pricing`, `/ops` and `/monitor` return HTTP `200`.
  `/relay-ops/api/ops-view` without a bearer token returns hidden `404`.
- Caddy SHA-256 remains
  `668b274207f7265affa03f4ecc22725db34b30e9d9ae0cc1b7d39b483250b292`.
- Feishu routing SHA-256 remains
  `3262403ac7e948e9453e1487922ac538e066f60fd7d23474e66f4ee917f7435e`.
- The expected three-line relay-ops projection change gives production
  Compose SHA-256
  `9ef0dcb95fea08c8a7574702026c7b5fd57b826bbb318b54605e17713966fc7e`.

No route, group, priority, account scheduling, multiplier, price, balance,
Key, model, D04 or Feishu configuration was changed. The lightweight account
quality loop is now the active production monitor; D04 controlled opening is
the next mainline and remains a separate decision.
