# frozen_string_literal: true

require "minitest/autorun"
require_relative "../../ops/manual-ledger"

class ManualLedgerTest < Minitest::Test
  def opened_payload
    {
      "event_id" => "EVT-OPEN-0001",
      "event_type" => "ledger_opened",
      "occurred_at" => "2026-07-15T00:00:00Z",
      "operator_ref" => "operator-example",
      "ledger_id" => "MVP-LEDGER-EXAMPLE",
      "mode" => "simulation",
      "payment_currency" => "CNY",
      "balance_currency" => "USD",
      "notes" => "Fictional ledger."
    }
  end

  def payment_payload
    {
      "event_id" => "EVT-PAY-0001",
      "event_type" => "payment_received",
      "occurred_at" => "2026-07-15T00:01:00Z",
      "operator_ref" => "operator-example",
      "user_ref" => "USR-EXAMPLE-01",
      "order_id" => "ORD-SIM-0001",
      "amount_cny" => 72.0,
      "usd_per_cny" => 1.0 / 7.2,
      "expected_balance_usd" => 10.0,
      "external_reference" => "SIMULATED-PAYMENT-0001",
      "notes" => "No real payment."
    }
  end

  def adjustment_payload(amount: 10.0, status: "simulated", event_id: "EVT-BAL-0001",
                         idempotency_key: "ledger-ord-sim-0001-add-01")
    {
      "event_id" => event_id,
      "event_type" => "balance_adjustment",
      "occurred_at" => "2026-07-15T00:02:00Z",
      "operator_ref" => "operator-example",
      "user_ref" => "USR-EXAMPLE-01",
      "reference_event_id" => "EVT-PAY-0001",
      "sub2api_user_id" => 1001,
      "operation" => "add",
      "amount_usd" => amount,
      "idempotency_key" => idempotency_key,
      "status" => status,
      "result_reference" => "SIMULATED-RESULT-0001",
      "notes" => "No API request sent."
    }
  end

  def usage_payload
    {
      "event_id" => "EVT-USAGE-0001",
      "event_type" => "usage_snapshot",
      "occurred_at" => "2026-07-15T23:59:00Z",
      "operator_ref" => "operator-example",
      "period_start" => "2026-07-15T00:00:00Z",
      "period_end" => "2026-07-15T23:59:00Z",
      "site_usage_usd" => 1.5,
      "upstream_cost_cny" => 8.0,
      "request_count" => 12,
      "notes" => "Fictional usage."
    }
  end

  def build_ledger(*payloads)
    payloads.reduce([]) do |events, payload|
      events + [ManualLedger.build_event(events, payload)]
    end
  end

  def valid_ledger
    build_ledger(opened_payload, payment_payload, adjustment_payload, usage_payload)
  end

  def test_builds_and_verifies_a_hash_chain
    events = valid_ledger

    assert_empty ManualLedger.verify(events)
    assert_equal [1, 2, 3, 4], events.map { |event| event["sequence"] }
    assert_equal "GENESIS", events.first["previous_hash"]
    assert_equal events[1]["event_hash"], events[2]["previous_hash"]
    assert events.all? { |event| event["event_hash"].match?(/\A[0-9a-f]{64}\z/) }
  end

  def test_detects_payload_tampering
    events = valid_ledger
    events[1]["amount_cny"] = 720.0

    assert_includes ManualLedger.verify(events), "events[1].event_hash: does not match event content"
  end

  def test_reconciles_payment_credit_and_usage
    summary = ManualLedger.summary(valid_ledger)

    assert_in_delta 72.0, summary["payment_received_cny"], 1e-12
    assert_in_delta 10.0, summary["expected_balance_usd"], 1e-12
    assert_in_delta 10.0, summary["balance_added_usd"], 1e-12
    assert_in_delta 0.0, summary["payment_credit_variance_usd"], 1e-12
    assert_equal [], summary["unreconciled_order_ids"]
    assert_in_delta 1.5, summary["site_usage_usd"], 1e-12
    assert_in_delta 8.0, summary["upstream_cost_cny"], 1e-12
  end

  def test_reports_under_credit_as_unreconciled
    events = build_ledger(opened_payload, payment_payload, adjustment_payload(amount: 9.0))
    summary = ManualLedger.summary(events)

    assert_in_delta(-1.0, summary["payment_credit_variance_usd"], 1e-12)
    assert_equal ["ORD-SIM-0001"], summary["unreconciled_order_ids"]
  end

  def test_rejects_duplicate_idempotency_keys
    events = build_ledger(opened_payload, payment_payload, adjustment_payload)
    duplicate = adjustment_payload(
      event_id: "EVT-BAL-0002",
      idempotency_key: "ledger-ord-sim-0001-add-01"
    )

    error = assert_raises(ManualLedger::ValidationError) do
      ManualLedger.build_event(events, duplicate)
    end
    assert_includes error.errors, "events[3].idempotency_key: duplicates ledger-ord-sim-0001-add-01"
  end

  def test_simulation_ledger_cannot_claim_succeeded_adjustment
    events = build_ledger(opened_payload, payment_payload)

    error = assert_raises(ManualLedger::ValidationError) do
      ManualLedger.build_event(events, adjustment_payload(status: "succeeded"))
    end
    assert_includes error.errors, "events[2].status: simulation ledger adjustments must use simulated"
  end

  def test_request_preview_has_no_authentication_header
    event = valid_ledger[2]
    preview = ManualLedger.request_preview(event)

    assert_equal "POST", preview["method"]
    assert_equal "/api/v1/admin/users/1001/balance", preview["path"]
    assert_equal "ledger-ord-sim-0001-add-01", preview.dig("headers", "Idempotency-Key")
    refute preview.fetch("headers").keys.any? { |key| key.downcase.include?("api") || key.downcase.include?("auth") }
    assert_equal({ "balance" => 10.0, "operation" => "add", "notes" => "ledger event EVT-BAL-0001" },
                 preview["body"])
  end

  def test_rejects_secret_fields_and_values
    payload = payment_payload.merge(
      "api_key" => "not-a-real-key",
      "notes" => "Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456"
    )

    error = assert_raises(ManualLedger::ValidationError) do
      build_ledger(opened_payload, payload)
    end
    assert_includes error.errors, "events[1].api_key: credential fields are forbidden"
    assert_includes error.errors, "events[1].notes: value looks like a secret"
  end
end
