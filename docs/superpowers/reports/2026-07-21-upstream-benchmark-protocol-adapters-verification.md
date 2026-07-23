# Upstream Benchmark Protocol Adapters Verification

**Date:** 2026-07-21 (Asia/Shanghai)  
**Result:** `OFFLINE PASS / LIVE QUALIFICATION NOT AUTHORIZED`

## Result

The V2 benchmark now supports protocol-driven Chat Completions and Responses evaluation as a shared capability. XM is represented only by ordinary registry entries and a generic Responses profile; there is no XM-, hostname-, channel- or model-specific implementation branch.

The protocol contract owns only:

- model-list request and parsing;
- generation request body;
- normalized input/output/total usage;
- SSE terminal-event recognition;
- redacted structured error classification.

The existing runner continues to own scheduling, concurrency/RPM levels, latency, billing evidence, reports and lifecycle decisions. Legacy V1 and V2 Chat Completions behavior remains covered.

## Protocol And Path Safety

- Profiles accept legacy `endpoint: chat_completions` or `protocol: responses`.
- `models_path`, `generate_path` and bounded `terminal_events` are profile data.
- Request paths must be absolute relative paths and reject schemes, hosts, query strings, fragments, `//` authority syntax, raw traversal and traversal revealed by up to three URL-decode passes.
- Responses requests use `input` and `max_output_tokens`; Chat Completions uses `messages` and `max_tokens`.
- Responses recognizes `response.completed` and optional `[DONE]`; Chat Completions preserves `[DONE]`.
- Sync and streaming response bodies are independently capped at 1 MiB; SSE accepts both LF and CRLF event boundaries and rejects an unbounded stream before report generation.
- Provider `type`/`code` values are mapped to a fixed category allowlist (`rate_limited`, `insufficient_balance`, `authentication`, `model_unavailable`, `request_rejected`, `upstream_http`, `protocol_error`); reports never include prompt/output bodies, authorization values or provider error messages.

## Offline Evidence

```text
upstream_benchmark_protocols_test.rb: 10 runs, 44 assertions
upstream_benchmark_test.rb: 18 runs, 63 assertions
upstream_benchmark_v2_test.rb: 32 runs, 194 assertions
validate_upstream_test.rb: 13 runs, 30 assertions
```

All suites passed with zero failures and zero errors. Both real registry entries completed the generic Responses dry-run:

```text
xm-plus: protocol=responses models_path=/models generate_path=/responses requests_sent=0 network_sent=false
xm-pro:  protocol=responses models_path=/models generate_path=/responses requests_sent=0 network_sent=false
```

No Key was required for dry-run and no upstream request was made.

## Generic Directory-only Discovery

The V2 CLI now also provides a public `discover` command. It reuses the same registry, profile, protocol adapter, HTTP client, model classifier, redactor and append-only ledger as a full run, but its runner has no generation path:

```text
ruby ops/upstream-benchmark-v2.rb discover \
  --channel <channel-id> \
  --key-env <RUNTIME_ENV_NAME> \
  --dry-run
```

The dry-run contract is fixed at one planned model-directory request, zero generation requests, `requests_sent=0`, and `network_sent=false`. A live success remains `partial / live_direct / discovered_not_qualified`; it cannot mark a channel qualified or build/apply a proposal. The result contains only sorted model IDs, generic classifications, counts, model-directory latency and fixed error categories.

Automated counting-client coverage proves the live orchestration calls `models` exactly once, never calls `generate`, writes the same partial evidence to the JSONL ledger, rejects an empty environment Key before building a client, and omits the Key value from stdout and the ledger. Existing protocol transport tests independently cover the configured models path and bounded response parsing.

Fresh XM registry dry-runs for this command returned:

```text
xm-plus: protocol=responses models_path=/models model_directory_requests=1 generation_requests=0 requests_sent=0 network_sent=false
xm-pro:  protocol=responses models_path=/models model_directory_requests=1 generation_requests=0 requests_sent=0 network_sent=false
```

No live XM request was authorized or sent.

The V1 registry remains validated by `ruby ops/upstream-benchmark.rb validate`; the V2 Responses profile is validated with its required pricing/scenario inputs. V2 validation now reports the profile ID actually parsed rather than a static Chat Completions label.

## Remaining Gate

XM Plus and XM Pro remain `discovered`. No candidate, proposal, paid probe or production route was created or changed. The next independent gate is authorization to create/install two bounded temporary Keys and send exactly one `GET /models` per channel through `discover`. Full qualification still requires a separately approved request/token/currency budget, stop thresholds, billing evidence and cleanup authorization. Only a later secret-free proposal explicitly accepted by ID/hash may authorize the target XM-primary/Wawazz-backup topology.
