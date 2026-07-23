# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "socket"
require "tmpdir"

require_relative "../../ops/collect-account-quality-pulse"

class CollectAccountQualityPulseTest < Minitest::Test
  NOW = Time.utc(2026, 7, 23, 0, 0, 0)

  class FakeClient
    attr_reader :probe_calls

    def initialize(accounts:, details:, models:, probes:)
      @accounts = accounts
      @details = details
      @models = models
      @probes = probes
      @probe_calls = []
    end

    def accounts
      @accounts
    end

    def account(account_id)
      @details.fetch(account_id)
    end

    def models(account_id)
      @models.fetch(account_id)
    end

    def probe(account_id:, model_id:, prompt:)
      @probe_calls << [account_id, model_id, prompt]
      @probes.fetch(account_id)
    end
  end

  def test_collects_only_active_schedulable_accounts_and_isolates_balance_failure
    client = FakeClient.new(
      accounts: [
        {"id" => 22, "status" => "active", "schedulable" => true},
        {"id" => 23, "status" => "disabled", "schedulable" => false},
        {"id" => 21, "status" => "active", "schedulable" => true}
      ],
      details: {
        21 => {"id" => 21, "rate_multiplier" => 0.05},
        22 => {"id" => 22, "rate_multiplier" => 0.10}
      },
      models: {
        21 => ["gpt-5.5", "gpt-5.6-sol", "dall-e-3"],
        22 => ["gpt-5.6-terra"]
      },
      probes: {
        21 => AccountQualityPulse::ProbeResult.new(result: "passed", ttft_ms: 120, error_code: ""),
        22 => AccountQualityPulse::ProbeResult.new(result: "balance_exhausted", ttft_ms: nil, error_code: "balance_exhausted")
      }
    )

    output = AccountQualityPulse::Collector.new(client: client, now: NOW).collect(history: [])
    accounts = output.fetch("result").fetch("accounts")

    assert_equal [21, 22], accounts.map { |item| item.fetch("account_id") }
    assert_equal "passed", accounts[0].fetch("last_result")
    assert_equal 120.0, accounts[0].fetch("ttft_p95_ms")
    assert_equal "balance_exhausted", accounts[1].fetch("last_result")
    assert_nil accounts[1].fetch("ttft_p95_ms")
    assert_equal [
      [21, "gpt-5.6-sol", "Reply with OK only."],
      [22, "gpt-5.6-terra", "Reply with OK only."]
    ], client.probe_calls
  end

  def test_one_account_error_does_not_stop_a_later_account
    client = FakeClient.new(
      accounts: [
        {"id" => 1, "status" => "active", "schedulable" => true},
        {"id" => 2, "status" => "active", "schedulable" => true}
      ],
      details: {
        1 => {"id" => 1, "rate_multiplier" => 0.1},
        2 => {"id" => 2, "rate_multiplier" => 0.2}
      },
      models: {1 => ["gpt-5.6"], 2 => ["gpt-5.6"]},
      probes: {
        1 => AccountQualityPulse::ProbeResult.new(result: "timeout", ttft_ms: nil, error_code: "timeout"),
        2 => AccountQualityPulse::ProbeResult.new(result: "passed", ttft_ms: 80, error_code: "")
      }
    )

    accounts = AccountQualityPulse::Collector.new(client: client, now: NOW)
                                         .collect(history: []).fetch("result").fetch("accounts")

    assert_equal %w[timeout passed], accounts.map { |item| item.fetch("last_result") }
    assert_equal [1, 2], client.probe_calls.map(&:first)
  end

  def test_model_selection_is_numeric_deterministic_and_provider_neutral
    assert_equal "gpt-5.10-alpha", AccountQualityPulse::ModelSelector.select([
      "gpt-5.9-sol", "gpt-5.10-zeta", "gpt-5.10-alpha", "dall-e-3"
    ])
    assert_equal "claude-sonnet", AccountQualityPulse::ModelSelector.select([
      "whisper-1", "claude-sonnet", "z-unknown"
    ])
    assert_nil AccountQualityPulse::ModelSelector.select(["dall-e-3", "whisper-1"])
  end

  def test_no_text_model_is_recorded_without_calling_the_probe
    client = FakeClient.new(
      accounts: [{"id" => 21, "status" => "active", "schedulable" => true}],
      details: {21 => {"id" => 21, "rate_multiplier" => 0.05}},
      models: {21 => ["dall-e-3", "whisper-1"]},
      probes: {}
    )

    account = AccountQualityPulse::Collector.new(client: client, now: NOW)
                                           .collect(history: []).fetch("result").fetch("accounts").first

    assert_equal "model_unavailable", account.fetch("last_result")
    assert_empty client.probe_calls
  end

  def test_rolling_metrics_prune_old_samples_and_ignore_failed_ttft
    history = [
      sample(21, "passed", 300, NOW - 25 * 60 * 60),
      sample(21, "passed", 100, NOW - 10 * 60),
      sample(21, "timeout", nil, NOW - 5 * 60)
    ]
    client = FakeClient.new(
      accounts: [{"id" => 21, "status" => "active", "schedulable" => true}],
      details: {21 => {"id" => 21, "rate_multiplier" => 0.05}},
      models: {21 => ["gpt-5.6-sol"]},
      probes: {21 => AccountQualityPulse::ProbeResult.new(result: "passed", ttft_ms: 200, error_code: "")}
    )

    output = AccountQualityPulse::Collector.new(client: client, now: NOW).collect(history: history)
    account = output.fetch("result").fetch("accounts").first

    assert_equal 3, account.fetch("sample_count")
    assert_equal 2, account.fetch("success_count")
    assert_in_delta 2.0 / 3.0, account.fetch("success_rate"), 0.0001
    assert_equal 100.0, account.fetch("ttft_p50_ms")
    assert_equal 200.0, account.fetch("ttft_p95_ms")
    assert_equal 3, output.fetch("history").length
  end

  def test_stream_parser_discards_content_and_classifies_only_explicit_balance_errors
    parser = AccountQualityPulse::SSEParser.new
    parser.feed("data: {\"type\":\"status\",\"text\":\"working\"}\n\n")
    parser.feed("data: {\"type\":\"content\",\"text\":\"fixture response text\"}\n\n")

    assert parser.first_content?
    refute_includes JSON.generate(parser.to_h), "fixture response text"
    assert_equal "balance_exhausted", AccountQualityPulse.classify_error("Insufficient balance")
    assert_equal "balance_exhausted", AccountQualityPulse.classify_error("credit is exhausted")
    assert_equal "account_test_error", AccountQualityPulse.classify_error("rate limit quota window")
  end

  def test_stream_parser_handles_split_events_and_rejects_malformed_json
    parser = AccountQualityPulse::SSEParser.new
    parser.feed("data: {\"type\":\"content\",")
    parser.feed("\"text\":\"OK\"}\n\n")

    assert parser.first_content?
    refute parser.malformed?

    malformed = AccountQualityPulse::SSEParser.new.feed("data: not-json\n\n").finish
    assert malformed.malformed?
    refute malformed.first_content?
  end

  def test_native_account_list_rejects_a_truncated_first_page
    client = AccountQualityPulse::NativeClient.allocate
    client.define_singleton_method(:json_request) do |_method, _path|
      {"items" => [{"id" => 1}], "total" => 101}
    end

    assert_raises(AccountQualityPulse::ValidationError) { client.accounts }
  end

  def test_native_probe_maps_timeout_and_generic_http_error_without_leaking_body
    timeout_client = allocated_native_client
    timeout_client.define_singleton_method(:with_http) { |_uri| raise Net::ReadTimeout }
    timeout = timeout_client.probe(account_id: 21, model_id: "gpt-5.6-sol", prompt: "Reply with OK only.")

    assert_equal "timeout", timeout.result
    assert_equal "timeout", timeout.error_code

    http_client = allocated_native_client
    http_client.define_singleton_method(:with_http) do |_uri, &block|
      response = Struct.new(:code) do
        def read_body
          yield "fixture forbidden upstream body"
        end
      end.new("503")
      transport = Object.new
      transport.define_singleton_method(:request) { |_request, &response_block| response_block.call(response) }
      block.call(transport)
    end
    http_error = http_client.probe(account_id: 21, model_id: "gpt-5.6-sol", prompt: "Reply with OK only.")

    assert_equal "http_error", http_error.result
    assert_equal "http_error", http_error.error_code
    refute_includes JSON.generate(http_error.to_h), "fixture forbidden upstream body"
  end

  def test_native_http_contract_and_sse_content_are_secret_free
    fixture = NativeFixture.new
    fixture.start
    Dir.mktmpdir("account-quality-native") do |dir|
      key_path = File.join(dir, "admin-key")
      File.write(key_path, "fixture-admin-key\n")
      File.chmod(0o600, key_path)
      client = AccountQualityPulse::NativeClient.new(base_url: fixture.base_url, admin_key_file: key_path)

      output = AccountQualityPulse::Collector.new(client: client, now: NOW).collect(history: [])
      accounts = output.fetch("result").fetch("accounts")

      assert_equal [21, 22], accounts.map { |item| item.fetch("account_id") }
      assert_equal %w[passed balance_exhausted], accounts.map { |item| item.fetch("last_result") }
      assert_operator accounts.first.fetch("ttft_p95_ms"), :>=, 0
      refute_includes JSON.generate(output), "fixture response text"
      refute fixture.requests.any? { |request| request.fetch(:path).include?("/23/") }
      assert fixture.requests.select { |request| request.fetch(:method) == "POST" }.all? do |request|
        JSON.parse(request.fetch(:body)) == {"model_id" => request.fetch(:path).include?("/21/") ? "gpt-5.6-sol" : "gpt-5.6-terra",
                                             "prompt" => "Reply with OK only."}
      end
    end
  ensure
    fixture&.stop
  end

  def test_publisher_writes_restricted_secret_free_files
    document = {
      "schema_version" => 1,
      "snapshot_id" => "ACCOUNT-QUALITY-20260723T000000Z",
      "observed_at" => NOW.iso8601,
      "account_set_sha256" => "a" * 64,
      "accounts" => []
    }
    Dir.mktmpdir("account-quality") do |dir|
      result_path = File.join(dir, "account-quality-result.json")
      history_path = File.join(dir, "account-quality-history.json")

      AccountQualityPulse::Publisher.publish(result_path: result_path, history_path: history_path,
                                             result: document, history: [])

      assert_equal 0o600, File.stat(result_path).mode & 0o777
      assert_equal 0o600, File.stat(history_path).mode & 0o777
      assert_equal document, JSON.parse(File.read(result_path))
    end
  end

  private

  class NativeFixture
    attr_reader :requests

    def initialize
      @server = TCPServer.new("127.0.0.1", 0)
      @requests = []
      @stopped = false
    end

    def base_url
      "http://127.0.0.1:#{@server.local_address.ip_port}"
    end

    def start
      @thread = Thread.new do
        until @stopped
          begin
            socket = @server.accept
            serve(socket)
          rescue IOError, Errno::EBADF
            break
          end
        end
      end
    end

    def stop
      @stopped = true
      @server.close
      @thread&.join(1)
    end

    private

    def serve(socket)
      request_line = socket.gets
      return unless request_line

      method, path, = request_line.split(" ")
      headers = {}
      while (line = socket.gets)
        break if line == "\r\n"

        key, value = line.split(":", 2)
        headers[key.downcase] = value.to_s.strip
      end
      body = socket.read(headers.fetch("content-length", "0").to_i)
      @requests << {method: method, path: path, body: body}
      status, content_type, response_body = response_for(method, path)
      socket.write("HTTP/1.1 #{status}\r\nContent-Type: #{content_type}\r\nContent-Length: #{response_body.bytesize}\r\nConnection: close\r\n\r\n")
      socket.write(response_body)
    rescue Errno::EPIPE, Errno::ECONNRESET
      nil
    ensure
      socket.close
    end

    def response_for(method, path)
      case [method, path]
      when ["GET", "/api/v1/admin/accounts?page=1&page_size=100"]
        json_response("items" => [
          {"id" => 22, "status" => "active", "schedulable" => true},
          {"id" => 23, "status" => "disabled", "schedulable" => false},
          {"id" => 21, "status" => "active", "schedulable" => true}
        ], "total" => 3)
      when ["GET", "/api/v1/admin/accounts/21"]
        json_response("id" => 21, "rate_multiplier" => 0.05)
      when ["GET", "/api/v1/admin/accounts/22"]
        json_response("id" => 22, "rate_multiplier" => 0.10)
      when ["GET", "/api/v1/admin/accounts/21/models"]
        json_response([{"id" => "gpt-5.6-sol"}])
      when ["GET", "/api/v1/admin/accounts/22/models"]
        json_response([{"id" => "gpt-5.6-terra"}])
      when ["POST", "/api/v1/admin/accounts/21/test"]
        ["200 OK", "text/event-stream", "data: {\"type\":\"status\",\"text\":\"working\"}\n\ndata: {\"type\":\"content\",\"text\":\"fixture response text\"}\n\n"]
      when ["POST", "/api/v1/admin/accounts/22/test"]
        ["200 OK", "text/event-stream", "data: {\"type\":\"error\",\"error\":\"Insufficient balance\"}\n\n"]
      else
        ["500 Internal Server Error", "application/json", "{}"]
      end
    end

    def json_response(data)
      ["200 OK", "application/json", JSON.generate("data" => data)]
    end
  end

  def sample(account_id, result, ttft_ms, observed_at)
    {
      "account_id" => account_id,
      "model_id" => "gpt-5.6-sol",
      "rate_multiplier" => 0.05,
      "result" => result,
      "error_code" => result == "passed" ? "" : result,
      "ttft_ms" => ttft_ms,
      "observed_at" => observed_at.iso8601
    }
  end

  def allocated_native_client
    AccountQualityPulse::NativeClient.allocate.tap do |client|
      client.instance_variable_set(:@base_url, "http://127.0.0.1")
      client.instance_variable_set(:@admin_key, "fixture-key")
    end
  end
end
