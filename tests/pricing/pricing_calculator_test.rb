# frozen_string_literal: true

require "minitest/autorun"
require_relative "../../ops/calculate-pricing"

class PricingCalculatorTest < Minitest::Test
  def upstream_document
    {
      "schema_version" => 1,
      "upstream_id" => "UP01",
      "display_name" => "Fictional relay",
      "readiness" => "draft",
      "reviewed_at" => "2026-07-15",
      "connection" => {
        "protocol" => "openai_compatible",
        "base_url" => "https://api.example-upstream.com/v1",
        "allowlist_host" => "api.example-upstream.com",
        "auth_scheme" => "bearer",
        "secret_ref" => "sub2api-admin://accounts/UP01"
      },
      "sub2api" => {
        "platform" => "openai",
        "account_type" => "apikey",
        "account_name" => "UP01-existing-relay",
        "group_name" => "up01-openai",
        "priority" => 50,
        "concurrency" => 2,
        "rate_multiplier" => 1.0,
        "pool_mode" => {
          "enabled" => false,
          "retry_count" => 1,
          "retry_status_codes" => [429]
        }
      },
      "models" => [
        {
          "public_name" => "example-chat-model",
          "upstream_name" => "example-chat-model-v1",
          "enabled" => true,
          "capabilities" => %w[chat_completions streaming],
          "pricing" => {
            "currency" => "CNY",
            "unit" => "per_1m_tokens",
            "input" => 1.0,
            "output" => 4.0,
            "cached_input" => nil,
            "cache_write" => nil
          }
        }
      ],
      "limits" => {
        "max_concurrency" => 2,
        "rpm" => 60,
        "tpm" => 100_000,
        "request_timeout_seconds" => 120,
        "daily_cost_cap" => { "currency" => "CNY", "amount" => 20.0 }
      },
      "rate_limit" => {
        "status_code" => 429,
        "retry_after" => "honor_when_present",
        "retryable" => true,
        "max_attempts" => 1,
        "reset_behavior" => "supplier_documented"
      },
      "balance" => {
        "query_method" => "dashboard",
        "query_reference" => "supplier dashboard",
        "minimum_top_up" => { "currency" => "CNY", "amount" => 50.0 },
        "low_balance_alert" => { "currency" => "CNY", "amount" => 20.0 }
      },
      "commercial" => {
        "resale_permission" => "unknown",
        "terms_reference" => "https://www.example-upstream.com/terms",
        "refund_policy" => "unknown",
        "support_reference" => "support portal",
        "risk_acknowledged" => true
      },
      "evidence" => {
        "checked_by" => "test",
        "checked_at" => "2026-07-15T00:00:00Z",
        "notes" => "Fictional fixture."
      }
    }
  end

  def scenario_document
    {
      "schema_version" => 1,
      "scenario_id" => "MVP-EXAMPLE",
      "status" => "fictional",
      "source_upstream_id" => "UP01",
      "currencies" => {
        "public" => "CNY",
        "sub2api_balance" => "USD",
        "cny_per_usd" => 7.2
      },
      "assumptions" => {
        "target_fully_loaded_margin_rate" => 0.25,
        "payment_fee_rate" => 0.0,
        "failure_compensation_rate" => 0.05,
        "rounding_increment_cny_per_1m" => 0.01
      },
      "monthly_fixed_costs" => [
        { "id" => "fictional-infrastructure", "amount_cny" => 10.0 }
      ],
      "model_mix" => [
        {
          "public_name" => "example-chat-model",
          "monthly_tokens" => {
            "input" => 80_000_000,
            "output" => 20_000_000,
            "cached_input" => 0,
            "cache_write" => 0
          }
        }
      ]
    }
  end

  def calculate(upstream = upstream_document, scenario = scenario_document)
    PricingCalculator.new(upstream: upstream, scenario: scenario).calculate
  end

  def validation_errors(upstream = upstream_document, scenario = scenario_document)
    calculate(upstream, scenario)
    flunk "expected validation to fail"
  rescue PricingCalculator::ValidationError => e
    e.errors
  end

  def test_calculates_worked_example_and_recovers_fixed_cost
    result = calculate
    model = result.fetch("models").first
    summary = result.fetch("forecast_summary")

    assert_in_delta 0.10, result.fetch("fixed_cost_cny_per_1m_tokens"), 1e-12
    assert_in_delta 1.54, model.dig("public_price_cny_per_1m", "input"), 1e-12
    assert_in_delta 5.74, model.dig("public_price_cny_per_1m", "output"), 1e-12
    assert_in_delta 10.0, summary.fetch("monthly_fixed_cost_cny"), 1e-12
    assert_operator summary.fetch("fully_loaded_margin_rate"), :>=, 0.25
  end

  def test_emits_exact_sub2api_channel_units
    result = calculate
    channel = result.dig("sub2api_recommendation", "channel")
    pricing = channel.fetch("model_pricing").first

    assert_equal "requested", channel.fetch("billing_model_source")
    assert_equal true, channel.fetch("restrict_models")
    assert_equal({ "openai" => { "example-chat-model" => "example-chat-model-v1" } },
                 channel.fetch("model_mapping"))
    assert_equal "token", pricing.fetch("billing_mode")
    assert_equal ["example-chat-model"], pricing.fetch("models")
    assert_in_delta 1.54 / 7.2 / 1_000_000, pricing.fetch("input_price"), 1e-18
    assert_in_delta 5.74 / 7.2 / 1_000_000, pricing.fetch("output_price"), 1e-18
    assert_nil pricing.fetch("cache_read_price")
    assert_nil pricing.fetch("cache_write_price")
    assert_in_delta 1.0 / 7.2,
                    result.dig("sub2api_recommendation", "balance_recharge_multiplier_usd_per_cny"),
                    1e-12
    assert_equal 1.0, result.dig("sub2api_recommendation", "group_rate_multiplier")
    assert_equal 1.0, result.dig("sub2api_recommendation", "account_rate_multiplier")
  end

  def test_converts_usd_upstream_prices_to_cny
    upstream = upstream_document
    upstream["models"].first["pricing"]["currency"] = "USD"
    scenario = scenario_document
    scenario["monthly_fixed_costs"].first["amount_cny"] = 0.0

    model = calculate(upstream, scenario).fetch("models").first

    assert_in_delta 10.08, model.dig("public_price_cny_per_1m", "input"), 1e-12
  end

  def test_rejects_target_margin_below_project_floor
    scenario = scenario_document
    scenario["assumptions"]["target_fully_loaded_margin_rate"] = 0.19

    assert_includes validation_errors(upstream_document, scenario),
                    "assumptions.target_fully_loaded_margin_rate: must be at least 0.2"
  end

  def test_rejects_margin_and_fee_that_leave_no_pricing_denominator
    scenario = scenario_document
    scenario["assumptions"]["target_fully_loaded_margin_rate"] = 0.80
    scenario["assumptions"]["payment_fee_rate"] = 0.20

    assert_includes validation_errors(upstream_document, scenario),
                    "assumptions: target margin plus payment fee must be less than 1"
  end

  def test_rejects_unknown_forecast_model
    scenario = scenario_document
    scenario["model_mix"].first["public_name"] = "not-enabled"

    assert_includes validation_errors(upstream_document, scenario),
                    "model_mix[0].public_name: is not an enabled model in UP01"
  end

  def test_rejects_nonzero_usage_for_unknown_cache_price
    scenario = scenario_document
    scenario["model_mix"].first["monthly_tokens"]["cached_input"] = 1_000_000

    assert_includes validation_errors(upstream_document, scenario),
                    "model_mix[0].monthly_tokens.cached_input: forecast is nonzero but upstream price is unknown"
  end

  def test_rejects_mismatched_source_upstream
    scenario = scenario_document
    scenario["source_upstream_id"] = "UP99"

    assert_includes validation_errors(upstream_document, scenario),
                    "source_upstream_id: must match upstream_id UP01"
  end
end
