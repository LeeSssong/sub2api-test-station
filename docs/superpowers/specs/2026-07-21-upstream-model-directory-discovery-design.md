# Spec: Generic Upstream Model Directory Discovery

**Date:** 2026-07-21 (Asia/Shanghai)  
**Status:** Approved

## Problem

The shared V2 benchmark can discover models only as the first step of a full qualification run. A full run immediately continues into paid generation and capacity requests, so it cannot safely establish the exact model count needed to calculate a bounded qualification budget.

## Goal

Add a public `discover` command that uses the existing protocol adapters and HTTP client to make exactly one authenticated model-directory request, classify the returned model IDs, and stop without making any generation request.

## Non-goals

- No XM-specific branch, hostname check, vendor price, or channel-ID behavior.
- No Key creation, copying, persistence, or display.
- No generation, SSE, capacity, billing, gateway, or production-route test.
- No qualification decision, proposal generation, candidate creation, or lifecycle promotion.
- No production deployment or route/configuration change.

## Command Contract

```text
ruby ops/upstream-benchmark-v2.rb discover \
  --channel CHANNEL_ID \
  --channels PATH \
  --profile PATH \
  --key-env ENV_NAME \
  --runs PATH
```

`--channel` is always required. Live execution also requires a non-empty Key supplied only through `--key-env`. `--dry-run` does not require the environment variable and emits:

```json
{
  "model_directory_requests": 1,
  "generation_requests": 0,
  "requests_sent": 0,
  "network_sent": false
}
```

Live execution calls the profile-selected `models_path` exactly once through the existing Chat Completions or Responses adapter. It writes one secret-free run record and prints its redacted form.

## Result Contract

A successful directory read is deliberately incomplete qualification evidence:

- `status`: `partial`
- `evidence_source`: `live_direct`
- `qualification_status`: `discovered_not_qualified`
- `metrics.request_count`: `1`
- `metrics.generation_request_count`: `0`
- `metrics.model_count`: number of unique non-empty model IDs
- `metrics.classifications`: stable array of existing generic `ModelCatalog` classifications; model IDs are values rather than object keys so the output redactor cannot drop `text-*` IDs
- `metrics.model_ids`: stable sorted IDs
- `metrics.testable_model_ids`: stable sorted IDs classified as text
- `metrics.latency_ms`: duration returned by the shared HTTP client
- `errors`: empty

A failed directory read uses `status=failed`, preserves request counts, returns empty model/classification arrays, and emits only one fixed error category from: `rate_limited`, `upstream_http`, `request_rejected`, `timeout`, `transport_error`, or `protocol_error`. Raw upstream error text is not included.

## Safety Invariants

- Exactly one `/models` request in live mode and zero generate calls.
- Dry-run performs zero network requests.
- Output and ledger contain no authorization value, environment value, response body, Cookie, Token, Key, password, or secret-shaped field.
- Model IDs are data, not executable input; they are only normalized, classified, sorted, and serialized.
- The command cannot mark a channel qualified or generate/apply a route proposal.

## Test Strategy

- Unit test the runner with a counting client for success, sorting/deduplication, and fixed error classification.
- CLI dry-run test proves one planned directory request, zero generation requests, and zero network requests.
- A local fake HTTP server test proves one authenticated model-directory request and zero generation requests without contacting a real supplier.
- Regression-run V2, protocol adapter, V1 benchmark, and registry tests.

## Acceptance Criteria

- [ ] `discover --dry-run` reports `1` planned model request, `0` generation requests, and `network_sent=false`.
- [ ] Live fake-server execution makes exactly one configured models-path request and no generation request.
- [ ] Successful output is `partial / live_direct / discovered_not_qualified` and includes sorted classified model IDs and latency.
- [ ] Failed output exposes only a fixed redacted error category.
- [ ] The run ledger receives the same secret-free partial evidence.
- [ ] Source contains no XM-specific execution branch.
- [ ] Existing benchmark and registry regressions pass.
