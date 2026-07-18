# frozen_string_literal: true

require "json"
require "minitest/autorun"
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

  def test_profile_accepts_v2_bounds
    profile = UpstreamBenchmarkV2::Profile.new(profile_document)

    assert_equal [1, 2, 3, 5, 8, 10], profile.concurrency_levels
    assert_equal [6, 12, 20, 30], profile.rpm_levels
    assert_equal 10, profile.rpm_window_seconds
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

  def test_runner_marks_incomplete_stream_as_partial
    record = UpstreamBenchmarkV2::Runner.new(
      client: ScriptedClient.new(models: ["gpt-a"], stream_complete: false),
      profile: UpstreamBenchmarkV2::Profile.new(profile_document),
      sleeper: ->(_seconds) {}
    ).run(channel_id: "neko")

    assert_equal "partial", record.fetch("status")
    assert_equal false, record.dig("metrics", "per_model", "gpt-a", "stream", "complete")
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
end
