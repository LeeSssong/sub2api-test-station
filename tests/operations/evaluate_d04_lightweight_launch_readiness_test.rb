# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "tempfile"
require "time"
require "yaml"

EVALUATOR_PATH = File.expand_path("../../ops/evaluate-d04-lightweight-launch-readiness.rb", __dir__)
load EVALUATOR_PATH if File.file?(EVALUATOR_PATH)

class EvaluateD04LightweightLaunchReadinessTest < Minitest::Test
  NOW = Time.iso8601("2026-07-22T10:00:00+08:00")

  def policy
    YAML.safe_load(File.read("config/operations/D04-lightweight-launch-readiness-v2.yaml"))
  end

  def healthy_snapshot
    snapshot = YAML.safe_load(File.read("config/operations/d04-lightweight-launch-snapshot.example.yaml"))
    snapshot["status"] = "live_non_sensitive"
    snapshot["approvals"]["launch_approved"] = true
    snapshot
  end

  def readiness_module
    return D04LightweightLaunchReadiness if defined?(D04LightweightLaunchReadiness)

    flunk "D04LightweightLaunchReadiness is not implemented"
  end

  def evaluate(snapshot = healthy_snapshot)
    readiness_module::Evaluator.new(policy, now: NOW).evaluate(snapshot)
  end

  def test_complete_fresh_snapshot_is_go_and_executes_nothing
    result = evaluate

    assert_equal "go", result.fetch("decision")
    assert_empty result.fetch("blocking_reasons")
    assert_equal false, result.fetch("real_action_executed")
    assert_equal false, result.fetch("external_system_contacted")
    assert_equal "D04-LIGHTWEIGHT-LAUNCH-v2", result.dig("policy", "policy_id")
    assert_equal 10.0, result.dig("policy", "active_upstream_balance_min_usd")
    refute result.fetch("derived").key?("balance_days")
    refute result.fetch("derived").key?("restore_drill_age_days")
  end

  def test_single_approval_and_generic_balance_finance_reasons
    snapshot = healthy_snapshot
    snapshot["approvals"]["launch_approved"] = false
    snapshot["active_upstream"]["balance_usd"] = nil
    snapshot["active_upstream"]["financial_recorded_at"] = "2026-07-22T09:30:00+08:00"

    result = evaluate(snapshot)

    assert_equal %w[
      launch_not_approved upstream_balance_unknown upstream_financial_evidence_stale
    ], result.fetch("blocking_reasons").take(3)
    assert_includes result.fetch("required_actions"), "record_launch_approval"
    assert_includes result.fetch("required_actions"), "refresh_upstream_financial_evidence"
  end

  def test_minimum_balance_and_quality_thresholds_are_blocking
    snapshot = healthy_snapshot
    snapshot["active_upstream"].merge!(
      "balance_usd" => 9.99,
      "sample_count" => 20,
      "success_rate" => 0.949,
      "error_rate" => 0.051,
      "ttft_p95_ms" => 5001,
      "total_latency_p95_ms" => 45_001
    )

    reasons = evaluate(snapshot).fetch("blocking_reasons")

    %w[
      upstream_balance_below_minimum upstream_success_rate_low
      upstream_error_rate_high upstream_ttft_p95_high
      upstream_total_latency_p95_high
    ].each { |reason| assert_includes reasons, reason }
  end

  def test_negative_active_upstream_balance_is_valid_but_blocks_opening
    snapshot = healthy_snapshot
    snapshot["active_upstream"]["balance_usd"] = -0.01

    reasons = evaluate(snapshot).fetch("blocking_reasons")

    assert_includes reasons, "upstream_balance_below_minimum"
  end

  def test_stale_or_non_natural_quality_and_insufficient_samples_fail_closed
    snapshot = healthy_snapshot
    snapshot["active_upstream"].merge!(
      "quality_source" => "manual_probe",
      "quality_recorded_at" => "2026-07-22T09:30:00+08:00",
      "sample_count" => 0,
      "success_rate" => 0.0,
      "error_rate" => 1.0,
      "ttft_p95_ms" => 99_999,
      "total_latency_p95_ms" => 99_999
    )

    reasons = evaluate(snapshot).fetch("blocking_reasons")

    assert_includes reasons, "upstream_quality_source_invalid"
    assert_includes reasons, "upstream_quality_metrics_stale"
    assert_includes reasons, "upstream_samples_insufficient"
    refute_includes reasons, "upstream_success_rate_low"
    refute_includes reasons, "upstream_error_rate_high"
    refute_includes reasons, "upstream_ttft_p95_high"
  end

  def test_backup_scope_and_operational_preflight_are_blocking
    snapshot = healthy_snapshot
    snapshot["account_backup"].merge!(
      "archive_created_at" => "2026-07-21T09:00:00+08:00",
      "sha256_verified" => false,
      "includes_d04_sqlite" => false
    )
    snapshot["modes"].merge!(
      "d04_mode" => "write", "registration_open" => true,
      "relay_ops_mode" => "probe", "feishu_command_mode" => "enabled"
    )
    snapshot["services"].merge!(
      "d04" => false, "unexplained_restart_count" => 1,
      "oom_killed" => true, "disk_used_ratio" => 0.81
    )
    snapshot["d04"].merge!(
      "launch_overlay_total_budget_usd" => 101.0,
      "registered_users" => 16,
      "balance_drift_usd" => 0.01,
      "read_only_reason" => "drift"
    )
    snapshot["operations"].merge!(
      "primary_owner" => "", "support_channel" => "", "rollback_validated" => false
    )

    reasons = evaluate(snapshot).fetch("blocking_reasons")

    %w[
      account_backup_stale account_backup_hash_unverified account_backup_scope_incomplete
      d04_not_read_only registration_not_closed relay_ops_not_read_only feishu_not_dry_run
      service_unhealthy container_restarted container_oom disk_pressure
      d04_configuration_mismatch d04_user_limit_exceeded d04_balance_drift
      d04_read_only_reason_present primary_owner_missing support_channel_missing
      rollback_unverified
    ].each { |reason| assert_includes reasons, reason }
  end

  def test_strict_validator_rejects_unknown_sections_and_credentials
    snapshot = healthy_snapshot
    snapshot["named_upstream"] = {"balance_usd" => 100}
    snapshot["api_key"] = "not-a-real-key"
    snapshot["operations"]["support_channel"] = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"

    errors = readiness_module::SnapshotValidator.new(snapshot).errors

    assert_includes errors, "named_upstream: is not allowed"
    assert_includes errors, "api_key: is not allowed"
    assert_includes errors, "api_key: credential fields are forbidden"
    assert_includes errors, "operations.support_channel: value looks like a secret"
  end

  def test_future_evidence_timestamp_is_validation_failure
    snapshot = healthy_snapshot
    snapshot["active_upstream"]["quality_recorded_at"] = "2026-07-22T10:00:01+08:00"

    error = assert_raises(readiness_module::ValidationError) { evaluate(snapshot) }
    assert_includes error.errors, "active_upstream.quality_recorded_at: must not be in the future"
  end

  def test_cli_emits_secret_free_json_and_does_not_contact_external_systems
    Tempfile.create(["d04-v2-policy", ".yaml"]) do |policy_file|
      Tempfile.create(["d04-v2-snapshot", ".yaml"]) do |snapshot_file|
        policy_file.write(YAML.dump(policy))
        snapshot_file.write(YAML.dump(healthy_snapshot))
        policy_file.flush
        snapshot_file.flush

        stdout, stderr, status = Open3.capture3(
          {"D04_LAUNCH_NOW" => NOW.iso8601},
          "ruby", EVALUATOR_PATH, "evaluate", policy_file.path, snapshot_file.path
        )

        assert status.success?, stderr
        payload = JSON.parse(stdout)
        assert_equal "go", payload.fetch("decision")
        assert_equal false, payload.fetch("real_action_executed")
        assert_equal false, payload.fetch("external_system_contacted")
        refute_match(/api[_-]?key|Bearer|Cookie|Authorization|password/i, stdout)
      end
    end
  end
end
