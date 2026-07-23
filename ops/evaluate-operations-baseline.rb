#!/usr/bin/env ruby
# frozen_string_literal: true

require "date"
require "json"
require "yaml"

module OperationsBaseline
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
      validate_root
      validate if @document.is_a?(Hash)
      scan_credentials(@document) if @document.is_a?(Hash)
    end

    private

    def validate_root
      add("root", "must be a mapping") unless @document.is_a?(Hash)
    end

    def require_key(mapping, key, path)
      add(path, "is required") unless mapping.key?(key)
    end

    def mapping(value, path)
      return value if value.is_a?(Hash)

      add(path, "must be a mapping") unless value.nil?
      nil
    end

    def array(value, path)
      return value if value.is_a?(Array)

      add(path, "must be an array") unless value.nil?
      nil
    end

    def string(value, path)
      return true if value.is_a?(String) && !value.strip.empty?

      add(path, "must be a non-empty string") unless value.nil?
      false
    end

    def boolean(value, path)
      return true if value == true || value == false

      add(path, "must be true or false") unless value.nil?
      false
    end

    def number(value, path, min: nil, max: nil)
      unless value.is_a?(Numeric)
        add(path, "must be numeric") unless value.nil?
        return false
      end
      add(path, "must be at least #{min}") if min && value < min
      add(path, "must be at most #{max}") if max && value > max
      true
    end

    def integer(value, path, min: nil, max: nil)
      unless value.is_a?(Integer)
        add(path, "must be an integer") unless value.nil?
        return false
      end
      add(path, "must be at least #{min}") if min && value < min
      add(path, "must be at most #{max}") if max && value > max
      true
    end

    def string_array(value, path)
      items = array(value, path)
      return unless items

      add(path, "must not be empty") if items.empty?
      items.each_with_index { |item, index| string(item, "#{path}[#{index}]") }
    end

    def scan_credentials(value, path = nil)
      case value
      when Hash
        value.each do |key, child|
          child_path = path ? "#{path}.#{key}" : key.to_s
          if key.to_s.match?(FORBIDDEN_CREDENTIAL_KEYS)
            add(child_path, "credential fields are forbidden")
          end
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
      %w[
        schema_version ops_id status reviewed_at external_actions_deferred action_execution_mode
        cadence thresholds backup_policy stop_loss_actions evidence
      ].each { |key| require_key(@document, key, key) }
      add("schema_version", "must equal 1") unless @document["schema_version"] == 1
      add("ops_id", "must equal OPS01") unless @document["ops_id"] == "OPS01"
      add("status", "must equal fictional") unless @document["status"] == "fictional"
      boolean(@document["external_actions_deferred"], "external_actions_deferred")
      unless @document["external_actions_deferred"] == true
        add("external_actions_deferred", "must be true")
      end
      unless @document["action_execution_mode"] == "report_only"
        add("action_execution_mode", "must equal report_only")
      end

      validate_cadence
      validate_thresholds
      validate_backup_policy
      validate_actions
      validate_evidence
    end

    def validate_cadence
      cadence = mapping(@document["cadence"], "cadence")
      return unless cadence

      %w[daily_review_time weekly_review_day monthly_restore_day].each do |key|
        require_key(cadence, key, "cadence.#{key}")
      end
      string(cadence["daily_review_time"], "cadence.daily_review_time")
      string(cadence["weekly_review_day"], "cadence.weekly_review_day")
      integer(cadence["monthly_restore_day"], "cadence.monthly_restore_day", min: 1, max: 28)
    end

    def validate_thresholds
      thresholds = mapping(@document["thresholds"], "thresholds")
      return unless thresholds

      numeric = {
        "balance_difference_usd_abs_max" => [0.0, nil],
        "backup_age_hours_max" => [1.0, nil],
        "restore_drill_age_days_max" => [1.0, nil],
        "certificate_days_min" => [1.0, nil],
        "disk_used_ratio_max" => [0.01, 1.0],
        "failed_admin_logins_1h_max" => [1.0, nil],
        "upstream_balance_days_min" => [0.0, nil],
        "upstream_success_rate_min" => [0.0, 1.0],
        "upstream_rate_limit_ratio_max" => [0.0, 1.0],
        "upstream_5xx_ratio_max" => [0.0, 1.0],
        "upstream_ttft_p95_ms_max" => [1.0, nil],
        "stream_interruption_ratio_max" => [0.0, 1.0],
        "daily_total_cost_usd_max" => [0.0, nil],
        "per_user_daily_cost_usd_max" => [0.0, nil],
        "request_id_coverage_ratio_min" => [0.0, 1.0],
        "gross_margin_percent_min" => [0.0, 100.0],
        "account_expiry_warning_hours" => [1.0, nil]
      }
      numeric.each do |key, (min, max)|
        require_key(thresholds, key, "thresholds.#{key}")
        number(thresholds[key], "thresholds.#{key}", min: min, max: max)
      end
    end

    def validate_backup_policy
      policy = mapping(@document["backup_policy"], "backup_policy")
      return unless policy

      %w[
        postgres_format offsite_provider offsite_tool local_retention_days offsite_retention_days
        encrypted_offsite_required restore_drill_interval_days dry_run_only
      ].each { |key| require_key(policy, key, "backup_policy.#{key}") }
      add("backup_policy.postgres_format", "must equal custom_pg_dump_fc") unless
        policy["postgres_format"] == "custom_pg_dump_fc"
      add("backup_policy.offsite_provider", "must equal cloudflare_r2_standard") unless
        policy["offsite_provider"] == "cloudflare_r2_standard"
      add("backup_policy.offsite_tool", "must equal restic") unless
        policy["offsite_tool"] == "restic"
      integer(policy["local_retention_days"], "backup_policy.local_retention_days", min: 1)
      integer(policy["offsite_retention_days"], "backup_policy.offsite_retention_days", min: 1)
      boolean(policy["encrypted_offsite_required"], "backup_policy.encrypted_offsite_required")
      unless policy["encrypted_offsite_required"] == true
        add("backup_policy.encrypted_offsite_required", "must be true")
      end
      integer(policy["restore_drill_interval_days"],
              "backup_policy.restore_drill_interval_days", min: 1)
      boolean(policy["dry_run_only"], "backup_policy.dry_run_only")
      add("backup_policy.dry_run_only", "must be true") unless policy["dry_run_only"] == true
    end

    def validate_actions
      actions = mapping(@document["stop_loss_actions"], "stop_loss_actions")
      return unless actions

      %w[
        credential_exposure billing_integrity all_upstreams_down core_service_down
      ].each do |key|
        require_key(actions, key, "stop_loss_actions.#{key}")
        string_array(actions[key], "stop_loss_actions.#{key}")
      end
    end

    def validate_evidence
      evidence = mapping(@document["evidence"], "evidence")
      return unless evidence

      %w[sub2api_version sub2api_commit notes].each do |key|
        require_key(evidence, key, "evidence.#{key}")
        string(evidence[key], "evidence.#{key}")
      end
    end
  end

  class SnapshotValidator < ValidatorBase
    private

    def validate
      %w[
        schema_version snapshot_id status captured_at system billing traffic profit
        upstreams account_pools
      ].each { |key| require_key(@document, key, key) }
      add("schema_version", "must equal 1") unless @document["schema_version"] == 1
      string(@document["snapshot_id"], "snapshot_id")
      add("status", "must equal fictional for tracked snapshots") unless @document["status"] == "fictional"
      string(@document["captured_at"], "captured_at")
      validate_system
      validate_billing
      validate_traffic
      validate_profit
      validate_upstreams
      validate_account_pools
    end

    def validate_system
      system = mapping(@document["system"], "system")
      return unless system

      %w[
        services certificate_days_remaining disk_used_ratio backup_age_hours
        restore_drill_age_days failed_admin_logins_1h credential_exposure_detected
      ].each { |key| require_key(system, key, "system.#{key}") }
      services = mapping(system["services"], "system.services")
      if services
        %w[sub2api postgres redis caddy].each do |key|
          require_key(services, key, "system.services.#{key}")
          boolean(services[key], "system.services.#{key}")
        end
      end
      number(system["certificate_days_remaining"], "system.certificate_days_remaining", min: 0.0)
      number(system["disk_used_ratio"], "system.disk_used_ratio", min: 0.0, max: 1.0)
      number(system["backup_age_hours"], "system.backup_age_hours", min: 0.0)
      number(system["restore_drill_age_days"], "system.restore_drill_age_days", min: 0.0)
      integer(system["failed_admin_logins_1h"], "system.failed_admin_logins_1h", min: 0)
      boolean(system["credential_exposure_detected"], "system.credential_exposure_detected")
    end

    def validate_billing
      billing = mapping(@document["billing"], "billing")
      return unless billing

      %w[
        balance_difference_usd duplicate_credit_count unexplained_adjustment_count
        daily_total_cost_usd max_user_daily_cost_usd
      ].each { |key| require_key(billing, key, "billing.#{key}") }
      number(billing["balance_difference_usd"], "billing.balance_difference_usd")
      integer(billing["duplicate_credit_count"], "billing.duplicate_credit_count", min: 0)
      integer(billing["unexplained_adjustment_count"],
              "billing.unexplained_adjustment_count", min: 0)
      number(billing["daily_total_cost_usd"], "billing.daily_total_cost_usd", min: 0.0)
      number(billing["max_user_daily_cost_usd"], "billing.max_user_daily_cost_usd", min: 0.0)
    end

    def validate_traffic
      traffic = mapping(@document["traffic"], "traffic")
      return unless traffic

      %w[request_count request_id_coverage_ratio].each do |key|
        require_key(traffic, key, "traffic.#{key}")
      end
      integer(traffic["request_count"], "traffic.request_count", min: 0)
      number(traffic["request_id_coverage_ratio"], "traffic.request_id_coverage_ratio",
             min: 0.0, max: 1.0)
    end

    def validate_profit
      profit = mapping(@document["profit"], "profit")
      return unless profit

      %w[weekly_revenue_cny weekly_full_cost_cny].each do |key|
        require_key(profit, key, "profit.#{key}")
      end
      number(profit["weekly_revenue_cny"], "profit.weekly_revenue_cny", min: 0.0)
      number(profit["weekly_full_cost_cny"], "profit.weekly_full_cost_cny", min: 0.0)
    end

    def validate_upstreams
      upstreams = array(@document["upstreams"], "upstreams")
      return unless upstreams

      add("upstreams", "must not be empty") if upstreams.empty?
      upstreams.each_with_index do |upstream, index|
        path = "upstreams[#{index}]"
        unless upstream.is_a?(Hash)
          add(path, "must be a mapping")
          next
        end
        %w[
          upstream_id status available balance_days_remaining success_rate rate_limit_ratio
          server_error_ratio ttft_p95_ms stream_interruption_ratio
        ].each { |key| require_key(upstream, key, "#{path}.#{key}") }
        string(upstream["upstream_id"], "#{path}.upstream_id")
        add("#{path}.status", "must equal fictional") unless upstream["status"] == "fictional"
        boolean(upstream["available"], "#{path}.available")
        number(upstream["balance_days_remaining"], "#{path}.balance_days_remaining", min: 0.0)
        number(upstream["success_rate"], "#{path}.success_rate", min: 0.0, max: 1.0)
        number(upstream["rate_limit_ratio"], "#{path}.rate_limit_ratio", min: 0.0, max: 1.0)
        number(upstream["server_error_ratio"], "#{path}.server_error_ratio", min: 0.0, max: 1.0)
        number(upstream["ttft_p95_ms"], "#{path}.ttft_p95_ms", min: 0.0)
        number(upstream["stream_interruption_ratio"],
               "#{path}.stream_interruption_ratio", min: 0.0, max: 1.0)
      end
    end

    def validate_account_pools
      pools = array(@document["account_pools"], "account_pools")
      return unless pools

      pools.each_with_index do |pool, index|
        path = "account_pools[#{index}]"
        unless pool.is_a?(Hash)
          add(path, "must be a mapping")
          next
        end
        %w[
          pool_id status available_accounts error_accounts minimum_expiry_hours
        ].each { |key| require_key(pool, key, "#{path}.#{key}") }
        string(pool["pool_id"], "#{path}.pool_id")
        add("#{path}.status", "must equal fictional") unless pool["status"] == "fictional"
        integer(pool["available_accounts"], "#{path}.available_accounts", min: 0)
        integer(pool["error_accounts"], "#{path}.error_accounts", min: 0)
        number(pool["minimum_expiry_hours"], "#{path}.minimum_expiry_hours", min: 0.0)
      end
    end
  end

  class Evaluator
    SEVERITY_ORDER = { "critical" => 0, "high" => 1, "warning" => 2, "info" => 3 }.freeze

    def initialize(policy)
      errors = PolicyValidator.new(policy).errors
      raise ValidationError, errors unless errors.empty?

      @policy = policy
      @thresholds = policy.fetch("thresholds")
      @stop_loss = policy.fetch("stop_loss_actions")
    end

    def evaluate(snapshot)
      errors = SnapshotValidator.new(snapshot).errors
      raise ValidationError, errors unless errors.empty?

      alerts = []
      evaluate_credential(snapshot, alerts)
      evaluate_billing(snapshot, alerts)
      evaluate_core_services(snapshot, alerts)
      evaluate_upstreams(snapshot, alerts)
      evaluate_system_thresholds(snapshot, alerts)
      evaluate_costs(snapshot, alerts)
      evaluate_traffic_and_profit(snapshot, alerts)
      evaluate_account_pools(snapshot, alerts)
      alerts.sort_by! do |alert|
        [SEVERITY_ORDER.fetch(alert.fetch("severity")), alert.fetch("code"), alert.fetch("object")]
      end
      actions = alerts.flat_map { |alert| alert.fetch("actions") }.uniq
      summary = SEVERITY_ORDER.keys.to_h do |severity|
        [severity, alerts.count { |alert| alert["severity"] == severity }]
      end

      {
        "ops_id" => @policy.fetch("ops_id"),
        "snapshot_id" => snapshot.fetch("snapshot_id"),
        "report_only" => true,
        "real_action_executed" => false,
        "external_system_contacted" => false,
        "summary" => summary,
        "alerts" => alerts,
        "recommended_actions" => actions
      }
    end

    private

    def evaluate_credential(snapshot, alerts)
      return unless snapshot.dig("system", "credential_exposure_detected")

      add_alert(alerts, "credential_exposure", "critical", "credentials",
                "Credential exposure flag is set.", @stop_loss.fetch("credential_exposure"))
    end

    def evaluate_billing(snapshot, alerts)
      billing = snapshot.fetch("billing")
      actions = @stop_loss.fetch("billing_integrity")
      if billing["balance_difference_usd"].abs > @thresholds["balance_difference_usd_abs_max"]
        add_alert(alerts, "billing_balance_difference", "critical", "billing",
                  "Balance reconciliation difference exceeds the allowed absolute value.", actions,
                  "difference_usd" => billing["balance_difference_usd"])
      end
      if billing["duplicate_credit_count"].positive?
        add_alert(alerts, "billing_duplicate_credit", "critical", "billing",
                  "Duplicate balance credits were reported.", actions,
                  "count" => billing["duplicate_credit_count"])
      end
      return unless billing["unexplained_adjustment_count"].positive?

      add_alert(alerts, "billing_unexplained_adjustment", "critical", "billing",
                "Unexplained balance adjustments were reported.", actions,
                "count" => billing["unexplained_adjustment_count"])
    end

    def evaluate_core_services(snapshot, alerts)
      services = snapshot.dig("system", "services")
      unhealthy = services.each_with_object([]) do |(name, healthy), result|
        result << name unless healthy
      end
      return if unhealthy.empty?

      add_alert(alerts, "core_service_down", "critical", "system",
                "One or more core services are unhealthy.",
                @stop_loss.fetch("core_service_down"), "services" => unhealthy.sort)
    end

    def evaluate_upstreams(snapshot, alerts)
      upstreams = snapshot.fetch("upstreams")
      unless upstreams.any? { |upstream| upstream["available"] }
        add_alert(alerts, "all_upstreams_down", "critical", "upstreams",
                  "No upstream is available.", @stop_loss.fetch("all_upstreams_down"))
        return
      end

      upstreams.each do |upstream|
        id = upstream.fetch("upstream_id")
        unless upstream["available"]
          add_alert(alerts, "upstream_unavailable", "warning", id,
                    "Upstream is unavailable.", ["review_or_disable_upstream"])
          next
        end
        compare_low(alerts, upstream, "balance_days_remaining", "upstream_balance_days_min",
                    "upstream_balance_low", id)
        compare_low(alerts, upstream, "success_rate", "upstream_success_rate_min",
                    "upstream_success_low", id)
        compare_high(alerts, upstream, "rate_limit_ratio", "upstream_rate_limit_ratio_max",
                     "upstream_rate_limit_high", id)
        compare_high(alerts, upstream, "server_error_ratio", "upstream_5xx_ratio_max",
                     "upstream_5xx_high", id)
        compare_high(alerts, upstream, "ttft_p95_ms", "upstream_ttft_p95_ms_max",
                     "upstream_ttft_high", id)
        compare_high(alerts, upstream, "stream_interruption_ratio",
                     "stream_interruption_ratio_max", "upstream_stream_interruptions", id)
      end
    end

    def compare_low(alerts, source, metric_key, threshold_key, code, object)
      return unless source[metric_key] < @thresholds[threshold_key]

      add_alert(alerts, code, "warning", object, "Metric is below the configured threshold.",
                ["review_upstream"], "metric" => metric_key,
                "value" => source[metric_key], "threshold" => @thresholds[threshold_key])
    end

    def compare_high(alerts, source, metric_key, threshold_key, code, object)
      return unless source[metric_key] > @thresholds[threshold_key]

      add_alert(alerts, code, "warning", object, "Metric exceeds the configured threshold.",
                ["review_upstream"], "metric" => metric_key,
                "value" => source[metric_key], "threshold" => @thresholds[threshold_key])
    end

    def evaluate_system_thresholds(snapshot, alerts)
      system = snapshot.fetch("system")
      if system["backup_age_hours"] > @thresholds["backup_age_hours_max"]
        add_alert(alerts, "backup_stale", "high", "backup",
                  "Latest backup is older than allowed.", ["create_and_verify_backup"])
      end
      if system["disk_used_ratio"] > @thresholds["disk_used_ratio_max"]
        add_alert(alerts, "disk_pressure", "high", "disk",
                  "Disk utilization exceeds the limit.", ["reduce_disk_usage"])
      end
      if system["failed_admin_logins_1h"] >= @thresholds["failed_admin_logins_1h_max"]
        add_alert(alerts, "failed_admin_logins", "high", "admin_auth",
                  "Failed admin logins reached the limit.", ["lockdown_admin_access", "preserve_evidence"])
      end
      if system["certificate_days_remaining"] < @thresholds["certificate_days_min"]
        add_alert(alerts, "certificate_expiry", "warning", "certificate",
                  "Certificate remaining lifetime is below the warning threshold.",
                  ["inspect_caddy_certificate_renewal"])
      end
      return unless system["restore_drill_age_days"] > @thresholds["restore_drill_age_days_max"]

      add_alert(alerts, "restore_drill_stale", "warning", "restore_drill",
                "Restore drill is overdue.", ["schedule_isolated_restore_drill"])
    end

    def evaluate_costs(snapshot, alerts)
      billing = snapshot.fetch("billing")
      if billing["daily_total_cost_usd"] > @thresholds["daily_total_cost_usd_max"]
        add_alert(alerts, "daily_cost_cap_exceeded", "high", "site_cost",
                  "Daily site cost exceeds the hard cap.",
                  ["disable_cost_outlier", "preserve_evidence"])
      end
      return unless billing["max_user_daily_cost_usd"] > @thresholds["per_user_daily_cost_usd_max"]

      add_alert(alerts, "user_cost_cap_exceeded", "high", "user_cost",
                "A user exceeded the daily cost cap.",
                ["suspend_high_cost_key", "preserve_evidence"])
    end

    def evaluate_traffic_and_profit(snapshot, alerts)
      traffic = snapshot.fetch("traffic")
      if traffic["request_count"].positive? &&
         traffic["request_id_coverage_ratio"] < @thresholds["request_id_coverage_ratio_min"]
        add_alert(alerts, "request_id_coverage", "warning", "request_logs",
                  "Not every request can be traced by request ID.", ["repair_request_id_logging"])
      end

      profit = snapshot.fetch("profit")
      revenue = profit["weekly_revenue_cny"]
      return unless revenue.positive?

      margin = (revenue - profit["weekly_full_cost_cny"]) / revenue * 100.0
      return unless margin < @thresholds["gross_margin_percent_min"]

      add_alert(alerts, "gross_margin_low", "warning", "weekly_profit",
                "Weekly full-cost gross margin is below target.",
                ["review_pricing_and_supply"], "gross_margin_percent" => margin.round(2))
    end

    def evaluate_account_pools(snapshot, alerts)
      snapshot.fetch("account_pools").each do |pool|
        id = pool.fetch("pool_id")
        if pool["available_accounts"].zero?
          add_alert(alerts, "account_pool_unavailable", "high", id,
                    "Account pool has no available account.",
                    ["disable_affected_models", "review_account_pool"])
        end
        if pool["error_accounts"].positive?
          add_alert(alerts, "account_pool_errors", "warning", id,
                    "Account pool contains accounts in error state.", ["review_account_pool"])
        end
        next unless pool["minimum_expiry_hours"] <= @thresholds["account_expiry_warning_hours"]

        add_alert(alerts, "account_pool_expiring", "warning", id,
                  "Account pool contains an account near expiry.", ["review_account_expiry"])
      end
    end

    def add_alert(alerts, code, severity, object, message, actions, evidence = {})
      alerts << {
        "code" => code,
        "severity" => severity,
        "object" => object,
        "message" => message,
        "actions" => actions,
        "evidence" => evidence
      }
    end
  end

  module CLI
    module_function

    def run(argv)
      command, policy_path, snapshot_path = argv
      unless %w[validate evaluate demo].include?(command) && policy_path
        warn "usage: ruby ops/evaluate-operations-baseline.rb <validate|evaluate|demo> POLICY [SNAPSHOT]"
        return 64
      end

      policy = load_document(policy_path)
      policy_errors = PolicyValidator.new(policy).errors
      if command == "validate"
        puts JSON.pretty_generate(
          "ops_id" => policy.is_a?(Hash) ? policy["ops_id"] : nil,
          "valid" => policy_errors.empty?,
          "errors" => policy_errors,
          "report_only" => true,
          "real_action_executed" => false,
          "external_system_contacted" => false
        )
        return policy_errors.empty? ? 0 : 1
      end
      raise ValidationError, policy_errors unless policy_errors.empty?
      raise ValidationError, ["snapshot: is required"] unless snapshot_path

      snapshot = load_document(snapshot_path)
      snapshot = incident_snapshot(snapshot) if command == "demo"
      puts JSON.pretty_generate(Evaluator.new(policy).evaluate(snapshot))
      0
    rescue Errno::ENOENT => e
      warn JSON.generate("valid" => false, "errors" => [e.message])
      66
    rescue Psych::Exception, ValidationError => e
      errors = e.respond_to?(:errors) ? e.errors : [e.message]
      warn JSON.generate("valid" => false, "errors" => errors)
      1
    end

    def load_document(path)
      YAML.safe_load(
        File.read(path), permitted_classes: [Date], aliases: false, filename: path
      )
    end

    def incident_snapshot(snapshot)
      copy = Marshal.load(Marshal.dump(snapshot))
      copy["snapshot_id"] = "OPS-SIM-INCIDENT"
      copy["system"]["credential_exposure_detected"] = true
      copy["billing"]["balance_difference_usd"] = 0.02
      copy["upstreams"].each { |upstream| upstream["available"] = false }
      copy
    end
  end
end

exit OperationsBaseline::CLI.run(ARGV) if $PROGRAM_NAME == __FILE__
