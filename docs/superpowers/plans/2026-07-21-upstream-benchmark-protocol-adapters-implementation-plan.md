# Upstream Benchmark Protocol Adapters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add reusable Chat Completions and Responses protocol adapters to the V2 upstream benchmark so XM and future relays can be evaluated without vendor-specific logic.

**Architecture:** Normalize legacy V2 profiles into a protocol-aware contract, select a strong adapter from a fixed registry, and retain scheduling, metrics, concurrency, billing, and reporting in the existing runner. Adapters own only request paths/bodies, usage extraction, and stream completion semantics.

**Tech Stack:** Ruby standard library, Minitest, YAML/JSON fixtures, existing V2 benchmark CLI.

## Global Constraints

- Never branch on XM, channel ID, hostname, model, or vendor price.
- No live request, Key copy/storage, candidate creation, or production topology change is authorized.
- Preserve V1 behavior and existing V2 Chat Completions profiles.
- Validate relative absolute paths; reject scheme, host, query, fragment, and traversal.
- Never expose authorization values in reports, errors, fixtures, ledgers, or process arguments.

---

### Task 1: Protocol-Aware Profile Normalization

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_test.rb`
- Create: `config/upstream-benchmarks/mvp-text-responses-v2.yaml`

**Interfaces:**
- Produces: `Profile#protocol`, `#models_path`, `#generate_path`, `#terminal_events`.
- Normalizes legacy `endpoint: chat_completions`.

- [ ] **Step 1: Write failing profile tests**

```ruby
def test_v2_profile_normalizes_legacy_chat_and_accepts_responses
  legacy = profile_v2("endpoint" => "chat_completions")
  assert_equal "chat_completions", legacy.protocol
  assert_equal "/v1/models", legacy.models_path
  assert_equal "/v1/chat/completions", legacy.generate_path

  responses = profile_v2("protocol" => "responses", "models_path" => "/models",
    "generate_path" => "/responses", "terminal_events" => ["response.completed"])
  assert_equal "responses", responses.protocol
end

def test_v2_profile_rejects_unsafe_paths
  ["https://evil.invalid/responses", "responses", "/../responses", "/responses?q=1", "//evil.invalid/responses"].each do |path|
    assert_raises(UpstreamBenchmark::ValidationError) {
      profile_v2("protocol" => "responses", "models_path" => "/models", "generate_path" => path)
    }
  end
end
```

- [ ] **Step 2: Run RED**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb --name '/profile.*(protocol|path)|normalizes_legacy/'
```

Expected: profile accessors are missing and Responses is rejected.

- [ ] **Step 3: Implement defaults and path validation**

```ruby
PROTOCOL_DEFAULTS = {
  "chat_completions" => {
    "models_path" => "/v1/models", "generate_path" => "/v1/chat/completions",
    "terminal_events" => ["[DONE]"]
  },
  "responses" => {
    "models_path" => "/v1/models", "generate_path" => "/v1/responses",
    "terminal_events" => ["response.completed"]
  }
}.freeze

def protocol
  @document["protocol"] || @document["endpoint"]
end

def validate_path!(key, value)
  uri = URI.parse(value)
  valid = value.start_with?("/") && !value.start_with?("//") &&
    uri.scheme.nil? && uri.host.nil? && uri.query.nil? && uri.fragment.nil? &&
    !value.split("/").include?("..")
  raise UpstreamBenchmark::ValidationError, "#{key} must be a safe absolute request path" unless valid
rescue URI::InvalidURIError
  raise UpstreamBenchmark::ValidationError, "#{key} must be a safe absolute request path"
end
```

Use `URI.parse`; require one leading slash, reject `//`, scheme, host, query, fragment, and any `..` segment. Require a registered protocol and a bounded non-empty terminal-event list.

