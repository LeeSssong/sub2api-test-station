# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "yaml"
require_relative "../../ops/upstream-benchmark-v2"

class UpstreamBenchmarkV2Test < Minitest::Test
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
end
