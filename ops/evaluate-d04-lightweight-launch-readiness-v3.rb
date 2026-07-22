#!/usr/bin/env ruby
# frozen_string_literal: true

require "digest"
require "json"
require "time"
require "yaml"

module D04LightweightLaunchReadinessV3
  class ValidationError < StandardError
    attr_reader :errors

    def initialize(errors)
      @errors = errors
      super(errors.join("; "))
    end
  end

  class Validator
    FORBIDDEN_KEYS = /\A(?:api[_-]?key|token|access[_-]?token|refresh[_-]?token|cookie|authorization|password|private[_-]?key|client[_-]?secret|oauth(?:[_-]?.*)?|credentials?)\z/i
    SECRET_VALUE = /(?:Authorization:\s*Bearer\s+\S{16,}|Cookie:\s*\S+|sk-[a-z0-9]{16,}|BEGIN [A-Z ]*PRIVATE KEY)/i

    attr_reader :errors

    def initialize(document)
      @document = document
      @errors = []
      unless document.is_a?(Hash)
        @errors << "root: must be a mapping"
        return
      end
      validate
      scan(document)
    end

    private

    def keys(mapping, path, required, optional = [])
      unless mapping.is_a?(Hash)
        @errors << "#{path}: must be a mapping"
        return false
      end
      required.each { |key| @errors << "#{join(path, key)}: is required" unless mapping.key?(key) }
      allowed = required + optional
      mapping.each_key { |key| @errors << "#{join(path, key.to_s)}: is not allowed" unless allowed.include?(key.to_s) }
      true
    end

    def join(path, key)
      path.to_s.empty? ? key : "#{path}.#{key}"
    end

    def timestamp(value, path, nullable: false)
      return if nullable && value.nil?
      unless value.is_a?(String) && !value.strip.empty?
        @errors << "#{path}: must be an ISO-8601 timestamp"
        return
      end
      Time.iso8601(value)
    rescue ArgumentError
      @errors << "#{path}: must be an ISO-8601 timestamp"
    end

    def number(value, path, min: nil, max: nil, nullable: false)
      return if nullable && value.nil?
      unless value.is_a?(Numeric) && value.finite?
        @errors << "#{path}: must be numeric"
        return
      end
      @errors << "#{path}: must be at least #{min}" if !min.nil? && value < min
      @errors << "#{path}: must be at most #{max}" if !max.nil? && value > max
    end

    def integer(value, path, min: nil, nullable: false)
      return if nullable && value.nil?
      unless value.is_a?(Integer)
        @errors << "#{path}: must be an integer"
        return
      end
      @errors << "#{path}: must be at least #{min}" if !min.nil? && value < min
    end

    def boolean(value, path)
      @errors << "#{path}: must be true or false" unless value == true || value == false
    end

    def text(value, path, allow_empty: false)
      valid = value.is_a?(String) && (allow_empty || !value.strip.empty?)
      @errors << "#{path}: must be #{allow_empty ? "a string" : "a non-empty string"}" unless valid
    end

    def scan(value, path = "")
      case value
      when Hash
        value.each do |key, child|
          child_path = join(path, key.to_s)
          @errors << "#{child_path}: credential fields are forbidden" if key.to_s.match?(FORBIDDEN_KEYS)
          scan(child, child_path)
        end
      when Array
        value.each_with_index { |child, index| scan(child, "#{path}[#{index}]") }
      when String
        @errors << "#{path}: value looks like a secret" if value.match?(SECRET_VALUE)
      end
    end
  end

  class PolicyValidator < Validator
    private

    def validate
      return unless keys(@document, "", %w[schema_version policy_id status action_execution_mode launch required_modes evidence])

      @errors << "schema_version: must equal 3" unless @document["schema_version"] == 3
      @errors << "policy_id: must equal D04-LIGHTWEIGHT-LAUNCH-v3" unless @document["policy_id"] == "D04-LIGHTWEIGHT-LAUNCH-v3"
      @errors << "status: must equal preparation_policy" unless @document["status"] == "preparation_policy"
      @errors << "action_execution_mode: must equal report_only" unless @document["action_execution_mode"] == "report_only"
      validate_launch(@document["launch"])
      validate_modes(@document["required_modes"], "required_modes")
      evidence = @document["evidence"]
      if keys(evidence, "evidence", %w[discovery_source quality_source notes])
        @errors << "evidence.discovery_source: invalid" unless evidence["discovery_source"] == "sub2api_admin_accounts"
        @errors << "evidence.quality_source: invalid" unless evidence["quality_source"] == "sub2api_account_attributed_natural_traffic"
        text(evidence["notes"], "evidence.notes")
      end
    end

    def validate_launch(launch)
      required = %w[
        max_users daily_login_credit_usd total_budget_usd budget_cost_bps
        active_upstream_balance_min_usd discovery_evidence_max_age_minutes
        financial_evidence_max_age_minutes quality_window_minutes
        quality_evidence_max_age_minutes samples_min success_rate_min error_rate_max
        ttft_p95_ms_max total_latency_p95_ms_max account_backup_age_hours_max
        disk_used_ratio_max
      ]
      return unless keys(launch, "launch", required)

      integer(launch["max_users"], "launch.max_users", min: 1)
      integer(launch["budget_cost_bps"], "launch.budget_cost_bps", min: 1)
      integer(launch["samples_min"], "launch.samples_min", min: 0)
      %w[daily_login_credit_usd total_budget_usd active_upstream_balance_min_usd
         discovery_evidence_max_age_minutes financial_evidence_max_age_minutes
         quality_window_minutes quality_evidence_max_age_minutes ttft_p95_ms_max
         total_latency_p95_ms_max account_backup_age_hours_max].each do |key|
        number(launch[key], "launch.#{key}", min: 0)
      end
      %w[success_rate_min error_rate_max disk_used_ratio_max].each do |key|
        number(launch[key], "launch.#{key}", min: 0, max: 1)
      end
      @errors << "launch.max_users: must equal 15" unless launch["max_users"] == 15
      @errors << "launch.daily_login_credit_usd: must equal 20" unless launch["daily_login_credit_usd"] == 20.0
      @errors << "launch.total_budget_usd: must equal 100" unless launch["total_budget_usd"] == 100.0
      @errors << "launch.budget_cost_bps: must equal 1000" unless launch["budget_cost_bps"] == 1000
    end

    def validate_modes(modes, path)
      return unless keys(modes, path, %w[d04_mode registration_open relay_ops_mode feishu_command_mode])

      @errors << "#{path}.d04_mode: must equal read_only" unless modes["d04_mode"] == "read_only"
      @errors << "#{path}.registration_open: must be false" unless modes["registration_open"] == false
      @errors << "#{path}.relay_ops_mode: must equal read_only" unless modes["relay_ops_mode"] == "read_only"
      @errors << "#{path}.feishu_command_mode: must equal dry_run" unless modes["feishu_command_mode"] == "dry_run"
    end
  end

  class SnapshotValidator < Validator
    private

    def validate
      required = %w[
        schema_version snapshot_id status captured_at approvals modes services d04
        upstream_discovery active_upstreams account_backup operations
      ]
      return unless keys(@document, "", required)

      @errors << "schema_version: must equal 3" unless @document["schema_version"] == 3
      text(@document["snapshot_id"], "snapshot_id")
      @errors << "status: must be live_non_sensitive or fictional" unless %w[live_non_sensitive fictional].include?(@document["status"])
      timestamp(@document["captured_at"], "captured_at")
      validate_approvals
      validate_modes
      validate_services
      validate_d04
      validate_discovery
      validate_upstreams
      validate_backup
      validate_operations
    end

    def validate_approvals
      value = @document["approvals"]
      boolean(value["launch_approved"], "approvals.launch_approved") if keys(value, "approvals", %w[launch_approved])
    end

    def validate_modes
      value = @document["modes"]
      return unless keys(value, "modes", %w[d04_mode registration_open relay_ops_mode feishu_command_mode])

      text(value["d04_mode"], "modes.d04_mode")
      boolean(value["registration_open"], "modes.registration_open")
      text(value["relay_ops_mode"], "modes.relay_ops_mode")
      text(value["feishu_command_mode"], "modes.feishu_command_mode")
    end

    def validate_services
      required = %w[sub2api postgres redis caddy d04 relay_ops unexplained_restart_count oom_killed disk_used_ratio]
      value = @document["services"]
      return unless keys(value, "services", required)

      %w[sub2api postgres redis caddy d04 relay_ops oom_killed].each { |key| boolean(value[key], "services.#{key}") }
      integer(value["unexplained_restart_count"], "services.unexplained_restart_count", min: 0)
      number(value["disk_used_ratio"], "services.disk_used_ratio", min: 0, max: 1)
    end

    def validate_d04
      required = %w[
        launch_overlay_max_users launch_overlay_daily_login_credit_usd
        launch_overlay_total_budget_usd launch_overlay_budget_cost_bps
        registered_users balance_drift_usd read_only_reason
      ]
      value = @document["d04"]
      return unless keys(value, "d04", required)

      integer(value["launch_overlay_max_users"], "d04.launch_overlay_max_users", min: 1)
      number(value["launch_overlay_daily_login_credit_usd"], "d04.launch_overlay_daily_login_credit_usd", min: 0)
      number(value["launch_overlay_total_budget_usd"], "d04.launch_overlay_total_budget_usd", min: 0)
      integer(value["launch_overlay_budget_cost_bps"], "d04.launch_overlay_budget_cost_bps", min: 1)
      integer(value["registered_users"], "d04.registered_users", min: 0)
      number(value["balance_drift_usd"], "d04.balance_drift_usd")
      text(value["read_only_reason"], "d04.read_only_reason", allow_empty: true)
    end

    def validate_discovery
      value = @document["upstream_discovery"]
      return unless keys(value, "upstream_discovery", %w[source recorded_at account_set_sha256])

      text(value["source"], "upstream_discovery.source")
      timestamp(value["recorded_at"], "upstream_discovery.recorded_at")
      unless value["account_set_sha256"].is_a?(String) && value["account_set_sha256"].match?(/\A[0-9a-f]{64}\z/)
        @errors << "upstream_discovery.account_set_sha256: must be lowercase SHA-256"
      end
    end

    def validate_upstreams
      value = @document["active_upstreams"]
      unless value.is_a?(Array)
        @errors << "active_upstreams: must be an array"
        return
      end
      ids = []
      value.each_with_index do |account, index|
        path = "active_upstreams[#{index}]"
        required = %w[
          account_id display_name platform status schedulable group_ids runtime_available
          balance_usd financial_recorded_at quality_source quality_recorded_at sample_count
          success_rate error_rate ttft_p95_ms total_latency_p95_ms
        ]
        next unless keys(account, path, required, %w[runtime_block_reason])

        integer(account["account_id"], "#{path}.account_id", min: 1)
        text(account["display_name"], "#{path}.display_name", allow_empty: true)
        text(account["platform"], "#{path}.platform", allow_empty: true)
        @errors << "#{path}.status: must equal active" unless account["status"] == "active"
        @errors << "#{path}.schedulable: must be true" unless account["schedulable"] == true
        unless account["group_ids"].is_a?(Array) && account["group_ids"].all? { |id| id.is_a?(Integer) && id.positive? }
          @errors << "#{path}.group_ids: must contain positive integer IDs"
        end
        boolean(account["runtime_available"], "#{path}.runtime_available")
        number(account["balance_usd"], "#{path}.balance_usd", nullable: true)
        timestamp(account["financial_recorded_at"], "#{path}.financial_recorded_at", nullable: true)
        text(account["quality_source"], "#{path}.quality_source", allow_empty: true)
        timestamp(account["quality_recorded_at"], "#{path}.quality_recorded_at", nullable: true)
        integer(account["sample_count"], "#{path}.sample_count", min: 0, nullable: true)
        number(account["success_rate"], "#{path}.success_rate", min: 0, max: 1, nullable: true)
        number(account["error_rate"], "#{path}.error_rate", min: 0, max: 1, nullable: true)
        number(account["ttft_p95_ms"], "#{path}.ttft_p95_ms", min: 0, nullable: true)
        number(account["total_latency_p95_ms"], "#{path}.total_latency_p95_ms", min: 0, nullable: true)
        ids << account["account_id"] if account["account_id"].is_a?(Integer)
      end
      @errors << "active_upstreams: account IDs must be unique and sorted" unless ids == ids.uniq.sort
    end

    def validate_backup
      value = @document["account_backup"]
      required = %w[archive_created_at sha256_verified includes_sub2api_postgres includes_d04_sqlite]
      return unless keys(value, "account_backup", required)

      timestamp(value["archive_created_at"], "account_backup.archive_created_at")
      required.drop(1).each { |key| boolean(value[key], "account_backup.#{key}") }
    end

    def validate_operations
      value = @document["operations"]
      return unless keys(value, "operations", %w[primary_owner support_channel rollback_validated])

      text(value["primary_owner"], "operations.primary_owner", allow_empty: true)
      text(value["support_channel"], "operations.support_channel", allow_empty: true)
      boolean(value["rollback_validated"], "operations.rollback_validated")
    end
  end

  class Evaluator
    ACTIONS = {
      "active_upstreams_empty" => "enable_at_least_one_schedulable_account",
      "upstream_discovery_failed" => "refresh_sub2api_account_discovery",
      "upstream_discovery_stale" => "refresh_sub2api_account_discovery",
      "upstream_account_set_changed" => "refresh_sub2api_account_discovery",
      "upstream_temporarily_unavailable" => "wait_for_upstream_availability",
      "upstream_balance_unknown" => "refresh_upstream_financial_evidence",
      "upstream_balance_below_minimum" => "replenish_active_upstream_balance",
      "upstream_financial_evidence_stale" => "refresh_upstream_financial_evidence",
      "upstream_quality_attribution_missing" => "collect_account_attributed_natural_quality",
      "upstream_quality_source_invalid" => "collect_account_attributed_natural_quality",
      "upstream_quality_metrics_stale" => "refresh_upstream_quality_metrics",
      "upstream_samples_insufficient" => "wait_for_natural_upstream_samples",
      "upstream_success_rate_low" => "restore_upstream_quality",
      "upstream_error_rate_high" => "restore_upstream_quality",
      "upstream_ttft_p95_high" => "restore_upstream_latency",
      "upstream_total_latency_p95_high" => "restore_upstream_latency",
      "launch_not_approved" => "record_launch_approval",
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

    def evaluate(snapshot, current_account_set_sha256: nil)
      errors = SnapshotValidator.new(snapshot).errors
      raise ValidationError, errors unless errors.empty?
      validate_future_times(snapshot)

      launch = @policy.fetch("launch")
      global = []
      global << "launch_not_approved" unless snapshot.dig("approvals", "launch_approved")
      discovery_reasons(snapshot, launch, current_account_set_sha256, global)

      upstream_results = snapshot.fetch("active_upstreams").map do |upstream|
        reasons = upstream_reasons(upstream, launch)
        global.concat(reasons)
        project_upstream(upstream, reasons)
      end
      global << "active_upstreams_empty" if upstream_results.empty?
      common_reasons(snapshot, launch, global)
      global.uniq!

      {
        "policy_id" => @policy.fetch("policy_id"),
        "snapshot_id" => snapshot.fetch("snapshot_id"),
        "account_set_sha256" => snapshot.dig("upstream_discovery", "account_set_sha256"),
        "evaluated_at" => @now.utc.iso8601,
        "decision" => global.empty? ? "go" : "no_go",
        "blocking_reasons" => global,
        "required_actions" => global.map { |reason| ACTIONS.fetch(reason) }.uniq,
        "upstreams" => upstream_results,
        "real_action_executed" => false,
        "external_system_contacted" => false
      }
    end

    private

    def discovery_reasons(snapshot, launch, current_hash, reasons)
      discovery = snapshot.fetch("upstream_discovery")
      reasons << "upstream_discovery_failed" unless discovery.fetch("source") == @policy.dig("evidence", "discovery_source")
      reasons << "upstream_discovery_stale" if age_minutes(discovery.fetch("recorded_at")) > launch.fetch("discovery_evidence_max_age_minutes")
      expected = account_set_hash(snapshot.fetch("active_upstreams"))
      recorded = discovery.fetch("account_set_sha256")
      reasons << "upstream_account_set_changed" unless expected == recorded
      reasons << "upstream_account_set_changed" if current_hash && current_hash != recorded
    end

    def account_set_hash(upstreams)
      canonical = upstreams.map do |account|
        {
          "account_id" => account.fetch("account_id"),
          "status" => account.fetch("status"),
          "schedulable" => account.fetch("schedulable"),
          "group_ids" => account.fetch("group_ids").sort
        }
      end
      Digest::SHA256.hexdigest(JSON.generate(canonical))
    end

    def upstream_reasons(upstream, launch)
      reasons = []
      reasons << "upstream_temporarily_unavailable" unless upstream.fetch("runtime_available")
      balance = upstream.fetch("balance_usd")
      financial_at = upstream.fetch("financial_recorded_at")
      reasons << "upstream_balance_unknown" if balance.nil? || financial_at.nil?
      reasons << "upstream_balance_below_minimum" if !balance.nil? && balance < launch.fetch("active_upstream_balance_min_usd")
      if financial_at && age_minutes(financial_at) > launch.fetch("financial_evidence_max_age_minutes")
        reasons << "upstream_financial_evidence_stale"
      end

      quality_fields = %w[quality_recorded_at sample_count success_rate error_rate ttft_p95_ms total_latency_p95_ms]
      attribution_missing = upstream.fetch("group_ids").empty? ||
                            upstream.fetch("quality_source").empty? ||
                            quality_fields.any? { |key| upstream[key].nil? }
      reasons << "upstream_quality_attribution_missing" if attribution_missing
      if !upstream.fetch("quality_source").empty? &&
         upstream.fetch("quality_source") != @policy.dig("evidence", "quality_source")
        reasons << "upstream_quality_source_invalid"
      end
      return reasons if attribution_missing

      reasons << "upstream_quality_metrics_stale" if age_minutes(upstream.fetch("quality_recorded_at")) > launch.fetch("quality_evidence_max_age_minutes")
      samples = upstream.fetch("sample_count")
      reasons << "upstream_samples_insufficient" if samples < launch.fetch("samples_min")
      return reasons if samples < launch.fetch("samples_min")

      reasons << "upstream_success_rate_low" if upstream.fetch("success_rate") < launch.fetch("success_rate_min")
      reasons << "upstream_error_rate_high" if upstream.fetch("error_rate") > launch.fetch("error_rate_max")
      reasons << "upstream_ttft_p95_high" if upstream.fetch("ttft_p95_ms") > launch.fetch("ttft_p95_ms_max")
      reasons << "upstream_total_latency_p95_high" if upstream.fetch("total_latency_p95_ms") > launch.fetch("total_latency_p95_ms_max")
      reasons
    end

    def project_upstream(upstream, reasons)
      keys = %w[
        account_id display_name group_ids status schedulable runtime_available
        balance_usd financial_recorded_at quality_recorded_at sample_count
        success_rate error_rate ttft_p95_ms total_latency_p95_ms
      ]
      result = keys.each_with_object({}) { |key, output| output[key] = upstream[key] }
      result["decision"] = reasons.empty? ? "go" : "no_go"
      result["blocking_reasons"] = reasons
      result
    end

    def common_reasons(snapshot, launch, reasons)
      backup = snapshot.fetch("account_backup")
      reasons << "account_backup_stale" if age_hours(backup.fetch("archive_created_at")) > launch.fetch("account_backup_age_hours_max")
      reasons << "account_backup_hash_unverified" unless backup.fetch("sha256_verified")
      unless backup.fetch("includes_sub2api_postgres") && backup.fetch("includes_d04_sqlite")
        reasons << "account_backup_scope_incomplete"
      end

      modes = snapshot.fetch("modes")
      reasons << "d04_not_read_only" unless modes.fetch("d04_mode") == "read_only"
      reasons << "registration_not_closed" unless modes.fetch("registration_open") == false
      reasons << "relay_ops_not_read_only" unless modes.fetch("relay_ops_mode") == "read_only"
      reasons << "feishu_not_dry_run" unless modes.fetch("feishu_command_mode") == "dry_run"

      services = snapshot.fetch("services")
      reasons << "service_unhealthy" unless %w[sub2api postgres redis caddy d04 relay_ops].all? { |key| services.fetch(key) }
      reasons << "container_restarted" if services.fetch("unexplained_restart_count").positive?
      reasons << "container_oom" if services.fetch("oom_killed")
      reasons << "disk_pressure" if services.fetch("disk_used_ratio") > launch.fetch("disk_used_ratio_max")

      d04 = snapshot.fetch("d04")
      expected = {
        "launch_overlay_max_users" => launch.fetch("max_users"),
        "launch_overlay_daily_login_credit_usd" => launch.fetch("daily_login_credit_usd"),
        "launch_overlay_total_budget_usd" => launch.fetch("total_budget_usd"),
        "launch_overlay_budget_cost_bps" => launch.fetch("budget_cost_bps")
      }
      reasons << "d04_configuration_mismatch" unless expected.all? { |key, value| d04.fetch(key) == value }
      reasons << "d04_user_limit_exceeded" if d04.fetch("registered_users") > d04.fetch("launch_overlay_max_users")
      reasons << "d04_balance_drift" unless d04.fetch("balance_drift_usd").abs < 0.000_001
      reasons << "d04_read_only_reason_present" unless d04.fetch("read_only_reason").strip.empty?

      operations = snapshot.fetch("operations")
      reasons << "primary_owner_missing" if operations.fetch("primary_owner").strip.empty?
      reasons << "support_channel_missing" if operations.fetch("support_channel").strip.empty?
      reasons << "rollback_unverified" unless operations.fetch("rollback_validated")
    end

    def validate_future_times(snapshot)
      values = {
        "captured_at" => snapshot.fetch("captured_at"),
        "upstream_discovery.recorded_at" => snapshot.dig("upstream_discovery", "recorded_at"),
        "account_backup.archive_created_at" => snapshot.dig("account_backup", "archive_created_at")
      }
      snapshot.fetch("active_upstreams").each_with_index do |account, index|
        values["active_upstreams[#{index}].financial_recorded_at"] = account["financial_recorded_at"]
        values["active_upstreams[#{index}].quality_recorded_at"] = account["quality_recorded_at"]
      end
      errors = []
      values.each do |path, value|
        errors << "#{path}: must not be in the future" if value && Time.iso8601(value) > @now
      end
      raise ValidationError, errors unless errors.empty?
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
        err.puts("usage: evaluate-d04-lightweight-launch-readiness-v3.rb evaluate POLICY SNAPSHOT")
        return 2
      end
      policy = YAML.safe_load(File.read(argv[1]))
      snapshot = YAML.safe_load(File.read(argv[2]))
      now = env["D04_LAUNCH_NOW"].to_s.empty? ? Time.now : Time.iso8601(env["D04_LAUNCH_NOW"])
      current_hash = env["D04_CURRENT_ACCOUNT_SET_SHA256"].to_s
      current_hash = nil if current_hash.empty?
      result = Evaluator.new(policy, now: now).evaluate(snapshot, current_account_set_sha256: current_hash)
      out.puts(JSON.generate(result))
      0
    rescue ValidationError, ArgumentError, KeyError, Errno::ENOENT, Psych::Exception => exception
      err.puts(exception.message)
      1
    end
  end
end

exit D04LightweightLaunchReadinessV3::CLI.run(ARGV) if $PROGRAM_NAME == __FILE__

