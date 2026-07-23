# frozen_string_literal: true

require "minitest/autorun"
require_relative "../../ops/validate-upstream"

class UpstreamConfigValidatorTest < Minitest::Test
  def valid_document
    {
      "schema_version" => 1,
      "upstream_id" => "UP01",
      "display_name" => "Example OpenAI-compatible relay",
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
          "retry_status_codes" => [429, 500, 502, 503, 504]
        }
      },
      "models" => [
        {
          "public_name" => "example-chat-model",
          "upstream_name" => "example-chat-model",
          "enabled" => true,
          "capabilities" => %w[models chat_completions streaming],
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
        "query_reference" => "supplier dashboard balance page",
        "minimum_top_up" => { "currency" => "CNY", "amount" => 50.0 },
        "low_balance_alert" => { "currency" => "CNY", "amount" => 20.0 }
      },
      "commercial" => {
        "resale_permission" => "unknown",
        "terms_reference" => "https://www.example-upstream.com/terms",
        "refund_policy" => "unknown",
        "support_reference" => "supplier support ticket portal",
        "risk_acknowledged" => true
      },
      "evidence" => {
        "checked_by" => "user_or_operator",
        "checked_at" => "2026-07-15T00:00:00Z",
        "notes" => "Fictional values for schema validation only."
      }
    }
  end

  def errors_for(document = valid_document, live_ready: false)
    UpstreamConfigValidator.new(document, live_ready: live_ready).errors
  end

  def test_accepts_a_complete_draft_document
    assert_empty errors_for
  end

  def test_live_ready_mode_requires_ready_status
    errors = errors_for(valid_document, live_ready: true)

    assert_includes errors, "readiness: must be ready_for_live_test in --live-ready mode"
  end

  def test_accepts_a_complete_live_ready_document
    document = valid_document
    document["readiness"] = "ready_for_live_test"

    assert_empty errors_for(document, live_ready: true)
  end

  def test_rejects_non_https_base_url
    document = valid_document
    document["connection"]["base_url"] = "http://api.example-upstream.com/v1"

    assert_includes errors_for(document), "connection.base_url: must use https"
  end

  def test_rejects_base_url_userinfo_query_and_fragment
    document = valid_document
    document["connection"]["base_url"] = "https://name:secret@api.example-upstream.com/v1?q=1#fragment"
    errors = errors_for(document)

    assert_includes errors, "connection.base_url: must not include user information"
    assert_includes errors, "connection.base_url: must not include a query string"
    assert_includes errors, "connection.base_url: must not include a fragment"
  end

  def test_rejects_allowlist_host_mismatch
    document = valid_document
    document["connection"]["allowlist_host"] = "other.example-upstream.com"

    assert_includes errors_for(document), "connection.allowlist_host: must exactly match connection.base_url host api.example-upstream.com"
  end

  def test_rejects_forbidden_credential_keys
    document = valid_document
    document["connection"]["api_key"] = "not-even-a-real-key"

    assert_includes errors_for(document), "connection.api_key: credential fields are forbidden; use connection.secret_ref"
  end

  def test_rejects_suspected_secret_values
    document = valid_document
    document["evidence"]["notes"] = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456"

    assert_includes errors_for(document), "evidence.notes: value looks like a secret"
  end

  def test_rejects_invalid_secret_reference
    document = valid_document
    document["connection"]["secret_ref"] = "sk-live-abcdefghijklmnopqrstuvwxyz"

    assert_includes errors_for(document), "connection.secret_ref: must be a symbolic secret location"
  end

  def test_rejects_missing_required_supplier_fact
    document = valid_document
    document["commercial"].delete("refund_policy")

    assert_includes errors_for(document), "commercial.refund_policy: is required"
  end

  def test_rejects_duplicate_public_model_names
    document = valid_document
    document["models"] << Marshal.load(Marshal.dump(document["models"].first))

    assert_includes errors_for(document), "models[1].public_name: duplicates example-chat-model"
  end

  def test_rejects_incomplete_model_pricing
    document = valid_document
    document["models"].first["pricing"].delete("output")

    assert_includes errors_for(document), "models[0].pricing.output: is required"
  end

  def test_rejects_sub2api_concurrency_above_supplier_limit
    document = valid_document
    document["sub2api"]["concurrency"] = 3

    assert_includes errors_for(document), "sub2api.concurrency: must not exceed limits.max_concurrency (2)"
  end
end
