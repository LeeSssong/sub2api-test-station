#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "time"
require "yaml"

module D04LaunchReadiness
  class ValidationError < StandardError
    attr_reader :errors

    def initialize(errors)
      @errors = errors
      super(errors.join("; "))
    end
  end

  class ValidatorBase
    FORBIDDEN_CREDENTIAL_KEYS = /\A(?:api[_-]?key|token|access[_-]?token|refresh[_-]?token|cookie|authorization|password|private[_-]?key|client[_-]?secret|oauth(?:[_-]?.*)?|credentials?)\z/i
    SECRET_VALUE = /(?:Authorization:\s*Bearer\s+\S{16,}|Cookie:\s*\S+|sk-[a-z0-9]{16,}|BEGIN [A-Z ]*PRIVATE KEY)/i

    attr_reader :errors

    def initialize(document)
      @document = document
      @errors = []
      unless document.is_a?(Hash)
        add("root", "must be a mapping")
        return
      end
      validate
      scan_credentials(document)
    end

    private

    def require_keys(mapping, path, keys)
      keys.each { |key| add(join(path, key), "is required") unless mapping.key?(key) }
    end

    def section(path, keys)
      value = @document[path]
      unless value.is_a?(Hash)
        add(path, "must be a mapping") unless value.nil?
        return nil
      end
      require_keys(value, path, keys)
      value
    end

    def join(path, key)
      path.nil? || path.empty? ? key : "#{path}.#{key}"
    end

    def string(value, path, allow_empty: false)
      valid = value.is_a?(String) && (allow_empty || !value.strip.empty?)
      add(path, allow_empty ? "must be a string" : "must be a non-empty string") unless valid || value.nil?
      valid
    end

    def boolean(value, path)
      valid = value == true || value == false
      add(path, "must be true or false") unless valid || value.nil?
      valid
    end

    def number(value, path, min: nil, max: nil, nullable: false)
      return true if nullable && value.nil?
      unless value.is_a?(Numeric)
        add(path, "must be numeric") unless value.nil?
        return false
      end
      add(path, "must be at least #{min}") if !min.nil? && value < min
      add(path, "must be at most #{max}") if !max.nil? && value > max
      true
    end

    def integer(value, path, min: nil, max: nil)
      unless value.is_a?(Integer)
        add(path, "must be an integer") unless value.nil?
        return false
      end
      add(path, "must be at least #{min}") if !min.nil? && value < min
      add(path, "must be at most #{max}") if !max.nil? && value > max
      true
    end

    def timestamp(value, path)
      return unless string(value, path)
      Time.iso8601(value)
    rescue ArgumentError
      add(path, "must be an ISO-8601 timestamp")
    end

    def scan_credentials(value, path = nil)
      case value
      when Hash
        value.each do |key, child|
          child_path = join(path, key.to_s)
          add(child_path, "credential fields are forbidden") if key.to_s.match?(FORBIDDEN_CREDENTIAL_KEYS)
          scan_credentials(child, child_path)
        end
      when Array
        value.each_with_index { |child, index| scan_credentials(child, "#{path}[#{index}]") }
      when String
        add(path, "value looks like a secret") if value.match?(SECRET_VALUE)
      end
    end

    def add(path, message)
      @errors << "#{path}: #{message}"
    end
  end

  class PolicyValidator < ValidatorBase
    private

    def validate
      require_keys(@document, nil, %w[schema_version policy_id status action_execution_mode launch required_modes evidence])
      add("schema_version", "must equal 1") unless @document["schema_version"] == 1
      add("policy_id", "must equal D04-LAUNCH-READINESS-v1") unless @document["policy_id"] == "D04-LAUNCH-READINESS-v1"
      add("status", "must equal preparation_policy") unless @document["status"] == "preparation_policy"
      add("action_execution_mode", "must equal report_only") unless @document["action_execution_mode"] == "report_only"

      launch = section("launch", %w[
        max_users daily_login_credit_usd total_budget_usd budget_cost_bps
        provider_balance_reserve_floor_usd provider_balance_days_min financial_evidence_max_age_minutes metrics_max_age_minutes
        samples_15m_min success_rate_15m_min error_rate_15m_max ttft_p95_ms_15m_max
        total_latency_p95_ms_15m_max backup_age_hours_max restore_drill_age_days_max
        disk_used_ratio_max first_day_window_hours_min
      ])
      if launch
        integer(launch["max_users"], "launch.max_users", min: 1, max: 15)
        add("launch.max_users", "must equal 15") unless launch["max_users"] == 15
        number(launch["daily_login_credit_usd"], "launch.daily_login_credit_usd", min: 0.01)
        add("launch.daily_login_credit_usd", "must equal 20") unless launch["daily_login_credit_usd"] == 20.0
        number(launch["total_budget_usd"], "launch.total_budget_usd", min: 0.01)
        integer(launch["budget_cost_bps"], "launch.budget_cost_bps", min: 1, max: 100_000)
        %w[provider_balance_reserve_floor_usd provider_balance_days_min financial_evidence_max_age_minutes metrics_max_age_minutes samples_15m_min
           ttft_p95_ms_15m_max total_latency_p95_ms_15m_max backup_age_hours_max restore_drill_age_days_max
           first_day_window_hours_min].each { |key| number(launch[key], "launch.#{key}", min: 0) }
        %w[success_rate_15m_min error_rate_15m_max disk_used_ratio_max].each do |key|
          number(launch[key], "launch.#{key}", min: 0, max: 1)
        end
      end

      modes = section("required_modes", %w[d04_mode registration_open relay_ops_mode feishu_command_mode])
      if modes
        add("required_modes.d04_mode", "must equal read_only") unless modes["d04_mode"] == "read_only"
        add("required_modes.registration_open", "must be false") unless modes["registration_open"] == false
        add("required_modes.relay_ops_mode", "must equal read_only") unless modes["relay_ops_mode"] == "read_only"
        add("required_modes.feishu_command_mode", "must equal dry_run") unless modes["feishu_command_mode"] == "dry_run"
      end
    end
  end

  class SnapshotValidator < ValidatorBase
    private

    def validate
      require_keys(@document, nil, %w[schema_version snapshot_id status captured_at approvals modes services d04 wawazz backup operations])
      add("schema_version", "must equal 1") unless @document["schema_version"] == 1
      string(@document["snapshot_id"], "snapshot_id")
      unless %w[live_non_sensitive fictional].include?(@document["status"])
        add("status", "must equal live_non_sensitive or fictional")
      end
      timestamp(@document["captured_at"], "captured_at")

      approvals = section("approvals", %w[budget_approved opening_window_approved])
      approvals&.each { |key, value| boolean(value, "approvals.#{key}") }

      modes = section("modes", %w[d04_mode registration_open relay_ops_mode feishu_command_mode])
      if modes
        string(modes["d04_mode"], "modes.d04_mode")
        boolean(modes["registration_open"], "modes.registration_open")
        string(modes["relay_ops_mode"], "modes.relay_ops_mode")
        string(modes["feishu_command_mode"], "modes.feishu_command_mode")
      end

      services = section("services", %w[sub2api postgres redis caddy d04 relay_ops restart_count_max unexplained_restart_count oom_killed disk_used_ratio])
      if services
        %w[sub2api postgres redis caddy d04 relay_ops oom_killed].each { |key| boolean(services[key], "services.#{key}") }
        integer(services["restart_count_max"], "services.restart_count_max", min: 0)
        integer(services["unexplained_restart_count"], "services.unexplained_restart_count", min: 0)
        number(services["disk_used_ratio"], "services.disk_used_ratio", min: 0, max: 1)
      end

      d04 = section("d04", %w[registered_users successful_grants usage_records balance_drift_usd read_only_reason])
      if d04
        %w[registered_users successful_grants usage_records].each { |key| integer(d04[key], "d04.#{key}", min: 0) }
        number(d04["balance_drift_usd"], "d04.balance_drift_usd")
        string(d04["read_only_reason"], "d04.read_only_reason", allow_empty: true)
      end

      wawazz = section("wawazz", %w[balance_usd observed_daily_spend_usd financial_recorded_at metrics_recorded_at sample_count_15m success_rate_15m error_rate_15m ttft_p95_ms_15m total_latency_p95_ms_15m])
      if wawazz
        number(wawazz["balance_usd"], "wawazz.balance_usd", min: 0, nullable: true)
        number(wawazz["observed_daily_spend_usd"], "wawazz.observed_daily_spend_usd", min: 0, nullable: true)
        timestamp(wawazz["financial_recorded_at"], "wawazz.financial_recorded_at")
        timestamp(wawazz["metrics_recorded_at"], "wawazz.metrics_recorded_at")
        integer(wawazz["sample_count_15m"], "wawazz.sample_count_15m", min: 0)
        %w[success_rate_15m error_rate_15m].each { |key| number(wawazz[key], "wawazz.#{key}", min: 0, max: 1) }
        %w[ttft_p95_ms_15m total_latency_p95_ms_15m].each { |key| number(wawazz[key], "wawazz.#{key}", min: 0) }
      end

      backup = section("backup", %w[archive_created_at restore_verified_at archive_sha256_verified isolated_restore_verified encrypted_offsite_ready])
      if backup
        timestamp(backup["archive_created_at"], "backup.archive_created_at")
        timestamp(backup["restore_verified_at"], "backup.restore_verified_at")
        %w[archive_sha256_verified isolated_restore_verified encrypted_offsite_ready].each do |key|
          boolean(backup[key], "backup.#{key}")
        end
      end

      operations = section("operations", %w[primary_owner support_channel rollback_validated first_day_window_hours])
      if operations
        string(operations["primary_owner"], "operations.primary_owner", allow_empty: true)
        string(operations["support_channel"], "operations.support_channel", allow_empty: true)
        boolean(operations["rollback_validated"], "operations.rollback_validated")
        integer(operations["first_day_window_hours"], "operations.first_day_window_hours", min: 0)
      end
    end
  end

  class Evaluator
    ACTIONS = {
      "budget_not_approved" => "approve_launch_budget",
      "opening_window_not_approved" => "approve_opening_window",
      "wawazz_balance_below_reserve" => "replenish_wawazz_balance",
      "wawazz_balance_days_low" => "replenish_wawazz_balance",
      "wawazz_balance_unknown" => "refresh_wawazz_balance",
      "wawazz_spend_rate_unknown" => "refresh_wawazz_spend_rate",
      "wawazz_financial_evidence_stale" => "refresh_wawazz_financial_evidence",
      "wawazz_metrics_stale" => "refresh_wawazz_metrics",
      "wawazz_samples_insufficient" => "collect_wawazz_quality_window",
      "wawazz_success_rate_low" => "stabilize_wawazz_quality",
      "wawazz_error_rate_high" => "stabilize_wawazz_quality",
      "wawazz_ttft_p95_high" => "stabilize_wawazz_latency",
      "wawazz_total_latency_p95_high" => "stabilize_wawazz_latency",
      "backup_stale" => "create_fresh_production_backup",
      "restore_drill_stale" => "run_isolated_restore_drill",
      "backup_hash_unverified" => "verify_backup_hash",
      "isolated_restore_unverified" => "run_isolated_restore_drill",
      "d04_not_read_only" => "restore_d04_read_only",
      "registration_not_closed" => "close_d04_registration",
      "relay_ops_not_read_only" => "restore_relay_ops_read_only",
      "feishu_not_dry_run" => "restore_feishu_dry_run",
      "service_unhealthy" => "restore_service_health",
      "container_restarted" => "investigate_container_restart",
      "container_oom" => "investigate_container_oom",
      "disk_pressure" => "reduce_disk_pressure",
      "d04_balance_drift" => "reconcile_d04_balance",
      "d04_read_only_reason_present" => "resolve_d04_read_only_reason",
      "primary_owner_missing" => "assign_primary_owner",
      "support_channel_missing" => "assign_support_channel",
      "rollback_unverified" => "verify_registration_rollback",
      "first_day_window_too_short" => "schedule_24h_observation"
    }.freeze

    def initialize(policy, now: Time.now)
      @policy = policy
      @now = now
      policy_errors = PolicyValidator.new(policy).errors
      raise ValidationError, policy_errors unless policy_errors.empty?
    end

    def evaluate(snapshot)
      snapshot_errors = SnapshotValidator.new(snapshot).errors
      raise ValidationError, snapshot_errors unless snapshot_errors.empty?

      launch = @policy.fetch("launch")
      reasons = []
      approvals = snapshot.fetch("approvals")
      reasons << "budget_not_approved" unless approvals.fetch("budget_approved")
      reasons << "opening_window_not_approved" unless approvals.fetch("opening_window_approved")

      modes = snapshot.fetch("modes")
      reasons << "d04_not_read_only" unless modes.fetch("d04_mode") == "read_only"
      reasons << "registration_not_closed" unless modes.fetch("registration_open") == false
      reasons << "relay_ops_not_read_only" unless modes.fetch("relay_ops_mode") == "read_only"
      reasons << "feishu_not_dry_run" unless modes.fetch("feishu_command_mode") == "dry_run"

      services = snapshot.fetch("services")
      reasons << "service_unhealthy" unless %w[sub2api postgres redis caddy d04 relay_ops].all? { |key| services.fetch(key) }
      reasons << "container_restarted" if services.fetch("unexplained_restart_count") > 0
      reasons << "container_oom" if services.fetch("oom_killed")
      reasons << "disk_pressure" if services.fetch("disk_used_ratio") > launch.fetch("disk_used_ratio_max")

      d04 = snapshot.fetch("d04")
      reasons << "d04_balance_drift" unless d04.fetch("balance_drift_usd").abs < 0.000_001
      reasons << "d04_read_only_reason_present" unless d04.fetch("read_only_reason").strip.empty?

      wawazz = snapshot.fetch("wawazz")
      balance = wawazz.fetch("balance_usd")
      spend = wawazz.fetch("observed_daily_spend_usd")
      balance_days = nil
      reasons << "wawazz_balance_unknown" if balance.nil?
      reasons << "wawazz_spend_rate_unknown" if spend.nil? || spend <= 0
      unless balance.nil?
        reasons << "wawazz_balance_below_reserve" if balance < launch.fetch("provider_balance_reserve_floor_usd")
      end
      if !balance.nil? && !spend.nil? && spend > 0
        balance_days = (balance / spend).round(4)
        reasons << "wawazz_balance_days_low" if balance_days < launch.fetch("provider_balance_days_min")
      end
      financial_age_minutes = age_seconds(wawazz.fetch("financial_recorded_at")) / 60.0
      if financial_age_minutes > launch.fetch("financial_evidence_max_age_minutes")
        reasons << "wawazz_financial_evidence_stale"
      end
      metrics_age_minutes = age_seconds(wawazz.fetch("metrics_recorded_at")) / 60.0
      reasons << "wawazz_metrics_stale" if metrics_age_minutes > launch.fetch("metrics_max_age_minutes")
      enough_samples = wawazz.fetch("sample_count_15m") >= launch.fetch("samples_15m_min")
      reasons << "wawazz_samples_insufficient" unless enough_samples
      if enough_samples
        reasons << "wawazz_success_rate_low" if wawazz.fetch("success_rate_15m") < launch.fetch("success_rate_15m_min")
        reasons << "wawazz_error_rate_high" if wawazz.fetch("error_rate_15m") > launch.fetch("error_rate_15m_max")
        reasons << "wawazz_ttft_p95_high" if wawazz.fetch("ttft_p95_ms_15m") > launch.fetch("ttft_p95_ms_15m_max")
        if wawazz.fetch("total_latency_p95_ms_15m") > launch.fetch("total_latency_p95_ms_15m_max")
          reasons << "wawazz_total_latency_p95_high"
        end
      end

      backup = snapshot.fetch("backup")
      backup_age_hours = age_seconds(backup.fetch("archive_created_at")) / 3600.0
      restore_age_days = age_seconds(backup.fetch("restore_verified_at")) / 86_400.0
      reasons << "backup_stale" if backup_age_hours > launch.fetch("backup_age_hours_max")
      reasons << "restore_drill_stale" if restore_age_days > launch.fetch("restore_drill_age_days_max")
      reasons << "backup_hash_unverified" unless backup.fetch("archive_sha256_verified")
      reasons << "isolated_restore_unverified" unless backup.fetch("isolated_restore_verified")

      operations = snapshot.fetch("operations")
      reasons << "primary_owner_missing" if operations.fetch("primary_owner").strip.empty?
      reasons << "support_channel_missing" if operations.fetch("support_channel").strip.empty?
      reasons << "rollback_unverified" unless operations.fetch("rollback_validated")
      if operations.fetch("first_day_window_hours") < launch.fetch("first_day_window_hours_min")
        reasons << "first_day_window_too_short"
      end

      reasons.uniq!
      {
        "decision" => reasons.empty? ? "go" : "no_go",
        "blocking_reasons" => reasons,
        "required_actions" => reasons.map { |reason| ACTIONS.fetch(reason) }.uniq,
        "policy" => {
          "policy_id" => @policy.fetch("policy_id"),
          "max_users" => launch.fetch("max_users"),
          "daily_login_credit_usd" => launch.fetch("daily_login_credit_usd"),
          "total_budget_usd" => launch.fetch("total_budget_usd"),
          "budget_cost_bps" => launch.fetch("budget_cost_bps")
        },
        "derived" => {
          "provider_balance_days" => balance_days,
          "financial_evidence_age_minutes" => financial_age_minutes.round(2),
          "metrics_age_minutes" => metrics_age_minutes.round(2),
          "backup_age_hours" => backup_age_hours.round(2),
          "restore_drill_age_days" => restore_age_days.round(2),
          "encrypted_offsite_ready" => backup.fetch("encrypted_offsite_ready")
        },
        "real_action_executed" => false,
        "external_system_contacted" => false
      }
    end

    private

    def age_seconds(value)
      [@now - Time.iso8601(value), 0].max
    end
  end

  module CLI
    module_function

    def run(argv, out: $stdout, err: $stderr, env: ENV)
      unless argv.length == 3 && argv[0] == "evaluate"
        err.puts("usage: evaluate-d04-launch-readiness.rb evaluate POLICY SNAPSHOT")
        return 2
      end
      policy = YAML.safe_load(File.read(argv[1]))
      snapshot = YAML.safe_load(File.read(argv[2]))
      now = env["D04_LAUNCH_NOW"].to_s.empty? ? Time.now : Time.iso8601(env["D04_LAUNCH_NOW"])
      out.puts(JSON.generate(Evaluator.new(policy, now: now).evaluate(snapshot)))
      0
    rescue ValidationError, ArgumentError, Errno::ENOENT, Psych::Exception => exception
      err.puts(exception.message)
      1
    end
  end
end

exit D04LaunchReadiness::CLI.run(ARGV) if $PROGRAM_NAME == __FILE__
