# frozen_string_literal: true

require "digest"
require "json"
require "minitest/autorun"
require "time"
require "yaml"

EVALUATOR_V3_PATH = File.expand_path("../../ops/evaluate-d04-lightweight-launch-readiness-v3.rb", __dir__)
load EVALUATOR_V3_PATH if File.file?(EVALUATOR_V3_PATH)

class EvaluateD04LightweightLaunchReadinessV3Test < Minitest::Test
  NOW = Time.iso8601("2026-07-22T10:05:00Z")

  def policy
    YAML.safe_load(File.read("config/operations/D04-lightweight-launch-readiness-v3.yaml"))
  end

  def healthy_snapshot
    snapshot = JSON.parse(File.read("config/operations/d04-lightweight-launch-snapshot-v3.example.json"))
    snapshot["status"] = "live_non_sensitive"
    snapshot["approvals"]["launch_approved"] = true
    snapshot["captured_at"] = "2026-07-22T10:00:00Z"
    snapshot["upstream_discovery"]["recorded_at"] = "2026-07-22T10:00:00Z"
    snapshot["account_backup"]["archive_created_at"] = "2026-07-22T09:30:00Z"
    second = Marshal.load(Marshal.dump(snapshot["active_upstreams"].first))
    second["account_id"] = 202
    second["display_name"] = "Second account"
    second["group_ids"] = [3]
    snapshot["active_upstreams"] << second
    snapshot["active_upstreams"].sort_by! { |account| account.fetch("account_id") }
    snapshot["upstream_discovery"]["account_set_sha256"] = account_set_hash(snapshot)
    snapshot
  end

  def account_set_hash(snapshot)
    canonical = snapshot.fetch("active_upstreams").map do |account|
      {
        "account_id" => account.fetch("account_id"),
        "status" => account.fetch("status"),
        "schedulable" => account.fetch("schedulable"),
        "group_ids" => account.fetch("group_ids").sort
      }
    end
    Digest::SHA256.hexdigest(JSON.generate(canonical))
  end

  def readiness
    return D04LightweightLaunchReadinessV3 if defined?(D04LightweightLaunchReadinessV3)

    flunk "D04LightweightLaunchReadinessV3 is not implemented"
  end

  def evaluate(snapshot = healthy_snapshot, current_hash: nil)
    readiness::Evaluator.new(policy, now: NOW).evaluate(snapshot, current_account_set_sha256: current_hash)
  end

  def test_every_discovered_account_passes_for_go
    result = evaluate

    assert_equal "go", result.fetch("decision")
    assert_equal [101, 202], result.fetch("upstreams").map { |item| item.fetch("account_id") }
    assert result.fetch("upstreams").all? { |item| item.fetch("decision") == "go" }
    assert_equal false, result.fetch("real_action_executed")
    assert_equal false, result.fetch("external_system_contacted")
  end

  def test_one_passing_account_cannot_mask_one_failing_account
    snapshot = healthy_snapshot
    snapshot["active_upstreams"].last["balance_usd"] = 9.99

    result = evaluate(snapshot)
    first, second = result.fetch("upstreams")

    assert_equal "go", first.fetch("decision")
    assert_equal "no_go", second.fetch("decision")
    assert_includes second.fetch("blocking_reasons"), "upstream_balance_below_minimum"
    assert_includes result.fetch("blocking_reasons"), "upstream_balance_below_minimum"
    assert_equal "no_go", result.fetch("decision")
  end

  def test_missing_account_evidence_and_runtime_block_fail_closed
    snapshot = healthy_snapshot
    account = snapshot["active_upstreams"].first
    account["runtime_available"] = false
    account["runtime_block_reason"] = "rate_limited"
    account["balance_usd"] = nil
    account["financial_recorded_at"] = nil
    account["quality_source"] = ""
    account["quality_recorded_at"] = nil
    account["sample_count"] = nil
    account["success_rate"] = nil
    account["error_rate"] = nil
    account["ttft_p95_ms"] = nil
    account["total_latency_p95_ms"] = nil

    reasons = evaluate(snapshot).fetch("upstreams").first.fetch("blocking_reasons")

    assert_includes reasons, "upstream_temporarily_unavailable"
    assert_includes reasons, "upstream_balance_unknown"
    assert_includes reasons, "upstream_quality_attribution_missing"
  end

  def test_account_set_hash_and_preflight_drift_are_blocking
    snapshot = healthy_snapshot
    snapshot["upstream_discovery"]["account_set_sha256"] = "0" * 64

    result = evaluate(snapshot, current_hash: "1" * 64)

    assert_includes result.fetch("blocking_reasons"), "upstream_account_set_changed"
    assert_equal "no_go", result.fetch("decision")
  end

  def test_empty_active_set_is_no_go
    snapshot = healthy_snapshot
    snapshot["active_upstreams"] = []
    snapshot["upstream_discovery"]["account_set_sha256"] = Digest::SHA256.hexdigest("[]")

    result = evaluate(snapshot)

    assert_equal "no_go", result.fetch("decision")
    assert_includes result.fetch("blocking_reasons"), "active_upstreams_empty"
    assert_empty result.fetch("upstreams")
  end
end

