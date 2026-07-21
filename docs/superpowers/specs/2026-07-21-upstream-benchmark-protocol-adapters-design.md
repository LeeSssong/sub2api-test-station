# Upstream Benchmark Protocol Adapters Design

**Date:** 2026-07-21
**Status:** Approved design
**Scope:** Generalize the offline/direct upstream benchmark runner by wire protocol, without vendor-specific branches.

## Goal

Extend the benchmark runner so a profile can evaluate OpenAI-compatible Chat Completions and Responses APIs through a shared result model. XM is an initial consumer of the Responses adapter, not a special case. The same capability must work for future root-path or versioned upstream relays.

## Explicit Non-Goals

- No live paid benchmark, production upstream change, topology change, account creation, Key copy, or Key storage is authorized by this design.
- No vendor name, channel ID, hostname, model name, or price creates a protocol branch.
- No change to V1 benchmark behavior or existing valid Chat Completions V2 profiles.
- No automatic promotion of any evaluated upstream to a Sub2API primary route.

## Architecture

```text
Benchmark profile
  -> profile validator and normalizer
  -> protocol adapter registry
       -> ChatCompletionsAdapter
       -> ResponsesAdapter
  -> common execution engine
  -> normalized benchmark sample and report
```

The common execution engine continues to own scheduling, concurrency, RPM pacing, timeout classification, billing comparison, fixture reporting, and output formatting. An adapter owns only protocol-specific request construction and response interpretation.

## Profile Contract

New profiles use:

```yaml
protocol: responses
models_path: /models
generate_path: /responses
terminal_events:
  - response.completed
```

Existing `endpoint: chat_completions` profiles remain valid and normalize to `protocol: chat_completions` with the present default paths. Profiles must contain a registered protocol and absolute-path-only request paths. They may not contain a scheme, host, query, fragment, relative traversal, or arbitrary redirect target.

The registry remains the source for channel metadata. A channel references a benchmark profile; it does not choose code paths by its own ID. XM's root `https://api3.xmhbao.cn` and `/responses` configuration are thus normal profile data, while a future `/v1/responses` provider uses the same adapter with different path fields.

## Adapter Interface

Each adapter implements a small, testable contract:

1. Construct the models catalog request.
2. Construct the synchronous generation request.
3. Construct the streaming generation request.
4. Parse successful response usage into common input/output/total-token fields.
5. Recognize protocol terminal events and completion semantics.
6. Classify structured protocol errors without exposing provider credentials.

`ChatCompletionsAdapter` retains the existing `/chat/completions` payload and `[DONE]` completion behavior. `ResponsesAdapter` creates the Responses wire payload, parses its corresponding usage shape, and recognizes `response.completed`; it may also accept `[DONE]` where a compatible gateway emits both.

Both adapters return the same sample fields: model, request kind, start time, first-token time, end time, TTFT, total duration, HTTP outcome, common error class, input tokens, output tokens, total tokens, and raw-free completion classification.

## Execution and Safety

- The runner builds request URLs from the channel base URL plus validated adapter paths only.
- Response bodies are bounded and redacted before diagnostics; authorization values never appear in reports, errors, fixtures, or process arguments.
- Existing deterministic concurrency and RPM controls apply identically to both protocols.
- Existing SSE parsing is extended through the adapter's terminal predicate, rather than by a channel-name check.
- Fixture and dry-run modes remain entirely local and must cover all new protocol behavior without any Key.

## Validation

The automated suite must prove:

1. Existing Chat Completions V2 profiles validate, execute fixtures, stream, and report unchanged.
2. A root-path Responses profile creates `/models` and `/responses` requests, while a versioned Responses profile creates `/v1/models` and `/v1/responses`.
3. Invalid paths and unregistered protocols fail validation before a request is constructed.
4. Responses synchronous usage and streamed `response.completed` events normalize into the common token and timing result.
5. Chat `[DONE]` behavior and existing error classification remain intact.
6. No fixture, error, report, or command line prints an authorization value.
7. Channel fixture tests demonstrate that two arbitrary channel IDs using the same Responses profile follow the same adapter path.

## Qualification and Routing Sequence

After implementation and offline verification, the next steps remain explicitly gated:

1. Obtain user approval for a bounded, paid XM Plus/Pro qualification budget and cleanup scope.
2. Run generic direct API benchmarks using the XM profile, without copying a Key into repository files or reports.
3. Produce a secret-free comparison and an explicit topology proposal: XM Plus for the Plus primary, XM Pro for the Pro primary, Wawazz as backup.
4. Apply that topology only after the user explicitly approves the proposal by ID or hash.
5. Verify the resulting production route, model list, synchronous and streaming behavior, billing, and fallback independently.

Until those gates are complete, Wawazz remains the current production upstream and XM stays discovered, not qualified.
