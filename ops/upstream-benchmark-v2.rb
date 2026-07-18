#!/usr/bin/env ruby
# frozen_string_literal: true

require "bigdecimal"
require "date"
require "digest"
require "json"
require "optparse"
require "securerandom"
require "time"
require "yaml"
require_relative "upstream-benchmark"

module UpstreamBenchmarkV2
  class Profile
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

    private

    def validate!
      raise UpstreamBenchmark::ValidationError, "v2 profile must be a mapping" unless @document.is_a?(Hash)

      required = %w[schema_version id endpoint prompt max_output_tokens timeout_seconds concurrency_levels rpm_levels rpm_window_seconds]
      required.each do |key|
        raise UpstreamBenchmark::ValidationError, "v2 profile.#{key} is required" if @document[key].nil?
      end
      raise UpstreamBenchmark::ValidationError, "v2 profile schema_version must be 2" unless @document["schema_version"] == 2
      raise UpstreamBenchmark::ValidationError, "v2 profile endpoint must be chat_completions" unless @document["endpoint"] == "chat_completions"
      bounded_integer!("max_output_tokens", 1, 512)
      bounded_integer!("timeout_seconds", 1, 300)
      bounded_integer!("rpm_window_seconds", 1, 60)
      validate_levels!("concurrency_levels", 1, 10)
      validate_levels!("rpm_levels", 1, 120)
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
      @client.chat(
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
      {
        "request_count" => results.length,
        "success_count" => successes,
        "success_rate" => results.empty? ? 0.0 : successes.to_f / results.length,
        "latency" => UpstreamBenchmark::Metrics.latency(durations),
        "first_event_ms" => results.map { |result| result["first_event_ms"] }.compact.min
      }.compact
    end

    def aggregate_usage(results)
      usage = { "prompt_tokens" => 0, "completion_tokens" => 0, "total_tokens" => 0 }
      results.each do |result|
        current = result["usage"] || {}
        usage.keys.each { |key| usage[key] += current[key].to_i }
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
      required = %w[failure_reserve_rate target_margin_rate payment_fee_rate recommendation_increment recommendation_buffer monthly_fixed_cost_usd monthly_standard_usage_usd internal_group_multiplier]
      required.each { |key| raise UpstreamBenchmark::ValidationError, "pricing scenario.#{key} is required" unless @scenario.key?(key) }
      numeric_keys = required - ["internal_group_multiplier"]
      numeric_keys.each do |key|
        value = @scenario[key]
        raise UpstreamBenchmark::ValidationError, "pricing scenario.#{key} must be a finite non-negative number" unless value.is_a?(Numeric) && value.finite? && value >= 0
      end
      if @scenario["target_margin_rate"] + @scenario["payment_fee_rate"] >= 1
        raise UpstreamBenchmark::ValidationError, "pricing scenario margin plus payment fee must be less than 1"
      end
      raise UpstreamBenchmark::ValidationError, "pricing scenario monthly_standard_usage_usd must be positive" unless @scenario["monthly_standard_usage_usd"].positive?
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
end

if $PROGRAM_NAME == __FILE__
  warn "V2 implementation is available through the project CLI after Task 4."
  exit 0
end
