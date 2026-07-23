# frozen_string_literal: true

require "minitest/autorun"
require_relative "../../ops/upstream-benchmark"
require_relative "../../ops/upstream-benchmark-protocols"

class UpstreamBenchmarkProtocolsTest < Minitest::Test
  class StubResponse
    attr_reader :code, :body

    def initialize(code:, body: "", chunks: [])
      @code = code.to_s
      @body = body
      @chunks = chunks
    end

    def read_body
      @chunks.each { |chunk| yield chunk }
    end
  end

  class RecordingHttpClient < UpstreamBenchmark::HttpClient
    attr_reader :calls

    def initialize(**options)
      super
      @calls = []
    end

    def perform(method, path, payload = nil)
      @calls << { method: method, path: path, payload: payload }
      response = if method == :get
                   StubResponse.new(code: 200, body: JSON.generate("data" => [{ "id" => "gpt-test" }]))
                 elsif payload["stream"]
                   StubResponse.new(code: 200, chunks: [
                     "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n",
                     "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n"
                   ])
                 else
                   StubResponse.new(code: 200, body: JSON.generate(
                     "usage" => { "input_tokens" => 1, "output_tokens" => 2, "total_tokens" => 3 }
                   ))
                 end
      if block_given?
        yield response
        response
      else
        response
      end
    end
  end

  class ErrorHttpClient < RecordingHttpClient
    def perform(method, path, payload = nil)
      @calls << { method: method, path: path, payload: payload }
      StubResponse.new(
        code: 400,
        body: JSON.generate(
          "error" => {
            "type" => "invalid_request_error",
            "message" => "Authorization: Bearer sk-secret-value"
          }
        )
      )
    end
  end

  class OversizedBodyHttpClient < RecordingHttpClient
    def perform(method, path, payload = nil)
      @calls << { method: method, path: path, payload: payload }
      StubResponse.new(
        code: 200,
        body: "x" * (UpstreamBenchmark::HttpClient::MAX_RESPONSE_BYTES + 1)
      )
    end
  end

  def test_responses_adapter_builds_payload_and_normalizes_usage
    adapter = UpstreamBenchmark::Protocols.build(
      "responses",
      terminal_events: ["response.completed", "[DONE]"]
    )

    assert_equal(
      {
        "model" => "gpt-test",
        "input" => "ping",
        "max_output_tokens" => 16,
        "stream" => true
      },
      adapter.generate_request(
        model: "gpt-test",
        prompt: "ping",
        max_output_tokens: 16,
        stream: true
      )
    )
    assert_equal(
      {
        "input_tokens" => 3,
        "output_tokens" => 5,
        "prompt_tokens" => 3,
        "completion_tokens" => 5,
        "total_tokens" => 8
      },
      adapter.normalize_usage(
        "input_tokens" => 3,
        "output_tokens" => 5,
        "total_tokens" => 8
      )
    )
    assert adapter.terminal_event?("type" => "response.completed")
    assert adapter.terminal_event?("[DONE]")
    refute adapter.terminal_event?("type" => "response.output_text.delta")
  end

  def test_chat_adapter_preserves_chat_wire_and_common_usage
    adapter = UpstreamBenchmark::Protocols.build(
      "chat_completions",
      terminal_events: ["[DONE]"]
    )

    assert_equal(
      {
        "model" => "gpt-test",
        "messages" => [{ "role" => "user", "content" => "ping" }],
        "max_tokens" => 16,
        "stream" => false
      },
      adapter.generate_request(
        model: "gpt-test",
        prompt: "ping",
        max_output_tokens: 16,
        stream: false
      )
    )
    usage = adapter.normalize_usage(
      "prompt_tokens" => 2,
      "completion_tokens" => 4,
      "total_tokens" => 6
    )
    assert_equal 2, usage.fetch("input_tokens")
    assert_equal 4, usage.fetch("output_tokens")
    assert_equal 6, usage.fetch("total_tokens")
    assert adapter.terminal_event?("[DONE]")
  end

  def test_adapter_owns_model_catalog_and_redacted_error_classification
    adapter = UpstreamBenchmark::Protocols.build(
      "responses",
      terminal_events: ["response.completed"]
    )

    assert_equal(
      { method: :get, path: "/models", payload: nil },
      adapter.models_request(path: "/models")
    )
    assert_equal(
      ["gpt-a", "gpt-b"],
      adapter.parse_models(
        "data" => [{ "id" => "gpt-a" }, { "id" => "gpt-b" }, { "object" => "model" }]
      )
    )
    assert_equal "request_rejected", adapter.classify_error("error" => { "type" => "invalid_request_error" })
    assert_equal "rate_limited", adapter.classify_error("error" => { "code" => "rate_limit_exceeded" })
    assert_equal "protocol_error", adapter.classify_error("error" => { "type" => "private_vendor_customer_123" })
    refute_match(
      /sk-secret/,
      adapter.classify_error("error" => { "message" => "Bearer sk-secret-value" })
    )
  end

  def test_registry_rejects_vendor_names
    error = assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmark::Protocols.build("xm", terminal_events: [])
    end
    assert_match(/unsupported benchmark protocol/, error.message)
  end

  def test_sse_parser_uses_responses_terminal_and_usage_contract
    adapter = UpstreamBenchmark::Protocols.build(
      "responses",
      terminal_events: ["response.completed"]
    )
    parser = UpstreamBenchmark::SseParser.new(adapter: adapter)
    parser.feed("data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n")
    refute parser.complete?
    parser.feed("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n")

    assert parser.complete?
    assert_equal 1, parser.usage.fetch("input_tokens")
    assert_equal 2, parser.usage.fetch("output_tokens")
    assert_equal 3, parser.usage.fetch("total_tokens")
    refute_match(/OK/, JSON.generate(parser.summary))
  end

  def test_sse_parser_accepts_crlf_event_boundaries
    adapter = UpstreamBenchmark::Protocols.build(
      "responses",
      terminal_events: ["response.completed"]
    )
    parser = UpstreamBenchmark::SseParser.new(adapter: adapter)
    parser.feed("data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\r\n\r\n")
    parser.feed("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\r\n\r\n")

    assert parser.complete?
    assert_equal 3, parser.usage.fetch("total_tokens")
  end

  def test_sse_parser_rejects_an_unbounded_response
    parser = UpstreamBenchmark::SseParser.new(max_bytes: 32)
    error = assert_raises(UpstreamBenchmark::ResponseTooLarge) do
      parser.feed("x" * 33)
    end
    assert_match(/response exceeds/i, error.message)
  end

  def test_http_client_uses_generic_profile_paths_for_responses
    adapter = UpstreamBenchmark::Protocols.build(
      "responses",
      terminal_events: ["response.completed"]
    )
    client = RecordingHttpClient.new(
      base_url: "https://api.example.invalid",
      api_key: "runtime-only",
      timeout_seconds: 2,
      adapter: adapter,
      models_path: "/models",
      generate_path: "/responses"
    )

    assert_equal ["gpt-test"], client.models.fetch("models")
    sync = client.generate(model: "gpt-test", prompt: "ping", max_output_tokens: 8, stream: false)
    stream = client.generate(model: "gpt-test", prompt: "ping", max_output_tokens: 8, stream: true)

    assert_equal ["/models", "/responses", "/responses"], client.calls.map { |call| call.fetch(:path) }
    assert_equal "ping", client.calls[1].fetch(:payload).fetch("input")
    assert_equal 3, sync.dig("usage", "total_tokens")
    assert_equal true, stream.fetch("stream_complete")
    assert_equal 3, stream.dig("usage", "total_tokens")
  end

  def test_http_client_uses_adapter_error_category_without_message_leak
    client = ErrorHttpClient.new(
      base_url: "https://api.example.invalid",
      api_key: "runtime-only",
      timeout_seconds: 2,
      adapter: UpstreamBenchmark::Protocols.build(
        "responses",
        terminal_events: ["response.completed"]
      ),
      models_path: "/models",
      generate_path: "/responses"
    )

    result = client.generate(
      model: "gpt-test",
      prompt: "ping",
      max_output_tokens: 8,
      stream: false
    )

    assert_equal 400, result.fetch("status")
    assert_equal "request_rejected", result.fetch("error")
    refute_match(/secret|Bearer/, JSON.generate(result))
  end

  def test_http_client_rejects_oversized_json_response
    client = OversizedBodyHttpClient.new(
      base_url: "https://api.example.invalid",
      api_key: "runtime-only",
      timeout_seconds: 2,
      adapter: UpstreamBenchmark::Protocols.build(
        "responses",
        terminal_events: ["response.completed"]
      ),
      models_path: "/models",
      generate_path: "/responses"
    )

    result = client.models
    assert_equal 0, result.fetch("status")
    assert_equal "response_too_large", result.fetch("error")
  end
end
