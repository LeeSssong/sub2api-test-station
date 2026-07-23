#!/usr/bin/env ruby
# frozen_string_literal: true

require "bigdecimal"
require "date"
require "digest"
require "fileutils"
require "json"
require "optparse"
require "securerandom"
require "time"
require "uri"
require "yaml"
require_relative "upstream-benchmark"
require_relative "upstream-benchmark-nonfunctional"

module UpstreamBenchmarkV2
  class Profile
    PROTOCOL_DEFAULTS = {
      "chat_completions" => {
        "models_path" => "/models",
        "generate_path" => "/chat/completions",
        "terminal_events" => ["[DONE]"]
      },
      "responses" => {
        "models_path" => "/v1/models",
        "generate_path" => "/v1/responses",
        "terminal_events" => ["response.completed"]
      }
    }.freeze

    attr_reader :document

    def initialize(document)
      @document = document
      validate!
    end

    def [](key)
      @document[key]
    end

    def concurrency_levels
      @document.fetch("concurrency_levels")
    end

    def rpm_levels
      @document.fetch("rpm_levels")
    end

    def rpm_window_seconds
      @document.fetch("rpm_window_seconds")
    end

    def representative_models
      Array(@document["representative_models"])
    end

    def representative_roles
      @document["representative_roles"] || {}
    end

    def protocol
      @document["protocol"] || @document["endpoint"]
    end

    def models_path
      @document["models_path"] || PROTOCOL_DEFAULTS.fetch(protocol).fetch("models_path")
    end

    def generate_path
      @document["generate_path"] || PROTOCOL_DEFAULTS.fetch(protocol).fetch("generate_path")
    end

    def terminal_events
      Array(@document["terminal_events"] || PROTOCOL_DEFAULTS.fetch(protocol).fetch("terminal_events"))
    end

    private

    def validate!
      raise UpstreamBenchmark::ValidationError, "v2 profile must be a mapping" unless @document.is_a?(Hash)

      required = %w[schema_version id prompt max_output_tokens timeout_seconds concurrency_levels rpm_levels rpm_window_seconds]
      required.each do |key|
        raise UpstreamBenchmark::ValidationError, "v2 profile.#{key} is required" if @document[key].nil?
      end
      raise UpstreamBenchmark::ValidationError, "v2 profile schema_version must be 2" unless @document["schema_version"] == 2
      unless PROTOCOL_DEFAULTS.key?(protocol)
        raise UpstreamBenchmark::ValidationError, "v2 profile protocol must be chat_completions or responses"
      end
      validate_path!("models_path", models_path)
      validate_path!("generate_path", generate_path)
      unless terminal_events.length.between?(1, 4) && terminal_events.all? { |event| event.is_a?(String) && !event.empty? && event.bytesize <= 64 }
        raise UpstreamBenchmark::ValidationError, "v2 profile terminal_events must contain 1-4 bounded strings"
      end
      bounded_integer!("max_output_tokens", 1, 512)
      bounded_integer!("timeout_seconds", 1, 300)
      bounded_integer!("rpm_window_seconds", 1, 60)
      validate_levels!("concurrency_levels", 1, 10)
      validate_levels!("rpm_levels", 1, 120)
      validate_representative_models!
      validate_representative_roles!
      UpstreamBenchmark::SecretGuard.validate!(@document)
    end

    def bounded_integer!(key, minimum, maximum)
      value = @document[key]
      return if value.is_a?(Integer) && value.between?(minimum, maximum)

      raise UpstreamBenchmark::ValidationError, "v2 profile #{key} must be between #{minimum} and #{maximum}"
    end

    def validate_levels!(key, minimum, maximum)
      values = @document[key]
      unless values.is_a?(Array) && !values.empty? && values.all? { |value| value.is_a?(Integer) && value.between?(minimum, maximum) }
        raise UpstreamBenchmark::ValidationError, "v2 profile #{key} must contain bounded integers"
      end
      raise UpstreamBenchmark::ValidationError, "v2 profile #{key} must be strictly increasing" unless values.each_cons(2).all? { |left, right| left < right }
    end

    def validate_representative_models!
      models = @document["representative_models"]
      return if models.nil?

      unless models.is_a?(Array) && models.length.between?(1, 3) && models.all? { |model| model.is_a?(String) && !model.strip.empty? }
        raise UpstreamBenchmark::ValidationError, "v2 profile representative_models must contain 1-3 model ids"
      end
      raise UpstreamBenchmark::ValidationError, "v2 profile representative_models must be unique" unless models.uniq.length == models.length
    end

    def validate_representative_roles!
      roles = @document["representative_roles"]
      return if roles.nil?

      required = %w[common expensive new]
      unless roles.is_a?(Hash) && roles.keys.sort == required.sort &&
             roles.values.all? { |model| model.is_a?(String) && !model.strip.empty? } &&
             roles.values.uniq.length == roles.values.length
        raise UpstreamBenchmark::ValidationError,
              "v2 profile representative_roles must define unique common, expensive, and new model ids"
      end
    end

    def validate_path!(key, value)
      uri = URI.parse(value)
      decoded = value.dup
      3.times do
        unescaped = URI::DEFAULT_PARSER.unescape(decoded)
        break if unescaped == decoded

        decoded = unescaped
      end
      decoded_segments = decoded.split("/")
      valid = value.start_with?("/") && !value.start_with?("//") &&
        uri.scheme.nil? && uri.host.nil? && uri.query.nil? && uri.fragment.nil? &&
        decoded.start_with?("/") && !decoded.start_with?("//") && !decoded.include?("\\") &&
        decoded.each_byte.none? { |byte| byte < 0x20 || byte == 0x7f } &&
        decoded_segments.none? { |segment| segment == "." || segment == ".." }
      return if valid

      raise UpstreamBenchmark::ValidationError, "v2 profile #{key} must be a safe absolute request path"
    rescue URI::InvalidURIError, ArgumentError
      raise UpstreamBenchmark::ValidationError, "v2 profile #{key} must be a safe absolute request path"
    end
  end

  module ModelCatalog
    module_function

    NON_TEXT_PATTERNS = {
      "image" => /(?:dall[-_]?e|image|flux|sdxl|stable[-_]?diffusion)/i,
      "audio" => /(?:whisper|tts|speech|audio)/i,
      "realtime" => /realtime/i
    }.freeze
    TEXT_PATTERN = /(?:gpt|o[1-4](?:[-.]|$)|claude|gemini|deepseek|qwen|llama|mistral|command|sonnet|opus|haiku|codex)/i.freeze

    def classify(model_id)
      identifier = model_id.to_s
      NON_TEXT_PATTERNS.each { |kind, pattern| return kind if identifier.match?(pattern) }
      return "text" if identifier.match?(TEXT_PATTERN)

      "unknown"
    end

    def discover(models)
      Array(models).each_with_object({}) do |raw, catalog|
        identifier = raw.is_a?(Hash) ? raw["id"] : raw
        next if identifier.nil? || identifier.to_s.empty?

        id = identifier.to_s
        kind = classify(id)
        catalog[id] = { "id" => id, "kind" => kind, "testable" => kind == "text" }
      end
    end
  end

  module PricingEvidence
    module_function

    def validate!(document)
      raise UpstreamBenchmark::ValidationError, "pricing evidence must be a mapping" unless document.is_a?(Hash)
      %w[schema_version channel_id currency models].each do |key|
        raise UpstreamBenchmark::ValidationError, "pricing evidence.#{key} is required" if document[key].nil?
      end
      raise UpstreamBenchmark::ValidationError, "pricing evidence schema_version must be 1" unless document["schema_version"] == 1
      unless document["currency"].is_a?(String) && %w[USD CNY].include?(document["currency"])
        raise UpstreamBenchmark::ValidationError, "pricing evidence currency must be USD or CNY"
      end
      models = document["models"]
      raise UpstreamBenchmark::ValidationError, "pricing evidence models must be a non-empty mapping" unless models.is_a?(Hash) && !models.empty?

      models.each do |model_id, prices|
        raise UpstreamBenchmark::ValidationError, "pricing evidence model id must be non-empty" if model_id.to_s.empty?
        unless prices.is_a?(Hash)
          raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id} must be a mapping"
        end
        %w[input output].each do |key|
          raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id}.#{key} is required" unless prices.key?(key)
        end
        %w[input output cache_read cache_write].each do |key|
          next if prices[key].nil?
          unless prices[key].is_a?(Numeric) && prices[key] >= 0 && prices[key].finite?
            raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id}.#{key} must be a non-negative number"
          end
        end
        raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id}.source is required" unless prices["source"].is_a?(String) && !prices["source"].strip.empty?
        raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id}.verified_at is required" if prices["verified_at"].nil?
        parse_evidence_date(prices["verified_at"])
      rescue ArgumentError
        raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id}.verified_at must be ISO 8601"
      end
      UpstreamBenchmark::SecretGuard.validate!(document)
      true
    end

    def parse_evidence_date(value)
      raw = value.to_s
      raw.match?(/\A\d{4}-\d{2}-\d{2}\z/) ? Date.iso8601(raw) : Time.iso8601(raw)
    end
  end

  class DiscoveryRunner
    def initialize(client:, profile:)
      @client = client
      @profile = profile
    end

    def run(channel_id:)
      result = fetch_models
      catalog = ModelCatalog.discover(result["models"])
      model_ids = catalog.keys.sort
      classifications = model_ids.map { |model_id| catalog.fetch(model_id) }
      errors = if result["status"] == 200
                 []
               else
                 [{ "stage" => "models", "category" => error_category(result) }]
               end

      {
        "schema_version" => 1,
        "run_id" => SecureRandom.uuid,
        "channel_id" => channel_id,
        "profile_id" => @profile["id"],
        "recorded_at" => Time.now.utc.iso8601,
        "status" => result["status"] == 200 ? "partial" : "failed",
        "evidence_source" => "live_direct",
        "qualification_status" => "discovered_not_qualified",
        "metrics" => {
          "request_count" => 1,
          "generation_request_count" => 0,
          "model_count" => model_ids.length,
          "model_ids" => model_ids,
          "testable_model_ids" => model_ids.select { |model_id| catalog.fetch(model_id)["testable"] },
          "classifications" => classifications,
          "latency_ms" => result["duration_ms"]
        },
        "errors" => errors
      }
    end

    private

    def fetch_models
      @client.models
    rescue Timeout::Error
      { "status" => 0, "models" => [], "duration_ms" => nil, "error" => "timeout" }
    rescue StandardError
      { "status" => 0, "models" => [], "duration_ms" => nil, "error" => "transport_error" }
    end

    def error_category(result)
      status = result["status"].to_i
      return "rate_limited" if status == 429
      return "upstream_http" if status >= 500
      return "request_rejected" if status >= 400
      return "timeout" if result["error"] == "timeout"
      return "transport_error" if %w[dns tls transport_error].include?(result["error"])

      "protocol_error"
    end
  end

  class CapacityProbe
    def initialize(invoke:, profile:, clock: nil, sleeper: nil)
      @invoke = invoke
      @profile = profile
      @clock = clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
      @sleeper = sleeper || ->(seconds) { sleep seconds }
    end

    def run
      concurrency = probe_concurrency
      rpm = probe_rpm
      {
        "concurrency" => concurrency,
        "rpm" => rpm,
        "recommendation" => {
          "concurrency" => safe_value(concurrency["last_stable"]),
          "rpm" => safe_value(rpm["last_stable"])
        }
      }
    end

    private

    def probe_concurrency
      levels = @profile.concurrency_levels
      records = {}
      last_stable = nil
      stop_reason = nil
      baseline_duration_ms = nil
      levels.each do |level|
        started = @clock.call
        batch = parallel(level)
        summary = summarize(batch).merge(
          "request_count" => level,
          "wall_ms" => elapsed_ms(started)
        )
        records[level.to_s] = summary
        if batch.any? { |result| failure?(result) }
          stop_reason = batch.map { |result| failure_reason(result) }.compact.first || "capacity_failure"
          break
        end
        if baseline_duration_ms && batch.any? { |result| result["duration_ms"].to_f > baseline_duration_ms * 3.0 }
          stop_reason = "queueing_detected"
          break
        end
        baseline_duration_ms ||= average_duration(batch)
        last_stable = level
      end
      records.merge(
        "last_stable" => last_stable,
        "limit" => last_stable == levels.last && stop_reason.nil? ? "at_least" : "stopped"
      ).tap { |result| result["stop_reason"] = stop_reason if stop_reason }
    end

    def probe_rpm
      levels = @profile.rpm_levels
      records = {}
      last_stable = nil
      stop_reason = nil
      levels.each do |target_rpm|
        count = [(target_rpm * @profile.rpm_window_seconds / 60.0).ceil, 1].max
        started = @clock.call
        results = []
        count.times do |index|
          result = @invoke.call
          results << result
          @sleeper.call(@profile.rpm_window_seconds.to_f / count) if index < count - 1
          break if failure?(result)
        end
        elapsed = elapsed_ms(started)
        records[target_rpm.to_s] = summarize(results).merge(
          "target_rpm" => target_rpm,
          "request_count" => count,
          "window_seconds" => @profile.rpm_window_seconds,
          "observed_rpm" => elapsed.positive? ? results.length * 60_000.0 / elapsed : nil
        )
        if results.any? { |result| failure?(result) }
          stop_reason = results.map { |result| failure_reason(result) }.compact.first || "rate_failure"
          break
        end
        last_stable = target_rpm
      end
      records.merge(
        "last_stable" => last_stable,
        "limit" => last_stable == levels.last && stop_reason.nil? ? "at_least" : "stopped"
      ).tap { |result| result["stop_reason"] = stop_reason if stop_reason }
    end

    def parallel(count)
      Array.new(count) { Thread.new { @invoke.call } }.map(&:value)
    end

    def summarize(results)
      successes = results.count { |result| result["status"] == 200 }
      durations = results.map { |result| result["duration_ms"] }.compact
      {
        "success_count" => successes,
        "success_rate" => results.empty? ? 0.0 : successes.to_f / results.length,
        "latency" => UpstreamBenchmark::Metrics.latency(durations)
      }
    end

    def failure?(result)
      result["status"].to_i != 200
    end

    def failure_reason(result)
      status = result["status"].to_i
      return "rate_limited" if status == 429
      return "upstream_http" if status >= 500
      return result["error"] if result["error"]
      return "request_rejected" if status >= 400

      "transport_error"
    end

    def average_duration(results)
      durations = results.map { |result| result["duration_ms"] }.compact.map(&:to_f)
      return nil if durations.empty?

      durations.sum / durations.length
    end

    def safe_value(value)
      return 1 if value.nil?

      [(value * 0.8).floor, 1].max
    end

    def elapsed_ms(started)
      (@clock.call - started) * 1000.0
    end
  end

  class Runner
    def initialize(client:, profile:, clock: nil, sleeper: nil)
      @client = client
      @profile = profile
      @clock = clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
      @sleeper = sleeper || ->(seconds) { sleep seconds }
    end

    def run(channel_id:)
      model_result = @client.models
      catalog = ModelCatalog.discover(model_result["models"])
      text_models = catalog.values.select { |item| item["testable"] }.map { |item| item["id"] }
      per_model = {}
      results = []
      text_models.each do |model|
        sync = invoke(model, false)
        stream = invoke(model, true)
        per_model[model] = {
          "sync" => summarize([sync]),
          "stream" => summarize([stream]).merge("complete" => stream["stream_complete"] == true),
          "usage" => aggregate_usage([sync, stream])
        }
        results.concat([sync, stream])
      end

      capacity = if text_models.empty?
                   { "status" => "unknown", "reason" => "no_text_models" }
                 else
                   CapacityProbe.new(
                     invoke: -> { invoke(text_models.first, false) },
                     profile: @profile,
                     clock: @clock,
                     sleeper: @sleeper
                   ).run
                 end

      errors = []
      errors << { "stage" => "models", "category" => error_for(model_result) } unless model_result["status"] == 200
      text_models.each do |model|
        sync = per_model.fetch(model).fetch("sync")
        stream = per_model.fetch(model).fetch("stream")
        errors << { "stage" => "#{model}.sync", "category" => "request_failed" } unless sync["success_count"] == 1
        errors << { "stage" => "#{model}.stream", "category" => "request_failed" } unless stream["success_count"] == 1
        errors << { "stage" => "#{model}.stream", "category" => "incomplete_sse" } unless stream["complete"]
      end
      primary_failed = model_result["status"] != 200 || text_models.empty? || results.any? { |result| result["status"] != 200 }
      status = if primary_failed
                 "failed"
               elsif errors.empty?
                 "passed"
               else
                 "partial"
               end
      status = "partial" if status == "passed" && errors.any?

      {
        "schema_version" => 1,
        "run_id" => SecureRandom.uuid,
        "channel_id" => channel_id,
        "profile_id" => @profile["id"],
        "recorded_at" => Time.now.utc.iso8601,
        "status" => status,
        "evidence_source" => "live_direct",
        "metrics" => {
          "catalog" => catalog,
          "text_models" => text_models,
          "per_model" => per_model,
          "capacity" => capacity,
          "usage" => aggregate_usage(results)
        },
        "errors" => errors
      }
    end

    private

    def invoke(model, stream)
      @client.generate(
        model: model,
        prompt: @profile["prompt"],
        max_output_tokens: @profile["max_output_tokens"],
        stream: stream
      )
    rescue Timeout::Error
      { "status" => 0, "duration_ms" => nil, "error" => "timeout", "stream_complete" => false }
    rescue StandardError => error
      { "status" => 0, "duration_ms" => nil, "error" => error.class.name, "stream_complete" => false }
    end

    def summarize(results)
      successes = results.count { |result| result["status"] == 200 }
      durations = results.map { |result| result["duration_ms"] }.compact
      first_events = results.map { |result| result["first_event_ms"] }.compact
      {
        "request_count" => results.length,
        "success_count" => successes,
        "success_rate" => results.empty? ? 0.0 : successes.to_f / results.length,
        "latency" => UpstreamBenchmark::Metrics.latency(durations),
        "ttft" => UpstreamBenchmark::Metrics.latency(first_events),
        "first_event_ms" => first_events.min
      }.compact
    end

    def aggregate_usage(results)
      usage = {
        "input_tokens" => 0,
        "output_tokens" => 0,
        "prompt_tokens" => 0,
        "completion_tokens" => 0,
        "total_tokens" => 0
      }
      results.each do |result|
        current = result["usage"] || {}
        input = current["input_tokens"] || current["prompt_tokens"]
        output = current["output_tokens"] || current["completion_tokens"]
        usage["input_tokens"] += input.to_i
        usage["output_tokens"] += output.to_i
        usage["prompt_tokens"] += input.to_i
        usage["completion_tokens"] += output.to_i
        usage["total_tokens"] += current["total_tokens"].to_i
      end
      usage
    end

    def error_for(result)
      status = result["status"].to_i
      return "rate_limited" if status == 429
      return "upstream_http" if status >= 500
      return result["error"] if result["error"]

      status >= 400 ? "request_rejected" : "transport_error"
    end
  end

  class CandidateWatchRunner < Runner
	def run(channel_id:)
	  model_result = @client.models
	  catalog = ModelCatalog.discover(model_result["models"])
	  text_models = catalog.values.select { |item| item["testable"] }.map { |item| item["id"] }
	  configured = @profile.representative_models
	  probed_models = if configured.empty?
					  text_models.first(1)
					else
					  configured.select { |model| text_models.include?(model) }
					end
	  per_model = {}
	  results = []
	  probed_models.each do |model|
		sync = invoke(model, false)
		stream = invoke(model, true)
		per_model[model] = {
		  "sync" => summarize([sync]),
		  "stream" => summarize([stream]).merge("complete" => stream["stream_complete"] == true),
		  "usage" => aggregate_usage([sync, stream])
		}
		results.concat([sync, stream])
	  end

	  errors = []
	  errors << { "stage" => "models", "category" => error_for(model_result) } unless model_result["status"] == 200
	  errors << { "stage" => "representative_models", "category" => "no_matching_text_model" } if probed_models.empty?
	  probed_models.each do |model|
		sync = per_model.fetch(model).fetch("sync")
		stream = per_model.fetch(model).fetch("stream")
		errors << { "stage" => "#{model}.sync", "category" => "request_failed" } unless sync["success_count"] == 1
		errors << { "stage" => "#{model}.stream", "category" => "request_failed" } unless stream["success_count"] == 1
		errors << { "stage" => "#{model}.stream", "category" => "incomplete_sse" } unless stream["complete"]
	  end
	  failed = model_result["status"] != 200 || probed_models.empty? || results.any? { |result| result["status"] != 200 }
	  status = failed ? "failed" : (errors.empty? ? "passed" : "partial")

	  {
		"schema_version" => 1,
		"run_id" => SecureRandom.uuid,
		"channel_id" => channel_id,
		"profile_id" => @profile["id"],
		"recorded_at" => Time.now.utc.iso8601,
		"status" => status,
		"evidence_source" => "candidate_watch",
		"metrics" => {
		  "catalog" => catalog,
		  "text_models" => text_models,
		  "probed_models" => probed_models,
		  "per_model" => per_model,
		  "usage" => aggregate_usage(results)
		},
		"errors" => errors
	  }
	end
  end

  class FastRunner < Runner
    JOB_KINDS = %w[health_pulse catalog_quick capacity_check].freeze

    def initialize(client:, profile:, job_kind:, candidate_models: nil, attempts_per_mode: 1, clock: nil, sleeper: nil)
      super(client: client, profile: profile, clock: clock, sleeper: sleeper)
      unless JOB_KINDS.include?(job_kind)
        raise UpstreamBenchmark::ValidationError, "fast job must be one of: #{JOB_KINDS.join(', ')}"
      end
      unless attempts_per_mode.is_a?(Integer) && attempts_per_mode.between?(1, 5)
        raise UpstreamBenchmark::ValidationError, "attempts per mode must be between 1 and 5"
      end
      if job_kind == "catalog_quick"
        unless candidate_models.is_a?(Array) && !candidate_models.empty?
          raise UpstreamBenchmark::ValidationError, "candidate models are required for catalog_quick"
        end
        unless candidate_models.length <= 256 && candidate_models.all? { |model| valid_candidate_model?(model) }
          raise UpstreamBenchmark::ValidationError, "candidate models are invalid"
        end
      end
      @job_kind = job_kind
      @candidate_models = candidate_models&.uniq&.sort
      @attempts_per_mode = attempts_per_mode
      @fast_client = client
      @fast_profile = profile
      @fast_clock = clock
      @fast_sleeper = sleeper
      @all_results = []
    end

    def run(channel_id:)
      model_result = @fast_client.models
      catalog = ModelCatalog.discover(model_result["models"])
      text_models = catalog.values.select { |item| item["testable"] }.map { |item| item["id"] }.sort
      selected_models, missing_roles, missing_candidates = select_models(text_models)
      unrelated_models = @job_kind == "catalog_quick" ? text_models - @candidate_models : []
      per_model = {}
      outcomes = {}

      selected_models.each do |model|
        sync = Array.new(@attempts_per_mode) { fast_invoke(model, false) }
        stream = Array.new(@attempts_per_mode) { fast_invoke(model, true) }
        outcomes[model] = { "sync" => sync, "stream" => stream }
        per_model[model] = {
          "sync" => summarize(sync),
          "stream" => summarize(stream).merge("complete" => stream.all? { |item| item["stream_complete"] == true }),
          "usage" => aggregate_usage(sync + stream)
        }
      end

      capacity = capacity_for(selected_models)
      errors = []
      errors << { "stage" => "models", "category" => error_for(model_result) } unless model_result["status"] == 200
      missing_roles.each do |role|
        errors << { "stage" => "representative_role.#{role}", "category" => "model_unavailable" }
      end
      missing_candidates.each do |model|
        errors << { "stage" => "candidate_model.#{model}", "category" => "candidate_not_discovered" }
      end
      per_model.each do |model, evidence|
        append_attempt_failures(errors, outcomes.dig(model, "sync"), "#{model}.sync")
        append_attempt_failures(errors, outcomes.dig(model, "stream"), "#{model}.stream")
        errors << { "stage" => "#{model}.stream", "category" => "incomplete_sse" } unless evidence.dig("stream", "complete")
      end
      failed = model_result["status"] != 200 || selected_models.empty? || !missing_roles.empty? ||
               !missing_candidates.empty? || !errors.empty?

      {
        "schema_version" => 1,
        "run_id" => SecureRandom.uuid,
        "channel_id" => channel_id,
        "profile_id" => @fast_profile["id"],
        "job_kind" => @job_kind,
        "recorded_at" => Time.now.utc.iso8601,
        "status" => failed ? "failed" : "passed",
        "evidence_source" => "live_direct",
        "metrics" => {
          "catalog" => catalog,
          "text_models" => text_models,
          "selected_models" => selected_models,
          "candidate_models" => (@candidate_models if @job_kind == "catalog_quick"),
          "unrelated_models_skipped" => (unrelated_models if @job_kind == "catalog_quick"),
          "representative_roles" => @fast_profile.representative_roles,
          "per_model" => per_model,
          "capacity" => capacity,
          "direct" => summarize(@all_results),
          "gateway" => { "status" => "unknown", "reason" => "not_measured" },
          "usage" => aggregate_usage(@all_results)
        }.compact,
        "errors" => errors
      }
    end

    private

    def select_models(text_models)
      if @job_kind == "catalog_quick"
        missing = @candidate_models - text_models
        return [missing.empty? ? @candidate_models : [], [], missing]
      end

      roles = @fast_profile.representative_roles
      missing = roles.select { |_role, model| !text_models.include?(model) }.keys.sort
      [roles.values.select { |model| text_models.include?(model) }, missing, []]
    end

    def valid_candidate_model?(model)
      model.is_a?(String) && model.match?(/\A[a-z0-9][a-z0-9._-]{0,127}\z/)
    end

    def fast_invoke(model, stream)
      result = invoke(model, stream)
      @all_results << result
      result
    end

    def failure_record(result, stage)
      {
        "stage" => stage,
        "category" => error_for(result || {}),
        "http_status" => (result || {})["status"].to_i
      }
    end

    def append_attempt_failures(errors, results, stage)
      results.each_with_index do |result, index|
        next if result["status"].to_i.between?(200, 299)

        attempt_stage = results.length == 1 ? stage : "#{stage}.attempt_#{index + 1}"
        errors << failure_record(result, attempt_stage)
      end
    end

    def capacity_for(models)
      return nil unless @job_kind == "capacity_check"

      models.each_with_object({}) do |model, evidence|
        evidence[model] = CapacityProbe.new(
          invoke: -> { fast_invoke(model, false) },
          profile: @fast_profile,
          clock: @fast_clock,
          sleeper: @fast_sleeper
        ).run
      end
    end
  end

  class QualityFirstEvaluator
    QUALITY_THRESHOLD = 80
    FRESHNESS_SECONDS = {
      "health_pulse" => 30 * 60,
      "catalog_quick" => 12 * 60 * 60,
      "capacity_check" => 36 * 60 * 60,
      "incident_recheck" => 30 * 60
    }.freeze

    def initialize(record:, baseline:, pricing:, now: Time.now.utc)
      @record = record
      @baseline = baseline
      @pricing = pricing || {}
      @now = now.utc
    end

    def evaluate
      reasons = hard_gate_reasons
      scores = component_scores
      quality_score = scores.values_at("reliability", "latency", "generation", "capacity").sum
      total_score = quality_score + scores.fetch("price")
      missing = missing_evidence
      improvement = material_improvement?
      status = if !reasons.empty?
                 "blocked"
               elsif quality_score < QUALITY_THRESHOLD
                 "not_better"
               elsif !missing.empty?
                 "needs_evidence"
               elsif !improvement
                 "not_better"
               else
                 "eligible_for_manual_switch"
               end
      {
        "status" => status,
        "eligible" => status == "eligible_for_manual_switch",
        "hard_gate_reasons" => reasons,
        "missing_evidence" => missing,
        "component_scores" => scores,
        "quality_score" => quality_score,
        "total_score" => total_score,
        "material_improvement" => improvement
      }
    end

    private

    def hard_gate_reasons
      reasons = []
      direct = @record.dig("metrics", "direct") || {}
      streams = @record.dig("metrics", "per_model") || {}
      roles = @record.dig("metrics", "representative_roles") || {}
      selected = Array(@record.dig("metrics", "selected_models"))
      technical_failure = @record["status"] != "passed" || !Array(@record["errors"]).empty? ||
        direct["request_count"].to_i <= 0 || direct["success_rate"].to_f < 1.0 ||
        streams.values.any? { |item| item.dig("stream", "complete") != true } ||
        (!roles.empty? && !roles.values.all? { |model| selected.include?(model) })
      reasons << "technical_failure" if technical_failure
      reasons << "stale_evidence" unless fresh?
      reasons << "latency_regression" if latency_regression?
      reasons << "gateway_overhead" if gateway_overhead?
      reasons.sort
    end

    def component_scores
      direct = @record.dig("metrics", "direct") || {}
      technical = @record["status"] == "passed" && Array(@record["errors"]).empty? && direct["success_rate"].to_f == 1.0
      {
        "reliability" => technical ? 40 : 0,
        "latency" => latency_score,
        "generation" => @record.dig("metrics", "usage", "total_tokens").to_i.positive? ? 10 : 0,
        "capacity" => capacity_score,
        "price" => verified_pricing? ? 10 : 0
      }
    end

    def latency_score
      ttft = direct_p95("ttft")
      latency = direct_p95("latency")
      return 0 unless ttft.positive? && latency.positive?
      return 15 if @baseline.nil?
      return 0 if latency_regression?

      ttft <= @baseline["ttft_p95_ms"].to_f && latency <= @baseline["latency_p95_ms"].to_f ? 25 : 20
    end

    def capacity_score
      capacity = @record.dig("metrics", "capacity")
      return 0 unless capacity.is_a?(Hash) && !capacity.empty?

      passed = capacity.values.all? do |item|
        item.dig("concurrency", "last_stable").to_i >= 1 && item.dig("rpm", "last_stable").to_i >= 6
      end
      passed ? 15 : 0
    end

    def missing_evidence
      missing = []
      missing << "production_baseline" if @baseline.nil?
      missing << "gateway_measurement" unless @record.dig("metrics", "gateway", "status") == "measured"
      missing << "verified_pricing" unless verified_pricing?
      missing.sort
    end

    def verified_pricing?
      %w[explicit_model_price multiplier_only].include?(@pricing["mode"]) && @pricing["verified"] == true
    end

    def fresh?
      recorded = Time.iso8601(@record.fetch("recorded_at"))
      maximum = FRESHNESS_SECONDS.fetch(@record["job_kind"], 30 * 60)
      age = @now - recorded
      age >= 0 && age <= maximum
    rescue ArgumentError, KeyError
      false
    end

    def latency_regression?
      return false if @baseline.nil?

      direct_p95("ttft") > @baseline["ttft_p95_ms"].to_f * 1.05 ||
        direct_p95("latency") > @baseline["latency_p95_ms"].to_f * 1.05
    end

    def gateway_overhead?
      gateway = @record.dig("metrics", "gateway") || {}
      return false unless gateway["status"] == "measured"

      ttft_delta = metric_p95(gateway, "ttft") - direct_p95("ttft")
      latency_delta = metric_p95(gateway, "latency") - direct_p95("latency")
      ttft_delta > [500.0, direct_p95("ttft") * 0.20].max ||
        latency_delta > [2_000.0, direct_p95("latency") * 0.20].max
    end

    def material_improvement?
      return false if @baseline.nil?

      faster = direct_p95("ttft") <= @baseline["ttft_p95_ms"].to_f * 0.90 ||
        direct_p95("latency") <= @baseline["latency_p95_ms"].to_f * 0.90
      capacity = @record.dig("metrics", "capacity") || {}
      concurrency = capacity.values.map { |item| item.dig("concurrency", "last_stable").to_i }.min.to_i
      rpm = capacity.values.map { |item| item.dig("rpm", "last_stable").to_i }.min.to_i
      faster || concurrency > @baseline["concurrency"].to_i || rpm > @baseline["rpm"].to_i
    end

    def direct_p95(name)
      metric_p95(@record.dig("metrics", "direct") || {}, name)
    end

    def metric_p95(container, name)
      metric = container[name] || {}
      (metric["p95_ms"] || metric["p95"] || 0).to_f
    end
  end

  class PricingAdvisor
    def initialize(evidence:, scenario:)
      @evidence = evidence
      @scenario = scenario
    end

    def calculate
      PricingEvidence.validate!(@evidence)
      validate_scenario!
      models = build_models
      openable = models.select { |_id, item| item["status"] == "verified" }.keys
      multipliers = models.values.select { |item| item["status"] == "verified" }.map { |item| item["cost_multiplier"] }
      account_multiplier = multipliers.max
      raise UpstreamBenchmark::ValidationError, "pricing evidence has no verified model multiplier" if account_multiplier.nil?

      reserve = decimal(@scenario.fetch("failure_reserve_rate"))
      margin = decimal(@scenario.fetch("target_margin_rate"))
      fee = decimal(@scenario.fetch("payment_fee_rate"))
      denominator = decimal(1) - fee - margin
      risk_adjusted = decimal(account_multiplier) * (decimal(1) + reserve)
      fixed_per_standard_dollar = decimal(@scenario.fetch("monthly_fixed_cost_usd")) / decimal(@scenario.fetch("monthly_standard_usage_usd"))
      variable_floor = risk_adjusted / (decimal(1) - margin)
      payment_adjusted_floor = risk_adjusted / denominator
      full_cost_floor = (risk_adjusted + fixed_per_standard_dollar) / denominator
      recommended = ceil_to_increment(full_cost_floor + decimal(@scenario.fetch("recommendation_buffer")), decimal(@scenario.fetch("recommendation_increment")))

      {
        "channel_id" => @evidence.fetch("channel_id"),
        "models" => models,
        "openable_models" => openable,
        "account_cost_multiplier" => number(account_multiplier),
        "internal" => { "group_multiplier" => @scenario.fetch("internal_group_multiplier") },
        "commercial" => {
          "failure_reserve_rate" => @scenario.fetch("failure_reserve_rate"),
          "target_margin_rate" => @scenario.fetch("target_margin_rate"),
          "payment_fee_rate" => @scenario.fetch("payment_fee_rate"),
          "risk_adjusted_cost_multiplier" => number(risk_adjusted),
          "fixed_cost_per_standard_dollar" => number(fixed_per_standard_dollar),
          "variable_floor" => number(variable_floor),
          "payment_adjusted_floor" => number(payment_adjusted_floor),
          "full_cost_floor" => number(full_cost_floor),
          "recommended_multiplier" => number(recommended)
        }
      }
    end

    private

    def build_models
      @evidence.fetch("models").each_with_object({}) do |(model_id, prices), result|
        multiplier = prices["actual_multiplier"]
        price_ready = prices["input"].is_a?(Numeric) && prices["output"].is_a?(Numeric)
        billing_ready = multiplier.is_a?(Numeric) && prices["billing_reconciliation"] == "verified"
        status = price_ready && billing_ready ? "verified" : "unknown"
        result[model_id] = {
          "status" => status,
          "openable" => status == "verified",
          "cost_multiplier" => multiplier,
          "prices" => {
            "input" => prices["input"],
            "output" => prices["output"],
            "cache_read" => prices["cache_read"],
            "cache_write" => prices["cache_write"]
          },
          "reason" => status == "verified" ? nil : "price_or_billing_evidence_unknown"
        }.compact
      end
    end

    def validate_scenario!
      Scenario.validate!(@scenario)
    end

    def decimal(value)
      BigDecimal(value.to_s)
    end

    def number(value)
      value.to_f
    end

    def ceil_to_increment(value, increment)
      (value / increment).ceil * increment
    end
  end

  module Scenario
    module_function

    REQUIRED = %w[failure_reserve_rate target_margin_rate payment_fee_rate recommendation_increment recommendation_buffer monthly_fixed_cost_usd monthly_standard_usage_usd internal_group_multiplier].freeze

    def validate!(document)
      raise UpstreamBenchmark::ValidationError, "pricing scenario must be a mapping" unless document.is_a?(Hash)
      REQUIRED.each { |key| raise UpstreamBenchmark::ValidationError, "pricing scenario.#{key} is required" unless document.key?(key) }
      REQUIRED.each do |key|
        value = document[key]
        unless value.is_a?(Numeric) && value.finite? && value >= 0
          raise UpstreamBenchmark::ValidationError, "pricing scenario.#{key} must be a finite non-negative number"
        end
      end
      if document["target_margin_rate"] + document["payment_fee_rate"] >= 1
        raise UpstreamBenchmark::ValidationError, "pricing scenario margin plus payment fee must be less than 1"
      end
      unless document["monthly_standard_usage_usd"].positive?
        raise UpstreamBenchmark::ValidationError, "pricing scenario monthly_standard_usage_usd must be positive"
      end
      true
    end
  end

  module ProposalBuilder
    module_function

    def build(run:, pricing:, proposal_id:, generated_at:)
      capacity = run.dig("metrics", "capacity") || {}
      models = pricing.fetch("openable_models").map do |model_id|
        item = pricing.fetch("models").fetch(model_id)
        {
          "public_name" => model_id,
          "upstream_name" => model_id,
          "status" => item.fetch("status"),
          "prices" => item.fetch("prices")
        }
      end
      proposal = {
        "schema_version" => 1,
        "proposal_id" => proposal_id,
        "generated_at" => generated_at,
        "channel_id" => run.fetch("channel_id"),
        "source_run_id" => run.fetch("run_id"),
        "models" => models,
        "pricing" => {
          "account_cost_multiplier" => pricing.fetch("account_cost_multiplier"),
          "internal_group_multiplier" => pricing.dig("internal", "group_multiplier"),
          "commercial_group_multiplier" => pricing.dig("commercial", "recommended_multiplier")
        },
        "capacity" => {
          "concurrency" => capacity.dig("recommendation", "concurrency"),
          "rpm" => capacity.dig("recommendation", "rpm")
        },
        "sub2api" => {
          "billing_model_source" => "requested",
          "restrict_models" => true,
          "model_mapping" => models.each_with_object({}) { |model, mapping| mapping[model["public_name"]] = model["upstream_name"] },
          "model_pricing" => models.map do |model|
            {
              "models" => [model["public_name"]],
              "input_price" => model.dig("prices", "input"),
              "output_price" => model.dig("prices", "output"),
              "cache_read_price" => model.dig("prices", "cache_read"),
              "cache_write_price" => model.dig("prices", "cache_write")
            }
          end
        }
      }
      proposal["proposal_hash"] = Digest::SHA256.hexdigest(JSON.generate(proposal))
      proposal
    end

    def markdown(proposal)
      lines = [
        "# Upstream Proposal #{proposal.fetch('proposal_id')}",
        "",
        "- Channel: `#{proposal.fetch('channel_id')}`",
        "- Source run: `#{proposal.fetch('source_run_id')}`",
        "- Proposal hash: `#{proposal.fetch('proposal_hash')}`",
        "- Account cost multiplier: `#{proposal.dig('pricing', 'account_cost_multiplier')}`",
        "- Internal group multiplier: `#{proposal.dig('pricing', 'internal_group_multiplier')}`",
        "- Commercial group multiplier: `#{proposal.dig('pricing', 'commercial_group_multiplier')}`",
        "- Recommended concurrency: `#{proposal.dig('capacity', 'concurrency')}`",
        "- Recommended RPM: `#{proposal.dig('capacity', 'rpm')}`",
        "",
        "| Model | Input | Output | Cache read | Cache write |",
        "|---|---:|---:|---:|---:|"
      ]
      proposal.fetch("models").each do |model|
        prices = model.fetch("prices")
        lines << "| #{model.fetch('public_name')} | #{prices['input']} | #{prices['output']} | #{prices['cache_read']} | #{prices['cache_write']} |"
      end
      lines.join("\n") + "\n"
    end
  end

  class CLI
    ROOT = File.expand_path("..", __dir__).freeze
    COMMANDS = %w[validate discover run watch fast advise proposal capacity-dry-run topology-dry-run].freeze
    DEFAULTS = {
      channels: File.join(ROOT, "config/upstream-benchmarks/channels.yaml"),
      profile: File.join(ROOT, "config/upstream-benchmarks/mvp-text-v2.yaml"),
      pricing: File.join(ROOT, "config/upstream-benchmarks/pricing-evidence.example.yaml"),
      scenario: File.join(ROOT, "config/upstream-benchmarks/v2-scenario-neko.example.yaml"),
      runs: File.join(ROOT, "config/upstream-benchmarks/ledger/runs.jsonl"),
      decisions: File.join(ROOT, "config/upstream-benchmarks/ledger/decisions.jsonl")
    }.freeze
    V3_PROFILE = File.join(ROOT, "config/upstream-benchmarks/bounded-text-capacity-v3.example.yaml").freeze
    V3_TOPOLOGY = File.join(ROOT, "config/upstream-benchmarks/topology-scenario-v3.example.yaml").freeze

    def self.run(argv, out: $stdout, err: $stderr, env: ENV, client_factory: nil)
      command = argv.shift
      unless COMMANDS.include?(command)
        raise UpstreamBenchmark::ValidationError, "command must be one of: #{COMMANDS.join(', ')}"
      end
      options = DEFAULTS.dup
      if command == "capacity-dry-run"
        options[:profile] = V3_PROFILE
        options[:include_discovery] = true
      end
      options[:scenario] = V3_TOPOLOGY if command == "topology-dry-run"
      option_parser(command, options).parse!(argv)
      raise UpstreamBenchmark::ValidationError, "unexpected arguments: #{argv.join(' ')}" unless argv.empty?

      case command
      when "validate" then validate_command(options, out)
      when "discover" then discover_command(options, out, env, client_factory: client_factory)
      when "run" then run_command(options, out, env)
	  when "watch" then watch_command(options, out, env)
      when "fast" then fast_command(options, out, env, client_factory: client_factory)
      when "advise" then advise_command(options, out)
      when "proposal" then proposal_command(options, out)
      when "capacity-dry-run" then capacity_dry_run_command(options, out)
      when "topology-dry-run" then topology_dry_run_command(options, out)
      end
      0
    rescue UpstreamBenchmark::ValidationError, OptionParser::ParseError, Errno::ENOENT, Psych::Exception => error
      err.puts("ERROR: #{error.message}")
      2
    end

    def self.option_parser(command, options)
      OptionParser.new do |parser|
        parser.banner = "Usage: upstream-benchmark-v2.rb #{command} [options]"
        parser.on("--channels PATH") { |value| options[:channels] = value }
        parser.on("--profile PATH") { |value| options[:profile] = value }
        parser.on("--pricing PATH") { |value| options[:pricing] = value }
        parser.on("--scenario PATH") { |value| options[:scenario] = value }
        parser.on("--runs PATH") { |value| options[:runs] = value }
        parser.on("--decisions PATH") { |value| options[:decisions] = value }
        parser.on("--channel ID") { |value| options[:channel] = value }
        parser.on("--job KIND") { |value| options[:job] = value }
        parser.on("--key-env NAME") { |value| options[:key_env] = value }
        parser.on("--run PATH") { |value| options[:run] = value }
        parser.on("--output PATH") { |value| options[:output] = value }
        parser.on("--format FORMAT") { |value| options[:format] = value }
        parser.on("--model-count N", Integer) { |value| options[:model_count] = value }
        parser.on("--models PATH") { |value| options[:models] = value }
        parser.on("--attempts-per-mode N", Integer) { |value| options[:attempts_per_mode] = value }
        parser.on("--include-discovery") { options[:include_discovery] = true }
        parser.on("--exclude-discovery") { options[:include_discovery] = false }
        parser.on("--topology-verification-requests N", Integer) { |value| options[:topology_verification_requests] = value }
        parser.on("--evidence PATH") { |value| options[:evidence] = value }
        parser.on("--dry-run") { options[:dry_run] = true }
      end
    end

    def self.validate_command(options, out)
      profile = UpstreamBenchmarkV2::Profile.new(load_yaml(options[:profile]))
      UpstreamBenchmarkV2::PricingEvidence.validate!(load_yaml(options[:pricing]))
      UpstreamBenchmarkV2::Scenario.validate!(load_yaml(options[:scenario]))
      out.puts("valid: profile #{profile['id']}, pricing evidence and scenario")
    end

    def self.discover_command(options, out, env, client_factory: nil)
      raise UpstreamBenchmark::ValidationError, "--channel is required" unless options[:channel]
      registry = UpstreamBenchmark::Registry.new(load_yaml(options[:channels]))
      profile = UpstreamBenchmarkV2::Profile.new(load_yaml(options[:profile]))
      channel = registry.fetch(options[:channel])
      if options[:dry_run]
        out.puts(JSON.pretty_generate(
          "channel_id" => channel["id"],
          "profile_id" => profile["id"],
          "protocol" => profile.protocol,
          "models_path" => profile.models_path,
          "request_estimate" => {
            "model_directory_requests" => 1,
            "generation_requests" => 0
          },
          "requests_sent" => 0,
          "network_sent" => false
        ))
        return
      end

      raise UpstreamBenchmark::ValidationError, "--key-env is required" unless options[:key_env]
      key = env[options[:key_env]]
      raise UpstreamBenchmark::ValidationError, "upstream key environment variable is empty" if key.to_s.empty?

      adapter = UpstreamBenchmark::Protocols.build(profile.protocol, terminal_events: profile.terminal_events)
      factory = client_factory || lambda { |**arguments| UpstreamBenchmark::HttpClient.new(**arguments) }
      client = factory.call(
        base_url: channel.fetch("base_url"),
        api_key: key,
        timeout_seconds: profile["timeout_seconds"],
        adapter: adapter,
        models_path: profile.models_path,
        generate_path: profile.generate_path
      )
      record = UpstreamBenchmarkV2::DiscoveryRunner.new(client: client, profile: profile).run(channel_id: channel.fetch("id"))
      UpstreamBenchmark::Ledger.new(options[:runs], options[:decisions]).append_run(record)
      out.puts(JSON.pretty_generate(UpstreamBenchmark::Redactor.clean(record)))
    end

    def self.run_command(options, out, env)
      raise UpstreamBenchmark::ValidationError, "--channel is required" unless options[:channel]
      registry = UpstreamBenchmark::Registry.new(load_yaml(options[:channels]))
      profile = UpstreamBenchmarkV2::Profile.new(load_yaml(options[:profile]))
      channel = registry.fetch(options[:channel])
      if options[:dry_run]
        out.puts(JSON.pretty_generate(
          "channel_id" => channel["id"],
          "profile_id" => profile["id"],
          "protocol" => profile.protocol,
          "models_path" => profile.models_path,
          "generate_path" => profile.generate_path,
          "model_discovery" => "live_required",
          "request_estimate" => {
            "per_text_model" => 2,
            "concurrency_levels" => profile.concurrency_levels,
            "rpm_levels" => profile.rpm_levels,
            "rpm_window_seconds" => profile.rpm_window_seconds
          },
          "capacity_probe_bounded" => true,
          "requests_sent" => 0,
          "network_sent" => false
        ))
        return
      end
      raise UpstreamBenchmark::ValidationError, "--key-env is required" unless options[:key_env]
      adapter = UpstreamBenchmark::Protocols.build(
        profile.protocol,
        terminal_events: profile.terminal_events
      )
      client = UpstreamBenchmark::HttpClient.new(
        base_url: channel.fetch("base_url"),
        api_key: env[options[:key_env]],
        timeout_seconds: profile["timeout_seconds"],
        adapter: adapter,
        models_path: profile.models_path,
        generate_path: profile.generate_path
      )
      record = UpstreamBenchmarkV2::Runner.new(client: client, profile: profile).run(channel_id: channel.fetch("id"))
      UpstreamBenchmark::Ledger.new(options[:runs], options[:decisions]).append_run(record)
      out.puts(JSON.pretty_generate(UpstreamBenchmark::Redactor.clean(record)))
    end

	def self.watch_command(options, out, env)
	  raise UpstreamBenchmark::ValidationError, "--channel is required" unless options[:channel]
	  registry = UpstreamBenchmark::Registry.new(load_yaml(options[:channels]))
	  profile = UpstreamBenchmarkV2::Profile.new(load_yaml(options[:profile]))
	  channel = registry.fetch(options[:channel])
	  if options[:dry_run]
		out.puts(JSON.pretty_generate(
		  "channel_id" => channel["id"],
		  "profile_id" => profile["id"],
		  "protocol" => profile.protocol,
		  "models_path" => profile.models_path,
		  "generate_path" => profile.generate_path,
		  "model_discovery" => "live_required",
		  "representative_models" => profile.representative_models,
		  "request_estimate" => {
			"model_discovery_requests" => 1,
			"maximum_generate_requests" => [profile.representative_models.length, 1].max * 2
		  },
		  "requests_sent" => 0,
		  "network_sent" => false
		))
		return
	  end
	  raise UpstreamBenchmark::ValidationError, "--key-env is required" unless options[:key_env]
	  key = env[options[:key_env]]
	  raise UpstreamBenchmark::ValidationError, "candidate key environment variable is empty" if key.to_s.empty?
	  adapter = UpstreamBenchmark::Protocols.build(
		profile.protocol,
		terminal_events: profile.terminal_events
	  )
	  client = UpstreamBenchmark::HttpClient.new(
		base_url: channel.fetch("base_url"),
		api_key: key,
		timeout_seconds: profile["timeout_seconds"],
		adapter: adapter,
		models_path: profile.models_path,
		generate_path: profile.generate_path
	  )
	  record = UpstreamBenchmarkV2::CandidateWatchRunner.new(client: client, profile: profile).run(channel_id: channel.fetch("id"))
	  out.puts(JSON.pretty_generate(UpstreamBenchmark::Redactor.clean(record)))
	end

    def self.fast_command(options, out, env, client_factory: nil)
      raise UpstreamBenchmark::ValidationError, "--channel is required" unless options[:channel]
      raise UpstreamBenchmark::ValidationError, "--job is required" unless options[:job]
      unless UpstreamBenchmarkV2::FastRunner::JOB_KINDS.include?(options[:job])
        raise UpstreamBenchmark::ValidationError,
              "fast job must be one of: #{UpstreamBenchmarkV2::FastRunner::JOB_KINDS.join(', ')}"
      end

      registry = UpstreamBenchmark::Registry.new(load_yaml(options[:channels]))
      profile = UpstreamBenchmarkV2::Profile.new(load_yaml(options[:profile]))
      channel = registry.fetch(options[:channel])
      roles = profile.representative_roles.length
      candidate_models = nil
      attempts_per_mode = options.fetch(:attempts_per_mode, options[:job] == "catalog_quick" ? 3 : 1)
      if options[:job] == "catalog_quick"
        raise UpstreamBenchmark::ValidationError, "--models is required for catalog_quick" unless options[:models]

        candidate_models = load_candidate_models(options[:models])
      elsif options[:models]
        raise UpstreamBenchmark::ValidationError, "--models is only valid for catalog_quick"
      end
      if options[:dry_run]
        generation_requests = fast_request_estimate(
          profile, options[:job], candidate_count: candidate_models&.length,
          attempts_per_mode: attempts_per_mode
        )
        out.puts(JSON.pretty_generate(
          "channel_id" => channel["id"],
          "profile_id" => profile["id"],
          "job_kind" => options[:job],
          "protocol" => profile.protocol,
          "representative_roles" => profile.representative_roles,
          "candidate_models" => candidate_models,
          "attempts_per_mode" => (attempts_per_mode if options[:job] == "catalog_quick"),
          "request_estimate" => {
            "model_directory_requests" => 1,
            "maximum_generation_requests" => generation_requests,
            "maximum_http_requests" => generation_requests.nil? ? nil : generation_requests + 1,
            "per_candidate_model" => options[:job] == "catalog_quick" ? attempts_per_mode * 2 : nil,
            "representative_model_count" => roles
          }.compact,
          "requests_sent" => 0,
          "network_sent" => false
        ))
        return
      end

      raise UpstreamBenchmark::ValidationError, "--key-env is required" unless options[:key_env]
      key = env[options[:key_env]]
      raise UpstreamBenchmark::ValidationError, "upstream key environment variable is empty" if key.to_s.empty?

      adapter = UpstreamBenchmark::Protocols.build(profile.protocol, terminal_events: profile.terminal_events)
      factory = client_factory || lambda { |**arguments| UpstreamBenchmark::HttpClient.new(**arguments) }
      client = factory.call(
        base_url: channel.fetch("base_url"), api_key: key,
        timeout_seconds: profile["timeout_seconds"], adapter: adapter,
        models_path: profile.models_path, generate_path: profile.generate_path
      )
      record = UpstreamBenchmarkV2::FastRunner.new(
        client: client, profile: profile, job_kind: options[:job],
        candidate_models: candidate_models, attempts_per_mode: attempts_per_mode
      ).run(channel_id: channel.fetch("id"))
      UpstreamBenchmark::Ledger.new(options[:runs], options[:decisions]).append_run(record)
      out.puts(JSON.pretty_generate(UpstreamBenchmark::Redactor.clean(record)))
    end

    def self.fast_request_estimate(profile, job_kind, candidate_count: nil, attempts_per_mode: 1)
      roles = profile.representative_roles.length
      return roles * 2 if job_kind == "health_pulse"
      return candidate_count * attempts_per_mode * 2 if job_kind == "catalog_quick"

      rpm_requests = profile.rpm_levels.sum do |rpm|
        [(rpm * profile.rpm_window_seconds / 60.0).ceil, 1].max
      end
      roles * (2 + profile.concurrency_levels.sum + rpm_requests)
    end

    def self.load_candidate_models(path)
      raise UpstreamBenchmark::ValidationError, "candidate model file exceeds 64 KiB" if File.size(path) > 64 * 1024

      document = JSON.parse(File.read(path))
      unless document.is_a?(Hash) && document.keys.sort == %w[models schema_version] && document["schema_version"] == 1
        raise UpstreamBenchmark::ValidationError, "candidate model file schema is invalid"
      end
      models = document["models"]
      valid = models.is_a?(Array) && models.length.between?(1, 256)
      valid &&= models.all? { |model| model.is_a?(String) && model.match?(/\A[a-z0-9][a-z0-9._-]{0,127}\z/) }
      valid &&= models.uniq.length == models.length
      raise UpstreamBenchmark::ValidationError, "candidate model list is invalid" unless valid

      models.sort
    rescue JSON::ParserError => error
      raise UpstreamBenchmark::ValidationError, "candidate model file JSON is invalid: #{error.message}"
    end

    def self.advise_command(options, out)
      pricing = UpstreamBenchmarkV2::PricingAdvisor.new(
        evidence: load_yaml(options[:pricing]),
        scenario: load_yaml(options[:scenario])
      ).calculate
      run = load_data(options.fetch(:run))
      if options[:format] == "markdown"
        out.puts("# Upstream Pricing Advice")
        out.puts
        out.puts("- Channel: `#{run.fetch('channel_id')}`")
        out.puts("- Openable models: `#{pricing.fetch('openable_models').join(', ')}`")
        out.puts("- Internal multiplier: `#{pricing.dig('internal', 'group_multiplier')}`")
        out.puts("- Recommended commercial multiplier: `#{pricing.dig('commercial', 'recommended_multiplier')}`")
      else
        out.puts(JSON.pretty_generate(pricing))
      end
    end

    def self.proposal_command(options, out)
      raise UpstreamBenchmark::ValidationError, "--output is required" unless options[:output]
      pricing = UpstreamBenchmarkV2::PricingAdvisor.new(
        evidence: load_yaml(options[:pricing]),
        scenario: load_yaml(options[:scenario])
      ).calculate
      run = load_data(options.fetch(:run))
      proposal = UpstreamBenchmarkV2::ProposalBuilder.build(
        run: run,
        pricing: pricing,
        proposal_id: "proposal-#{Time.now.utc.strftime('%Y%m%d%H%M%S')}",
        generated_at: Time.now.utc.iso8601
      )
      FileUtils.mkdir_p(File.dirname(options[:output]))
      if File.extname(options[:output]) == ".yaml"
        File.write(options[:output], YAML.dump(proposal))
      else
        File.write(options[:output], JSON.pretty_generate(proposal) + "\n")
      end
      out.puts("wrote proposal #{proposal['proposal_id']} #{proposal['proposal_hash']}")
    end

    def self.capacity_dry_run_command(options, out)
      raise UpstreamBenchmark::ValidationError, "--model-count is required" if options[:model_count].nil?

      profile = UpstreamBenchmarkNonfunctional::Profile.new(load_yaml(options[:profile]))
      result = UpstreamBenchmarkNonfunctional::RequestBudget.new(profile: profile).calculate(
        model_count: options[:model_count],
        include_discovery: options[:include_discovery] == true,
        topology_verification_requests: options[:topology_verification_requests] || 0
      )
      out.puts(JSON.pretty_generate(UpstreamBenchmark::Redactor.clean(result)))
    end

    def self.topology_dry_run_command(options, out)
      raise UpstreamBenchmark::ValidationError, "--evidence is required" if options[:evidence].nil?

      scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(load_yaml(options[:scenario]))
      evidence = load_data(options[:evidence])
      result = UpstreamBenchmarkNonfunctional::OfflineTopologyEvaluator.new(
        scenario: scenario,
        evidence: evidence
      ).evaluate
      out.puts(JSON.pretty_generate(UpstreamBenchmark::Redactor.clean(result)))
    end

    def self.load_yaml(path)
      YAML.safe_load(File.read(path), permitted_classes: [Date, Time], permitted_symbols: [], aliases: false, filename: path)
    end

    def self.load_data(path)
      raw = File.read(path)
      File.extname(path) == ".yaml" ? YAML.safe_load(raw, permitted_classes: [Date, Time], permitted_symbols: [], aliases: false, filename: path) : JSON.parse(raw)
    end
  end
end

if $PROGRAM_NAME == __FILE__
  exit UpstreamBenchmarkV2::CLI.run(ARGV)
end