- [ ] **Step 4: Run GREEN and validate both profiles**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb --name '/profile.*(protocol|path)|normalizes_legacy/'
ruby ops/upstream-benchmark.rb validate
ruby ops/upstream-benchmark-v2.rb validate --profile config/upstream-benchmarks/mvp-text-responses-v2.yaml --pricing config/upstream-benchmarks/pricing-evidence.example.yaml --scenario config/upstream-benchmarks/v2-scenario-neko.example.yaml
```

- [ ] **Step 5: Commit**

```bash
git add ops/upstream-benchmark-v2.rb tests/upstream_benchmarks/upstream_benchmark_test.rb config/upstream-benchmarks/mvp-text-responses-v2.yaml
git commit -m "feat: validate protocol-aware benchmark profiles"
```

### Task 2: Strong Protocol Adapter Registry

**Files:**
- Create: `ops/upstream-benchmark-protocols.rb`
- Create: `tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb`

**Interfaces:**
- Produces: `UpstreamBenchmark::Protocols.build(name, terminal_events:)`.
- Adapter methods: `models_request`, `parse_models`, `generate_request`, `normalize_usage`, `terminal_event?`, and `classify_error`.

- [ ] **Step 1: Write failing adapter tests**

```ruby
def test_responses_adapter_builds_payload_and_usage
  adapter = UpstreamBenchmark::Protocols.build("responses", terminal_events: ["response.completed"])
  assert_equal({method: Net::HTTP::Get, path: "/models", payload: nil}, adapter.models_request(path: "/models"))
  assert_equal ["gpt-test"], adapter.parse_models("data" => [{"id" => "gpt-test"}])
  request = adapter.generate_request(model: "gpt-test", prompt: "ping", max_output_tokens: 16, stream: true)
  assert_equal({"model"=>"gpt-test", "input"=>"ping", "max_output_tokens"=>16, "stream"=>true}, request)
  usage = adapter.normalize_usage("input_tokens"=>3, "output_tokens"=>5, "total_tokens"=>8)
  assert_equal 8, usage["total_tokens"]
  assert adapter.terminal_event?({"type"=>"response.completed"})
end

def test_registry_has_no_vendor_adapter
  assert_raises(UpstreamBenchmark::ValidationError) { UpstreamBenchmark::Protocols.build("xm", terminal_events: []) }
end
```

- [ ] **Step 2: Run RED**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
```

Expected: registry constant is missing.

- [ ] **Step 3: Implement two fixed adapters**

```ruby
module UpstreamBenchmark
  module Protocols
    class BaseAdapter
      def initialize(terminal_events) = @terminal_events = terminal_events
      def models_request(path:) = { method: Net::HTTP::Get, path: path, payload: nil }
      def parse_models(document) = Array(document["data"]).filter_map { |item| item["id"] }
      def classify_error(document)
        error = document.is_a?(Hash) ? document["error"] : nil
        value = error.is_a?(Hash) ? (error["type"] || error["code"]) : "protocol_error"
        UpstreamBenchmark::Redactor.clean(value.to_s)
      end
    end

    class ChatCompletionsAdapter < BaseAdapter
      def generate_request(model:, prompt:, max_output_tokens:, stream:)
        { "model" => model, "messages" => [{ "role" => "user", "content" => prompt }],
          "max_tokens" => max_output_tokens, "stream" => stream }
      end
      def normalize_usage(raw)
        { "input_tokens" => raw["prompt_tokens"].to_i,
          "output_tokens" => raw["completion_tokens"].to_i,
          "total_tokens" => raw["total_tokens"].to_i }
      end
      def terminal_event?(event) = @terminal_events.include?(event)
    end

    class ResponsesAdapter < BaseAdapter
      def generate_request(model:, prompt:, max_output_tokens:, stream:)
        { "model" => model, "input" => prompt,
          "max_output_tokens" => max_output_tokens, "stream" => stream }
      end
      def normalize_usage(raw)
        { "input_tokens" => raw["input_tokens"].to_i,
          "output_tokens" => raw["output_tokens"].to_i,
          "total_tokens" => raw["total_tokens"].to_i }
      end
      def terminal_event?(event)
        type = event.is_a?(Hash) ? event["type"] : event
        @terminal_events.include?(type)
      end
    end

    REGISTRY = {
      "chat_completions" => ChatCompletionsAdapter,
      "responses" => ResponsesAdapter
    }.freeze

    def self.build(name, terminal_events:)
      klass = REGISTRY.fetch(name) { raise ValidationError, "unsupported benchmark protocol: #{name}" }
      klass.new(terminal_events)
    end
  end
end
```

