# frozen_string_literal: true

require "digest"
require "json"
require "time"
require "uri"
require_relative "upstream-benchmark"

module UpstreamBenchmarkNonfunctional
  module Canonical
    module_function

    def value(input)
      case input
      when Hash
        input.keys.sort.each_with_object({}) { |key, result| result[key] = value(input.fetch(key)) }
      when Array
        input.map { |item| value(item) }
      else
        input
      end
    end

    def hash(input)
      Digest::SHA256.hexdigest(JSON.generate(value(input)))
    end
  end

  class Profile
    PROTOCOLS = %w[chat_completions responses].freeze

    attr_reader :document

    def initialize(document)
      @document = document
      validate!
    end

    def [](key)
      @document[key]
    end

    def concurrency_levels(request_kind)
      key = case request_kind.to_s
            when "sync" then "sync_concurrency_levels"
            when "sse" then "sse_concurrency_levels"
            else
              raise UpstreamBenchmark::ValidationError, "request_kind must be sync or sse"
            end
      capacity.fetch(key)
    end

    def rpm_levels
      capacity.fetch("rpm_levels")
    end

    def rpm_window_seconds
      capacity.fetch("rpm_window_seconds")
    end

    def waves_per_level
      capacity.fetch("waves_per_level")
    end

    def profile_hash
      Canonical.hash(@document)
    end

    private

    def capacity
      @document.fetch("capacity")
    end

    def validate!
      unless @document.is_a?(Hash)
        raise UpstreamBenchmark::ValidationError, "nonfunctional profile must be a mapping"
      end

      %w[schema_version id protocol models_path generate_path terminal_events prompt max_output_tokens request_timeout_seconds capacity metrics].each do |key|
        raise UpstreamBenchmark::ValidationError, "nonfunctional profile.#{key} is required" if @document[key].nil?
      end
      raise UpstreamBenchmark::ValidationError, "nonfunctional profile schema_version must be 3" unless @document["schema_version"] == 3
      raise UpstreamBenchmark::ValidationError, "nonfunctional profile protocol is unsupported" unless PROTOCOLS.include?(@document["protocol"])
      validate_path!("models_path")
      validate_path!("generate_path")
      bounded_integer!("max_output_tokens", @document["max_output_tokens"], 1, 512)
      bounded_integer!("request_timeout_seconds", @document["request_timeout_seconds"], 1, 300)
      validate_terminal_events!
      validate_capacity!
      validate_metrics!
      UpstreamBenchmark::SecretGuard.validate!(@document)
    end

    def validate_terminal_events!
      events = @document["terminal_events"]
      valid = events.is_a?(Array) && events.length.between?(1, 4) &&
        events.uniq.length == events.length &&
        events.all? { |event| event.is_a?(String) && !event.empty? && event.bytesize <= 64 }
      raise UpstreamBenchmark::ValidationError, "nonfunctional profile terminal_events must contain 1-4 unique bounded strings" unless valid

      allowed = @document["protocol"] == "responses" ? ["response.completed", "[DONE]"] : ["[DONE]"]
      required = @document["protocol"] == "responses" ? "response.completed" : "[DONE]"
      unless (events - allowed).empty? && events.include?(required)
        raise UpstreamBenchmark::ValidationError, "nonfunctional profile terminal_events do not complete the selected protocol"
      end
    end

    def validate_capacity!
      unless capacity.is_a?(Hash)
        raise UpstreamBenchmark::ValidationError, "nonfunctional profile capacity must be a mapping"
      end
      %w[sync_concurrency_levels sse_concurrency_levels rpm_levels rpm_window_seconds waves_per_level].each do |key|
        raise UpstreamBenchmark::ValidationError, "nonfunctional profile capacity.#{key} is required" if capacity[key].nil?
      end
      validate_levels!("sync_concurrency_levels", 1, 10)
      validate_levels!("sse_concurrency_levels", 1, 10)
      validate_levels!("rpm_levels", 1, 120)
      bounded_integer!("capacity.rpm_window_seconds", capacity["rpm_window_seconds"], 1, 60)
      bounded_integer!("capacity.waves_per_level", capacity["waves_per_level"], 1, 10)
    end

    def validate_metrics!
      metrics = @document["metrics"]
      unless metrics.is_a?(Hash) && metrics["percentile_method"] == "nearest_rank"
        raise UpstreamBenchmark::ValidationError, "nonfunctional profile percentile_method must be nearest_rank"
      end
      unless metrics["percentiles"] == [50, 95]
        raise UpstreamBenchmark::ValidationError, "nonfunctional profile percentiles must be [50, 95]"
      end
      unless [true, false].include?(metrics["record_queue_signal"])
        raise UpstreamBenchmark::ValidationError, "nonfunctional profile record_queue_signal must be boolean"
      end
    end

    def validate_levels!(key, minimum, maximum)
      values = capacity[key]
      valid = values.is_a?(Array) && !values.empty? &&
        values.all? { |value| value.is_a?(Integer) && value.between?(minimum, maximum) } &&
        values.each_cons(2).all? { |left, right| left < right }
      raise UpstreamBenchmark::ValidationError, "nonfunctional profile capacity.#{key} must be strictly increasing bounded integers" unless valid
    end

    def bounded_integer!(key, value, minimum, maximum)
      return if value.is_a?(Integer) && value.between?(minimum, maximum)

      raise UpstreamBenchmark::ValidationError, "nonfunctional profile #{key} must be between #{minimum} and #{maximum}"
    end

    def validate_path!(key)
      value = @document[key]
      uri = URI.parse(value)
      decoded = value.dup
      3.times do
        unescaped = URI::DEFAULT_PARSER.unescape(decoded)
        break if unescaped == decoded

        decoded = unescaped
      end
      segments = decoded.split("/")
      valid = value.start_with?("/") && !value.start_with?("//") &&
        uri.scheme.nil? && uri.host.nil? && uri.query.nil? && uri.fragment.nil? &&
        decoded.start_with?("/") && !decoded.start_with?("//") && !decoded.include?("\\") &&
        decoded.each_byte.none? { |byte| byte < 0x20 || byte == 0x7f } &&
        segments.none? { |segment| segment == "." || segment == ".." }
      return if valid

      raise UpstreamBenchmark::ValidationError, "nonfunctional profile #{key} must be a safe absolute request path"
    rescue URI::InvalidURIError, ArgumentError, TypeError
      raise UpstreamBenchmark::ValidationError, "nonfunctional profile #{key} must be a safe absolute request path"
    end
  end

  class RequestBudget
    def initialize(profile:)
      @profile = profile
    end

    def calculate(model_count:, include_discovery:, topology_verification_requests:)
      bounded_count!("model_count", model_count)
      bounded_count!("topology_verification_requests", topology_verification_requests)
      unless [true, false].include?(include_discovery)
        raise UpstreamBenchmark::ValidationError, "include_discovery must be boolean"
      end

      discovery = include_discovery ? 1 : 0
      compatibility = model_count * 2
      sync_capacity = @profile.concurrency_levels("sync").sum * @profile.waves_per_level
      sse_capacity = @profile.concurrency_levels("sse").sum * @profile.waves_per_level
      rpm_capacity = @profile.rpm_levels.sum do |level|
        (level * @profile.rpm_window_seconds / 60.0).ceil
      end
      generation = compatibility + sync_capacity + sse_capacity + rpm_capacity + topology_verification_requests

      {
        "profile_id" => @profile["id"],
        "profile_hash" => @profile.profile_hash,
        "phases" => {
          "discovery" => discovery,
          "compatibility" => compatibility,
          "sync_capacity" => sync_capacity,
          "sse_capacity" => sse_capacity,
          "rpm_capacity" => rpm_capacity,
          "topology_verification" => topology_verification_requests
        },
        "maximum_http_requests" => discovery + generation,
        "maximum_generation_requests" => generation,
        "requests_sent" => 0,
        "network_sent" => false
      }
    end

    private

    def bounded_count!(name, value)
      return if value.is_a?(Integer) && value.between?(0, 100_000)

      raise UpstreamBenchmark::ValidationError, "#{name} must be an integer from 0 to 100000"
    end
  end

  module Sample
    IDENTITY_KEYS = %w[
      channel_id role group account_evidence_ref model_id profile_id profile_hash measurement_location
      run_id recorded_at
    ].freeze
    REQUEST_KINDS = %w[sync sse].freeze
    SAFE_ERRORS = %w[
      authentication insufficient_balance model_unavailable rate_limited upstream_http
      timeout cancelled tls connection protocol_error invalid_framing stream_incomplete
      response_too_large request_rejected transport_error billing_unknown budget_exhausted unknown
    ].freeze

    module_function

    def normalize(raw, request_kind:, identity:)
      validate_identity!(identity)
      unless REQUEST_KINDS.include?(request_kind.to_s)
        raise UpstreamBenchmark::ValidationError, "request_kind must be sync or sse"
      end
      source = raw.is_a?(Hash) ? raw : {}
      usage = source["usage"].is_a?(Hash) ? source["usage"] : {}
      status = source["status"].to_i
      stream_completed = request_kind.to_s == "sse" ? source["stream_complete"] == true : nil
      error_category = classify_error(source, request_kind.to_s)

      {
        "identity" => identity.slice(*IDENTITY_KEYS),
        "request_kind" => request_kind.to_s,
        "scheduled_at" => source["scheduled_at"],
        "started_at" => source["started_at"],
        "first_event_at" => source["first_event_at"],
        "completed_at" => source["completed_at"],
        "ttft_ms" => numeric_or_nil(source["first_event_ms"] || source["ttft_ms"]),
        "total_duration_ms" => numeric_or_nil(source["duration_ms"] || source["total_duration_ms"]),
        "queue_wait_ms" => numeric_or_nil(source["queue_wait_ms"]),
        "http_status_class" => status_class(status),
        "status" => status,
        "error_category" => error_category,
        "stream_started" => request_kind.to_s == "sse" ? !source["first_event_ms"].nil? : nil,
        "stream_completed" => stream_completed,
        "terminal_event_class" => request_kind.to_s == "sse" ? (stream_completed ? "complete" : "missing") : nil,
        "input_tokens" => integer_or_zero(usage["input_tokens"]),
        "output_tokens" => integer_or_zero(usage["output_tokens"]),
        "total_tokens" => integer_or_zero(usage["total_tokens"]),
        "client_timeout" => error_category == "timeout",
        "client_cancelled" => error_category == "cancelled",
        "estimated_cost_usd" => numeric_or_nil(source["estimated_cost_usd"]),
        "actual_cost_usd" => numeric_or_nil(source["actual_cost_usd"]),
        "topology_phase" => source["topology_phase"],
        "wave_id" => source["wave_id"]
      }
    end

    def success?(sample)
      sample["status"] == 200 && sample["error_category"].nil? &&
        (sample["request_kind"] != "sse" || sample["stream_completed"] == true)
    end

    def validate_identity!(identity)
      unless identity.is_a?(Hash)
        raise UpstreamBenchmark::ValidationError, "sample identity must be a mapping"
      end
      IDENTITY_KEYS.each do |key|
        value = identity[key]
        if !value.is_a?(String) || value.empty?
          raise UpstreamBenchmark::ValidationError, "sample identity.#{key} is required"
        end
      end
      unless identity["role"].match?(/\A(?:direct|gateway_primary|gateway_backup)\z/)
        raise UpstreamBenchmark::ValidationError, "sample identity.role is invalid"
      end
      unless identity["profile_hash"].match?(/\A[0-9a-f]{64}\z/)
        raise UpstreamBenchmark::ValidationError, "sample identity.profile_hash is invalid"
      end
      begin
        Time.iso8601(identity.fetch("recorded_at"))
      rescue ArgumentError
        raise UpstreamBenchmark::ValidationError, "sample identity.recorded_at must be ISO-8601"
      end
      UpstreamBenchmark::SecretGuard.validate!(identity)
    end

    def classify_error(source, request_kind)
      status = source["status"].to_i
      explicit = source["error_category"] || source["error"]
      category = if explicit
                   SAFE_ERRORS.include?(explicit.to_s) ? explicit.to_s : "unknown"
                 elsif status == 401 || status == 403
                   "authentication"
                 elsif status == 429
                   "rate_limited"
                 elsif status >= 500
                   "upstream_http"
                 elsif status >= 400
                   "request_rejected"
                 elsif status != 200
                   "transport_error"
                 elsif request_kind == "sse" && source["stream_complete"] != true
                   "stream_incomplete"
                 end
      category || nil
    end

    def status_class(status)
      return "none" if status.zero?

      "#{status / 100}xx"
    end

    def numeric_or_nil(value)
      value.is_a?(Numeric) && value.finite? ? value.to_f : nil
    end

    def integer_or_zero(value)
      value.is_a?(Integer) && value >= 0 ? value : 0
    end
  end

  module Metrics
    module_function

    def nearest_rank(values)
      sorted = values.compact.map(&:to_f).sort
      return { "p50" => nil, "p95" => nil, "n" => 0 } if sorted.empty?

      {
        "p50" => percentile(sorted, 50),
        "p95" => percentile(sorted, 95),
        "n" => sorted.length
      }
    end

    def percentile(sorted, percentile)
      rank = (percentile / 100.0 * sorted.length).ceil
      sorted[[rank - 1, 0].max]
    end

    def summarize(samples, achieved_overlap:)
      successes = samples.count { |sample| Sample.success?(sample) }
      sse = samples.select { |sample| sample["request_kind"] == "sse" }
      queue_values = samples.map { |sample| sample["queue_wait_ms"] }.compact
      errors = samples.map { |sample| sample["error_category"] }.compact
      started = samples.map { |sample| sample["started_at"] }.compact.sort
      completed = samples.map { |sample| sample["completed_at"] }.compact.sort
      actual_costs = samples.map { |sample| sample["actual_cost_usd"] }.compact
      estimated_costs = samples.map { |sample| sample["estimated_cost_usd"] }.compact
      {
        "request_count" => samples.length,
        "achieved_overlap" => achieved_overlap,
        "success_count" => successes,
        "error_count" => samples.length - successes,
        "error_rate" => samples.empty? ? 0.0 : (samples.length - successes).to_f / samples.length,
        "rate_limited_count" => samples.count { |sample| sample["error_category"] == "rate_limited" },
        "upstream_5xx_count" => samples.count { |sample| sample["error_category"] == "upstream_http" },
        "error_categories" => errors.each_with_object(Hash.new(0)) { |category, counts| counts[category] += 1 }.to_h,
        "ttft_ms" => nearest_rank(samples.map { |sample| sample["ttft_ms"] }),
        "total_duration_ms" => nearest_rank(samples.map { |sample| sample["total_duration_ms"] }),
        "stream_complete_count" => sse.count { |sample| sample["stream_completed"] == true },
        "stream_interruption_ratio" => sse.empty? ? nil : sse.count { |sample| sample["stream_completed"] != true }.to_f / sse.length,
        "queue_wait_ms" => queue_values.empty? ? "unknown" : nearest_rank(queue_values),
        "first_started_at" => started.first,
        "last_completed_at" => completed.last,
        "cost_usd" => {
          "estimated" => estimated_costs.empty? ? "unknown" : estimated_costs.sum,
          "actual" => actual_costs.empty? ? "unknown" : actual_costs.sum
        },
        "tokens" => {
          "input" => samples.sum { |sample| sample["input_tokens"] },
          "output" => samples.sum { |sample| sample["output_tokens"] },
          "total" => samples.sum { |sample| sample["total_tokens"] }
        }
      }
    end
  end

  class ExecutionBudget
    def initialize(max_requests:, max_tokens:, max_cost_usd:, max_wall_seconds:,
                   max_total_duration_ms: nil, max_queue_wait_ms: nil, clock: nil)
      @limits = {
        "max_requests" => max_requests, "max_tokens" => max_tokens,
        "max_cost_usd" => max_cost_usd, "max_wall_seconds" => max_wall_seconds,
        "max_total_duration_ms" => max_total_duration_ms, "max_queue_wait_ms" => max_queue_wait_ms
      }
      validate!
      @clock = clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
      @started_at = @clock.call
      @requests_used = 0
      @tokens_used = 0
      @cost_used = 0.0
      @mutex = Mutex.new
    end

    def reserve_request
      @mutex.synchronize do
        return "budget_exhausted" if @requests_used >= @limits["max_requests"]
        return "budget_exhausted" if elapsed_seconds >= @limits["max_wall_seconds"]

        @requests_used += 1
        nil
      end
    end

    def observe(sample)
      @mutex.synchronize do
        actual_cost = sample["actual_cost_usd"]
        return "billing_unknown" if !@limits["max_cost_usd"].nil? && actual_cost.nil?

        @tokens_used += sample["total_tokens"].to_i
        @cost_used += actual_cost.to_f if actual_cost
        return "budget_exhausted" if @tokens_used > @limits["max_tokens"]
        return "budget_exhausted" if !@limits["max_cost_usd"].nil? && @cost_used > @limits["max_cost_usd"]
        return "budget_exhausted" if elapsed_seconds > @limits["max_wall_seconds"]
        if @limits["max_total_duration_ms"] && sample["total_duration_ms"].to_f > @limits["max_total_duration_ms"]
          return "total_duration_threshold"
        end
        if @limits["max_queue_wait_ms"] && sample["queue_wait_ms"].to_f > @limits["max_queue_wait_ms"]
          return "queue_wait_threshold"
        end

        nil
      end
    end

    def snapshot
      @mutex.synchronize do
        @limits.merge(
          "requests_used" => @requests_used,
          "tokens_used" => @tokens_used,
          "cost_used_usd" => @cost_used,
          "wall_seconds_used" => elapsed_seconds
        )
      end
    end

    private

    def validate!
      unless @limits["max_requests"].is_a?(Integer) && @limits["max_requests"].positive? &&
             @limits["max_tokens"].is_a?(Integer) && @limits["max_tokens"].positive? &&
             @limits["max_wall_seconds"].is_a?(Numeric) && @limits["max_wall_seconds"].positive? &&
             (@limits["max_cost_usd"].nil? || (@limits["max_cost_usd"].is_a?(Numeric) && @limits["max_cost_usd"].positive?))
        raise UpstreamBenchmark::ValidationError, "execution budget limits are invalid"
      end
    end

    def elapsed_seconds
      @clock.call - @started_at
    end
  end

  class CapacityProbe
    def initialize(invoke:, profile:, request_kind:, identity:, clock: nil, budget: nil)
      @invoke = invoke
      @profile = profile
      @request_kind = request_kind.to_s
      @identity = identity
      @clock = clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
      @budget = budget
      @profile.concurrency_levels(@request_kind)
      Sample.validate_identity!(@identity)
    end

    def run
      levels = @profile.concurrency_levels(@request_kind)
      records = {}
      last_stable = nil
      stop_reason = nil

      levels.each do |level|
        waves = Array.new(@profile.waves_per_level) { run_wave(level) }
        samples = waves.flat_map { |wave| wave.fetch("samples") }
        achieved_overlap = waves.map { |wave| wave.fetch("achieved_overlap") }.min
        summary = Metrics.summarize(samples, achieved_overlap: achieved_overlap).merge(
          "planned_concurrency" => level,
          "waves" => @profile.waves_per_level
        )
        records[level.to_s] = summary
        stop_reason = failure_reason(samples, achieved_overlap, level)
        break if stop_reason

        last_stable = level
      end

      {
        "request_kind" => @request_kind,
        "identity" => @identity.slice(*Sample::IDENTITY_KEYS),
        "levels" => records,
        "last_stable" => last_stable,
        "limit" => last_stable == levels.last && stop_reason.nil? ? "at_least" : "stopped",
        "stop_reason" => stop_reason,
        "recommendation" => last_stable.nil? ? nil : [(last_stable * 0.8).floor, 1].max,
        "budget" => @budget&.snapshot
      }
    end

    private

    def run_wave(level)
      gate = Queue.new
      intervals = []
      mutex = Mutex.new
      threads = Array.new(level) do
        Thread.new do
          gate.pop
          started = @clock.call
          budget_reason = @budget&.reserve_request
          raw = budget_reason ? { "status" => 0, "error_category" => budget_reason } : invoke_safely
          completed = @clock.call
          mutex.synchronize { intervals << [started, completed] }
          sample = Sample.normalize(raw, request_kind: @request_kind, identity: @identity)
          observed_reason = budget_reason || @budget&.observe(sample)
          observed_reason ? sample.merge("status" => 0, "error_category" => observed_reason) : sample
        end
      end
      level.times { gate << true }
      samples = threads.map(&:value)
      { "samples" => samples, "achieved_overlap" => maximum_overlap(intervals) }
    end

    def invoke_safely
      @invoke.call(request_kind: @request_kind)
    rescue Timeout::Error
      { "status" => 0, "error_category" => "timeout" }
    rescue StandardError
      { "status" => 0, "error_category" => "transport_error" }
    end

    def maximum_overlap(intervals)
      events = intervals.flat_map { |started, completed| [[started, 1], [completed, -1]] }
      current = 0
      maximum = 0
      events.sort_by { |time, delta| [time, -delta] }.each do |_time, delta|
        current += delta
        maximum = [maximum, current].max
      end
      maximum
    end

    def failure_reason(samples, achieved_overlap, planned)
      failed = samples.find { |sample| !Sample.success?(sample) }
      return failed["error_category"] || "capacity_failure" if failed
      return "overlap_not_demonstrated" if achieved_overlap < planned

      nil
    end
  end

  class RpmProbe
    def initialize(invoke:, profile:, request_kind:, identity:, sleeper: nil, clock: nil, budget: nil)
      @invoke = invoke
      @profile = profile
      @request_kind = request_kind.to_s
      @identity = identity
      @sleeper = sleeper || ->(seconds) { sleep seconds }
      @clock = clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
      @budget = budget
      @profile.concurrency_levels(@request_kind)
      Sample.validate_identity!(@identity)
    end

    def run
      levels = {}
      last_stable = nil
      stop_reason = nil

      @profile.rpm_levels.each do |target_rpm|
        planned = [(target_rpm * @profile.rpm_window_seconds / 60.0).ceil, 1].max
        samples = []
        launch_lags = []
        window_started = @clock.call
        interval = @profile.rpm_window_seconds.to_f / planned
        planned.times do |index|
          scheduled = window_started + index * interval
          delay = scheduled - @clock.call
          @sleeper.call(delay) if delay.positive?
          launch_lags << [(@clock.call - scheduled) * 1000.0, 0.0].max
          budget_reason = @budget&.reserve_request
          raw = budget_reason ? { "status" => 0, "error_category" => budget_reason } : invoke_safely
          sample = Sample.normalize(raw, request_kind: @request_kind, identity: @identity)
          observed_reason = budget_reason || @budget&.observe(sample)
          sample = sample.merge("status" => 0, "error_category" => observed_reason) if observed_reason
          samples << sample
          break unless Sample.success?(sample)
        end
        levels[target_rpm.to_s] = Metrics.summarize(samples, achieved_overlap: 1).merge(
          "target_rpm" => target_rpm,
          "planned_request_count" => planned,
          "window_seconds" => @profile.rpm_window_seconds,
          "launch_lag_ms" => Metrics.nearest_rank(launch_lags)
        )
        failed = samples.find { |sample| !Sample.success?(sample) }
        if failed
          stop_reason = failed["error_category"] || "capacity_failure"
          break
        end
        lag_tolerance_ms = [interval * 1000.0 * 0.05, 10.0].max
        if levels[target_rpm.to_s].dig("launch_lag_ms", "p95").to_f > lag_tolerance_ms
          stop_reason = "target_rate_not_demonstrated"
          break
        end
        last_stable = target_rpm
      end

      {
        "mode" => "rpm",
        "request_kind" => @request_kind,
        "identity" => @identity.slice(*Sample::IDENTITY_KEYS),
        "levels" => levels,
        "last_stable" => last_stable,
        "limit" => last_stable == @profile.rpm_levels.last && stop_reason.nil? ? "at_least" : "stopped",
        "stop_reason" => stop_reason,
        "recommendation" => last_stable.nil? ? nil : [(last_stable * 0.8).floor, 1].max,
        "budget" => @budget&.snapshot
      }
    end

    private

    def invoke_safely
      @invoke.call(request_kind: @request_kind)
    rescue Timeout::Error
      { "status" => 0, "error_category" => "timeout" }
    rescue StandardError
      { "status" => 0, "error_category" => "transport_error" }
    end
  end

  class TopologyScenario
    ROLES = %w[gateway_primary gateway_backup].freeze
    REQUEST_KINDS = %w[sync sse].freeze

    attr_reader :document

    def initialize(document)
      @document = document
      validate!
    end

    def roles
      @document.fetch("roles")
    end

    def shared_capacity_pools
      @document.fetch("shared_capacity_pools", [])
    end

    def scenario_hash
      Canonical.hash(@document)
    end

    def role(group:, role:)
      roles.find { |item| item["group"] == group && item["role"] == role }
    end

    private

    def validate!
      unless @document.is_a?(Hash) && @document["schema_version"] == 3 &&
             @document["id"].is_a?(String) && !@document["id"].empty?
        raise UpstreamBenchmark::ValidationError, "topology scenario must be a version 3 mapping with an id"
      end
      unless roles.is_a?(Array) && !roles.empty?
        raise UpstreamBenchmark::ValidationError, "topology scenario roles must be a non-empty array"
      end

      identities = {}
      roles.each_with_index do |item, index|
        validate_role!(item, index)
        identity = [item["group"], item["role"]]
        if identities[identity]
          raise UpstreamBenchmark::ValidationError, "topology role is duplicated: #{identity.join(':')}"
        end
        identities[identity] = true
      end
      roles.group_by { |item| item["group"] }.each do |group, group_roles|
        unless group_roles.map { |item| item["role"] }.sort == ROLES.sort
          raise UpstreamBenchmark::ValidationError, "topology group #{group} must have one primary and one backup"
        end
      end

      primary_accounts = roles.select { |item| item["role"] == "gateway_primary" }.map { |item| item["account_evidence_ref"] }
      if primary_accounts.uniq.length != primary_accounts.length
        raise UpstreamBenchmark::ValidationError, "topology primary accounts must be unique"
      end
      backup_accounts = roles.select { |item| item["role"] == "gateway_backup" }.map { |item| item["account_evidence_ref"] }
      unless (primary_accounts & backup_accounts).empty?
        raise UpstreamBenchmark::ValidationError, "topology primary accounts cannot also be backups"
      end

      validate_pools!
      validate_shared_backup_coverage!
      UpstreamBenchmark::SecretGuard.validate!(@document)
    end

    def validate_role!(item, index)
      unless item.is_a?(Hash)
        raise UpstreamBenchmark::ValidationError, "topology roles[#{index}] must be a mapping"
      end
      %w[group role channel account_evidence_ref model_id profile_id profile_hash measurement_location required_request_kinds].each do |key|
        unless item[key].is_a?(key == "required_request_kinds" ? Array : String) && !item[key].empty?
          raise UpstreamBenchmark::ValidationError, "topology roles[#{index}].#{key} is required"
        end
      end
      raise UpstreamBenchmark::ValidationError, "topology role is invalid" unless ROLES.include?(item["role"])
      unless item["profile_hash"].match?(/\A[0-9a-f]{64}\z/)
        raise UpstreamBenchmark::ValidationError, "topology profile_hash is invalid"
      end
      kinds = item["required_request_kinds"]
      unless kinds.uniq.length == kinds.length && !kinds.empty? && (kinds - REQUEST_KINDS).empty?
        raise UpstreamBenchmark::ValidationError, "topology required_request_kinds are invalid"
      end
    end

    def validate_pools!
      unless shared_capacity_pools.is_a?(Array)
        raise UpstreamBenchmark::ValidationError, "shared_capacity_pools must be an array"
      end
      ids = {}
      shared_capacity_pools.each_with_index do |pool, index|
        unless pool.is_a?(Hash) && pool["id"].is_a?(String) && !pool["id"].empty?
          raise UpstreamBenchmark::ValidationError, "shared_capacity_pools[#{index}].id is required"
        end
        raise UpstreamBenchmark::ValidationError, "shared capacity pool id is duplicated" if ids[pool["id"]]

        ids[pool["id"]] = true
        members = pool["members"]
        unless members.is_a?(Array) && members.length >= 2
          raise UpstreamBenchmark::ValidationError, "shared capacity pool must have at least two members"
        end
        member_ids = {}
        members.each do |member|
          validate_member!(member)
          key = [member["group"], member["role"]]
          raise UpstreamBenchmark::ValidationError, "shared capacity pool member is duplicated" if member_ids[key]

          member_ids[key] = true
          configured = role(group: member["group"], role: member["role"])
          unless configured && configured["channel"] == member["channel"]
            raise UpstreamBenchmark::ValidationError, "shared capacity pool member does not match a topology role"
          end
        end
        limit = pool["aggregate_concurrency_limit"]
        unless limit.is_a?(Integer) && limit.between?(1, 100) &&
               limit <= members.sum { |member| member["requested_concurrency"] }
          raise UpstreamBenchmark::ValidationError, "shared capacity pool aggregate_concurrency_limit is invalid"
        end
        unless %w[equal_demand approved_asymmetric].include?(pool["allocation_policy"])
          raise UpstreamBenchmark::ValidationError, "shared capacity pool allocation_policy is invalid"
        end
        account_refs = members.map do |member|
          role(group: member["group"], role: member["role"]).fetch("account_evidence_ref")
        end
        unless account_refs.uniq.length == 1
          raise UpstreamBenchmark::ValidationError, "shared capacity pool members must reference one shared account"
        end
      end
    end

    def validate_member!(member)
      unless member.is_a?(Hash) && member["group"].is_a?(String) && !member["group"].empty? &&
             member["role"] == "gateway_backup" && member["channel"].is_a?(String) && !member["channel"].empty? &&
             member["requested_concurrency"].is_a?(Integer) && member["requested_concurrency"].between?(1, 10)
        raise UpstreamBenchmark::ValidationError, "shared capacity pool member is invalid"
      end
    end

    def validate_shared_backup_coverage!
      shared = roles.select { |item| item["role"] == "gateway_backup" }
                    .group_by { |item| item["account_evidence_ref"] }
                    .select { |_account, items| items.length > 1 }
      shared.each_value do |items|
        expected = items.map { |item| [item["group"], item["role"]] }.sort
        covered = shared_capacity_pools.any? do |pool|
          pool.fetch("members").map { |member| [member["group"], member["role"]] }.sort == expected
        end
        raise UpstreamBenchmark::ValidationError, "shared backup account must be declared as one capacity pool" unless covered
      end
    end
  end

  class SharedCapacityPoolEvaluator
    PHASES = %w[isolated equal_demand approved_mix].freeze
    INHERITED = {
      "success_rate_min" => 0.95,
      "rate_limited_ratio_max" => 0.15,
      "upstream_5xx_ratio_max" => 0.05,
      "ttft_p95_ms_max" => 5000.0,
      "stream_interruption_ratio_max" => 0.01
    }.freeze

    def initialize(scenario:, thresholds:)
      @scenario = scenario
      @thresholds = thresholds
      validate_thresholds!
    end

    def evaluate(pool_id:, samples:)
      pool = @scenario.shared_capacity_pools.find { |item| item["id"] == pool_id }
      raise UpstreamBenchmark::ValidationError, "unknown shared capacity pool: #{pool_id}" unless pool
      unless samples.is_a?(Array)
        raise UpstreamBenchmark::ValidationError, "shared capacity samples must be an array"
      end

      completed_total = samples.count { |sample| Sample.success?(sample) }
      reasons = []
      unexpected = samples.reject do |sample|
        pool.fetch("members").any? { |member| member_sample?(sample, member) }
      end
      reasons << "unexpected_sample_identity" unless unexpected.empty?
      phases = evaluate_phases(pool, samples, reasons)
      members = pool.fetch("members").each_with_object({}) do |member, result|
        key = member_key(member)
        selected = samples.select { |sample| member_sample?(sample, member) }
        if selected.empty?
          reasons << "missing_member_evidence:#{key}"
          result[key] = { "request_count" => 0, "completed_share" => 0.0, "status" => "failed" }
          next
        end
        summary = Metrics.summarize(selected, achieved_overlap: nil)
        expected_share = member["requested_concurrency"].to_f /
          pool.fetch("members").sum { |item| item["requested_concurrency"] }
        completed = selected.count { |sample| Sample.success?(sample) }
        completed_share = completed_total.zero? ? 0.0 : completed.to_f / completed_total
        member_reasons = metric_reasons(summary)
        if (completed_share - expected_share).abs > @thresholds["shared_pool_share_deviation_max"]
          member_reasons << "completed_share_deviation"
        end
        reasons.concat(member_reasons.map { |reason| "#{key}:#{reason}" })
        result[key] = summary.merge(
          "expected_share" => expected_share,
          "completed_share" => completed_share,
          "status" => member_reasons.empty? ? "passed" : "failed"
        )
      end
      aggregate = Metrics.summarize(samples, achieved_overlap: nil)
      status = if reasons.any?
                 "failed"
               elsif @thresholds["approved"]
                 "passed"
               else
                 "pending_threshold_approval"
               end
      {
        "scenario_id" => @scenario.document.fetch("id"),
        "scenario_hash" => @scenario.scenario_hash,
        "pool_id" => pool_id,
        "status" => status,
        "aggregate" => aggregate,
        "phases" => phases,
        "members" => members,
        "reasons" => reasons
      }
    end

    private

    def validate_thresholds!
      required = %w[approved shared_pool_share_deviation_max]
      required.each do |key|
        raise UpstreamBenchmark::ValidationError, "topology threshold #{key} is required" if @thresholds[key].nil?
      end
      unless [true, false].include?(@thresholds["approved"]) &&
             @thresholds["shared_pool_share_deviation_max"].is_a?(Numeric) &&
             @thresholds["shared_pool_share_deviation_max"].between?(0, 1)
        raise UpstreamBenchmark::ValidationError, "topology shared pool thresholds are invalid"
      end
    end

    def member_key(member)
      "#{member.fetch('group')}:#{member.fetch('role')}"
    end

    def member_sample?(sample, member)
      identity = sample["identity"] || {}
      role = @scenario.role(group: member["group"], role: member["role"])
      return false unless role

      {
        "group" => role["group"], "role" => role["role"], "channel_id" => role["channel"],
        "account_evidence_ref" => role["account_evidence_ref"], "model_id" => role["model_id"],
        "profile_id" => role["profile_id"], "profile_hash" => role["profile_hash"],
        "measurement_location" => role["measurement_location"]
      }.all? { |key, value| identity[key] == value }
    end

    def evaluate_phases(pool, samples, reasons)
      samples.map { |sample| sample["topology_phase"] }.uniq.each do |phase|
        next if PHASES.include?(phase)

        reasons << "unexpected_topology_phase:#{phase || 'missing'}"
      end
      PHASES.each_with_object({}) do |phase, result|
        phase_samples = samples.select { |sample| sample["topology_phase"] == phase }
        if phase_samples.empty?
          reasons << "missing_phase:#{phase}"
          result[phase] = { "status" => "failed" }
          next
        end
        kinds = pool.fetch("members").flat_map do |member|
          @scenario.role(group: member["group"], role: member["role"]).fetch("required_request_kinds")
        end.uniq
        phase_result = {}
        kinds.each do |kind|
          kind_samples = phase_samples.select { |sample| sample["request_kind"] == kind }
          pool.fetch("members").each do |member|
            unless kind_samples.any? { |sample| member_sample?(sample, member) }
              reasons << "#{member_key(member)}:#{phase}:missing_request_kind:#{kind}"
            end
          end
          if kind_samples.any? { |sample| !sample["wave_id"].is_a?(String) || sample["wave_id"].empty? }
            reasons << "missing_wave_id:#{phase}:#{kind}"
          end
          wave_overlaps = kind_samples.reject { |sample| sample["wave_id"].to_s.empty? }
                                            .group_by { |sample| sample["wave_id"] }.values.map do |wave|
            sample_overlap(wave)
          end
          achieved = wave_overlaps.max || 0
          required = phase == "isolated" ? pool.fetch("members").map { |item| item["requested_concurrency"] }.max : pool.fetch("aggregate_concurrency_limit")
          if achieved < required
            reasons << "#{phase}:#{kind}:aggregate_overlap_not_demonstrated"
          end
          phase_result[kind] = {
            "request_count" => kind_samples.length,
            "achieved_overlap" => achieved,
            "required_overlap" => required
          }
        end
        result[phase] = phase_result
      end
    end

    def sample_overlap(samples)
      intervals = samples.each_with_object([]) do |sample, result|
        started = parse_time(sample["started_at"])
        completed = parse_time(sample["completed_at"])
        result << [started.to_f, completed.to_f] if started && completed && completed >= started
      end
      events = intervals.flat_map { |started, completed| [[started, 1], [completed, -1]] }
      current = 0
      events.sort_by { |time, delta| [time, -delta] }.map do |_time, delta|
        current += delta
      end.max || 0
    end

    def parse_time(value)
      Time.iso8601(value.to_s).utc
    rescue ArgumentError
      nil
    end

    def metric_reasons(summary)
      count = summary["request_count"]
      reasons = []
      success_rate = count.zero? ? 0.0 : summary["success_count"].to_f / count
      reasons << "success_rate_below_gate" if success_rate < INHERITED["success_rate_min"]
      reasons << "rate_limited_ratio_above_gate" if count.positive? && summary["rate_limited_count"].to_f / count > INHERITED["rate_limited_ratio_max"]
      reasons << "upstream_5xx_ratio_above_gate" if count.positive? && summary["upstream_5xx_count"].to_f / count > INHERITED["upstream_5xx_ratio_max"]
      ttft = summary.dig("ttft_ms", "p95")
      reasons << "missing_ttft_p95" if summary.dig("ttft_ms", "n").to_i.zero?
      reasons << "ttft_p95_above_gate" if ttft && ttft > INHERITED["ttft_p95_ms_max"]
      reasons << "missing_total_duration_p95" if summary.dig("total_duration_ms", "n").to_i.zero?
      interruption = summary["stream_interruption_ratio"]
      reasons << "stream_interruption_ratio_above_gate" if interruption && interruption > INHERITED["stream_interruption_ratio_max"]
      reasons
    end
  end

  class ObservationEvaluator
    INHERITED = SharedCapacityPoolEvaluator::INHERITED

    def initialize(scenario:, thresholds:)
      @scenario = scenario
      @thresholds = thresholds
    end

    def evaluate(windows:)
      minimum = @thresholds["sustained_observation_hours_min"]
      unless minimum.is_a?(Integer) && minimum.between?(1, 168)
        raise UpstreamBenchmark::ValidationError, "sustained_observation_hours_min must be between 1 and 168"
      end
      reasons = []
      observed = {}
      @scenario.roles.each do |role|
        key = "#{role.fetch('group')}:#{role.fetch('role')}"
        selected = Array(windows).select { |window| role_window?(window, role) }
        parsed = selected.map { |window| parse_window_hour(window) }.compact.sort.uniq
        observed[key] = parsed.length
        reasons << "#{key}:insufficient_observation_hours" if parsed.length < minimum
        if parsed.length >= minimum && parsed.each_cons(2).any? { |left, right| (right - left) != 3600 }
          reasons << "#{key}:non_contiguous_observation_windows"
        end
        selected.each do |window|
          role.fetch("required_request_kinds").each do |kind|
            reasons << "#{key}:missing_request_kind:#{kind}" unless window.dig("request_kind_counts", kind).to_i.positive?
          end
          count = window["sample_count"].to_i
          success = window["success_count"].to_i
          reasons << "#{key}:success_rate_below_gate" if count <= 0 || success.to_f / count < INHERITED["success_rate_min"]
          reasons << "#{key}:rate_limited_ratio_above_gate" if count.positive? && window["rate_limited_count"].to_i.to_f / count > INHERITED["rate_limited_ratio_max"]
          reasons << "#{key}:upstream_5xx_ratio_above_gate" if count.positive? && window["upstream_5xx_count"].to_i.to_f / count > INHERITED["upstream_5xx_ratio_max"]
          reasons << "#{key}:ttft_p95_above_gate" if window["ttft_p95_ms"].is_a?(Numeric) && window["ttft_p95_ms"] > INHERITED["ttft_p95_ms_max"]
          if window["stream_interruption_ratio"].is_a?(Numeric) && window["stream_interruption_ratio"] > INHERITED["stream_interruption_ratio_max"]
            reasons << "#{key}:stream_interruption_ratio_above_gate"
          end
          reasons << "#{key}:missing_ttft_p95" unless finite_number?(window["ttft_p95_ms"])
          reasons << "#{key}:missing_total_duration_p95" unless finite_number?(window["total_duration_p95_ms"])
          if window.dig("request_kind_counts", "sse").to_i.positive? &&
             !finite_number?(window["stream_interruption_ratio"])
            reasons << "#{key}:missing_stream_interruption_ratio"
          end
        end
      end
      status = if reasons.any?
                 "failed"
               elsif @thresholds["approved"]
                 "passed"
               else
                 "pending_threshold_approval"
               end
      { "status" => status, "observed_hours" => observed.values.min || 0,
        "observed_hours_by_role" => observed, "required_hours" => minimum, "reasons" => reasons.uniq }
    end

    private

    def role_window?(window, role)
      return false unless window.is_a?(Hash) && window["sample_count"].to_i.positive? && window["missing"] == false

      identity = window["identity"] || {}
      {
        "group" => role["group"], "role" => role["role"], "channel_id" => role["channel"],
        "account_evidence_ref" => role["account_evidence_ref"], "model_id" => role["model_id"],
        "profile_id" => role["profile_id"], "profile_hash" => role["profile_hash"],
        "measurement_location" => role["measurement_location"]
      }.all? { |key, value| identity[key] == value }
    end

    def parse_window_hour(window)
      Time.iso8601(window.fetch("hour")).utc
    rescue ArgumentError, KeyError
      nil
    end

    def finite_number?(value)
      value.is_a?(Numeric) && value.finite?
    end
  end

  class DrillEvaluator
    TIMESTAMPS = %w[
      t_fault_observed t_detection_confirmed t_change_requested t_change_accepted
      t_route_converged t_first_backup_success t_primary_recovery_confirmed
      t_failback_requested t_failback_converged t_first_primary_success
    ].freeze

    def initialize(scenario:, thresholds:)
      @scenario = scenario
      @thresholds = thresholds
    end

    def evaluate(timeline:)
      parsed = TIMESTAMPS.to_h { |key| [key, parse_time(timeline[key])] }
      durations = {
        "service_failover_rto" => duration(parsed["t_fault_observed"], parsed["t_first_backup_success"]),
        "control_failover_time" => duration(parsed["t_change_requested"], parsed["t_route_converged"]),
        "service_failback_rto" => duration(parsed["t_failback_requested"], parsed["t_first_primary_success"])
      }
      reasons = []
      reasons << "incomplete_timeline" if parsed.values.any?(&:nil?)
      ordered = TIMESTAMPS.map { |key| parsed[key] }
      if ordered.each_cons(2).any? { |left, right| left && right && right < left }
        reasons << "non_monotonic_timeline"
      end
      group = timeline["group"]
      primary = @scenario.role(group: group, role: "gateway_primary")
      backup = @scenario.role(group: group, role: "gateway_backup")
      reasons << "unknown_drill_group" unless primary && backup
      reasons << "failover_route_state_unproved" unless route_proved?(timeline["route_evidence_after_failover"], "backup")
      reasons << "failback_route_state_unproved" unless route_proved?(timeline["route_evidence_after_failback"], "primary")
      reasons << "backup_sync_sse_unproved" unless role_samples_proved?(timeline["backup_verification_samples"], backup)
      reasons << "primary_sync_sse_unproved" unless role_samples_proved?(timeline["primary_verification_samples"], primary)
      reasons << "primary_recovery_window_unproved" unless recovery_window_proved?(timeline["primary_recovery_window"])
      if durations["service_failover_rto"].is_a?(Numeric) &&
         durations["service_failover_rto"] > @thresholds.fetch("failover_rto_seconds_max")
        reasons << "failover_rto_above_gate"
      end
      if durations["service_failback_rto"].is_a?(Numeric) &&
         durations["service_failback_rto"] > @thresholds.fetch("failback_rto_seconds_max")
        reasons << "failback_rto_above_gate"
      end
      status = if reasons.any? { |reason| reason != "incomplete_timeline" }
                 "failed"
               elsif reasons.include?("incomplete_timeline")
                 "partial"
               elsif @thresholds["approved"]
                 "passed"
               else
                 "pending_threshold_approval"
               end
      { "status" => status, "durations_seconds" => durations, "reasons" => reasons }
    rescue KeyError
      raise UpstreamBenchmark::ValidationError, "drill thresholds are incomplete"
    end

    private

    def parse_time(value)
      return nil if value.nil?

      Time.iso8601(value.to_s).utc
    rescue ArgumentError
      nil
    end

    def duration(started, completed)
      return "unknown" unless started && completed

      value = completed - started
      value.negative? ? "unknown" : value
    end

    def route_proved?(evidence, state)
      evidence.is_a?(Hash) && evidence["state"] == state && evidence["canonical_hash"].to_s.match?(/\A[0-9a-f]{64}\z/)
    end

    def role_samples_proved?(samples, role)
      return false unless role && samples.is_a?(Array)

      required = role.fetch("required_request_kinds")
      required.all? do |kind|
        samples.any? do |sample|
          sample["request_kind"] == kind && Sample.success?(sample) && role_identity?(sample["identity"], role)
        end
      end
    end

    def role_identity?(identity, role)
      identity ||= {}
      {
        "group" => role["group"], "role" => role["role"], "channel_id" => role["channel"],
        "account_evidence_ref" => role["account_evidence_ref"], "model_id" => role["model_id"],
        "profile_id" => role["profile_id"], "profile_hash" => role["profile_hash"],
        "measurement_location" => role["measurement_location"]
      }.all? { |key, value| identity[key] == value }
    end

    def recovery_window_proved?(window)
      return false unless window.is_a?(Hash)

      started = parse_time(window["started_at"])
      completed = parse_time(window["completed_at"])
      minimum = @thresholds.fetch("primary_recovery_window_seconds_min")
      minimum.is_a?(Numeric) && started && completed && completed - started >= minimum
    rescue KeyError
      false
    end
  end

  class OfflineTopologyEvaluator
    def initialize(scenario:, evidence:)
      @scenario = scenario
      @evidence = evidence
      validate!
    end

    def evaluate
      thresholds = @evidence.fetch("thresholds")
      pool_samples = @evidence.fetch("shared_capacity_samples", {})
      pools = @scenario.shared_capacity_pools.each_with_object({}) do |pool, result|
        samples = pool_samples.fetch(pool.fetch("id"), [])
        result[pool.fetch("id")] = SharedCapacityPoolEvaluator.new(
          scenario: @scenario,
          thresholds: thresholds
        ).evaluate(pool_id: pool.fetch("id"), samples: samples)
      end
      observation = if Array(@evidence["observation_windows"]).empty?
                      { "status" => "not_evaluated" }
                    else
                      ObservationEvaluator.new(scenario: @scenario, thresholds: thresholds).evaluate(
                        windows: @evidence.fetch("observation_windows")
                      )
                    end
      drill = if !@evidence["drill_timeline"].is_a?(Hash) || @evidence["drill_timeline"].empty?
                { "status" => "not_evaluated" }
              else
                DrillEvaluator.new(scenario: @scenario, thresholds: thresholds).evaluate(
                  timeline: @evidence.fetch("drill_timeline")
                )
              end
      statuses = pools.values.map { |result| result.fetch("status") } +
        [observation.fetch("status"), drill.fetch("status")]
      {
        "scenario_id" => @scenario.document.fetch("id"),
        "scenario_hash" => @scenario.scenario_hash,
        "status" => combined_status(statuses),
        "shared_capacity_pools" => pools,
        "observation" => observation,
        "drill" => drill,
        "requests_sent" => 0,
        "network_sent" => false
      }
    end

    private

    def validate!
      unless @evidence.is_a?(Hash) && @evidence["schema_version"] == 3 && @evidence["thresholds"].is_a?(Hash)
        raise UpstreamBenchmark::ValidationError, "topology evidence must be a version 3 mapping with thresholds"
      end
      unless @evidence["scenario_id"] == @scenario.document.fetch("id")
        raise UpstreamBenchmark::ValidationError, "topology evidence scenario_id does not match scenario"
      end
      unless @evidence["scenario_hash"] == @scenario.scenario_hash
        raise UpstreamBenchmark::ValidationError, "topology evidence scenario_hash does not match scenario"
      end
      UpstreamBenchmark::SecretGuard.validate!(@evidence)
    end

    def combined_status(statuses)
      return "failed" if statuses.include?("failed")
      return "partial" if statuses.any? { |status| %w[partial not_evaluated].include?(status) }
      return "pending_threshold_approval" if statuses.include?("pending_threshold_approval")

      "passed"
    end
  end
end
