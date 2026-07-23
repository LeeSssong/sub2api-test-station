# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "tempfile"
require "yaml"
require_relative "../../ops/evaluate-operations-baseline"

class EvaluateOperationsBaselineTest < Minitest::Test
  def valid_policy
    {
      "schema_version" => 1,
      "ops_id" => "OPS01",
      "status" => "fictional",
      "reviewed_at" => "2026-07-15",
      "external_actions_deferred" => true,
      "action_execution_mode" => "report_only",
      "cadence" => {
        "daily_review_time" => "09:00 Asia/Shanghai",
        "weekly_review_day" => "monday",
        "monthly_restore_day" => 1
      },
      "thresholds" => {
        "balance_difference_usd_abs_max" => 0.01,
        "backup_age_hours_max" => 24,
        "restore_drill_age_days_max" => 31,
        "certificate_days_min" => 14,
        "disk_used_ratio_max" => 0.80,
        "failed_admin_logins_1h_max" => 5,
        "upstream_balance_days_min" => 3.0,
        "upstream_success_rate_min" => 0.95,
        "upstream_rate_limit_ratio_max" => 0.15,
        "upstream_5xx_ratio_max" => 0.05,
        "upstream_ttft_p95_ms_max" => 5000,
        "stream_interruption_ratio_max" => 0.01,
        "daily_total_cost_usd_max" => 20.0,
        "per_user_daily_cost_usd_max" => 5.0,
        "request_id_coverage_ratio_min" => 1.0,
        "gross_margin_percent_min" => 20.0,
        "account_expiry_warning_hours" => 72
      },
      "backup_policy" => {
        "postgres_format" => "custom_pg_dump_fc",
        "offsite_provider" => "cloudflare_r2_standard",
        "offsite_tool" => "restic",
        "local_retention_days" => 7,
        "offsite_retention_days" => 30,
        "encrypted_offsite_required" => true,
        "restore_drill_interval_days" => 31,
        "dry_run_only" => true
      },
      "stop_loss_actions" => {
        "credential_exposure" => %w[
          disable_affected_channel revoke_exposed_credential rotate_credential preserve_evidence
        ],
        "billing_integrity" => %w[
          disable_recharge freeze_balance_adjustments disable_affected_channel
          preserve_evidence reconcile_ledger
        ],
        "all_upstreams_down" => %w[
          disable_registration disable_affected_models publish_status_notice preserve_evidence
        ],
        "core_service_down" => %w[
          disable_registration disable_affected_models publish_status_notice preserve_evidence
        ]
      },
      "evidence" => {
        "sub2api_version" => "v0.1.155",
        "sub2api_commit" => "41cec0db059ffb82d0efdcfcf07a24ab51fbfe97",
        "notes" => "Fictional report-only operations policy."
      }
    }
  end

  def healthy_snapshot
    {
      "schema_version" => 1,
      "snapshot_id" => "OPS-SIM-20260715",
      "status" => "fictional",
      "captured_at" => "2026-07-15T09:00:00+08:00",
      "system" => {
        "services" => {
          "sub2api" => true,
          "postgres" => true,
          "redis" => true,
          "caddy" => true
        },
        "certificate_days_remaining" => 60,
        "disk_used_ratio" => 0.35,
        "backup_age_hours" => 8,
        "restore_drill_age_days" => 10,
        "failed_admin_logins_1h" => 0,
        "credential_exposure_detected" => false
      },
      "billing" => {
        "balance_difference_usd" => 0.0,
        "duplicate_credit_count" => 0,
        "unexplained_adjustment_count" => 0,
        "daily_total_cost_usd" => 4.0,
        "max_user_daily_cost_usd" => 1.0
      },
      "traffic" => {
        "request_count" => 100,
        "request_id_coverage_ratio" => 1.0
      },
      "profit" => {
        "weekly_revenue_cny" => 100.0,
        "weekly_full_cost_cny" => 70.0
      },
      "upstreams" => [
        {
          "upstream_id" => "UP01",
          "status" => "fictional",
          "available" => true,
          "balance_days_remaining" => 10.0,
          "success_rate" => 0.99,
          "rate_limit_ratio" => 0.02,
          "server_error_ratio" => 0.01,
          "ttft_p95_ms" => 1200,
          "stream_interruption_ratio" => 0.0
        }
      ],
      "account_pools" => [
        {
          "pool_id" => "ACC01",
          "status" => "fictional",
          "available_accounts" => 1,
          "error_accounts" => 0,
          "minimum_expiry_hours" => 720
        }
      ]
    }
  end

  def policy_errors(document = valid_policy)
    OperationsBaseline::PolicyValidator.new(document).errors
  end

  def snapshot_errors(document = healthy_snapshot)
    OperationsBaseline::SnapshotValidator.new(document).errors
  end

  def evaluate(snapshot = healthy_snapshot, policy = valid_policy)
    OperationsBaseline::Evaluator.new(policy).evaluate(snapshot)
  end

  def test_accepts_complete_policy_and_healthy_snapshot
    assert_empty policy_errors
    assert_empty snapshot_errors
  end

  def test_requires_report_only_mode_dry_run_backup_and_no_credentials
    policy = valid_policy
    policy["action_execution_mode"] = "automatic"
    policy["backup_policy"]["dry_run_only"] = false
    policy["backup_policy"]["offsite_provider"] = "unreviewed_paid_storage"
    policy["evidence"]["notes"] = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456"
    policy["api_key"] = "not-a-real-key"

    errors = policy_errors(policy)

    assert_includes errors, "action_execution_mode: must equal report_only"
    assert_includes errors, "backup_policy.dry_run_only: must be true"
    assert_includes errors, "backup_policy.offsite_provider: must equal cloudflare_r2_standard"
    assert_includes errors, "evidence.notes: value looks like a secret"
    assert_includes errors, "api_key: credential fields are forbidden"
  end

  def test_rejects_missing_snapshot_fields_and_snapshot_credentials
    snapshot = healthy_snapshot
    snapshot["traffic"].delete("request_id_coverage_ratio")
    snapshot["profit"].delete("weekly_full_cost_cny")
    snapshot["api_key"] = "not-a-real-key"

    errors = snapshot_errors(snapshot)

    assert_includes errors, "traffic.request_id_coverage_ratio: is required"
    assert_includes errors, "profit.weekly_full_cost_cny: is required"
    assert_includes errors, "api_key: credential fields are forbidden"
  end

  def test_healthy_snapshot_has_no_critical_or_high_alerts
    result = evaluate

    assert_equal 0, result.dig("summary", "critical")
    assert_equal 0, result.dig("summary", "high")
    assert_empty result.fetch("recommended_actions")
    assert_equal true, result.fetch("report_only")
    assert_equal false, result.fetch("real_action_executed")
    assert_equal false, result.fetch("external_system_contacted")
  end

  def test_billing_integrity_incident_outputs_stop_loss_in_order
    snapshot = healthy_snapshot
    snapshot["billing"]["balance_difference_usd"] = 0.02
    snapshot["billing"]["duplicate_credit_count"] = 1

    result = evaluate(snapshot)

    codes = result.fetch("alerts").map { |alert| alert.fetch("code") }
    assert_includes codes, "billing_balance_difference"
    assert_includes codes, "billing_duplicate_credit"
    assert_equal %w[
      disable_recharge freeze_balance_adjustments disable_affected_channel
      preserve_evidence reconcile_ledger
    ], result.fetch("recommended_actions")
  end

  def test_credential_exposure_outputs_revoke_and_rotation_sequence
    snapshot = healthy_snapshot
    snapshot["system"]["credential_exposure_detected"] = true

    result = evaluate(snapshot)

    assert_equal "credential_exposure", result.fetch("alerts").first.fetch("code")
    assert_equal %w[
      disable_affected_channel revoke_exposed_credential rotate_credential preserve_evidence
    ], result.fetch("recommended_actions")
  end

  def test_core_service_and_all_upstream_outage_deduplicate_actions
    snapshot = healthy_snapshot
    snapshot["system"]["services"]["sub2api"] = false
    snapshot["upstreams"].each { |upstream| upstream["available"] = false }

    result = evaluate(snapshot)

    assert_equal 2, result.dig("summary", "critical")
    assert_equal %w[
      disable_registration disable_affected_models publish_status_notice preserve_evidence
    ], result.fetch("recommended_actions")
  end

  def test_operational_thresholds_generate_high_alerts
    snapshot = healthy_snapshot
    snapshot["system"].merge!(
      "backup_age_hours" => 25,
      "disk_used_ratio" => 0.81,
      "failed_admin_logins_1h" => 5
    )
    snapshot["billing"].merge!(
      "daily_total_cost_usd" => 21.0,
      "max_user_daily_cost_usd" => 6.0
    )

    result = evaluate(snapshot)
    high_codes = result.fetch("alerts").select { |alert| alert["severity"] == "high" }
                       .map { |alert| alert.fetch("code") }

    assert_equal %w[
      backup_stale daily_cost_cap_exceeded disk_pressure failed_admin_logins
      user_cost_cap_exceeded
    ], high_codes.sort
  end

  def test_upstream_and_maintenance_thresholds_generate_warnings
    snapshot = healthy_snapshot
    snapshot["system"]["certificate_days_remaining"] = 13
    snapshot["system"]["restore_drill_age_days"] = 32
    snapshot["traffic"]["request_id_coverage_ratio"] = 0.99
    snapshot["upstreams"].first.merge!(
      "balance_days_remaining" => 2.0,
      "success_rate" => 0.94,
      "rate_limit_ratio" => 0.16,
      "server_error_ratio" => 0.06,
      "ttft_p95_ms" => 5001,
      "stream_interruption_ratio" => 0.02
    )

    codes = evaluate(snapshot).fetch("alerts").map { |alert| alert.fetch("code") }

    %w[
      certificate_expiry restore_drill_stale request_id_coverage upstream_balance_low
      upstream_success_low upstream_rate_limit_high upstream_5xx_high upstream_ttft_high
      upstream_stream_interruptions
    ].each { |code| assert_includes codes, code }
  end

  def test_profit_and_account_pool_warnings
    snapshot = healthy_snapshot
    snapshot["profit"]["weekly_full_cost_cny"] = 85.0
    snapshot["account_pools"].first["minimum_expiry_hours"] = 48

    codes = evaluate(snapshot).fetch("alerts").map { |alert| alert.fetch("code") }

    assert_includes codes, "gross_margin_low"
    assert_includes codes, "account_pool_expiring"
  end

  def test_zero_request_day_does_not_warn_about_request_id_coverage
    snapshot = healthy_snapshot
    snapshot["traffic"]["request_count"] = 0
    snapshot["traffic"]["request_id_coverage_ratio"] = 0.0

    codes = evaluate(snapshot).fetch("alerts").map { |alert| alert.fetch("code") }

    refute_includes codes, "request_id_coverage"
  end

  def test_account_pool_errors_generate_warning
    snapshot = healthy_snapshot
    snapshot["account_pools"].first["error_accounts"] = 1

    codes = evaluate(snapshot).fetch("alerts").map { |alert| alert.fetch("code") }

    assert_includes codes, "account_pool_errors"
  end

  def test_unavailable_account_pool_is_high_and_disables_affected_models
    snapshot = healthy_snapshot
    snapshot["account_pools"].first["available_accounts"] = 0

    result = evaluate(snapshot)

    alert = result.fetch("alerts").find { |item| item["code"] == "account_pool_unavailable" }
    assert_equal "high", alert.fetch("severity")
    assert_includes result.fetch("recommended_actions"), "disable_affected_models"
  end

  def test_evaluate_cli_is_non_sensitive_and_offline
    Tempfile.create(["ops01", ".yaml"]) do |policy_file|
      Tempfile.create(["snapshot", ".yaml"]) do |snapshot_file|
        policy_file.write(YAML.dump(valid_policy))
        snapshot_file.write(YAML.dump(healthy_snapshot))
        policy_file.flush
        snapshot_file.flush

        stdout, stderr, status = Open3.capture3(
          "ruby", "ops/evaluate-operations-baseline.rb", "evaluate",
          policy_file.path, snapshot_file.path
        )

        assert status.success?, stderr
        payload = JSON.parse(stdout)
        assert_equal true, payload.fetch("report_only")
        assert_equal false, payload.fetch("real_action_executed")
        assert_equal false, payload.fetch("external_system_contacted")
        refute_match(/api_key|Bearer|Cookie|Authorization/i, stdout)
      end
    end
  end
end