Chat emits `messages` and maps `prompt_tokens/completion_tokens`; Responses emits `input`, maps `input_tokens/output_tokens`, recognizes `response.completed`, and accepts literal `[DONE]` as a compatibility terminal.

- [ ] **Step 4: Run GREEN using the Step 2 command**
- [ ] **Step 5: Commit**

```bash
git add ops/upstream-benchmark-protocols.rb tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
git commit -m "feat: add benchmark protocol adapters"
```

### Task 3: Adapter-Aware HTTP and SSE Transport

**Files:**
- Modify: `ops/upstream-benchmark.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_test.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb`

**Interfaces:**
- Changes: `HttpClient.new(..., adapter:, models_path:, generate_path:)`.
- Produces: `HttpClient#generate(model:, prompt:, max_output_tokens:, stream:)`.
- Changes: `SseParser.new(adapter:)`.
- Preserves: V1 `HttpClient#chat` wrapper and defaults.

- [ ] **Step 1: Write failing root/versioned-path and stream tests**

```ruby
def test_http_client_uses_profile_paths_and_response_terminal
  server = scripted_server
  client = UpstreamBenchmark::HttpClient.new(
    base_url: server.url, api_key: "redacted", timeout_seconds: 2,
    adapter: UpstreamBenchmark::Protocols.build("responses", terminal_events: ["response.completed"]),
    models_path: "/models", generate_path: "/responses"
  )
  client.models
  result = client.generate(model: "gpt-test", prompt: "ping", max_output_tokens: 8, stream: true)
  assert_equal ["/models", "/responses"], server.request_paths
  assert result["stream_complete"]
  assert_equal 3, result.dig("usage", "total_tokens")
end
```

Add `/v1/models + /v1/responses`, arbitrary channel-label equality, malformed SSE, and Chat `[DONE]` regression cases.

- [ ] **Step 2: Run RED**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb --name '/http_client|stream|path/'
```

Expected: initializer and `generate` reject adapter/path inputs.

- [ ] **Step 3: Delegate wire logic to the adapter**

```ruby
def generate(model:, prompt:, max_output_tokens:, stream:)
  payload = @adapter.generate_request(
    model: model, prompt: prompt, max_output_tokens: max_output_tokens, stream: stream
  )
  return request_stream(@generate_path, payload) if stream
  response = request_json(Net::HTTP::Post, @generate_path, payload)
  response["usage"] = @adapter.normalize_usage(response["usage"] || response.dig("response", "usage") || {})
  response
end

def models
  request = @adapter.models_request(path: @models_path)
  document = request_json(request.fetch(:method), request.fetch(:path), request.fetch(:payload))
  @adapter.parse_models(document)
end
```

Construct `SseParser` with the adapter; pass each parsed JSON event to `terminal_event?`; normalize usage through the adapter. Retain V1 defaults and `chat`.

- [ ] **Step 4: Run all transport regressions**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb
```

Expected: both suites pass.

- [ ] **Step 5: Commit**

```bash
git add ops/upstream-benchmark.rb tests/upstream_benchmarks
git commit -m "feat: make benchmark transport protocol-aware"
```

### Task 4: V2 Runner and Generic XM Profile Integration

**Files:**
- Modify: `ops/upstream-benchmark-v2.rb`
- Modify: `tests/upstream_benchmarks/upstream_benchmark_test.rb`
- Modify: `config/upstream-benchmarks/channels.yaml`
- Modify: `config/upstream-benchmarks/mvp-text-responses-v2.yaml`

**Interfaces:**
- V2 creates an adapter-aware client from normalized profile data.
- XM entries use the generic Responses profile and remain `discovered`.

- [ ] **Step 1: Write failing dry-run and no-vendor-branch tests**

