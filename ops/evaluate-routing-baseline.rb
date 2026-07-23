#!/usr/bin/env ruby
# frozen_string_literal: true

require "date"
require "json"
require "time"
require "yaml"

module RoutingBaseline
  class ConfigError < StandardError
    attr_reader :errors

    def initialize(errors)
      @errors = errors
      super(errors.join("; "))
    end
  end

  class ConfigValidator
    WEIGHT_KEYS = %w[success_rate cost ttft capacity_headroom support_response].freeze
    TRAFFIC_STAGES = %w[private_test paid_public].freeze
    CIRCUIT_STATES = %w[closed open half_open manual_open].freeze
    TERMS_STATUSES = %w[confirmed unknown forbidden].freeze
    FORBIDDEN_CREDENTIAL_KEYS = /\A(?:api[_-]?key|token|access[_-]?token|refresh[_-]?token|cookie|authorization|password|private[_-]?key|client[_-]?secret|oauth(?:[_-]?.*)?|credentials?)\z/i
    SECRET_VALUE = /(?:Authorization:\s*Bearer\s+\S{16,}|Cookie:\s*\S+|sk-[a-z0-9]{16,}|BEGIN [A-Z ]*PRIVATE KEY)/i
    SECRET_REFERENCE = %r{\A(?:sub2api-admin|password-manager)://[A-Za-z0-9._/-]+\z}

    attr_reader :errors

    def initialize(document)
      @document = document
      @errors = []
      validate
    end

    private

    def validate
      unless @document.is_a?(Hash)
        add("root", "must be a mapping")
        return
      end

      %w[
        schema_version routing_id status reviewed_at external_actions_deferred traffic_stage
        policy retry_policy circuit_breaker capacity_policy upstreams network_measurements evidence
      ].each { |key| require_key(@document, key, key) }

      add("schema_version", "must equal 1") unless @document["schema_version"] == 1
      add("routing_id", "must equal ROUTE01") unless @document["routing_id"] == "ROUTE01"
      add("status", "must equal fictional for the offline baseline") unless @document["status"] == "fictional"
      boolean(@document["external_actions_deferred"], "external_actions_deferred")
      unless @document["external_actions_deferred"] == true
        add("external_actions_deferred", "must be true")
      end
      enum(@document["traffic_stage"], "traffic_stage", TRAFFIC_STAGES)

      validate_policy
      validate_retry_policy
      validate_circuit_breaker
      validate_capacity_policy
      validate_upstreams
      validate_measurements
      validate_evidence
      scan_credentials(@document)
    end

    def validate_policy
      policy = mapping(@document["policy"], "policy")
      return unless policy

      %w[weights thresholds normalization].each { |key| require_key(policy, key, "policy.#{key}") }
      weights = mapping(policy["weights"], "policy.weights")
      if weights
        WEIGHT_KEYS.each do |key|
          require_key(weights, key, "policy.weights.#{key}")
          number(weights[key], "policy.weights.#{key}", min: 0.0, max: 1.0)
        end
        values = WEIGHT_KEYS.map { |key| weights[key] }
        if values.all? { |value| value.is_a?(Numeric) } && (values.sum - 1.0).abs > 1e-9
          add("policy.weights", "values must sum to 1.0")
        end
      end

      thresholds = mapping(policy["thresholds"], "policy.thresholds")
      if thresholds
        integer(thresholds["min_observations"], "policy.thresholds.min_observations", min: 1)
        number(thresholds["min_balance_days"], "policy.thresholds.min_balance_days", min: 0.0)
        number(thresholds["min_concurrency_headroom_ratio"],
               "policy.thresholds.min_concurrency_headroom_ratio", min: 0.0, max: 1.0)
        number(thresholds["max_rate_limit_ratio"], "policy.thresholds.max_rate_limit_ratio",
               min: 0.0, max: 1.0)
        number(thresholds["switch_margin_points"], "policy.thresholds.switch_margin_points",
               min: 0.0, max: 100.0)
        integer(thresholds["switch_confirmation_windows"],
                "policy.thresholds.switch_confirmation_windows", min: 1)
      end

      normalization = mapping(policy["normalization"], "policy.normalization")
      return unless normalization

      %w[
        success_rate_bad success_rate_good cost_usd_per_million_low
        cost_usd_per_million_high ttft_ms_good ttft_ms_bad headroom_ratio_low
        headroom_ratio_high support_minutes_good support_minutes_bad
      ].each do |key|
        require_key(normalization, key, "policy.normalization.#{key}")
        number(normalization[key], "policy.normalization.#{key}", min: 0.0)
      end
      ordered_pair(normalization, "success_rate_bad", "success_rate_good", "policy.normalization")
      ordered_pair(normalization, "cost_usd_per_million_low", "cost_usd_per_million_high",
                   "policy.normalization")
      ordered_pair(normalization, "ttft_ms_good", "ttft_ms_bad", "policy.normalization")
      ordered_pair(normalization, "headroom_ratio_low", "headroom_ratio_high",
                   "policy.normalization")
      ordered_pair(normalization, "support_minutes_good", "support_minutes_bad",
                   "policy.normalization")
    end

    def validate_retry_policy
      policy = mapping(@document["retry_policy"], "retry_policy")
      return unless policy

      %w[
        max_additional_attempts retryable_failure_types retryable_status_codes
        never_retry_status_codes require_response_not_started require_charge_not_observed
      ].each { |key| require_key(policy, key, "retry_policy.#{key}") }
      unless [0, 1].include?(policy["max_additional_attempts"])
        add("retry_policy.max_additional_attempts", "must be 0 or 1")
      end
      string_array(policy["retryable_failure_types"], "retry_policy.retryable_failure_types")
      integer_array(policy["retryable_status_codes"], "retry_policy.retryable_status_codes")
      integer_array(policy["never_retry_status_codes"], "retry_policy.never_retry_status_codes")
      boolean(policy["require_response_not_started"], "retry_policy.require_response_not_started")
      boolean(policy["require_charge_not_observed"], "retry_policy.require_charge_not_observed")
    end

    def validate_circuit_breaker
      policy = mapping(@document["circuit_breaker"], "circuit_breaker")
      return unless policy

      integer_keys = %w[
        window_size minimum_samples consecutive_failures_to_open base_cooldown_seconds
        max_cooldown_seconds half_open_probe_interval_seconds half_open_successes_to_close
        rate_limit_fallback_cooldown_seconds overload_529_cooldown_seconds
      ]
      integer_keys.each do |key|
        require_key(policy, key, "circuit_breaker.#{key}")
        integer(policy[key], "circuit_breaker.#{key}", min: 1)
      end
      require_key(policy, "failure_ratio_to_open", "circuit_breaker.failure_ratio_to_open")
      number(policy["failure_ratio_to_open"], "circuit_breaker.failure_ratio_to_open",
             min: 0.01, max: 1.0)
      if policy["minimum_samples"].is_a?(Integer) && policy["window_size"].is_a?(Integer) &&
         policy["minimum_samples"] > policy["window_size"]
        add("circuit_breaker.minimum_samples", "must not exceed window_size")
      end
      if policy["base_cooldown_seconds"].is_a?(Integer) &&
         policy["max_cooldown_seconds"].is_a?(Integer) &&
         policy["base_cooldown_seconds"] > policy["max_cooldown_seconds"]
        add("circuit_breaker.base_cooldown_seconds", "must not exceed max_cooldown_seconds")
      end
    end

    def validate_capacity_policy
      policy = mapping(@document["capacity_policy"], "capacity_policy")
      return unless policy

      %w[vertical network second_node].each do |section|
        require_key(policy, section, "capacity_policy.#{section}")
        mapping(policy[section], "capacity_policy.#{section}")
      end

      vertical = policy["vertical"]
      if vertical.is_a?(Hash)
        %w[
          oom_restarts_24h mem_available_mib_below swap_used_mib_above
          cpu_percent_above postgres_connection_ratio_above sustained_minutes
        ].each { |key| require_key(vertical, key, "capacity_policy.vertical.#{key}") }
        integer(vertical["oom_restarts_24h"], "capacity_policy.vertical.oom_restarts_24h", min: 1)
        integer(vertical["mem_available_mib_below"],
                "capacity_policy.vertical.mem_available_mib_below", min: 1)
        integer(vertical["swap_used_mib_above"],
                "capacity_policy.vertical.swap_used_mib_above", min: 0)
        number(vertical["cpu_percent_above"], "capacity_policy.vertical.cpu_percent_above",
               min: 1.0, max: 100.0)
        number(vertical["postgres_connection_ratio_above"],
               "capacity_policy.vertical.postgres_connection_ratio_above", min: 0.01, max: 1.0)
        integer(vertical["sustained_minutes"], "capacity_policy.vertical.sustained_minutes", min: 1)
      end

      network = policy["network"]
      if network.is_a?(Hash)
        %w[
          minimum_samples_per_grid entry_latency_p95_ms_above
          entry_packet_loss_percent_above
        ].each { |key| require_key(network, key, "capacity_policy.network.#{key}") }
        integer(network["minimum_samples_per_grid"],
                "capacity_policy.network.minimum_samples_per_grid", min: 1)
        number(network["entry_latency_p95_ms_above"],
               "capacity_policy.network.entry_latency_p95_ms_above", min: 1.0)
        number(network["entry_packet_loss_percent_above"],
               "capacity_policy.network.entry_packet_loss_percent_above", min: 0.0, max: 100.0)
      end

      second_node = policy["second_node"]
      return unless second_node.is_a?(Hash)

      %w[
        minimum_memory_gib entry_availability_7d_below
        sustained_sse_concurrency_above loss_to_cost_ratio
      ].each { |key| require_key(second_node, key, "capacity_policy.second_node.#{key}") }
      integer(second_node["minimum_memory_gib"],
              "capacity_policy.second_node.minimum_memory_gib", min: 2)
      number(second_node["entry_availability_7d_below"],
             "capacity_policy.second_node.entry_availability_7d_below", min: 0.0, max: 100.0)
      integer(second_node["sustained_sse_concurrency_above"],
              "capacity_policy.second_node.sustained_sse_concurrency_above", min: 1)
      number(second_node["loss_to_cost_ratio"],
             "capacity_policy.second_node.loss_to_cost_ratio", min: 1.0)
    end

    def validate_upstreams
      upstreams = array(@document["upstreams"], "upstreams")
      return unless upstreams

      add("upstreams", "must contain at least one fictional candidate") if upstreams.empty?
      seen = {}
      upstreams.each_with_index do |upstream, index|
        path = "upstreams[#{index}]"
        unless upstream.is_a?(Hash)
          add(path, "must be a mapping")
          next
        end
        %w[
          upstream_id status enabled manual_disabled terms_status models circuit_state metrics secret_ref
        ].each { |key| require_key(upstream, key, "#{path}.#{key}") }
        id = upstream["upstream_id"]
        string(id, "#{path}.upstream_id")
        add("#{path}.upstream_id", "duplicates #{id}") if seen[id]
        seen[id] = true if id
        add("#{path}.status", "must equal fictional") unless upstream["status"] == "fictional"
        boolean(upstream["enabled"], "#{path}.enabled")
        boolean(upstream["manual_disabled"], "#{path}.manual_disabled")
        enum(upstream["terms_status"], "#{path}.terms_status", TERMS_STATUSES)
        enum(upstream["circuit_state"], "#{path}.circuit_state", CIRCUIT_STATES)
        string_array(upstream["models"], "#{path}.models")
        unless upstream["secret_ref"].is_a?(String) && upstream["secret_ref"].match?(SECRET_REFERENCE)
          add("#{path}.secret_ref", "must be a symbolic secret location")
        end
        validate_metrics(upstream["metrics"], "#{path}.metrics")
      end
    end

    def validate_metrics(value, path)
      metrics = mapping(value, path)
      return unless metrics

      integer(metrics["observations"], "#{path}.observations", min: 0)
      number(metrics["success_rate"], "#{path}.success_rate", min: 0.0, max: 1.0)
      number(metrics["unit_cost_usd_per_million_tokens"],
             "#{path}.unit_cost_usd_per_million_tokens", min: 0.0)
      number(metrics["ttft_p95_ms"], "#{path}.ttft_p95_ms", min: 0.0)
      number(metrics["rate_limit_ratio"], "#{path}.rate_limit_ratio", min: 0.0, max: 1.0)
      number(metrics["balance_days_remaining"], "#{path}.balance_days_remaining", min: 0.0)
      number(metrics["concurrency_headroom_ratio"], "#{path}.concurrency_headroom_ratio",
             min: 0.0, max: 1.0)
      number(metrics["support_response_minutes"], "#{path}.support_response_minutes", min: 0.0)
    end

    def validate_measurements
      measurements = array(@document["network_measurements"], "network_measurements")
      return unless measurements

      if @document["status"] == "fictional" && !measurements.empty?
        add("network_measurements", "must be empty while status is fictional")
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

    def scan_credentials(value, path = nil)
      case value
      when Hash
        value.each do |key, child|
          child_path = path ? "#{path}.#{key}" : key.to_s
          if key.to_s != "secret_ref" && key.to_s.match?(FORBIDDEN_CREDENTIAL_KEYS)
            add(child_path, "credential fields are forbidden; use secret_ref")
          end
          scan_credentials(child, child_path)
        end
      when Array
        value.each_with_index { |child, index| scan_credentials(child, "#{path}[#{index}]") }
      when String
        add(path, "value looks like a secret") if value.match?(SECRET_VALUE)
      end
    end

    def require_key(mapping, key, path)
      add(path, "is required") unless mapping.key?(key)
    end

    def ordered_pair(mapping, low_key, high_key, path)
      low = mapping[low_key]
      high = mapping[high_key]
      return unless low.is_a?(Numeric) && high.is_a?(Numeric)

      add("#{path}.#{low_key}", "must be lower than #{high_key}") unless low < high
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

    def enum(value, path, allowed)
      add(path, "must be one of #{allowed.join(', ')}") unless allowed.include?(value)
    end

    def string_array(value, path)
      items = array(value, path)
      return unless items

      add(path, "must not be empty") if items.empty?
      items.each_with_index { |item, index| string(item, "#{path}[#{index}]") }
    end

    def integer_array(value, path)
      items = array(value, path)
      return unless items

      items.each_with_index { |item, index| integer(item, "#{path}[#{index}]", min: 100, max: 599) }
    end

    def add(path, message)
      @errors << "#{path}: #{message}"
    end
  end

  class Ranker
    def initialize(document)
      @document = document
      errors = ConfigValidator.new(document).errors
      raise ConfigError, errors unless errors.empty?
    end

    def rank(model)
      eligible = []
      probe_only = []
      excluded = []

      @document.fetch("upstreams").each do |upstream|
        reasons = exclusion_reasons(upstream, model)
        item = scored_item(upstream)
        if reasons.empty? && upstream["circuit_state"] == "closed"
          eligible << item
        elsif reasons.empty? && upstream["circuit_state"] == "half_open"
          probe_only << item
        else
          excluded << {
            "upstream_id" => upstream.fetch("upstream_id"),
            "reasons" => reasons
          }
        end
      end

      eligible.sort_by! { |item| [-item.fetch("score"), item.fetch("upstream_id")] }
      probe_only.sort_by! { |item| [-item.fetch("score"), item.fetch("upstream_id")] }
      excluded.sort_by! { |item| item.fetch("upstream_id") }
      thresholds = @document.dig("policy", "thresholds")

      {
        "model" => model,
        "traffic_stage" => @document.fetch("traffic_stage"),
        "recommended_primary" => eligible.dig(0, "upstream_id"),
        "eligible" => eligible,
        "probe_only" => probe_only,
        "excluded" => excluded,
        "switch_rule" => {
          "minimum_margin_points" => thresholds.fetch("switch_margin_points"),
          "confirmation_windows" => thresholds.fetch("switch_confirmation_windows")
        }
      }
    end

    private

    def exclusion_reasons(upstream, model)
      thresholds = @document.dig("policy", "thresholds")
      metrics = upstream.fetch("metrics")
      reasons = []
      reasons << "disabled" unless upstream["enabled"]
      reasons << "manually_disabled" if upstream["manual_disabled"]
      reasons << "terms_forbidden" if upstream["terms_status"] == "forbidden"
      if @document["traffic_stage"] == "paid_public" && upstream["terms_status"] != "confirmed"
        reasons << "terms_not_confirmed_for_paid_public"
      end
      reasons << "model_not_supported" unless upstream.fetch("models").include?(model)
      if %w[open manual_open].include?(upstream["circuit_state"])
        reasons << "circuit_#{upstream['circuit_state']}"
      end
      if metrics["observations"] < thresholds["min_observations"]
        reasons << "insufficient_observations"
      end
      if metrics["balance_days_remaining"] < thresholds["min_balance_days"]
        reasons << "insufficient_balance_days"
      end
      if metrics["concurrency_headroom_ratio"] < thresholds["min_concurrency_headroom_ratio"]
        reasons << "insufficient_concurrency_headroom"
      end
      if metrics["rate_limit_ratio"] > thresholds["max_rate_limit_ratio"]
        reasons << "rate_limit_ratio_too_high"
      end
      reasons
    end

    def scored_item(upstream)
      metrics = upstream.fetch("metrics")
      normal = @document.dig("policy", "normalization")
      weights = @document.dig("policy", "weights")
      components = {
        "success_rate" => normalize_high(metrics["success_rate"], normal["success_rate_bad"],
                                          normal["success_rate_good"]),
        "cost" => normalize_low(metrics["unit_cost_usd_per_million_tokens"],
                                normal["cost_usd_per_million_low"],
                                normal["cost_usd_per_million_high"]),
        "ttft" => normalize_low(metrics["ttft_p95_ms"], normal["ttft_ms_good"],
                                normal["ttft_ms_bad"]),
        "capacity_headroom" => normalize_high(metrics["concurrency_headroom_ratio"],
                                              normal["headroom_ratio_low"],
                                              normal["headroom_ratio_high"]),
        "support_response" => normalize_low(metrics["support_response_minutes"],
                                            normal["support_minutes_good"],
                                            normal["support_minutes_bad"])
      }
      score = components.sum { |key, value| value * weights.fetch(key) }
      {
        "upstream_id" => upstream.fetch("upstream_id"),
        "score" => score.round(2),
        "components" => components.transform_values { |value| value.round(2) }
      }
    end

    def normalize_high(value, low, high)
      clamp((value - low).to_f / (high - low) * 100.0)
    end

    def normalize_low(value, low, high)
      clamp((high - value).to_f / (high - low) * 100.0)
    end

    def clamp(value)
      [[value, 0.0].max, 100.0].min
    end
  end

  class RetryPolicy
    def initialize(policy)
      @policy = policy
    end

    def decide(event)
      return reject("attempt_limit_reached") if event["attempts_used"] >= @policy["max_additional_attempts"]
      return reject("target_not_different_or_eligible") unless event["different_eligible_target"]
      if @policy["require_response_not_started"] && event["response_started"]
        return reject("response_already_started")
      end
      status_code = event["status_code"]
      if status_code && @policy.fetch("never_retry_status_codes").include?(status_code)
        return reject("status_never_retry")
      end
      return reject("charge_already_observed") if event["charge_state"] == "charged"

      pre_write_failure = %w[connect_error tls_error].include?(event["failure_type"]) &&
                          !event["request_body_sent"]
      idempotent = event["idempotency_supported"] == true
      confirmed_not_charged = event["charge_state"] == "not_charged"
      unless pre_write_failure || idempotent || confirmed_not_charged
        return reject("charge_state_unknown")
      end

      failure_allowed = @policy.fetch("retryable_failure_types").include?(event["failure_type"])
      status_allowed = status_code && @policy.fetch("retryable_status_codes").include?(status_code)
      return reject("failure_not_retryable") unless failure_allowed || status_allowed

      { "retry" => true, "reason" => "safe_retry_allowed" }
    end

    private

    def reject(reason)
      { "retry" => false, "reason" => reason }
    end
  end

  class CircuitBreaker
    def initialize(policy)
      @policy = policy
    end

    def initial_state
      {
        "state" => "closed",
        "window" => [],
        "consecutive_failures" => 0,
        "reopen_count" => 0,
        "opened_at" => nil,
        "cooldown_seconds" => @policy.fetch("base_cooldown_seconds"),
        "half_open_successes" => 0,
        "last_probe_at" => nil,
        "last_action" => nil,
        "manual_disabled" => false
      }
    end

    def apply(source_state, event, now: Time.now.utc)
      state = Marshal.load(Marshal.dump(source_state))
      return manual_disable(state) if event == "manual_disable"
      return manual_enable if event == "manual_enable"
      return state if state["manual_disabled"] || state["state"] == "manual_open"

      case state["state"]
      when "closed"
        apply_closed(state, event, now)
      when "open"
        apply_open(state, event, now)
      when "half_open"
        apply_half_open(state, event, now)
      else
        raise ArgumentError, "unknown circuit state: #{state['state']}"
      end
    end

    private

    def manual_disable(state)
      state.merge("state" => "manual_open", "manual_disabled" => true)
    end

    def manual_enable
      initial_state
    end

    def apply_closed(state, event, now)
      case event
      when "success"
        append_window(state, false)
        state["consecutive_failures"] = 0
      when "failure"
        append_window(state, true)
        state["consecutive_failures"] += 1
        if should_open?(state)
          return open_state(state, now)
        end
      when "tick"
        nil
      else
        raise ArgumentError, "unknown circuit event: #{event}"
      end
      state
    end

    def apply_open(state, event, now)
      return state unless event == "tick"
      return state unless state["opened_at"]

      opened_at = Time.iso8601(state.fetch("opened_at"))
      if now >= opened_at + state.fetch("cooldown_seconds")
        state["state"] = "half_open"
        state["half_open_successes"] = 0
      end
      state
    end

    def apply_half_open(state, event, now)
      case event
      when "probe"
        interval = @policy.fetch("half_open_probe_interval_seconds")
        last_probe_at = state["last_probe_at"] && Time.iso8601(state["last_probe_at"])
        if last_probe_at.nil? || now >= last_probe_at + interval
          state["last_probe_at"] = now.utc.iso8601
          state["last_action"] = "probe_allowed"
        else
          state["last_action"] = "probe_rejected_interval"
        end
      when "success"
        state["half_open_successes"] += 1
        state["last_action"] = "probe_succeeded"
        if state["half_open_successes"] >= @policy.fetch("half_open_successes_to_close")
          return initial_state
        end
      when "failure"
        return open_state(state, now)
      when "tick"
        nil
      else
        raise ArgumentError, "unknown circuit event: #{event}"
      end
      state
    end

    def append_window(state, failed)
      state["window"] << failed
      state["window"] = state["window"].last(@policy.fetch("window_size"))
    end

    def should_open?(state)
      return true if state["consecutive_failures"] >= @policy.fetch("consecutive_failures_to_open")
      return false if state["window"].length < @policy.fetch("minimum_samples")

      failures = state["window"].count(true)
      failures.to_f / state["window"].length >= @policy.fetch("failure_ratio_to_open")
    end

    def open_state(state, now)
      reopen_count = state.fetch("reopen_count", 0) + 1
      base = @policy.fetch("base_cooldown_seconds")
      maximum = @policy.fetch("max_cooldown_seconds")
      cooldown = [base * (2**(reopen_count - 1)), maximum].min
      state.merge(
        "state" => "open",
        "reopen_count" => reopen_count,
        "opened_at" => now.utc.iso8601,
        "cooldown_seconds" => cooldown,
        "half_open_successes" => 0,
        "last_action" => "circuit_opened"
      )
    end
  end

  class CapacityAdvisor
    def initialize(policy)
      @policy = policy
    end

    def recommend(metrics)
      vertical = vertical_upgrade?(metrics)
      optimized_line = optimized_line?(metrics)
      second_node = second_node?(metrics)
      {
        "vertical_upgrade" => vertical,
        "optimized_line" => optimized_line,
        "second_node" => second_node,
        "purchase_action_taken" => false
      }
    end

    private

    def vertical_upgrade?(metrics)
      policy = @policy.fetch("vertical")
      return true if metrics["oom_restarts_24h"] >= policy.fetch("oom_restarts_24h")
      return false if metrics["resource_pressure_minutes"] < policy.fetch("sustained_minutes")

      metrics["mem_available_mib"] < policy.fetch("mem_available_mib_below") ||
        metrics["swap_used_mib"] > policy.fetch("swap_used_mib_above") ||
        metrics["cpu_percent"] > policy.fetch("cpu_percent_above") ||
        metrics["postgres_connection_ratio"] > policy.fetch("postgres_connection_ratio_above")
    end

    def optimized_line?(metrics)
      policy = @policy.fetch("network")
      return false unless metrics["grid_samples_complete"] && metrics["entry_is_primary_bottleneck"]

      metrics["entry_latency_p95_ms"] > policy.fetch("entry_latency_p95_ms_above") ||
        metrics["entry_packet_loss_percent"] > policy.fetch("entry_packet_loss_percent_above")
    end

    def second_node?(metrics)
      policy = @policy.fetch("second_node")
      vertical = @policy.fetch("vertical")
      return false unless metrics["vertical_upgrade_completed"]
      return false if metrics["current_memory_gib"] < policy.fetch("minimum_memory_gib")

      availability_trigger = metrics["entry_availability_7d_percent"] <
                             policy.fetch("entry_availability_7d_below")
      concurrency_trigger = metrics["sse_concurrency"] >=
                            policy.fetch("sustained_sse_concurrency_above") &&
                            metrics["pressure_minutes"] >= vertical.fetch("sustained_minutes")
      return false unless availability_trigger || concurrency_trigger

      cost = metrics["second_node_monthly_cost_cny"]
      return false unless cost.is_a?(Numeric) && cost.positive?

      metrics["expected_monthly_incident_loss_cny"].to_f / cost >=
        policy.fetch("loss_to_cost_ratio")
    end
  end

  module CLI
    module_function

    def run(argv)
      command, path, model = argv
      unless %w[validate score demo].include?(command) && path
        warn "usage: ruby ops/evaluate-routing-baseline.rb <validate|score|demo> CONFIG [MODEL]"
        return 64
      end

      document = load_document(path)
      validator = ConfigValidator.new(document)
      if command == "validate"
        puts JSON.pretty_generate(
          "routing_id" => document.is_a?(Hash) ? document["routing_id"] : nil,
          "valid" => validator.errors.empty?,
          "errors" => validator.errors,
          "offline_simulation" => true,
          "real_traffic_sent" => false
        )
        return validator.errors.empty? ? 0 : 1
      end
      raise ConfigError, validator.errors unless validator.errors.empty?

      case command
      when "score"
        raise ConfigError, ["model: is required for score"] unless model

        puts JSON.pretty_generate(offline_envelope.merge("ranking" => Ranker.new(document).rank(model)))
      when "demo"
        puts JSON.pretty_generate(demo(document))
      end
      0
    rescue Errno::ENOENT => e
      warn JSON.generate("valid" => false, "errors" => [e.message])
      66
    rescue Psych::Exception, ConfigError, ArgumentError => e
      errors = e.respond_to?(:errors) ? e.errors : [e.message]
      warn JSON.generate("valid" => false, "errors" => errors)
      1
    end

    def load_document(path)
      YAML.safe_load(
        File.read(path), permitted_classes: [Date], aliases: false, filename: path
      )
    end

    def offline_envelope
      {
        "offline_simulation" => true,
        "real_traffic_sent" => false,
        "purchase_action_taken" => false
      }
    end

    def demo(document)
      model = document.fetch("upstreams").first.fetch("models").first
      retry_policy = RetryPolicy.new(document.fetch("retry_policy"))
      breaker = CircuitBreaker.new(document.fetch("circuit_breaker"))
      now = Time.utc(2026, 7, 15, 0, 0, 0)
      state = breaker.initial_state
      5.times { |index| state = breaker.apply(state, "failure", now: now + index) }
      opened = state
      state = breaker.apply(state, "tick", now: now + 64)
      half_open = state
      state = breaker.apply(state, "success", now: now + 65)
      state = breaker.apply(state, "success", now: now + 66)

      offline_envelope.merge(
        "ranking" => Ranker.new(document).rank(model),
        "retry_examples" => {
          "pre_write_connect" => retry_policy.decide(
            "attempts_used" => 0,
            "failure_type" => "connect_error",
            "status_code" => nil,
            "response_started" => false,
            "request_body_sent" => false,
            "charge_state" => "not_charged",
            "idempotency_supported" => false,
            "different_eligible_target" => true
          ),
          "unknown_charge" => retry_policy.decide(
            "attempts_used" => 0,
            "failure_type" => "http_error",
            "status_code" => 503,
            "response_started" => false,
            "request_body_sent" => true,
            "charge_state" => "unknown",
            "idempotency_supported" => false,
            "different_eligible_target" => true
          )
        },
        "circuit_demo" => {
          "after_failures" => opened.fetch("state"),
          "after_cooldown" => half_open.fetch("state"),
          "after_two_successes" => state.fetch("state")
        }
      )
    end
  end
end

exit RoutingBaseline::CLI.run(ARGV) if $PROGRAM_NAME == __FILE__
