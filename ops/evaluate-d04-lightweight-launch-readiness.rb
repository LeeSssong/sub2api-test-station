#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "time"
require "yaml"

module D04LightweightLaunchReadiness
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

    def exact_keys(mapping, path, keys)
      keys.each { |key| add(join(path, key), "is required") unless mapping.key?(key) }
      mapping.each_key do |key|
        add(join(path, key.to_s), "is not allowed") unless keys.include?(key.to_s)
      end
    end

    def section(path, keys)
      value = @document[path]
      unless value.is_a?(Hash)
        add(path, "must be a mapping") unless value.nil?
        return nil
      end
      exact_keys(value, path, keys)
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
      exact_keys(@document, nil, %w[
        schema_version policy_id status action_execution_mode launch required_modes evidence
      ])
      add("schema_version", "must equal 2") unless @document["schema_version"] == 2
      unless @document["policy_id"] == "D04-LIGHTWEIGHT-LAUNCH-v2"
        add("policy_id", "must equal D04-LIGHTWEIGHT-LAUNCH-v2")
      end
      add("status", "must equal preparation_policy") unless @document["status"] == "preparation_policy"
      unless @document["action_execution_mode"] == "report_only"
        add("action_execution_mode", "must equal report_only")
      end

      launch = section("launch", %w[
        max_users daily_login_credit_usd total_budget_usd budget_cost_bps
        active_upstream_balance_min_usd financial_evidence_max_age_minutes
        quality_window_minutes quality_evidence_max_age_minutes samples_min
        success_rate_min error_rate_max ttft_p95_ms_max total_latency_p95_ms_max
        account_backup_age_hours_max disk_used_ratio_max
      ])
      validate_launch(launch) if launch

      modes = section("required_modes", %w[
        d04_mode registration_open relay_ops_mode feishu_command_mode
      ])
      validate_modes(modes, "required_modes") if modes

      evidence = section("evidence", %w[upstream_role quality_source notes])
      if evidence
        add("evidence.upstream_role", "must equal active_upstream") unless evidence["upstream_role"] == "active_upstream"
        unless evidence["quality_source"] == "natural_production_traffic"
          add("evidence.quality_source", "must equal natural_production_traffic")
        end
        string(evidence["notes"], "evidence.notes")
      end
    end

    def validate_launch(launch)
      integer(launch["max_users"], "launch.max_users", min: 1, max: 15)
      add("launch.max_users", "must equal 15") unless launch["max_users"] == 15
      number(launch["daily_login_credit_usd"], "launch.daily_login_credit_usd", min: 0.01)
      add("launch.daily_login_credit_usd", "must equal 20") unless launch["daily_login_credit_usd"] == 20.0
      number(launch["total_budget_usd"], "launch.total_budget_usd", min: 0.01)
      add("launch.total_budget_usd", "must equal 100") unless launch["total_budget_usd"] == 100.0
      integer(launch["budget_cost_bps"], "launch.budget_cost_bps", min: 1, max: 100_000)
      add("launch.budget_cost_bps", "must equal 1000") unless launch["budget_cost_bps"] == 1000
      %w[
        active_upstream_balance_min_usd financial_evidence_max_age_minutes
        quality_window_minutes quality_evidence_max_age_minutes samples_min
        ttft_p95_ms_max total_latency_p95_ms_max account_backup_age_hours_max
      ].each { |key| number(launch[key], "launch.#{key}", min: 0) }
      %w[success_rate_min error_rate_max disk_used_ratio_max].each do |key|
        number(launch[key], "launch.#{key}", min: 0, max: 1)
      end
    end

    def validate_modes(modes, path)
      add("#{path}.d04_mode", "must equal read_only") unless modes["d04_mode"] == "read_only"
      add("#{path}.registration_open", "must be false") unless modes["registration_open"] == false
      add("#{path}.relay_ops_mode", "must equal read_only") unless modes["relay_ops_mode"] == "read_only"
      unless modes["feishu_command_mode"] == "dry_run"
        add("#{path}.feishu_command_mode", "must equal dry_run")
      end
    end
  end

  class SnapshotValidator < ValidatorBase
    private

    def validate
      exact_keys(@document, nil, %w[
        schema_version snapshot_id status captured_at approvals modes services
        d04 active_upstream account_backup operations
      ])
      add("schema_version", "must equal 2") unless @document["schema_version"] == 2
      string(@document["snapshot_id"], "snapshot_id")
      unless %w[live_non_sensitive fictional].include?(@document["status"])
        add("status", "must equal live_non_sensitive or fictional")
      end
      timestamp(@document["captured_at"], "captured_at")

      approvals = section("approvals", %w[launch_approved])
      boolean(approvals["launch_approved"], "approvals.launch_approved") if approvals

      modes = section("modes", %w[d04_mode registration_open relay_ops_mode feishu_command_mode])
      if modes
        string(modes["d04_mode"], "modes.d04_mode")
        boolean(modes["registration_open"], "modes.registration_open")
        string(modes["relay_ops_mode"], "modes.relay_ops_mode")
        string(modes["feishu_command_mode"], "modes.feishu_command_mode")
      end

      validate_services
      validate_d04
      validate_active_upstream
      validate_account_backup
      validate_operations
    end

    def validate_services
      services = section("services", %w[
        sub2api postgres redis caddy d04 relay_ops unexplained_restart_count
        oom_killed disk_used_ratio
      ])
      return unless services

      %w[sub2api postgres redis caddy d04 relay_ops oom_killed].each do |key|
        boolean(services[key], "services.#{key}")
      end
      integer(services["unexplained_restart_count"], "services.unexplained_restart_count", min: 0)
      number(services["disk_used_ratio"], "services.disk_used_ratio", min: 0, max: 1)
    end

    def validate_d04
      d04 = section("d04", %w[
        configured_max_users configured_daily_login_credit_usd configured_total_budget_usd
        configured_budget_cost_bps registered_users balance_drift_usd read_only_reason
      ])
      return unless d04

      integer(d04["configured_max_users"], "d04.configured_max_users", min: 1)
      number(d04["configured_daily_login_credit_usd"], "d04.configured_daily_login_credit_usd", min: 0)
      number(d04["configured_total_budget_usd"], "d04.configured_total_budget_usd", min: 0)
      integer(d04["configured_budget_cost_bps"], "d04.configured_budget_cost_bps", min: 1)
      integer(d04["registered_users"], "d04.registered_users", min: 0)
      number(d04["balance_drift_usd"], "d04.balance_drift_usd")
      string(d04["read_only_reason"], "d04.read_only_reason", allow_empty: true)
    end

    def validate_active_upstream
      upstream = section("active_upstream", %w[
        role balance_usd financial_recorded_at quality_source quality_recorded_at
        sample_count success_rate error_rate ttft_p95_ms total_latency_p95_ms
      ])
      return unless upstream

      add("active_upstream.role", "must equal active_upstream") unless upstream["role"] == "active_upstream"
      number(upstream["balance_usd"], "active_upstream.balance_usd", min: 0, nullable: true)
      timestamp(upstream["financial_recorded_at"], "active_upstream.financial_recorded_at")
      string(upstream["quality_source"], "active_upstream.quality_source")
      timestamp(upstream["quality_recorded_at"], "active_upstream.quality_recorded_at")
      integer(upstream["sample_count"], "active_upstream.sample_count", min: 0)
      number(upstream["success_rate"], "active_upstream.success_rate", min: 0, max: 1)
      number(upstream["error_rate"], "active_upstream.error_rate", min: 0, max: 1)
      number(upstream["ttft_p95_ms"], "active_upstream.ttft_p95_ms", min: 0)
      number(upstream["total_latency_p95_ms"], "active_upstream.total_latency_p95_ms", min: 0)
    end

    def validate_account_backup
      backup = section("account_backup", %w[
        archive_created_at sha256_verified includes_sub2api_postgres includes_d04_sqlite
      ])
      return unless backup

      timestamp(backup["archive_created_at"], "account_backup.archive_created_at")
      %w[sha256_verified includes_sub2api_postgres includes_d04_sqlite].each do |key|
        boolean(backup[key], "account_backup.#{key}")
      end
    end

    def validate_operations
      operations = section("operations", %w[primary_owner support_channel rollback_validated])
      return unless operations

      string(operations["primary_owner"], "operations.primary_owner", allow_empty: true)
      string(operations["support_channel"], "operations.support_channel", allow_empty: true)
      boolean(operations["rollback_validated"], "operations.rollback_validated")
    end
  end

  class Evaluator
    ACTIONS = {
      "launch_not_approved" => "record_launch_approval",
      "upstream_balance_unknown" => "refresh_upstream_financial_evidence",
      "upstream_balance_below_minimum" => "replenish_active_upstream_balance",
      "upstream_financial_evidence_stale" => "refresh_upstream_financial_evidence",
      "upstream_quality_source_invalid" => "use_natural_production_quality_evidence",
      "upstream_quality_metrics_stale" => "refresh_upstream_quality_metrics",
      "upstream_samples_insufficient" => "wait_for_natural_upstream_samples",
      "upstream_success_rate_low" => "restore_upstream_quality",
      "upstream_error_rate_high" => "restore_upstream_quality",
      "upstream_ttft_p95_high" => "restore_upstream_latency",
      "upstream_total_latency_p95_high" => "restore_upstream_latency",
      "account_backup_stale" => "create_verified_local_account_backup",
      "account_backup_hash_unverified" => "verify_local_account_backup_hash",
      "account_backup_scope_incomplete" => "create_complete_local_account_backup",
      "d04_not_read_only" => "restore_d04_read_only",
      "registration_not_closed" => "close_d04_registration",
      "relay_ops_not_read_only" => "restore_relay_ops_read_only",
      "feishu_not_dry_run" => "restore_feishu_dry_run",
      "service_unhealthy" => "restore_service_health",
      "container_restarted" => "investigate_container_restart",
      "container_oom" => "investigate_container_oom",
      "disk_pressure" => "reduce_disk_pressure",
      "d04_configuration_mismatch" => "restore_d04_launch_configuration",
      "d04_user_limit_exceeded" => "close_d04_registration",
      "d04_balance_drift" => "reconcile_d04_balance",
      "d04_read_only_reason_present" => "resolve_d04_read_only_reason",
      "primary_owner_missing" => "assign_primary_owner",
      "support_channel_missing" => "assign_support_channel",
      "rollback_unverified" => "verify_registration_rollback"
    }.freeze

    def initialize(policy, now: Time.now)
      @policy = policy
      @now = now
      errors = PolicyValidator.new(policy).errors
      raise ValidationError, errors unless errors.empty?
    end

    def evaluate(snapshot)
      errors = SnapshotValidator.new(snapshot).errors
      raise ValidationError, errors unless errors.empty?

      validate_evidence_times(snapshot)
      launch = @policy.fetch("launch")
      reasons = []

      reasons << "launch_not_approved" unless snapshot.dig("approvals", "launch_approved")
      upstream_reasons(snapshot.fetch("active_upstream"), launch, reasons)
      backup_reasons(snapshot.fetch("account_backup"), launch, reasons)
      mode_reasons(snapshot.fetch("modes"), reasons)
      service_reasons(snapshot.fetch("services"), launch, reasons)
      d04_reasons(snapshot.fetch("d04"), launch, reasons)
      operations_reasons(snapshot.fetch("operations"), reasons)
      reasons.uniq!

      build_result(snapshot, launch, reasons)
    end

    private

    def validate_evidence_times(snapshot)
      paths = {
        "captured_at" => snapshot.fetch("captured_at"),
        "active_upstream.financial_recorded_at" => snapshot.dig("active_upstream", "financial_recorded_at"),
        "active_upstream.quality_recorded_at" => snapshot.dig("active_upstream", "quality_recorded_at"),
        "account_backup.archive_created_at" => snapshot.dig("account_backup", "archive_created_at")
      }
      errors = paths.each_with_object([]) do |(path, value), result|
        result << "#{path}: must not be in the future" if Time.iso8601(value) > @now
      end
      raise ValidationError, errors unless errors.empty?
    end

    def upstream_reasons(upstream, launch, reasons)
      balance = upstream.fetch("balance_usd")
      reasons << "upstream_balance_unknown" if balance.nil?
      if !balance.nil? && balance < launch.fetch("active_upstream_balance_min_usd")
        reasons << "upstream_balance_below_minimum"
      end
      if age_minutes(upstream.fetch("financial_recorded_at")) > launch.fetch("financial_evidence_max_age_minutes")
        reasons << "upstream_financial_evidence_stale"
      end
      unless upstream.fetch("quality_source") == @policy.dig("evidence", "quality_source")
        reasons << "upstream_quality_source_invalid"
      end
      if age_minutes(upstream.fetch("quality_recorded_at")) > launch.fetch("quality_evidence_max_age_minutes")
        reasons << "upstream_quality_metrics_stale"
      end
      enough_samples = upstream.fetch("sample_count") >= launch.fetch("samples_min")
      reasons << "upstream_samples_insufficient" unless enough_samples
      return unless enough_samples

      reasons << "upstream_success_rate_low" if upstream.fetch("success_rate") < launch.fetch("success_rate_min")
      reasons << "upstream_error_rate_high" if upstream.fetch("error_rate") > launch.fetch("error_rate_max")
      reasons << "upstream_ttft_p95_high" if upstream.fetch("ttft_p95_ms") > launch.fetch("ttft_p95_ms_max")
      if upstream.fetch("total_latency_p95_ms") > launch.fetch("total_latency_p95_ms_max")
        reasons << "upstream_total_latency_p95_high"
      end
    end

    def backup_reasons(backup, launch, reasons)
      if age_hours(backup.fetch("archive_created_at")) > launch.fetch("account_backup_age_hours_max")
        reasons << "account_backup_stale"
      end
      reasons << "account_backup_hash_unverified" unless backup.fetch("sha256_verified")
      unless backup.fetch("includes_sub2api_postgres") && backup.fetch("includes_d04_sqlite")
        reasons << "account_backup_scope_incomplete"
      end
    end

    def mode_reasons(modes, reasons)
      reasons << "d04_not_read_only" unless modes.fetch("d04_mode") == "read_only"
      reasons << "registration_not_closed" unless modes.fetch("registration_open") == false
      reasons << "relay_ops_not_read_only" unless modes.fetch("relay_ops_mode") == "read_only"
      reasons << "feishu_not_dry_run" unless modes.fetch("feishu_command_mode") == "dry_run"
    end

    def service_reasons(services, launch, reasons)
      healthy = %w[sub2api postgres redis caddy d04 relay_ops].all? { |key| services.fetch(key) }
      reasons << "service_unhealthy" unless healthy
      reasons << "container_restarted" if services.fetch("unexplained_restart_count").positive?
      reasons << "container_oom" if services.fetch("oom_killed")
      reasons << "disk_pressure" if services.fetch("disk_used_ratio") > launch.fetch("disk_used_ratio_max")
    end

    def d04_reasons(d04, launch, reasons)
      expected = {
        "configured_max_users" => launch.fetch("max_users"),
        "configured_daily_login_credit_usd" => launch.fetch("daily_login_credit_usd"),
        "configured_total_budget_usd" => launch.fetch("total_budget_usd"),
        "configured_budget_cost_bps" => launch.fetch("budget_cost_bps")
      }
      reasons << "d04_configuration_mismatch" unless expected.all? { |key, value| d04.fetch(key) == value }
      if d04.fetch("registered_users") > d04.fetch("configured_max_users")
        reasons << "d04_user_limit_exceeded"
      end
      reasons << "d04_balance_drift" unless d04.fetch("balance_drift_usd").abs < 0.000_001
      reasons << "d04_read_only_reason_present" unless d04.fetch("read_only_reason").strip.empty?
    end

    def operations_reasons(operations, reasons)
      reasons << "primary_owner_missing" if operations.fetch("primary_owner").strip.empty?
      reasons << "support_channel_missing" if operations.fetch("support_channel").strip.empty?
      reasons << "rollback_unverified" unless operations.fetch("rollback_validated")
    end

    def build_result(snapshot, launch, reasons)
      upstream = snapshot.fetch("active_upstream")
      backup = snapshot.fetch("account_backup")
      {
        "decision" => reasons.empty? ? "go" : "no_go",
        "blocking_reasons" => reasons,
        "required_actions" => reasons.map { |reason| ACTIONS.fetch(reason) }.uniq,
        "policy" => {
          "policy_id" => @policy.fetch("policy_id"),
          "max_users" => launch.fetch("max_users"),
          "daily_login_credit_usd" => launch.fetch("daily_login_credit_usd"),
          "total_budget_usd" => launch.fetch("total_budget_usd"),
          "budget_cost_bps" => launch.fetch("budget_cost_bps"),
          "active_upstream_balance_min_usd" => launch.fetch("active_upstream_balance_min_usd")
        },
        "derived" => {
          "financial_evidence_age_minutes" => age_minutes(upstream.fetch("financial_recorded_at")).round(2),
          "quality_evidence_age_minutes" => age_minutes(upstream.fetch("quality_recorded_at")).round(2),
          "account_backup_age_hours" => age_hours(backup.fetch("archive_created_at")).round(2)
        },
        "real_action_executed" => false,
        "external_system_contacted" => false
      }
    end

    def age_minutes(value)
      (@now - Time.iso8601(value)) / 60.0
    end

    def age_hours(value)
      (@now - Time.iso8601(value)) / 3600.0
    end
  end

  module CLI
    module_function

    def run(argv, out: $stdout, err: $stderr, env: ENV)
      unless argv.length == 3 && argv[0] == "evaluate"
        err.puts("usage: evaluate-d04-lightweight-launch-readiness.rb evaluate POLICY SNAPSHOT")
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

exit D04LightweightLaunchReadiness::CLI.run(ARGV) if $PROGRAM_NAME == __FILE__
