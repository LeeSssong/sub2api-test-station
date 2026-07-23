# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "tempfile"
require "yaml"
require_relative "../../ops/payment-control-simulator"

class PaymentControlSimulatorTest < Minitest::Test
  def valid_config
    {
      "schema_version" => 1,
      "payment_id" => "PAY01",
      "status" => "fictional",
      "reviewed_at" => "2026-07-15",
      "merchant_context" => {
        "current_state" => "unknown",
        "assumed_primary_market" => "mainland_china",
        "settlement_currency" => "CNY",
        "external_actions_deferred" => true
      },
      "decision" => {
        "current_mode" => "manual_ledger_simulation",
        "mainland_primary" => "alipay",
        "mainland_secondary" => "wxpay",
        "overseas_primary" => "stripe",
        "aggregation_fallback" => "easypay",
        "public_registration" => "invitation_only"
      },
      "settings" => {
        "payment_enabled" => false,
        "enabled_payment_types" => [],
        "payment_min_amount_cny" => 20.0,
        "payment_max_amount_cny" => 200.0,
        "payment_daily_limit_cny" => 200.0,
        "payment_order_timeout_minutes" => 15,
        "payment_max_pending_orders" => 2,
        "cny_per_usd" => 7.2,
        "balance_recharge_multiplier_usd_per_cny" => 1.0 / 7.2,
        "payment_recharge_fee_rate" => 0.0,
        "payment_load_balance_strategy" => "round_robin",
        "cancel_rate_limit" => {
          "enabled" => true,
          "max_cancels" => 3,
          "window" => 1,
          "unit" => "hour",
          "mode" => "rolling"
        }
      },
      "providers" => [
        provider("alipay", "mainland_primary", "recommended_conditional"),
        provider("wxpay", "mainland_secondary", "recommended_conditional"),
        provider("stripe", "overseas_primary", "recommended_conditional"),
        provider("easypay", "aggregation_fallback", "deferred_due_diligence").merge(
          "due_diligence" => {
            "funds_flow_verified" => false,
            "settlement_verified" => false,
            "freeze_and_refund_verified" => false,
            "reconciliation_verified" => false,
            "continuity_verified" => false
          }
        )
      ],
      "evidence" => {
        "sub2api_version" => "v0.1.155",
        "sub2api_commit" => "41cec0db059ffb82d0efdcfcf07a24ab51fbfe97",
        "source_reference" => "Sub2API docs/PAYMENT.md",
        "notes" => "Fictional disabled configuration. No merchant, payment, or collection exists."
      }
    }
  end

  def provider(key, role, readiness)
    {
      "provider_key" => key,
      "role" => role,
      "readiness" => readiness,
      "required_condition" => "eligible_merchant_and_terms_accepted",
      "callback_url" => "https://api.example.invalid/api/v1/payment/webhook/#{key}",
      "capabilities" => {
        "signature_verification" => true,
        "active_query" => true,
        "refund" => true,
        "reconciliation" => true
      },
      "secret_ref" => "sub2api-admin://payment/providers/PAY01-#{key}",
      "enabled" => false
    }
  end

  def config_errors(document = valid_config)
    PaymentControl::ConfigValidator.new(document).errors
  end

  def initial_order
    PaymentControl::OrderSimulator.create_order(
      out_trade_no: "PAY01-SIM-ORDER-0001",
      amount_cny: 72.0,
      balance_recharge_multiplier: 1.0 / 7.2
    )
  end

  def provider_event(type: "payment_succeeded", event_id: "EVT-PAY-SUCCESS-0001",
                     digest: "a" * 64, amount_cny: 72.0, currency: "CNY",
                     signature_verified: true)
    {
      "event_id" => event_id,
      "event_type" => type,
      "source" => "provider",
      "out_trade_no" => "PAY01-SIM-ORDER-0001",
      "amount_cny" => amount_cny,
      "currency" => currency,
      "occurred_at" => "2026-07-15T00:01:00Z",
      "raw_body_sha256" => digest,
      "signature_verified" => signature_verified
    }
  end

  def internal_event(type:, event_id:, digest:)
    provider_event(type: type, event_id: event_id, digest: digest).merge(
      "source" => "internal",
      "signature_verified" => false
    )
  end

  def test_accepts_complete_disabled_configuration
    assert_empty config_errors
  end

  def test_rejects_enabling_payment_without_live_accepted_provider
    document = valid_config
    document["settings"]["payment_enabled"] = true
    document["settings"]["enabled_payment_types"] = ["alipay"]
    document["providers"].first["enabled"] = true

    errors = config_errors(document)

    assert_includes errors, "settings.payment_enabled: requires an enabled live_accepted provider"
    assert_includes errors, "providers[0].enabled: provider must be live_accepted before enabling"
  end

  def test_rejects_amount_limits_and_inconsistent_balance_multiplier
    document = valid_config
    document["settings"].merge!(
      "payment_min_amount_cny" => 220.0,
      "payment_max_amount_cny" => 200.0,
      "payment_daily_limit_cny" => 100.0,
      "balance_recharge_multiplier_usd_per_cny" => 0.2
    )

    errors = config_errors(document)

    assert_includes errors, "settings.payment_min_amount_cny: must not exceed payment_max_amount_cny"
    assert_includes errors, "settings.payment_daily_limit_cny: must be at least payment_max_amount_cny"
    assert_includes errors,
                    "settings.balance_recharge_multiplier_usd_per_cny: must equal 1 / settings.cny_per_usd"
  end

  def test_rejects_bad_callback_and_missing_live_capability
    document = valid_config
    document["providers"].first["callback_url"] = "http://api.example.invalid/callback?secret=value"
    document["providers"].first["capabilities"]["active_query"] = false

    errors = config_errors(document)

    assert_includes errors, "providers[0].callback_url: must use https"
    assert_includes errors, "providers[0].callback_url: must not include a query string"
    assert_includes errors,
                    "providers[0].capabilities.active_query: recommended providers must support this capability"
  end

  def test_rejects_easypay_activation_before_due_diligence
    document = valid_config
    easypay = document["providers"].last
    easypay["readiness"] = "recommended_conditional"

    assert_includes config_errors(document),
                    "providers[3].due_diligence: all checks must pass before EasyPay can be recommended or enabled"
  end

  def test_rejects_payment_secret_fields_and_values
    document = valid_config
    document["providers"].first["private_key"] = "not-a-real-key"
    document["evidence"]["notes"] = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456"

    errors = config_errors(document)

    assert_includes errors, "providers[0].private_key: credential fields are forbidden; use secret_ref"
    assert_includes errors, "evidence.notes: value looks like a secret"
  end

  def test_payment_success_credits_once_across_duplicate_and_second_success_event
    first = PaymentControl::OrderSimulator.apply(initial_order, provider_event)
    duplicate = PaymentControl::OrderSimulator.apply(first.fetch("order"), provider_event)
    second_event = provider_event(event_id: "EVT-PAY-SUCCESS-0002", digest: "b" * 64)
    second = PaymentControl::OrderSimulator.apply(duplicate.fetch("order"), second_event)

    assert_equal "payment_completed", first.fetch("action")
    assert_equal "duplicate_event_noop", duplicate.fetch("action")
    assert_equal "already_fulfilled_noop", second.fetch("action")
    assert_equal "COMPLETED", second.dig("order", "status")
    assert_equal 1, second.dig("order", "credit_count")
    assert_in_delta 10.0, second.dig("order", "credited_balance_usd"), 1e-12
  end

  def test_rejects_forged_or_mismatched_provider_event
    forged = provider_event(signature_verified: false)
    error = assert_raises(PaymentControl::ValidationError) do
      PaymentControl::OrderSimulator.apply(initial_order, forged)
    end
    assert_includes error.errors, "event.signature_verified: provider event was not verified"

    mismatch = provider_event(amount_cny: 71.0, currency: "USD")
    error = assert_raises(PaymentControl::ValidationError) do
      PaymentControl::OrderSimulator.apply(initial_order, mismatch)
    end
    assert_includes error.errors, "event.amount_cny: does not match order amount"
    assert_includes error.errors, "event.currency: does not match order currency CNY"
  end

  def test_ignores_out_of_order_failure_after_completion
    completed = PaymentControl::OrderSimulator.apply(initial_order, provider_event).fetch("order")
    failure = provider_event(
      type: "payment_failed",
      event_id: "EVT-PAY-FAILED-0001",
      digest: "c" * 64
    )

    result = PaymentControl::OrderSimulator.apply(completed, failure)

    assert_equal "out_of_order_event_noop", result.fetch("action")
    assert_equal "COMPLETED", result.dig("order", "status")
    assert_equal 1, result.dig("order", "credit_count")
  end

  def test_refund_reverses_balance_once
    completed = PaymentControl::OrderSimulator.apply(initial_order, provider_event).fetch("order")
    request = internal_event(
      type: "refund_requested",
      event_id: "EVT-REFUND-REQUEST-0001",
      digest: "d" * 64
    )
    requested = PaymentControl::OrderSimulator.apply(completed, request).fetch("order")
    success = provider_event(
      type: "refund_succeeded",
      event_id: "EVT-REFUND-SUCCESS-0001",
      digest: "e" * 64
    )
    refunded = PaymentControl::OrderSimulator.apply(requested, success)
    duplicate = PaymentControl::OrderSimulator.apply(refunded.fetch("order"), success)

    assert_equal "REFUND_REQUESTED", requested.fetch("status")
    assert_equal "refund_completed", refunded.fetch("action")
    assert_equal "REFUNDED", duplicate.dig("order", "status")
    assert_equal 1, duplicate.dig("order", "reversal_count")
    assert_in_delta 10.0, duplicate.dig("order", "reversed_balance_usd"), 1e-12
  end

  def test_rejects_event_id_reuse_with_different_raw_body
    completed = PaymentControl::OrderSimulator.apply(initial_order, provider_event).fetch("order")
    conflict = provider_event(digest: "f" * 64)

    error = assert_raises(PaymentControl::ValidationError) do
      PaymentControl::OrderSimulator.apply(completed, conflict)
    end

    assert_includes error.errors, "event.event_id: was already used with a different raw body digest"
  end

  def test_validate_cli_returns_non_sensitive_json
    Tempfile.create(["pay01", ".yaml"]) do |file|
      file.write(YAML.dump(valid_config))
      file.flush

      stdout, stderr, status = Open3.capture3(
        "ruby", "ops/payment-control-simulator.rb", "validate", file.path
      )

      assert status.success?, stderr
      payload = JSON.parse(stdout)
      assert_equal true, payload.fetch("valid")
      assert_equal "disabled_simulation_only", payload.fetch("activation_state")
      refute_match(/private_key|merchant_key|webhook_secret|Bearer/i, stdout)
    end
  end
end
