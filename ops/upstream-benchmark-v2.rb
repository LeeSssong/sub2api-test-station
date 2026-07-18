#!/usr/bin/env ruby
# frozen_string_literal: true

require "bigdecimal"
require "date"
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
        %w[input output source verified_at].each do |key|
          raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id}.#{key} is required" if prices[key].nil?
        end
        %w[input output cache_read cache_write].each do |key|
          next if prices[key].nil?
          unless prices[key].is_a?(Numeric) && prices[key] >= 0 && prices[key].finite?
            raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id}.#{key} must be a non-negative number"
          end
        end
        raise UpstreamBenchmark::ValidationError, "pricing evidence.#{model_id}.source must be non-empty" unless prices["source"].is_a?(String) && !prices["source"].strip.empty?
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
end

if $PROGRAM_NAME == __FILE__
  warn "V2 implementation is available through the project CLI after Task 4."
  exit 0
end
