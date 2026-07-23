# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "tempfile"
require "yaml"
require_relative "../../ops/evaluate-subscription-account"

class SubscriptionAccountEvaluatorTest < Minitest::Test
  def valid_document
    {
      "schema_version" => 1,
      "candidate_id" => "ACC-CANDIDATE-PLUS-01",
      "status" => "fictional",
      "reviewed_at" => "2026-07-15",
      "seller" => {
        "public_name" => "Fictional independent-account seller",
        "storefront_reference" => "fictional-listing-plus-01",
        "support_days" => 14,
        "replacement_policy" => "refund_or_replace"
      },
      "listing" => {
        "public_label" => "Fictional independent OpenAI ChatGPT Plus account",
        "price_cny" => 160.0,
        "currency" => "CNY",
        "quantity" => 1,
        "remaining_days" => 30
      },
      "entitlement" => {
        "platform" => "openai",
        "official_product" => "chatgpt",
        "official_tier" => "plus",
        "ownership_mode" => "independent",
        "account_origin" => "new",
        "organization_managed" => false
      },
      "control" => {
        "buyer_controls_primary_email" => true,
        "buyer_controls_recovery" => true,
        "buyer_controls_password" => true,
        "buyer_controls_2fa" => true,
        "seller_retains_recovery" => false
      },
      "authorization" => {
        "sub2api_platform" => "openai",
        "sub2api_account_type" => "oauth",
        "normal_authorization_only" => true,
        "delivery_mode" => "account_control",
        "requires_credential_extraction" => false,
        "requires_bypass" => false
      },
      "operations" => {
        "isolated_group" => true,
        "individually_disableable" => true,
        "cost_traceable" => true,
        "auto_pause_on_expiry" => true,
        "initial_concurrency" => 1
      },
      "evidence" => {
        "listing_snapshot_ref" => "fictional-snapshot-plus-01",
        "terms_snapshot_ref" => "fictional-terms-plus-01",
        "notes" => "Fictional candidate for offline comparison only. No purchase or login."
      }
    }
  end

  def evaluate(document = valid_document)
    SubscriptionAccountEvaluator.new(document).evaluate
  end

  def test_recommends_complete_independent_openai_plus_candidate
    result = evaluate

    assert_equal true, result.fetch("valid")
    assert_equal [], result.fetch("errors")
    assert_equal [], result.fetch("hard_rejections")
    assert_equal "recommended", result.fetch("decision")
    assert_equal 100, result.fetch("score")
    assert_equal 30, result.dig("score_breakdown", "account_control")
    assert_equal 25, result.dig("score_breakdown", "authorization_compatibility")
  end

  def test_hard_rejects_organization_managed_k12_candidate
    document = valid_document
    document["candidate_id"] = "ACC-CANDIDATE-K12-01"
    document["listing"]["public_label"] = "Fictional K12 managed account"
    document["entitlement"].merge!(
      "platform" => "gemini",
      "official_product" => "google_workspace_for_education",
      "official_tier" => "education_managed",
      "ownership_mode" => "managed",
      "account_origin" => "invited",
      "organization_managed" => true
    )
    document["authorization"].merge!(
      "sub2api_platform" => "gemini",
      "sub2api_account_type" => "oauth"
    )

    result = evaluate(document)

    assert_equal "rejected", result.fetch("decision")
    assert_nil result.fetch("score")
    assert_includes result.fetch("hard_rejections"), "shared_or_managed_account"
    assert_includes result.fetch("hard_rejections"), "organization_managed_account"
  end

  def test_hard_rejects_shared_pro_with_token_only_delivery
    document = valid_document
    document["candidate_id"] = "ACC-CANDIDATE-PRO-01"
    document["entitlement"]["official_tier"] = "pro"
    document["entitlement"]["ownership_mode"] = "shared"
    document["control"]["buyer_controls_primary_email"] = false
    document["control"]["seller_retains_recovery"] = true
    document["authorization"]["delivery_mode"] = "token_only"
    document["authorization"]["normal_authorization_only"] = false
    document["authorization"]["requires_credential_extraction"] = true

    result = evaluate(document)

    assert_equal "rejected", result.fetch("decision")
    assert_includes result.fetch("hard_rejections"), "shared_or_managed_account"
    assert_includes result.fetch("hard_rejections"), "incomplete_buyer_control"
    assert_includes result.fetch("hard_rejections"), "seller_retains_recovery"
    assert_includes result.fetch("hard_rejections"), "unsupported_delivery_mode"
    assert_includes result.fetch("hard_rejections"), "credential_extraction_required"
  end

  def test_hard_rejects_platform_and_account_type_mismatch
    document = valid_document
    document["authorization"]["sub2api_platform"] = "anthropic"
    document["authorization"]["sub2api_account_type"] = "setup-token"

    result = evaluate(document)

    assert_equal "rejected", result.fetch("decision")
    assert_includes result.fetch("hard_rejections"), "unsupported_sub2api_mapping"
  end

  def test_hard_rejects_over_budget_or_bulk_first_purchase
    document = valid_document
    document["listing"]["price_cny"] = 301.0
    document["listing"]["quantity"] = 2

    result = evaluate(document)

    assert_equal "rejected", result.fetch("decision")
    assert_includes result.fetch("hard_rejections"), "sample_budget_exceeded"
    assert_includes result.fetch("hard_rejections"), "bulk_first_purchase_required"
  end

  def test_rejects_missing_required_field_as_invalid
    document = valid_document
    document["seller"].delete("support_days")

    result = evaluate(document)

    assert_equal false, result.fetch("valid")
    assert_equal "invalid", result.fetch("decision")
    assert_includes result.fetch("errors"), "seller.support_days: is required"
  end

  def test_rejects_forbidden_credential_fields_and_secret_values
    document = valid_document
    document["authorization"]["refresh_token"] = "not-even-a-real-token"
    document["evidence"]["notes"] = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456"

    result = evaluate(document)

    assert_equal "invalid", result.fetch("decision")
    assert_includes result.fetch("errors"),
                    "authorization.refresh_token: credential fields are forbidden"
    assert_includes result.fetch("errors"), "evidence.notes: value looks like a secret"
  end

  def test_emits_conditional_decision_for_low_evidence_but_eligible_candidate
    document = valid_document
    document["seller"].merge!(
      "support_days" => 1,
      "replacement_policy" => "unknown"
    )
    document["listing"].merge!(
      "price_cny" => 290.0,
      "remaining_days" => 14
    )
    document["evidence"].merge!(
      "listing_snapshot_ref" => "unknown",
      "terms_snapshot_ref" => "unknown"
    )

    result = evaluate(document)

    assert_equal "conditional", result.fetch("decision")
    assert_equal 79, result.fetch("score")
    assert_equal 3, result.dig("score_breakdown", "support_and_traceability")
  end

  def test_compare_cli_returns_json_without_secret_material
    Tempfile.create(["subscription-candidate", ".yaml"]) do |file|
      file.write(YAML.dump(valid_document))
      file.flush

      stdout, stderr, status = Open3.capture3(
        "ruby", "ops/evaluate-subscription-account.rb", "compare", file.path
      )

      assert status.success?, stderr
      payload = JSON.parse(stdout)
      assert_equal "recommended", payload.fetch("candidates").first.fetch("decision")
      assert_equal 1, payload.dig("summary", "recommended")
      refute_match(/access_token|refresh_token|session_key|cookie/i, stdout)
    end
  end
end
