# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "stringio"
require "tmpdir"
require "yaml"
require_relative "../../ops/upstream-benchmark-v2"

class UpstreamBenchmarkV2Test < Minitest::Test
  class ScriptedClient
    attr_reader :calls

    def initialize(models:, stream_complete: true)
      @models = models
      @stream_complete = stream_complete
      @calls = []
      @mutex = Mutex.new
    end

    def models
      { "status" => 200, "models" => @models, "duration_ms" => 1.0 }
    end

    def chat(model:, prompt:, max_output_tokens:, stream:)
      @mutex.synchronize { @calls << { "model" => model, "stream" => stream } }
      {
        "status" => 200,
        "duration_ms" => 1.0,
        "first_event_ms" => stream ? 0.5 : nil,
        "stream_complete" => stream ? @stream_complete : nil,
        "usage" => { "prompt_tokens" => 4, "completion_tokens" => 1, "total_tokens" => 5 }
      }.compact
    end

    alias generate chat
  end

  class GenerateOnlyClient
    attr_reader :calls

    def initialize
      @calls = []
    end

    def models
      { "status" => 200, "models" => ["gpt-a"], "duration_ms" => 1.0 }
    end

    def generate(model:, prompt:, max_output_tokens:, stream:)
      @calls << { "model" => model, "stream" => stream }
      {
        "status" => 200,
        "duration_ms" => 1.0,
        "first_event_ms" => stream ? 0.5 : nil,
        "stream_complete" => stream ? true : nil,
        "usage" => {
          "input_tokens" => 4,
          "output_tokens" => 1,
          "prompt_tokens" => 4,
          "completion_tokens" => 1,
          "total_tokens" => 5
        }
      }.compact
    end
  end

  class DiscoveryClient
    attr_reader :models_calls, :generate_calls

    def initialize(result)
      @result = result
      @models_calls = 0
      @generate_calls = 0
    end

    def models
      @models_calls += 1
      @result
    end

    def generate(**)
      @generate_calls += 1
      raise "discovery must not generate"
    end
  end

  class FixedFailureClient
    def models
      { "status" => 200, "models" => ["gpt-common"], "duration_ms" => 1.0 }
    end

    def generate(model:, prompt:, max_output_tokens:, stream:)
      return {
        "status" => 0,
        "duration_ms" => 45_000.0,
        "error" => "timeout",
        "stream_complete" => false
      } if stream

      { "status" => 429, "duration_ms" => 20.0, "error" => "rate_limited" }
    end
  end

  def profile_document
    {
      "schema_version" => 2,
      "id" => "mvp-text-v2",
      "endpoint" => "chat_completions",
      "prompt" => "Reply with OK only.",
      "max_output_tokens" => 8,
      "timeout_seconds" => 45,
      "concurrency_levels" => [1, 2, 3, 5, 8, 10],
      "rpm_levels" => [6, 12, 20, 30],
      "rpm_window_seconds" => 10
    }
  end

  def fast_profile_document
    profile_document.merge(
      "id" => "quality-first-fast-v1",
      "representative_roles" => {
        "common" => "gpt-common",
        "expensive" => "gpt-expensive",
        "new" => "gpt-new"
      }
    )
  end

  def test_profile_accepts_v2_bounds
    profile = UpstreamBenchmarkV2::Profile.new(profile_document)

    assert_equal [1, 2, 3, 5, 8, 10], profile.concurrency_levels
    assert_equal [6, 12, 20, 30], profile.rpm_levels
    assert_equal 10, profile.rpm_window_seconds
  end

  def test_profile_normalizes_legacy_chat_and_accepts_responses
    legacy = UpstreamBenchmarkV2::Profile.new(profile_document)
    assert_equal "chat_completions", legacy.protocol
    assert_equal "/models", legacy.models_path
    assert_equal "/chat/completions", legacy.generate_path
    assert_equal ["[DONE]"], legacy.terminal_events

    responses = UpstreamBenchmarkV2::Profile.new(profile_document.reject { |key, _| key == "endpoint" }.merge(
      "protocol" => "responses",
      "models_path" => "/models",
      "generate_path" => "/responses",
      "terminal_events" => ["response.completed", "[DONE]"]
    ))
    assert_equal "responses", responses.protocol
    assert_equal "/models", responses.models_path
    assert_equal "/responses", responses.generate_path
    assert_equal ["response.completed", "[DONE]"], responses.terminal_events
  end

  def test_profile_rejects_unsafe_protocol_paths
    [
      "https://evil.invalid/responses",
      "responses",
      "/../responses",
      "/responses?q=1",
      "/responses#fragment",
      "//evil.invalid/responses",
      "/%2e%2e/admin",
      "/safe/%2E%2e/admin",
      "/%252e%252e/admin"
    ].each do |path|
      document = profile_document.reject { |key, _| key == "endpoint" }.merge(
        "protocol" => "responses",
        "models_path" => "/models",
        "generate_path" => path,
        "terminal_events" => ["response.completed"]
      )
      error = assert_raises(UpstreamBenchmark::ValidationError, path) do
        UpstreamBenchmarkV2::Profile.new(document)
      end
      assert_match(/safe absolute request path/, error.message)
    end
  end

  def test_profile_rejects_unbounded_workload
    invalid = profile_document.merge("max_output_tokens" => 513)

    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkV2::Profile.new(invalid)
    end
  end

  def test_catalog_classifies_non_text_before_text_fallback
    assert_equal "image", UpstreamBenchmarkV2::ModelCatalog.classify("dall-e-3")
    assert_equal "audio", UpstreamBenchmarkV2::ModelCatalog.classify("whisper-1")
    assert_equal "realtime", UpstreamBenchmarkV2::ModelCatalog.classify("gpt-4o-realtime-preview")
    assert_equal "text", UpstreamBenchmarkV2::ModelCatalog.classify("gpt-5.6-sol")
    assert_equal "unknown", UpstreamBenchmarkV2::ModelCatalog.classify("vendor-special-01")
  end

  def test_catalog_discover_marks_only_text_models_testable
    catalog = UpstreamBenchmarkV2::ModelCatalog.discover(%w[gpt-5.6-sol dall-e-3 vendor-special-01])

    assert_equal true, catalog.fetch("gpt-5.6-sol").fetch("testable")
    assert_equal false, catalog.fetch("dall-e-3").fetch("testable")
    assert_equal false, catalog.fetch("vendor-special-01").fetch("testable")
  end

  def test_discovery_runner_calls_models_once_and_never_generates
    client = DiscoveryClient.new(
      "status" => 200,
      "models" => ["vendor-special-01", { "id" => "gpt-5.6-sol" }, "gpt-5.6-sol", "dall-e-3", "text-gpt-model"],
      "duration_ms" => 12.5
    )

    record = UpstreamBenchmarkV2::DiscoveryRunner.new(client: client, profile: UpstreamBenchmarkV2::Profile.new(profile_document))
      .run(channel_id: "any-relay")

    assert_equal 1, client.models_calls
    assert_equal 0, client.generate_calls
    assert_equal "partial", record.fetch("status")
    assert_equal "live_direct", record.fetch("evidence_source")
    assert_equal "discovered_not_qualified", record.fetch("qualification_status")
    assert_equal 1, record.dig("metrics", "request_count")
    assert_equal 0, record.dig("metrics", "generation_request_count")
    assert_equal 4, record.dig("metrics", "model_count")
    assert_equal ["dall-e-3", "gpt-5.6-sol", "text-gpt-model", "vendor-special-01"], record.dig("metrics", "model_ids")
    assert_equal ["gpt-5.6-sol", "text-gpt-model"], record.dig("metrics", "testable_model_ids")
    assert_equal(
      { "id" => "dall-e-3", "kind" => "image", "testable" => false },
      record.dig("metrics", "classifications", 0)
    )
    assert_equal 12.5, record.dig("metrics", "latency_ms")
    assert_empty record.fetch("errors")
  end

  def test_discovery_runner_replaces_raw_failure_with_fixed_category
    client = DiscoveryClient.new(
      "status" => 0,
      "models" => [],
      "duration_ms" => 4.0,
      "error" => "sk-sensitive-upstream-message"
    )

    record = UpstreamBenchmarkV2::DiscoveryRunner.new(client: client, profile: UpstreamBenchmarkV2::Profile.new(profile_document))
      .run(channel_id: "any-relay")
    serialized = JSON.generate(record)

    assert_equal "failed", record.fetch("status")
    assert_equal [{ "stage" => "models", "category" => "protocol_error" }], record.fetch("errors")
    refute_includes serialized, "sk-sensitive-upstream-message"
    assert_equal 1, client.models_calls
    assert_equal 0, client.generate_calls
  end

  def test_pricing_evidence_accepts_non_sensitive_model_prices
    evidence = {
      "schema_version" => 1,
      "channel_id" => "neko",
      "currency" => "USD",
      "models" => {
        "gpt-5.6-sol" => {
          "input" => 1.25e-6,
          "output" => 10.0e-6,
          "cache_read" => 0.125e-6,
          "cache_write" => nil,
          "source" => "provider-dashboard",
          "verified_at" => "2026-07-19"
        }
      }
    }

    assert_equal true, UpstreamBenchmarkV2::PricingEvidence.validate!(evidence)
  end

  def test_pricing_evidence_rejects_secret_shaped_fields
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkV2::PricingEvidence.validate!("api_key" => "temporary")
    end
  end

  def test_runner_tests_each_text_model_sync_and_stream
    client = ScriptedClient.new(models: ["gpt-a", "gpt-b", "dall-e-3"])
    runner = UpstreamBenchmarkV2::Runner.new(
      client: client,
      profile: UpstreamBenchmarkV2::Profile.new(profile_document),
      clock: -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) },
      sleeper: ->(_seconds) {}
    )

    record = runner.run(channel_id: "neko")

    assert_equal %w[gpt-a gpt-b], record.dig("metrics", "text_models")
    assert_equal 1, record.dig("metrics", "per_model", "gpt-a", "sync", "success_count")
    assert_equal true, record.dig("metrics", "per_model", "gpt-b", "stream", "complete")
    assert_equal "image", record.dig("metrics", "catalog", "dall-e-3", "kind")
    assert_operator client.calls.count { |call| call["model"] == "gpt-a" }, :>=, 2
  end

  def test_runner_uses_generic_generate_contract
    client = GenerateOnlyClient.new
    record = UpstreamBenchmarkV2::Runner.new(
      client: client,
      profile: UpstreamBenchmarkV2::Profile.new(profile_document),
      sleeper: ->(_seconds) {}
    ).run(channel_id: "generic-channel")

    assert_equal "passed", record.fetch("status")
    assert_operator client.calls.length, :>=, 2
    assert_equal 10, record.dig("metrics", "per_model", "gpt-a", "usage", "total_tokens")
  end

  def test_runner_marks_incomplete_stream_as_partial
    record = UpstreamBenchmarkV2::Runner.new(
      client: ScriptedClient.new(models: ["gpt-a"], stream_complete: false),
      profile: UpstreamBenchmarkV2::Profile.new(profile_document),
      sleeper: ->(_seconds) {}
    ).run(channel_id: "neko")

    assert_equal "partial", record.fetch("status")
    assert_equal false, record.dig("metrics", "per_model", "gpt-a", "stream", "complete")
  end

  def test_candidate_watch_uses_only_configured_representative_models_without_capacity
	client = ScriptedClient.new(models: ["gpt-a", "gpt-b", "dall-e-3"])
	profile = UpstreamBenchmarkV2::Profile.new(
	  profile_document.merge("representative_models" => ["gpt-b"])
	)

	record = UpstreamBenchmarkV2::CandidateWatchRunner.new(client: client, profile: profile).run(channel_id: "candidate")

	assert_equal %w[gpt-a gpt-b], record.dig("metrics", "text_models")
	assert_equal ["gpt-b"], record.dig("metrics", "probed_models")
	assert_equal [{ "model" => "gpt-b", "stream" => false }, { "model" => "gpt-b", "stream" => true }], client.calls
	assert_equal 1, record.dig("metrics", "per_model", "gpt-b", "sync", "success_count")
	assert_equal true, record.dig("metrics", "per_model", "gpt-b", "stream", "complete")
	assert_in_delta 0.5, record.dig("metrics", "per_model", "gpt-b", "stream", "first_event_ms"), 0.001
	assert_equal 10, record.dig("metrics", "usage", "total_tokens")
	refute record.dig("metrics").key?("capacity")
	refute_match(/concurrency|rpm/i, JSON.generate(record))
  end

  def test_candidate_watch_redacts_secret_shaped_failures
	client = Class.new do
	  def models
		{ "status" => 200, "models" => ["gpt-a"] }
	  end

	  def chat(**)
		raise "Authorization: Bearer sk-secret-value"
	  end
	end.new
	profile = UpstreamBenchmarkV2::Profile.new(profile_document.merge("representative_models" => ["gpt-a"]))

	record = UpstreamBenchmarkV2::CandidateWatchRunner.new(client: client, profile: profile).run(channel_id: "candidate")
	clean = JSON.generate(UpstreamBenchmark::Redactor.clean(record))

	refute_includes clean, "sk-secret-value"
	assert_equal "failed", record.fetch("status")
  end

  def test_capacity_stops_after_rate_limit_and_keeps_previous_stable_level
    calls = 0
    probe = UpstreamBenchmarkV2::CapacityProbe.new(
      invoke: lambda {
        calls += 1
        calls > 6 ? { "status" => 429, "duration_ms" => 1.0 } : { "status" => 200, "duration_ms" => 1.0 }
      },
      profile: UpstreamBenchmarkV2::Profile.new(profile_document),
      sleeper: ->(_seconds) {}
    )

    result = probe.run

    assert_equal 3, result.dig("concurrency", "last_stable")
    assert_equal "rate_limited", result.dig("concurrency", "stop_reason")
  end

  def test_capacity_reports_at_least_when_highest_concurrency_passes
    probe = UpstreamBenchmarkV2::CapacityProbe.new(
      invoke: -> { { "status" => 200, "duration_ms" => 1.0 } },
      profile: UpstreamBenchmarkV2::Profile.new(profile_document),
      sleeper: ->(_seconds) {}
    )

    result = probe.run

    assert_equal 10, result.dig("concurrency", "last_stable")
    assert_equal "at_least", result.dig("concurrency", "limit")
    assert_equal 8, result.dig("recommendation", "concurrency")
  end

  def test_capacity_stops_when_latency_is_clearly_queued
    calls = 0
    probe = UpstreamBenchmarkV2::CapacityProbe.new(
      invoke: lambda {
        calls += 1
        { "status" => 200, "duration_ms" => calls == 1 ? 1.0 : 5.0 }
      },
      profile: UpstreamBenchmarkV2::Profile.new(profile_document),
      sleeper: ->(_seconds) {}
    )

    result = probe.run

    assert_equal 1, result.dig("concurrency", "last_stable")
    assert_equal "queueing_detected", result.dig("concurrency", "stop_reason")
  end

  def neko_pricing_evidence
    {
      "schema_version" => 1,
      "channel_id" => "neko",
      "currency" => "USD",
      "models" => {
        "gpt-a" => {
          "input" => 1.25e-6,
          "output" => 10.0e-6,
          "cache_read" => 0.125e-6,
          "cache_write" => nil,
          "source" => "provider-dashboard",
          "verified_at" => "2026-07-19",
          "actual_multiplier" => 0.07,
          "billing_reconciliation" => "verified"
        }
      }
    }
  end

  def pricing_scenario
    {
      "failure_reserve_rate" => 0.10,
      "target_margin_rate" => 0.50,
      "payment_fee_rate" => 0.03,
      "recommendation_increment" => 0.01,
      "recommendation_buffer" => 0.01,
      "monthly_fixed_cost_usd" => 0.0,
      "monthly_standard_usage_usd" => 1.0,
      "internal_group_multiplier" => 1.0
    }
  end

  def test_pricing_advisor_recommends_neko_point_eighteen
    result = UpstreamBenchmarkV2::PricingAdvisor.new(
      evidence: neko_pricing_evidence,
      scenario: pricing_scenario
    ).calculate

    assert_in_delta 0.154, result.fetch("commercial").fetch("variable_floor"), 0.0001
    assert_in_delta 0.18, result.fetch("commercial").fetch("recommended_multiplier"), 0.0001
    assert_equal 1.0, result.fetch("internal").fetch("group_multiplier")
    assert_equal ["gpt-a"], result.fetch("openable_models")
  end

  def test_unknown_input_price_blocks_model_from_proposal
    evidence = neko_pricing_evidence
    evidence["models"]["gpt-unknown"] = evidence["models"]["gpt-a"].merge("input" => nil)

    result = UpstreamBenchmarkV2::PricingAdvisor.new(
      evidence: evidence,
      scenario: pricing_scenario
    ).calculate

    refute_includes result.fetch("openable_models"), "gpt-unknown"
    assert_equal "unknown", result.dig("models", "gpt-unknown", "status")
  end

  def test_pricing_scenario_rejects_non_numeric_internal_multiplier
    invalid = pricing_scenario.merge("internal_group_multiplier" => "1.0")

    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkV2::Scenario.validate!(invalid)
    end
  end

  def test_proposal_builder_is_secret_free_and_contains_requested_billing
    pricing = UpstreamBenchmarkV2::PricingAdvisor.new(
      evidence: neko_pricing_evidence,
      scenario: pricing_scenario
    ).calculate
    run = {
      "channel_id" => "neko",
      "run_id" => "run-v2",
      "metrics" => {
        "catalog" => { "gpt-a" => { "kind" => "text", "testable" => true } },
        "capacity" => { "recommendation" => { "concurrency" => 2, "rpm" => 20 } }
      }
    }

    proposal = UpstreamBenchmarkV2::ProposalBuilder.build(
      run: run,
      pricing: pricing,
      proposal_id: "proposal-v2",
      generated_at: "2026-07-19T12:00:00Z"
    )

    assert_equal "requested", proposal.dig("sub2api", "billing_model_source")
    assert_equal true, proposal.dig("sub2api", "restrict_models")
    assert_equal ["gpt-a"], proposal.fetch("models").map { |model| model.fetch("public_name") }
    assert_match(/\A[0-9a-f]{64}\z/, proposal.fetch("proposal_hash"))
    refute_match(/key|authorization|secret/i, JSON.generate(proposal))
  end

  def test_cli_dry_run_does_not_send_network
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "profile.yaml")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "neko", "display_name" => "Neko", "base_url" => "https://api.example.com/v1",
          "protocol" => "openai_compatible", "resale_permission" => "unknown", "lifecycle" => "candidate"
        }]
      ))
      File.write(profile, YAML.dump(profile_document))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "run", "--channels", channels, "--profile", profile,
        "--channel", "neko", "--key-env", "UNUSED", "--dry-run"
      ], out: output, err: error)
      result = JSON.parse(output.string)
      assert_equal false, result.fetch("network_sent")
      assert_equal true, result.fetch("capacity_probe_bounded")
      assert_empty error.string
    end
  end

  def test_cli_responses_dry_run_needs_no_key_and_sends_zero_requests
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "responses.yaml")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "any-relay",
          "display_name" => "Any Relay",
          "base_url" => "https://api.example.com",
          "protocol" => "openai_compatible",
          "resale_permission" => "unknown",
          "lifecycle" => "discovered"
        }]
      ))
      File.write(profile, YAML.dump(profile_document.reject { |key, _| key == "endpoint" }.merge(
        "protocol" => "responses",
        "models_path" => "/models",
        "generate_path" => "/responses",
        "terminal_events" => ["response.completed"]
      )))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "run", "--channels", channels, "--profile", profile,
        "--channel", "any-relay", "--dry-run"
      ], out: output, err: error)
      result = JSON.parse(output.string)
      assert_equal "responses", result.fetch("protocol")
      assert_equal "/models", result.fetch("models_path")
      assert_equal "/responses", result.fetch("generate_path")
      assert_equal 0, result.fetch("requests_sent")
      assert_equal false, result.fetch("network_sent")
      refute result.key?("key_env")
      assert_empty error.string
    end
  end

  def test_cli_discover_dry_run_plans_one_models_request_and_zero_generation
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "responses.yaml")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "any-relay",
          "display_name" => "Any Relay",
          "base_url" => "https://api.example.com",
          "protocol" => "openai_compatible",
          "resale_permission" => "unknown",
          "lifecycle" => "discovered"
        }]
      ))
      File.write(profile, YAML.dump(profile_document.reject { |key, _| key == "endpoint" }.merge(
        "protocol" => "responses",
        "models_path" => "/models",
        "generate_path" => "/responses",
        "terminal_events" => ["response.completed"]
      )))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "discover", "--channels", channels, "--profile", profile,
        "--channel", "any-relay", "--dry-run"
      ], out: output, err: error, env: {})
      result = JSON.parse(output.string)

      assert_equal "responses", result.fetch("protocol")
      assert_equal "/models", result.fetch("models_path")
      assert_equal 1, result.dig("request_estimate", "model_directory_requests")
      assert_equal 0, result.dig("request_estimate", "generation_requests")
      assert_equal 0, result.fetch("requests_sent")
      assert_equal false, result.fetch("network_sent")
      refute result.key?("key_env")
      assert_empty error.string
    end
  end

  def test_cli_discover_live_uses_one_models_request_zero_generation_and_writes_partial_ledger
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "responses.yaml")
      runs = File.join(dir, "runs.jsonl")
      decisions = File.join(dir, "decisions.jsonl")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "any-relay",
          "display_name" => "Any Relay",
          "base_url" => "https://api.example.com",
          "protocol" => "openai_compatible",
          "resale_permission" => "unknown",
          "lifecycle" => "discovered"
        }]
      ))
      File.write(profile, YAML.dump(profile_document.reject { |key, _| key == "endpoint" }.merge(
        "protocol" => "responses",
        "models_path" => "/models",
        "generate_path" => "/responses",
        "terminal_events" => ["response.completed"]
      )))
      client = DiscoveryClient.new(
        "status" => 200,
        "models" => ["gpt-5.6-sol", "dall-e-3", "text-gpt-model"],
        "duration_ms" => 7.25
      )
      factory_calls = []
      factory = lambda do |**arguments|
        factory_calls << arguments
        client
      end
      output = StringIO.new
      error = StringIO.new
      secret = "sk-sensitive-temporary-value"

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "discover", "--channels", channels, "--profile", profile,
        "--channel", "any-relay", "--key-env", "TEMP_UPSTREAM_KEY",
        "--runs", runs, "--decisions", decisions
      ], out: output, err: error, env: { "TEMP_UPSTREAM_KEY" => secret }, client_factory: factory)
      result = JSON.parse(output.string)
      ledger = JSON.parse(File.read(runs))

      assert_equal 1, factory_calls.length
      assert_equal secret, factory_calls.first.fetch(:api_key)
      assert_equal 1, client.models_calls
      assert_equal 0, client.generate_calls
      assert_equal "partial", result.fetch("status")
      assert_equal "discovered_not_qualified", result.fetch("qualification_status")
      assert_equal 1, result.dig("metrics", "request_count")
      assert_equal 0, result.dig("metrics", "generation_request_count")
      assert_equal "text-gpt-model", result.dig("metrics", "classifications", 2, "id")
      assert_equal result, ledger
      refute_includes output.string, secret
      refute_includes File.read(runs), secret
      assert_empty error.string
    end
  end

  def test_cli_discover_live_rejects_empty_key_before_building_client
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "profile.yaml")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "any-relay", "display_name" => "Any Relay", "base_url" => "https://api.example.com",
          "protocol" => "openai_compatible", "resale_permission" => "unknown", "lifecycle" => "discovered"
        }]
      ))
      File.write(profile, YAML.dump(profile_document))
      factory_calls = 0
      factory = lambda do |**|
        factory_calls += 1
        raise "must not build a client"
      end
      output = StringIO.new
      error = StringIO.new

      assert_equal 2, UpstreamBenchmarkV2::CLI.run([
        "discover", "--channels", channels, "--profile", profile,
        "--channel", "any-relay", "--key-env", "EMPTY_KEY"
      ], out: output, err: error, env: { "EMPTY_KEY" => "" }, client_factory: factory)

      assert_equal 0, factory_calls
      assert_empty output.string
      assert_match(/environment variable is empty/, error.string)
    end
  end

  def test_cli_watch_dry_run_has_fixed_bounded_request_estimate
	Dir.mktmpdir do |dir|
	  channels = File.join(dir, "channels.yaml")
	  profile = File.join(dir, "profile.yaml")
	  File.write(channels, YAML.dump(
		"schema_version" => 1,
		"channels" => [{
		  "id" => "candidate", "display_name" => "Candidate", "base_url" => "https://api.example.com/v1",
		  "protocol" => "openai_compatible", "resale_permission" => "unknown", "lifecycle" => "candidate"
		}]
	  ))
	  File.write(profile, YAML.dump(profile_document.merge("representative_models" => ["gpt-a", "gpt-b"])))
	  output = StringIO.new
	  error = StringIO.new

	  assert_equal 0, UpstreamBenchmarkV2::CLI.run([
		"watch", "--channels", channels, "--profile", profile,
		"--channel", "candidate", "--key-env", "CANDIDATE_KEY", "--dry-run"
	  ], out: output, err: error)
	  result = JSON.parse(output.string)
	  assert_equal 4, result.dig("request_estimate", "maximum_generate_requests")
	  refute result.fetch("request_estimate").key?("maximum_chat_requests")
	  assert_equal false, result.fetch("network_sent")
	  refute result.key?("capacity_probe_bounded")
	  refute_match(/concurrency|rpm/i, output.string)
	  assert_empty error.string
	end
  end

  def test_cli_watch_responses_dry_run_uses_profile_without_key
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "profile.yaml")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "candidate",
          "display_name" => "Candidate",
          "base_url" => "https://api.example.com",
          "protocol" => "openai_compatible",
          "resale_permission" => "unknown",
          "lifecycle" => "candidate"
        }]
      ))
      File.write(profile, YAML.dump(profile_document.reject { |key, _| key == "endpoint" }.merge(
        "protocol" => "responses",
        "models_path" => "/models",
        "generate_path" => "/responses",
        "terminal_events" => ["response.completed"],
        "representative_models" => ["gpt-a"]
      )))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "watch", "--channels", channels, "--profile", profile,
        "--channel", "candidate", "--dry-run"
      ], out: output, err: error)
      result = JSON.parse(output.string)
      assert_equal "responses", result.fetch("protocol")
      assert_equal "/responses", result.fetch("generate_path")
      assert_equal 0, result.fetch("requests_sent")
      assert_equal false, result.fetch("network_sent")
      assert_empty error.string
    end
  end

  def test_cli_validates_example_inputs
    output = StringIO.new
    error = StringIO.new

    assert_equal 0, UpstreamBenchmarkV2::CLI.run([
      "validate",
      "--profile", File.expand_path("../../config/upstream-benchmarks/mvp-text-v2.yaml", __dir__),
      "--pricing", File.expand_path("../../config/upstream-benchmarks/pricing-evidence.example.yaml", __dir__),
      "--scenario", File.expand_path("../../config/upstream-benchmarks/v2-scenario-neko.example.yaml", __dir__)
    ], out: output, err: error)
    assert_match(/valid/, output.string)
    assert_empty error.string
  end

  def test_cli_validate_reports_the_actual_profile_id
    Dir.mktmpdir do |dir|
      profile = File.join(dir, "responses.yaml")
      File.write(profile, YAML.dump(profile_document.reject { |key, _| key == "endpoint" }.merge(
        "id" => "responses-fixture",
        "protocol" => "responses",
        "models_path" => "/models",
        "generate_path" => "/responses",
        "terminal_events" => ["response.completed"]
      )))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "validate", "--profile", profile,
        "--pricing", File.expand_path("../../config/upstream-benchmarks/pricing-evidence.example.yaml", __dir__),
        "--scenario", File.expand_path("../../config/upstream-benchmarks/v2-scenario-neko.example.yaml", __dir__)
      ], out: output, err: error)
      assert_match(/responses-fixture/, output.string)
      assert_empty error.string
    end
  end

  def test_cli_writes_secret_free_proposal
    Dir.mktmpdir do |dir|
      pricing_path = File.join(dir, "pricing.yaml")
      scenario_path = File.join(dir, "scenario.yaml")
      run_path = File.join(dir, "run.json")
      output_path = File.join(dir, "proposal.json")
      File.write(pricing_path, YAML.dump(neko_pricing_evidence))
      File.write(scenario_path, YAML.dump(pricing_scenario))
      File.write(run_path, JSON.generate(
        "channel_id" => "neko", "run_id" => "run-v2",
        "metrics" => { "capacity" => { "recommendation" => { "concurrency" => 2, "rpm" => 20 } } }
      ))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "proposal", "--pricing", pricing_path, "--scenario", scenario_path,
        "--run", run_path, "--output", output_path
      ], out: output, err: error)
      proposal = JSON.parse(File.read(output_path))
      assert_match(/\A[0-9a-f]{64}\z/, proposal.fetch("proposal_hash"))
      refute_match(/authorization|api_key|secret/i, File.read(output_path))
      assert_empty error.string
    end
  end

  def test_fast_profile_requires_exact_representative_roles
    profile = UpstreamBenchmarkV2::Profile.new(fast_profile_document)

    assert_equal %w[common expensive new], profile.representative_roles.keys.sort
    assert_equal "gpt-common", profile.representative_roles.fetch("common")

    %w[common expensive new].each do |missing|
      invalid = fast_profile_document
      invalid["representative_roles"] = invalid.fetch("representative_roles").reject { |role, _| role == missing }
      assert_raises(UpstreamBenchmark::ValidationError) { UpstreamBenchmarkV2::Profile.new(invalid) }
    end
  end

  def test_live_fast_profile_uses_the_shared_expensive_representative
    profile_path = File.expand_path(
      "../../config/upstream-benchmarks/quality-first-fast-v1.yaml",
      __dir__
    )
    profile = UpstreamBenchmarkV2::Profile.new(YAML.safe_load(File.read(profile_path)))

    assert_equal "gpt-5.5", profile.representative_roles.fetch("expensive")
  end

  def test_fast_runner_health_pulse_uses_three_roles_without_capacity
    client = ScriptedClient.new(models: %w[gpt-common gpt-expensive gpt-new gpt-extra gpt-image-1])
    profile = UpstreamBenchmarkV2::Profile.new(fast_profile_document)

    record = UpstreamBenchmarkV2::FastRunner.new(
      client: client, profile: profile, job_kind: "health_pulse"
    ).run(channel_id: "candidate")

    assert_equal "health_pulse", record.fetch("job_kind")
    assert_equal "passed", record.fetch("status")
    assert_equal %w[gpt-common gpt-expensive gpt-new], record.dig("metrics", "selected_models")
    assert_equal 6, client.calls.length
    assert_nil record.dig("metrics", "capacity")
    assert_equal "unknown", record.dig("metrics", "gateway", "status")
  end

  def test_fast_runner_catalog_quick_tests_only_explicit_candidates_three_times_per_mode
    client = ScriptedClient.new(models: %w[gpt-b gpt-a gpt-extra gpt-image-1 vendor-unknown])
    profile = UpstreamBenchmarkV2::Profile.new(fast_profile_document)

    record = UpstreamBenchmarkV2::FastRunner.new(
      client: client, profile: profile, job_kind: "catalog_quick",
      candidate_models: %w[gpt-b gpt-a], attempts_per_mode: 3
    ).run(channel_id: "candidate")

    assert_equal %w[gpt-a gpt-b], record.dig("metrics", "selected_models")
    assert_equal %w[gpt-a gpt-b], record.dig("metrics", "candidate_models")
    assert_equal %w[gpt-extra], record.dig("metrics", "unrelated_models_skipped")
    assert_equal 12, client.calls.length
    assert_equal 6, client.calls.count { |call| call == { "model" => "gpt-a", "stream" => false } || call == { "model" => "gpt-b", "stream" => false } }
    assert_equal 6, client.calls.count { |call| call.fetch("stream") }
    refute client.calls.any? { |call| call.fetch("model") == "gpt-extra" }
    assert_equal 12, record.dig("metrics", "direct", "request_count")
    assert_equal 3, record.dig("metrics", "per_model", "gpt-a", "sync", "success_count")
    assert_equal 3, record.dig("metrics", "per_model", "gpt-a", "stream", "success_count")
  end

  def test_fast_runner_catalog_quick_fails_before_generation_when_candidate_was_not_discovered
    client = ScriptedClient.new(models: %w[gpt-a gpt-extra])
    profile = UpstreamBenchmarkV2::Profile.new(fast_profile_document)

    record = UpstreamBenchmarkV2::FastRunner.new(
      client: client, profile: profile, job_kind: "catalog_quick",
      candidate_models: %w[gpt-a gpt-missing], attempts_per_mode: 3
    ).run(channel_id: "candidate")

    assert_equal "failed", record.fetch("status")
    assert_empty client.calls
    assert_includes record.fetch("errors").map { |error| error.fetch("category") }, "candidate_not_discovered"
  end

  def test_fast_runner_catalog_quick_requires_explicit_candidates
    profile = UpstreamBenchmarkV2::Profile.new(fast_profile_document)

    error = assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkV2::FastRunner.new(
        client: ScriptedClient.new(models: %w[gpt-a]), profile: profile, job_kind: "catalog_quick"
      )
    end

    assert_match(/candidate models are required/, error.message)
  end

  def test_fast_runner_records_fixed_failure_categories_without_response_content
    profile = UpstreamBenchmarkV2::Profile.new(fast_profile_document)
    record = UpstreamBenchmarkV2::FastRunner.new(
      client: FixedFailureClient.new, profile: profile, job_kind: "catalog_quick",
      candidate_models: %w[gpt-common], attempts_per_mode: 1
    ).run(channel_id: "candidate")

    assert_includes record.fetch("errors"), {
      "stage" => "gpt-common.sync", "category" => "rate_limited", "http_status" => 429
    }
    assert_includes record.fetch("errors"), {
      "stage" => "gpt-common.stream", "category" => "timeout", "http_status" => 0
    }
    assert_includes record.fetch("errors"), {
      "stage" => "gpt-common.stream", "category" => "incomplete_sse"
    }
    refute_match(/content|private output/i, JSON.generate(record))
  end

  def test_cli_fast_capacity_dry_run_has_exact_bound
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "profile.yaml")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "candidate", "display_name" => "Candidate", "base_url" => "https://api.example.com/v1",
          "protocol" => "openai_compatible", "resale_permission" => "unknown", "lifecycle" => "candidate"
        }]
      ))
      File.write(profile, YAML.dump(fast_profile_document))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "fast", "--channels", channels, "--profile", profile,
        "--channel", "candidate", "--job", "capacity_check", "--dry-run"
      ], out: output, err: error)
      result = JSON.parse(output.string)
      assert_equal 1, result.dig("request_estimate", "model_directory_requests")
      assert_equal 129, result.dig("request_estimate", "maximum_generation_requests")
      assert_equal 130, result.dig("request_estimate", "maximum_http_requests")
      assert_equal false, result.fetch("network_sent")
      assert_empty error.string
    end
  end

  def test_cli_fast_catalog_dry_run_has_exact_candidate_bound_and_no_network
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "profile.yaml")
      models = File.join(dir, "models.json")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "candidate", "display_name" => "Candidate", "base_url" => "https://api.example.com/v1",
          "protocol" => "openai_compatible", "resale_permission" => "unknown", "lifecycle" => "candidate"
        }]
      ))
      File.write(profile, YAML.dump(fast_profile_document))
      File.write(models, JSON.generate("schema_version" => 1, "models" => %w[gpt-5.7 gpt-5.7-sol]))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "fast", "--channels", channels, "--profile", profile,
        "--channel", "candidate", "--job", "catalog_quick",
        "--models", models, "--attempts-per-mode", "3", "--dry-run"
      ], out: output, err: error)
      result = JSON.parse(output.string)
      assert_equal 2, result.fetch("candidate_models").length
      assert_equal 3, result.fetch("attempts_per_mode")
      assert_equal 12, result.dig("request_estimate", "maximum_generation_requests")
      assert_equal 13, result.dig("request_estimate", "maximum_http_requests")
      assert_equal false, result.fetch("network_sent")
      assert_empty error.string
    end
  end

  def test_cli_fast_live_appends_secret_free_record
    Dir.mktmpdir do |dir|
      channels = File.join(dir, "channels.yaml")
      profile = File.join(dir, "profile.yaml")
      runs = File.join(dir, "runs.jsonl")
      decisions = File.join(dir, "decisions.jsonl")
      File.write(channels, YAML.dump(
        "schema_version" => 1,
        "channels" => [{
          "id" => "candidate", "display_name" => "Candidate", "base_url" => "https://api.example.com/v1",
          "protocol" => "openai_compatible", "resale_permission" => "unknown", "lifecycle" => "candidate"
        }]
      ))
      File.write(profile, YAML.dump(fast_profile_document))
      output = StringIO.new
      error = StringIO.new
      secret = "sk-sensitive-fast-test-value"
      client = ScriptedClient.new(models: %w[gpt-common gpt-expensive gpt-new])
      factory = lambda do |**arguments|
        assert_equal secret, arguments.fetch(:api_key)
        client
      end

      assert_equal 0, UpstreamBenchmarkV2::CLI.run([
        "fast", "--channels", channels, "--profile", profile,
        "--runs", runs, "--decisions", decisions,
        "--channel", "candidate", "--key-env", "FAST_TEST_KEY", "--job", "health_pulse"
      ], out: output, err: error, env: { "FAST_TEST_KEY" => secret }, client_factory: factory)

      ledger = File.readlines(runs).map { |line| JSON.parse(line) }
      assert_equal 1, ledger.length
      assert_equal "health_pulse", ledger.first.fetch("job_kind")
      refute_includes output.string, secret
      refute_includes File.read(runs), secret
      assert_empty error.string
    end
  end

  def test_quality_first_evaluator_applies_hard_gates_before_score
    record = quality_fast_record
    record["errors"] = [{ "stage" => "gpt-common.stream", "category" => "incomplete_sse" }]

    decision = UpstreamBenchmarkV2::QualityFirstEvaluator.new(
      record: record,
      baseline: quality_baseline,
      pricing: { "mode" => "explicit_model_price", "verified" => true },
      now: Time.iso8601("2026-07-22T04:10:00Z")
    ).evaluate

    assert_equal "blocked", decision.fetch("status")
    assert_includes decision.fetch("hard_gate_reasons"), "technical_failure"
    assert_operator decision.fetch("total_score"), :>, 0
    assert_equal false, decision.fetch("eligible")
  end

  def test_quality_first_evaluator_requires_quality_80_and_mandatory_evidence
    record = quality_fast_record
    record["metrics"]["gateway"] = { "status" => "unknown", "reason" => "not_measured" }
    unknown = UpstreamBenchmarkV2::QualityFirstEvaluator.new(
      record: record,
      baseline: nil,
      pricing: { "mode" => "unknown", "verified" => false },
      now: Time.iso8601("2026-07-22T04:10:00Z")
    ).evaluate
    assert_equal "needs_evidence", unknown.fetch("status")
    assert_includes unknown.fetch("missing_evidence"), "production_baseline"
    assert_includes unknown.fetch("missing_evidence"), "gateway_measurement"
    assert_includes unknown.fetch("missing_evidence"), "verified_pricing"

    low_quality = quality_fast_record
    low_quality["metrics"]["capacity"] = nil
    result = UpstreamBenchmarkV2::QualityFirstEvaluator.new(
      record: low_quality,
      baseline: quality_baseline,
      pricing: { "mode" => "explicit_model_price", "verified" => true },
      now: Time.iso8601("2026-07-22T04:10:00Z")
    ).evaluate
    assert_equal 75, result.fetch("quality_score")
    assert_equal "not_better", result.fetch("status")
  end

  def test_quality_first_evaluator_rejects_latency_regression_and_gateway_overhead
    record = quality_fast_record
    record["metrics"]["direct"]["ttft"] = { "p95" => 1_300.0 }
    record["metrics"]["direct"]["latency"] = { "p95" => 2_300.0 }
    record["metrics"]["gateway"] = {
      "status" => "measured",
      "ttft" => { "p95" => 2_000.0 },
      "latency" => { "p95" => 5_000.0 }
    }

    decision = UpstreamBenchmarkV2::QualityFirstEvaluator.new(
      record: record,
      baseline: quality_baseline,
      pricing: { "mode" => "multiplier_only", "verified" => true },
      now: Time.iso8601("2026-07-22T04:10:00Z")
    ).evaluate

    assert_equal "blocked", decision.fetch("status")
    assert_includes decision.fetch("hard_gate_reasons"), "latency_regression"
    assert_includes decision.fetch("hard_gate_reasons"), "gateway_overhead"
  end

  def test_quality_first_evaluator_marks_complete_better_evidence_eligible
    decision = UpstreamBenchmarkV2::QualityFirstEvaluator.new(
      record: quality_fast_record,
      baseline: quality_baseline,
      pricing: { "mode" => "explicit_model_price", "verified" => true },
      now: Time.iso8601("2026-07-22T04:10:00Z")
    ).evaluate

    assert_equal 90, decision.fetch("quality_score")
    assert_equal 100, decision.fetch("total_score")
    assert_equal "eligible_for_manual_switch", decision.fetch("status")
    assert_equal true, decision.fetch("eligible")
    assert_equal true, decision.fetch("material_improvement")
  end

  private

  def quality_fast_record
    {
      "status" => "passed",
      "job_kind" => "capacity_check",
      "recorded_at" => "2026-07-22T04:00:00Z",
      "errors" => [],
      "metrics" => {
        "selected_models" => %w[gpt-common gpt-expensive gpt-new],
        "representative_roles" => {
          "common" => "gpt-common", "expensive" => "gpt-expensive", "new" => "gpt-new"
        },
        "per_model" => {
          "gpt-common" => { "stream" => { "complete" => true } },
          "gpt-expensive" => { "stream" => { "complete" => true } },
          "gpt-new" => { "stream" => { "complete" => true } }
        },
        "direct" => {
          "request_count" => 129, "success_count" => 129, "success_rate" => 1.0,
          "ttft" => { "p95" => 800.0 }, "latency" => { "p95" => 1_500.0 }
        },
        "gateway" => {
          "status" => "measured", "ttft" => { "p95" => 900.0 }, "latency" => { "p95" => 1_700.0 }
        },
        "capacity" => {
          "gpt-common" => { "concurrency" => { "last_stable" => 10 }, "rpm" => { "last_stable" => 30 } },
          "gpt-expensive" => { "concurrency" => { "last_stable" => 8 }, "rpm" => { "last_stable" => 20 } },
          "gpt-new" => { "concurrency" => { "last_stable" => 5 }, "rpm" => { "last_stable" => 20 } }
        },
        "usage" => { "total_tokens" => 645 }
      }
    }
  end

  def quality_baseline
    {
      "success_rate" => 0.995,
      "sse_completion_rate" => 1.0,
      "ttft_p95_ms" => 1_000.0,
      "latency_p95_ms" => 2_000.0,
      "concurrency" => 3,
      "rpm" => 12
    }
  end
end
