# Local Sub Account Quality-first Verification

**Date:** 2026-07-22 (Asia/Shanghai)  
**Result:** `PASS WITH BLOCKED PROMOTION`  
**Scope:** local Sub2API accounts `73`, `74`, and `75`; no production candidate or route change

## Decision

Quality gates take precedence over price and aggregate score. Accounts `73` and `74` failed mandatory catalog compatibility. Account `75` is the strongest technical result, but its price basis, upstream billing semantics, balance behavior, and commercial terms remain unverified. Therefore none of the three accounts is eligible for customer exposure or a production switch.

| Account | Pulse | Full text catalog | Capacity | Quality result | Promotion result |
|---|---:|---:|---:|---|---|
| `73` | `6/6` | `14/16` | not run | `gpt-5.6` sync/SSE failed and SSE was incomplete | `blocked` |
| `74` | `6/6` | `14/26` | not run | six models failed both sync and SSE | `blocked` |
| `75` | `6/6` | `18/18` | `129/129` | strongest technical candidate | `needs_evidence` |

The six failed account `74` models were `gpt-5.2`, `gpt-5.2-openai-compact`, `gpt-5.3-codex`, `gpt-5.3-codex-openai-compact`, `gpt-5.4-openai-compact`, and `gpt-5.5-openai-compact`.

## Durable Run Evidence

The content-free JSONL ledger records the following run IDs:

```text
73 pulse:    fe532a9d-e1b6-43fc-b6ba-b21c4227dbd8
74 pulse:    6fa6a00f-ee2d-4173-be32-3be3bdd5cd7d
75 pulse:    4b7c0ecc-4456-4baa-883a-ded67976b725
73 catalog:  0a6f4e91-f19a-4b56-976d-379efc2631c2
74 catalog:  e2cf7093-0596-47f9-b003-cf04d2239377
75 catalog:  ba13eef1-049c-4152-b058-ecccb763576c
75 capacity: c4e5cd64-8c43-4a35-880d-b45e8f001d4f
```

Account `75` passed the bounded concurrency ladder through `10` and the configured RPM target ladder through `30`. These are observed lower bounds rather than provider limits. The conservative operating recommendation is concurrency `8` and RPM `24`; direct capacity-run P95 total latency was `4891.645 ms`.

## Isolated Gateway Evidence

A temporary local UI Key and isolated group were used only to prove gateway account selection. One account was schedulable at a time, and `usage_logs.account_id` confirmed the intended account.

| Account | Sync | SSE | TTFT |
|---|---|---|---:|
| `73` | HTTP 200 / `5.28s` | HTTP 200 / `2.37s` | `2.26s` |
| `74` | HTTP 200 / `3.58s` | HTTP 200 / `2.05s` | `1.99s` |
| `75` | HTTP 200 / `4.83s` | HTTP 200 / `2.75s` | `0.84s` |

This smoke does not override the failed full-catalog hard gates for `73` or `74`, and it is not a full gateway-overhead or billing qualification for `75`.

## Billing And Commercial Unknowns

- The tiny gateway requests for `73` and `74` reported about `4.39k` input tokens, while their direct evidence was about `8.8k-9.3k`; account `75` reported `307` input tokens for its gateway request.
- The differences may reflect provider prompt injection, normalization, cache accounting, or another billing rule. No cause is asserted without provider-side evidence.
- Per-model price, trusted base price, actual multiplier, provider debit, balance floor, resale permission, and terms remain unknown.
- Price cannot compensate for a quality failure. Unknown billing keeps account `75` at `needs_evidence` even though its technical checks passed.

## Cleanup

- The temporary local UI Key was deleted.
- The isolated group has no remaining Key or account binding.
- Accounts `73/74/75` were restored to group `5` with `schedulable=false`.
- The non-sensitive final account-state canonical SHA-256 is:

```text
145ba7085e8da2d319a05fe293ef1b488a7a38295a96e92cfb06cf41547d0ef1
```

No production candidate was created, no production route or price was changed, and no credential, prompt, model output, cookie, or token was persisted in the report or ledger.

Fresh verification passed with `42/250` V2 tests, `10/44` protocol tests, `32/160` nonfunctional tests, and registry/profile validation. Relay-ops full race tests, `go vet`, static JavaScript checks, and the deployment contract also passed.

## Operational Capability

The quality-first loop now supports bounded `health_pulse`, `catalog_quick`, and `capacity_check` runs; hard-gate-first evaluation; `15m/6h/24h` scheduler cadence; a hash-bound read-only `/ops` preview; and a notification-only Feishu quality card without switch actions. Final review added the previously missing `fast result -> stored report -> stable incident state -> card delivery` wiring. Equivalent reports use a semantic evidence hash that excludes run ID/time, and real PostgreSQL now permits a failed delivery to be retried without deleting its row while still suppressing delivered or in-flight duplicates.

The increment was deployed after launch-readiness preparation as `sub2api-relay-ops:quality-report-read-only-20260722-v1`. Production remains `read_only + dry_run`; candidates, paid probes, quality-report rows, and `candidate-fast:*` jobs remain zero. Deployment did not send a Feishu message or synthetic event and did not change the account decisions above. See `2026-07-22-quality-report-feishu-production-verification.md`.
