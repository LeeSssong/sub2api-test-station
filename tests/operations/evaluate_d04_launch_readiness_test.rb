# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "tempfile"
require "yaml"
require_relative "../../ops/evaluate-d04-launch-readiness"

class EvaluateD04LaunchReadinessTest < Minitest::Test
  def policy
    YAML.safe_load(File.read("config/operations/D04-launch-readiness-v1.yaml"))
  end

  def healthy_snapshot
    {
      "schema_version" => 1,
      "snapshot_id" => "D04-LAUNCH-HEALTHY",
      "status" => "live_non_sensitive",
      "captured_at" => "2026-07-22T10:00:00+08:00",
      "approvals" => {"budget_approved" => true, "opening_window_approved" => true},
      "modes" => {
        "d04_mode" => "read_only", "registration_open" => false,
        "relay_ops_mode" => "read_only", "feishu_command_mode" => "dry_run"
      },
      "services" => {
        "sub2api" => true, "postgres" => true, "redis" => true, "caddy" => true,
        "d04" => true, "relay_ops" => true, "restart_count_max" => 0,
        "unexplained_restart_count" => 0, "oom_killed" => false,
        "disk_used_ratio" => 0.30
      },
      "d04" => {
        "registered_users" => 1, "successful_grants" => 1, "usage_records" => 0,
        "balance_drift_usd" => 0.0, "read_only_reason" => ""
      },
      "wawazz" => {
        "balance_usd" => 400.0, "observed_daily_spend_usd" => 50.0,
        "financial_recorded_at" => "2026-07-22T09:50:00+08:00",
        "metrics_recorded_at" => "2026-07-22T09:50:00+08:00", "sample_count_15m" => 30,
        "success_rate_15m" => 0.99, "error_rate_15m" => 0.01,
        "ttft_p95_ms_15m" => 2000, "total_latency_p95_ms_15m" => 8000
      },
      "backup" => {
        "archive_created_at" => "2026-07-22T09:30:00+08:00",
        "restore_verified_at" => "2026-07-22T09:40:00+08:00",
        "archive_sha256_verified" => true, "isolated_restore_verified" => true,
        "encrypted_offsite_ready" => false
      },
      "operations" => {
        "primary_owner" => "site-owner", "support_channel" => "feishu-operations-group",
        "rollback_validated" => true, "first_day_window_hours" => 24
      }
    }
  end

  def evaluate(snapshot = healthy_snapshot, now: Time.iso8601("2026-07-22T10:00:00+08:00"))
    D04LaunchReadiness::Evaluator.new(policy, now: now).evaluate(snapshot)
  end

  def test_complete_fresh_snapshot_is_go_but_executes_nothing
    result = evaluate

    assert_equal "go", result.fetch("decision")
    assert_empty result.fetch("blocking_reasons")
    assert_equal false, result.fetch("real_action_executed")
    assert_equal false, result.fetch("external_system_contacted")
    assert_equal 8.0, result.dig("derived", "provider_balance_days")
    assert_equal 100.0, result.dig("policy", "total_budget_usd")
  end

  def test_balance_must_cover_three_days_and_the_reserve_floor
    snapshot = healthy_snapshot
    snapshot["wawazz"]["balance_usd"] = 9.62
    snapshot["wawazz"]["observed_daily_spend_usd"] = 40.0

    result = evaluate(snapshot)

    assert_equal "no_go", result.fetch("decision")
    assert_includes result.fetch("blocking_reasons"), "wawazz_balance_below_reserve"
    assert_includes result.fetch("blocking_reasons"), "wawazz_balance_days_low"
    assert_includes result.fetch("required_actions"), "replenish_wawazz_balance"
  end

  def test_unknown_spend_rate_and_stale_metrics_fail_closed
    snapshot = healthy_snapshot
    snapshot["wawazz"]["observed_daily_spend_usd"] = nil
    snapshot["wawazz"]["metrics_recorded_at"] = "2026-07-22T09:30:00+08:00"

    reasons = evaluate(snapshot).fetch("blocking_reasons")

    assert_includes reasons, "wawazz_spend_rate_unknown"
    assert_includes reasons, "wawazz_metrics_stale"
  end

  def test_stale_balance_and_spend_evidence_fail_closed
    snapshot = healthy_snapshot
    snapshot["wawazz"]["financial_recorded_at"] = "2026-07-22T09:30:00+08:00"

    result = evaluate(snapshot)

    assert_includes result.fetch("blocking_reasons"), "wawazz_financial_evidence_stale"
    assert_includes result.fetch("required_actions"), "refresh_wawazz_financial_evidence"
  end

  def test_each_quality_gate_is_blocking
    snapshot = healthy_snapshot
    snapshot["wawazz"].merge!(
      "sample_count_15m" => 20,
      "success_rate_15m" => 0.949,
      "error_rate_15m" => 0.051,
      "ttft_p95_ms_15m" => 5001,
      "total_latency_p95_ms_15m" => 45_001
    )

    reasons = evaluate(snapshot).fetch("blocking_reasons")

    %w[
      wawazz_success_rate_low wawazz_error_rate_high
      wawazz_ttft_p95_high wawazz_total_latency_p95_high
    ].each { |code| assert_includes reasons, code }
  end

  def test_zero_sample_window_reports_insufficient_evidence_not_false_quality
    snapshot = healthy_snapshot
    snapshot["wawazz"].merge!(
      "sample_count_15m" => 0, "success_rate_15m" => 0.0, "error_rate_15m" => 0.0,
      "ttft_p95_ms_15m" => 0, "total_latency_p95_ms_15m" => 0
    )

    reasons = evaluate(snapshot).fetch("blocking_reasons")

    assert_includes reasons, "wawazz_samples_insufficient"
    refute_includes reasons, "wawazz_success_rate_low"
    refute_includes reasons, "wawazz_error_rate_high"
  end

  def test_acknowledged_historical_restart_is_not_a_permanent_blocker
    snapshot = healthy_snapshot
    snapshot["services"]["restart_count_max"] = 1
    snapshot["services"]["unexplained_restart_count"] = 0

    refute_includes evaluate(snapshot).fetch("blocking_reasons"), "container_restarted"
  end

  def test_backup_health_modes_and_ownership_are_hard_gates
    snapshot = healthy_snapshot
    snapshot["backup"].merge!(
      "archive_created_at" => "2026-07-21T09:00:00+08:00",
      "restore_verified_at" => "2026-06-20T09:00:00+08:00",
      "archive_sha256_verified" => false,
      "isolated_restore_verified" => false
    )
    snapshot["modes"].merge!("d04_mode" => "write", "registration_open" => true,
                              "relay_ops_mode" => "probe", "feishu_command_mode" => "enabled")
    snapshot["services"].merge!("d04" => false, "restart_count_max" => 1,
                                 "unexplained_restart_count" => 1,
                                 "oom_killed" => true, "disk_used_ratio" => 0.81)
    snapshot["d04"].merge!("balance_drift_usd" => 0.01, "read_only_reason" => "drift")
    snapshot["operations"].merge!("primary_owner" => "", "support_channel" => "",
                                   "rollback_validated" => false, "first_day_window_hours" => 8)

    reasons = evaluate(snapshot).fetch("blocking_reasons")

    %w[
      backup_stale restore_drill_stale backup_hash_unverified isolated_restore_unverified
      d04_not_read_only registration_not_closed relay_ops_not_read_only feishu_not_dry_run
      service_unhealthy container_restarted container_oom disk_pressure d04_balance_drift
      d04_read_only_reason_present primary_owner_missing support_channel_missing
      rollback_unverified first_day_window_too_short
    ].each { |code| assert_includes reasons, code }
  end

  def test_missing_business_approvals_are_blocking
    snapshot = healthy_snapshot
    snapshot["approvals"]["budget_approved"] = false
    snapshot["approvals"]["opening_window_approved"] = false

    result = evaluate(snapshot)

    assert_includes result.fetch("blocking_reasons"), "budget_not_approved"
    assert_includes result.fetch("blocking_reasons"), "opening_window_not_approved"
    assert_equal %w[approve_launch_budget approve_opening_window],
                 result.fetch("required_actions").grep(/approve/)
  end

  def test_validator_rejects_credentials_and_missing_fields
    snapshot = healthy_snapshot
    snapshot.delete("backup")
    snapshot["api_key"] = "not-a-real-key"
    snapshot["operations"]["support_channel"] = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"

    errors = D04LaunchReadiness::SnapshotValidator.new(snapshot).errors

    assert_includes errors, "backup: is required"
    assert_includes errors, "api_key: credential fields are forbidden"
    assert_includes errors, "operations.support_channel: value looks like a secret"
  end

  def test_cli_emits_secret_free_json_and_never_contacts_or_executes
    Tempfile.create(["d04-policy", ".yaml"]) do |policy_file|
      Tempfile.create(["d04-snapshot", ".yaml"]) do |snapshot_file|
        policy_file.write(YAML.dump(policy))
        snapshot_file.write(YAML.dump(healthy_snapshot))
        policy_file.flush
        snapshot_file.flush

        stdout, stderr, status = Open3.capture3(
          {"D04_LAUNCH_NOW" => "2026-07-22T10:00:00+08:00"},
          "ruby", "ops/evaluate-d04-launch-readiness.rb", "evaluate",
          policy_file.path, snapshot_file.path
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