```ruby
def test_v2_responses_dry_run_is_generic_and_zero_request
  out = StringIO.new
  status = UpstreamBenchmarkV2::CLI.run([
    "run", "--channel", "xm-plus", "--channels", channels_path,
    "--profile", responses_profile_path, "--dry-run"
  ], out: out, env: {})
  document = JSON.parse(out.string)
  assert_equal 0, status
  assert_equal "responses", document["protocol"]
  assert_equal 0, document["requests_sent"]
end

def test_v2_source_has_no_xm_branch
  source = File.read(File.expand_path("../../ops/upstream-benchmark-v2.rb", __dir__))
  refute_match(/channel.*==.*xm|when\s+["']xm|api3\.xmhbao/i, source)
end
```

- [ ] **Step 2: Run RED**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb --name '/responses_dry_run|no_xm_branch/'
```

Expected: V2 rejects Responses or omits normalized protocol.

- [ ] **Step 3: Instantiate from profile**

```ruby
adapter = UpstreamBenchmark::Protocols.build(profile.protocol, terminal_events: profile.terminal_events)
client = UpstreamBenchmark::HttpClient.new(
  base_url: channel.fetch("base_url"), api_key: api_key,
  timeout_seconds: profile["timeout_seconds"], adapter: adapter,
  models_path: profile.models_path, generate_path: profile.generate_path
)
```

Replace V2 `chat` calls with `generate`. Dry-run reports protocol, paths, and zero sent requests without requiring a Key. Both XM channels reference the same generic profile metadata; no credential or status promotion is added.

- [ ] **Step 4: Run GREEN, validation, and zero-request dry-run**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
ruby ops/upstream-benchmark-v2.rb validate --profile config/upstream-benchmarks/mvp-text-responses-v2.yaml --pricing config/upstream-benchmarks/pricing-evidence.example.yaml --scenario config/upstream-benchmarks/v2-scenario-neko.example.yaml
ruby ops/upstream-benchmark-v2.rb run --channel xm-plus --channels config/upstream-benchmarks/channels.yaml --profile config/upstream-benchmarks/mvp-text-responses-v2.yaml --dry-run
```

Expected: all exit 0 and dry-run sends zero requests.

- [ ] **Step 5: Commit**

```bash
git add ops/upstream-benchmark-v2.rb tests/upstream_benchmarks config/upstream-benchmarks
git commit -m "feat: run V2 benchmarks through protocol adapters"
```

### Task 5: Full Offline Regression and Qualification Handoff

**Files:**
- Modify: `docs/project/current-state.md`
- Modify: `docs/project/llm-handoff.md`
- Create: `docs/superpowers/reports/2026-07-21-upstream-benchmark-protocol-adapters-verification.md`

- [ ] **Step 1: Run complete offline verification**

```bash
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_test.rb
ruby -Itest tests/upstream_benchmarks/upstream_benchmark_protocols_test.rb
ruby ops/upstream-benchmark.rb validate
ruby ops/upstream-benchmark-v2.rb validate --profile config/upstream-benchmarks/mvp-text-responses-v2.yaml --pricing config/upstream-benchmarks/pricing-evidence.example.yaml --scenario config/upstream-benchmarks/v2-scenario-neko.example.yaml
bash tests/upstreams/validate-upstream-registry.sh
git diff --check
```

Expected: all exit 0 with no network traffic and no Key.

- [ ] **Step 2: Audit vendor branches and secrets**

```bash
rg -n 'channel.*==.*xm|when\s+["'"']xm|api3\.xmhbao' ops/upstream-benchmark*.rb
rg -n '(sk-[A-Za-z0-9_-]{12,}|Bearer [A-Za-z0-9._-]{12,})' ops config/upstream-benchmarks tests/upstream_benchmarks
```

Expected: no implementation branch and no credential value.

- [ ] **Step 3: Update authority documents and verification report**

Record legacy Chat compatibility, root/versioned Responses fixtures, zero live requests, and the next explicit gate: bounded paid XM Plus/Pro qualification budget and cleanup scope. Keep XM `discovered` and Wawazz current production.

- [ ] **Step 4: Commit**

```bash
git add docs/project/current-state.md docs/project/llm-handoff.md docs/superpowers/reports/2026-07-21-upstream-benchmark-protocol-adapters-verification.md
git commit -m "docs: verify generic upstream protocol adapters"
```
