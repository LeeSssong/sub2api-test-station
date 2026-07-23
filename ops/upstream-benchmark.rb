#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "fileutils"
require "net/http"
require "openssl"
require "optparse"
require "securerandom"
require "time"
require "timeout"
require "uri"
require "yaml"
require_relative "upstream-benchmark-protocols"

module UpstreamBenchmark
  class ValidationError < StandardError; end
  class ResponseTooLarge < StandardError; end

  module SecretGuard
    SECRET_KEY = /(?:authorization|api_?key|access_?token|refresh_?token|auth_?token|password|passwd|cookie|secret|credential)/i.freeze
    SECRET_VALUE = /(?:Bearer\s+[A-Za-z0-9._~+\/=:-]{16,}|\bsk[-_][A-Za-z0-9_-]{16,})/i.freeze

    module_function

    def validate!(value, path = "root")
      case value
      when Hash
        value.each do |key, child|
          raise ValidationError, "#{path}.#{key}: credential fields are forbidden" if key.to_s.match?(SECRET_KEY)

          validate!(child, "#{path}.#{key}")
        end
      when Array
        value.each_with_index { |child, index| validate!(child, "#{path}[#{index}]") }
      when String
        raise ValidationError, "#{path}: value looks like a secret" if value.match?(SECRET_VALUE)
      end
    end
  end

  class Registry
    LIFECYCLES = %w[discovered candidate qualified production paused rejected].freeze
    RESALE = %w[confirmed denied unknown].freeze

    def initialize(document)
      @document = document
      validate!
      @channels = document.fetch("channels").each_with_object({}) do |channel, memo|
        memo[channel.fetch("id")] = channel
      end
    end

    def fetch(id)
      @channels.fetch(id) { raise ValidationError, "unknown channel: #{id}" }
    end

    def channels
      @channels.values
    end

    private

    def validate!
      raise ValidationError, "root must be a mapping" unless @document.is_a?(Hash)
      raise ValidationError, "schema_version must be 1" unless @document["schema_version"] == 1

      channels = @document["channels"]
      raise ValidationError, "channels must be a non-empty array" unless channels.is_a?(Array) && !channels.empty?

      ids = {}
      channels.each_with_index do |channel, index|
        path = "channels[#{index}]"
        raise ValidationError, "#{path} must be a mapping" unless channel.is_a?(Hash)

        %w[id display_name base_url protocol resale_permission lifecycle].each do |key|
          raise ValidationError, "#{path}.#{key} is required" if channel[key].nil? || channel[key].to_s.empty?
        end
        id = channel["id"]
        raise ValidationError, "#{path}.id is duplicated" if ids[id]
        ids[id] = true
        raise ValidationError, "#{path}.protocol is unsupported" unless channel["protocol"] == "openai_compatible"
        raise ValidationError, "#{path}.resale_permission is invalid" unless RESALE.include?(channel["resale_permission"])
        raise ValidationError, "#{path}.lifecycle is invalid" unless LIFECYCLES.include?(channel["lifecycle"])
        validate_url!(channel["base_url"], path)
      end
      SecretGuard.validate!(@document)
    end

    def validate_url!(raw, path)
      uri = URI.parse(raw)
      raise ValidationError, "#{path}.base_url must use https" unless uri.scheme == "https"
      raise ValidationError, "#{path}.base_url must include a host" if uri.host.nil? || uri.host.empty?
      raise ValidationError, "#{path}.base_url must not include credentials" if uri.userinfo
    rescue URI::InvalidURIError
      raise ValidationError, "#{path}.base_url must be an absolute URL"
    end
  end

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

    private

    def validate!
      raise ValidationError, "profile must be a mapping" unless @document.is_a?(Hash)
      %w[id endpoint models prompt max_output_tokens timeout_seconds repeat_count concurrency_levels].each do |key|
        raise ValidationError, "profile.#{key} is required" if @document[key].nil?
      end
      raise ValidationError, "profile endpoint must be chat_completions" unless @document["endpoint"] == "chat_completions"
      raise ValidationError, "profile models must be non-empty" unless @document["models"].is_a?(Array) && !@document["models"].empty?
      bounded_integer!("max_output_tokens", 1, 512)
      bounded_integer!("timeout_seconds", 1, 300)
      bounded_integer!("repeat_count", 1, 100)
      levels = @document["concurrency_levels"]
      unless levels.is_a?(Array) && levels.all? { |value| value.is_a?(Integer) && value.between?(1, 10) }
        raise ValidationError, "profile concurrency_levels must contain integers from 1 to 10"
      end
      SecretGuard.validate!(@document)
    end

    def bounded_integer!(key, minimum, maximum)
      value = @document[key]
      return if value.is_a?(Integer) && value.between?(minimum, maximum)

      raise ValidationError, "profile #{key} must be between #{minimum} and #{maximum}"
    end
  end

  class Ledger
    RUN_STATUSES = %w[passed partial failed].freeze
    EVIDENCE_SOURCES = %w[historical_report live_direct live_gateway live_billing live_network manual_terms].freeze

    def initialize(runs_path, decisions_path)
      @runs_path = runs_path
      @decisions_path = decisions_path
    end

    def runs
      read_jsonl(@runs_path)
    end

    def decisions
      read_jsonl(@decisions_path)
    end

    def append_run(record)
      validate_run!(record)
      append(@runs_path, record)
    end

    def append_decision(record)
      validate_decision!(record)
      append(@decisions_path, record)
    end

    def validate!
      seen = {}
      runs.each do |record|
        validate_run!(record, existing_ids: seen)
        seen[record.fetch("run_id")] = true
      end
      decisions.each { |record| validate_decision!(record) }
      true
    end

    private

    def validate_run!(record, existing_ids: nil)
      raise ValidationError, "run must be a mapping" unless record.is_a?(Hash)
      %w[schema_version run_id channel_id recorded_at status evidence_source metrics].each do |key|
        raise ValidationError, "run.#{key} is required" if record[key].nil?
      end
      raise ValidationError, "run schema_version must be 1" unless record["schema_version"] == 1
      raise ValidationError, "run status is invalid" unless RUN_STATUSES.include?(record["status"])
      unless EVIDENCE_SOURCES.include?(record["evidence_source"])
        raise ValidationError, "run evidence_source is invalid"
      end
      Time.iso8601(record["recorded_at"])
      ids = existing_ids || runs.each_with_object({}) { |item, memo| memo[item["run_id"]] = true }
      raise ValidationError, "duplicate run_id: #{record['run_id']}" if ids[record["run_id"]]
      if record["supersedes"] && !ids[record["supersedes"]]
        raise ValidationError, "supersedes references unknown run: #{record['supersedes']}"
      end
      SecretGuard.validate!(record)
    rescue ArgumentError
      raise ValidationError, "run recorded_at must be ISO 8601"
    end

    def validate_decision!(record)
      raise ValidationError, "decision must be a mapping" unless record.is_a?(Hash)
      %w[schema_version decision_id recorded_at selected_channel status rationale run_ids review_triggers].each do |key|
        raise ValidationError, "decision.#{key} is required" if record[key].nil?
      end
      Time.iso8601(record["recorded_at"])
      SecretGuard.validate!(record)
    rescue ArgumentError
      raise ValidationError, "decision recorded_at must be ISO 8601"
    end

    def read_jsonl(path)
      return [] unless File.exist?(path)

      File.readlines(path, chomp: true).reject(&:empty?).map { |line| JSON.parse(line) }
    rescue JSON::ParserError => error
      raise ValidationError, "invalid JSONL in #{path}: #{error.message}"
    end

    def append(path, record)
      FileUtils.mkdir_p(File.dirname(path))
      File.open(path, "a", 0o600) do |file|
        file.flock(File::LOCK_EX)
        file.puts(JSON.generate(record))
        file.flush
        file.fsync
      ensure
        file.flock(File::LOCK_UN)
      end
    end
  end

  module Redactor
    DROP_KEYS = /(?:authorization|api_?key|token|password|cookie|secret|credential|content|text)/i.freeze
    SAFE_METADATA_KEYS = %w[text_models].freeze

    module_function

    def clean(value)
      case value
      when Hash
        value.each_with_object({}) do |(key, child), output|
          next if key.to_s.match?(DROP_KEYS) &&
                  key.to_s !~ /tokens?\z/i &&
                  !SAFE_METADATA_KEYS.include?(key.to_s)

          output[key] = clean(child)
        end
      when Array
        value.map { |child| clean(child) }
      when String
        value.gsub(SecretGuard::SECRET_VALUE, "[REDACTED]")
      else
        value
      end
    end
  end

  module Metrics
    module_function

    def percentile(values, ratio)
      return nil if values.empty?

      sorted = values.map(&:to_f).sort
      rank = (ratio * sorted.length).ceil
      sorted[[rank - 1, 0].max]
    end

    def latency(values)
      {
        "count" => values.length,
        "p50_ms" => percentile(values, 0.50),
        "p95_ms" => percentile(values, 0.95)
      }
    end
  end

  class Runner
    def initialize(client:, profile:, clock: nil)
      @client = client
      @profile = profile
      @clock = clock || -> { Process.clock_gettime(Process::CLOCK_MONOTONIC) }
    end

    def run(channel_id:)
      errors = []
      results = []
      model_result = @client.models
      targets = @profile["models"]
      available = model_result["models"] || []
      models_present = targets.all? { |model| available.include?(model) }
      errors << error_for(model_result, "models") unless model_result["status"] == 200
      errors << { "stage" => "models", "category" => "target_model_missing" } unless models_present

      sync_results = targets.map { |model| invoke(model, false) }
      stream_results = targets.map { |model| invoke(model, true) }
      results.concat(sync_results).concat(stream_results)

      repeat_results = Array.new(@profile["repeat_count"]) { invoke(targets.first, false) }
      results.concat(repeat_results)

      concurrency = {}
      @profile.concurrency_levels.each do |level|
        started = @clock.call
        batch = parallel(level) { invoke(targets.first, false) }
        wall_ms = (@clock.call - started) * 1000.0
        results.concat(batch)
        concurrency[level.to_s] = {
          "request_count" => level,
          "success_count" => batch.count { |result| result["status"] == 200 },
          "wall_ms" => wall_ms,
          "latency" => Metrics.latency(batch.map { |result| result["duration_ms"] }.compact)
        }
      end

      results.each_with_index do |result, index|
        errors << error_for(result, "request_#{index}") unless result["status"] == 200
      end
      stream_complete = stream_results.all? { |result| result["status"] == 200 && result["stream_complete"] == true }
      errors << { "stage" => "stream", "category" => "incomplete_sse" } unless stream_complete

      primary_failed = !models_present || model_result["status"] != 200 ||
                       sync_results.any? { |result| result["status"] != 200 } ||
                       stream_results.any? { |result| result["status"] != 200 }
      status = if primary_failed
                 "failed"
               elsif errors.empty?
                 "passed"
               else
                 "partial"
               end

      {
        "schema_version" => 1,
        "run_id" => SecureRandom.uuid,
        "channel_id" => channel_id,
        "profile_id" => @profile["id"],
        "recorded_at" => Time.now.utc.iso8601,
        "status" => status,
        "evidence_source" => "live_direct",
        "metrics" => {
          "models" => {
            "http_status" => model_result["status"],
            "available_count" => available.length,
            "target_models_present" => models_present
          },
          "sync" => summarize(sync_results),
          "stream" => summarize(stream_results).merge("complete" => stream_complete),
          "repeat" => summarize(repeat_results),
          "concurrency" => concurrency,
          "usage" => aggregate_usage(results)
        },
        "errors" => errors.compact
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
      { "status" => 0, "duration_ms" => nil, "error" => "timeout" }
    rescue StandardError => error
      { "status" => 0, "duration_ms" => nil, "error" => error.class.name }
    end

    def parallel(count, &block)
      Array.new(count) { Thread.new(&block) }.map(&:value)
    end

    def summarize(results)
      successes = results.count { |result| result["status"] == 200 }
      durations = results.map { |result| result["duration_ms"] }.compact
      {
        "request_count" => results.length,
        "success_count" => successes,
        "success_rate" => results.empty? ? 0.0 : successes.to_f / results.length,
        "latency" => Metrics.latency(durations)
      }
    end

    def aggregate_usage(results)
      usage = { "prompt_tokens" => 0, "completion_tokens" => 0, "total_tokens" => 0 }
      results.each do |result|
        current = result["usage"] || {}
        usage.keys.each { |key| usage[key] += current[key].to_i }
      end
      usage
    end

    def error_for(result, stage)
      status = result["status"].to_i
      category = if status == 429
                   "rate_limited"
                 elsif status >= 500
                   "upstream_http"
                 elsif status >= 400
                   "request_rejected"
                 elsif result["error"] == "timeout"
                   "timeout"
                 else
                   result["error"] || "transport_error"
                 end
      { "stage" => stage, "category" => category, "http_status" => status }
    end
  end

  class SseParser
    DEFAULT_MAX_BYTES = 1 << 20

    attr_reader :usage

    def initialize(adapter: nil, max_bytes: DEFAULT_MAX_BYTES)
      @adapter = adapter
      @buffer = +""
      @max_bytes = max_bytes
      @received_bytes = 0
      @complete = false
      @usage = if @adapter
                 @adapter.normalize_usage({})
               else
                 { "prompt_tokens" => 0, "completion_tokens" => 0, "total_tokens" => 0 }
               end
      @event_count = 0
    end

    def feed(chunk)
      value = chunk.to_s
      @received_bytes += value.bytesize
      raise ResponseTooLarge, "upstream response exceeds #{@max_bytes} bytes" if @received_bytes > @max_bytes

      @buffer << value
      while (boundary = @buffer.match(/\r?\n\r?\n/))
        event = @buffer.slice!(0, boundary.end(0))
        parse_event(event)
      end
    end

    def finish
      parse_event(@buffer) unless @buffer.empty?
      @buffer.clear
      self
    end

    def complete?
      @complete
    end

    def summary
      { "event_count" => @event_count, "complete" => @complete, "usage" => @usage }
    end

    private

    def parse_event(event)
      data = event.lines.map do |line|
        line.sub(/\Adata:\s?/, "").strip if line.start_with?("data:")
      end.compact.join("\n")
      return if data.empty?

      @event_count += 1
      if data == "[DONE]"
        @complete = @adapter ? @adapter.terminal_event?(data) : true
        return
      end

      object = JSON.parse(data)
      @complete = true if @adapter ? @adapter.terminal_event?(object) : object["type"] == "response.completed"
      candidate = object["usage"] || object.dig("response", "usage")
      merge_usage(candidate) if candidate.is_a?(Hash)
    rescue JSON::ParserError
      nil
    end

    def merge_usage(candidate)
      source = @adapter ? @adapter.normalize_usage(candidate) : candidate
      source.each { |key, value| @usage[key] = value.to_i if key.to_s.end_with?("tokens") }
    end
  end

  class HttpClient
    MAX_RESPONSE_BYTES = 1 << 20

    def initialize(base_url:, api_key:, timeout_seconds:, adapter: nil, models_path: "/models", generate_path: "/chat/completions")
      @base_url = base_url.sub(%r{/+\z}, "")
      @api_key = api_key
      @timeout_seconds = timeout_seconds
      @adapter = adapter || Protocols.build("chat_completions", terminal_events: ["[DONE]"])
      @models_path = models_path
      @generate_path = generate_path
      raise ValidationError, "runtime API key is missing" if @api_key.nil? || @api_key.empty?
    end

    def models
      request = @adapter.models_request(path: @models_path)
      request_json(request.fetch(:method), request.fetch(:path), request.fetch(:payload)) do |object|
        { "models" => @adapter.parse_models(object) }
      end
    end

    def chat(model:, prompt:, max_output_tokens:, stream:)
      generate(model: model, prompt: prompt, max_output_tokens: max_output_tokens, stream: stream)
    end

    def generate(model:, prompt:, max_output_tokens:, stream:)
      payload = @adapter.generate_request(
        model: model,
        prompt: prompt,
        max_output_tokens: max_output_tokens,
        stream: stream
      )
      return request_stream(@generate_path, payload) if stream

      request_json(:post, @generate_path, payload) do |object|
        source = object["usage"] || object.dig("response", "usage") || {}
        { "usage" => @adapter.normalize_usage(source) }
      end
    end

    private

    def request_json(method, path, payload = nil)
      started = monotonic
      response_body = nil
      response = perform(method, path, payload) do |current|
        response_body = read_bounded_body(current)
      end
      response_body ||= read_bounded_body(response)
      result = { "status" => response.code.to_i, "duration_ms" => elapsed_ms(started) }
      unless response.code.to_i == 200
        category = begin
          @adapter.classify_error(JSON.parse(response_body))
        rescue JSON::ParserError
          "upstream_http"
        end
        return result.merge("error" => category)
      end

      object = JSON.parse(response_body)
      result.merge(block_given? ? yield(object) : {})
    rescue ResponseTooLarge
      { "status" => 0, "duration_ms" => elapsed_ms(started), "error" => "response_too_large" }
    rescue JSON::ParserError
      { "status" => 0, "duration_ms" => elapsed_ms(started), "error" => "invalid_json" }
    rescue Net::OpenTimeout, Net::ReadTimeout, Timeout::Error
      { "status" => 0, "duration_ms" => elapsed_ms(started), "error" => "timeout" }
    rescue StandardError => error
      { "status" => 0, "duration_ms" => elapsed_ms(started), "error" => transport_category(error) }
    end

    def request_stream(path, payload)
      started = monotonic
      first_event_ms = nil
      parser = SseParser.new(adapter: @adapter)
      response_status = 0
      perform(:post, path, payload) do |response|
        response_status = response.code.to_i
        if response_status == 200
          response.read_body do |chunk|
            first_event_ms ||= elapsed_ms(started)
            parser.feed(chunk)
          end
        end
      end
      parser.finish
      {
        "status" => response_status,
        "duration_ms" => elapsed_ms(started),
        "first_event_ms" => first_event_ms,
        "stream_complete" => parser.complete?,
        "usage" => parser.usage,
        "error" => response_status == 200 ? nil : "upstream_http"
      }.compact
    rescue ResponseTooLarge
      { "status" => 0, "duration_ms" => elapsed_ms(started), "error" => "response_too_large", "stream_complete" => false }
    rescue Net::OpenTimeout, Net::ReadTimeout, Timeout::Error
      { "status" => 0, "duration_ms" => elapsed_ms(started), "error" => "timeout", "stream_complete" => false }
    rescue StandardError => error
      { "status" => 0, "duration_ms" => elapsed_ms(started), "error" => transport_category(error), "stream_complete" => false }
    end

    def perform(method, path, payload = nil, &block)
      uri = URI.parse("#{@base_url}#{path}")
      request = method == :get ? Net::HTTP::Get.new(uri) : Net::HTTP::Post.new(uri)
      request["Authorization"] = "Bearer #{@api_key}"
      request["Content-Type"] = "application/json"
      request["Accept"] = "text/event-stream" if payload && payload["stream"]
      request.body = JSON.generate(payload) if payload
      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = uri.scheme == "https"
      http.verify_mode = OpenSSL::SSL::VERIFY_PEER if http.use_ssl?
      http.open_timeout = @timeout_seconds
      http.read_timeout = @timeout_seconds
      block ? http.request(request, &block) : http.request(request)
    end

    def read_bounded_body(response)
      body = +""
      append = lambda do |chunk|
        body << chunk.to_s
        raise ResponseTooLarge, "upstream response exceeds #{MAX_RESPONSE_BYTES} bytes" if body.bytesize > MAX_RESPONSE_BYTES
      end
      if response.is_a?(Net::HTTPResponse)
        response.read_body { |chunk| append.call(chunk) }
      else
        append.call(response.body)
      end
      body
    end

    def monotonic
      Process.clock_gettime(Process::CLOCK_MONOTONIC)
    end

    def elapsed_ms(started)
      (monotonic - started) * 1000.0
    end

    def transport_category(error)
      return "dns" if error.is_a?(SocketError)
      return "tls" if error.is_a?(OpenSSL::SSL::SSLError)

      "transport_error"
    end
  end

  module Importer
    ALLOWED_SOURCES = %w[historical_report manual_terms].freeze

    module_function

    def build(document)
      required = %w[channel_id recorded_at status metrics]
      required.each do |key|
        raise ValidationError, "import.#{key} is required" if document[key].nil?
      end
      source = document["evidence_source"] || "historical_report"
      unless ALLOWED_SOURCES.include?(source)
        raise ValidationError, "import evidence_source must be historical_report or manual_terms"
      end
      SecretGuard.validate!(document)
      {
        "schema_version" => 1,
        "run_id" => SecureRandom.uuid,
        "channel_id" => document.fetch("channel_id"),
        "recorded_at" => Time.iso8601(document.fetch("recorded_at")).utc.iso8601,
        "status" => document.fetch("status"),
        "evidence_source" => source,
        "metrics" => document.fetch("metrics"),
        "notes" => document["notes"] || []
      }
    rescue ArgumentError
      raise ValidationError, "import.recorded_at must be ISO 8601"
    end
  end

  class Comparator
    def initialize(registry:, runs:, as_of: nil)
      @registry = registry
      @runs = runs
      @as_of = as_of && Time.iso8601(as_of)
    end

    def build
      @registry.channels.each_with_object({}) do |channel, output|
        eligible = @runs.select { |run| run["channel_id"] == channel["id"] }
        eligible = eligible.select { |run| Time.iso8601(run["recorded_at"]) <= @as_of } if @as_of
        superseded_ids = eligible.map { |run| run["supersedes"] }.compact
        eligible = eligible.reject { |run| superseded_ids.include?(run["run_id"]) }
        latest_by_source = eligible.group_by { |run| run["evidence_source"] || "unknown" }.values.map do |records|
          records.max_by { |run| Time.iso8601(run["recorded_at"]) }
        end
        output[channel["id"]] = summarize(channel, latest_by_source)
      end
    end

    def self.to_markdown(comparison)
      lines = [
        "| Channel | Lifecycle | Latest status | Evidence | Billing | Models | Stream | Max verified concurrency |",
        "|---|---|---|---|---|---|---|---:|"
      ]
      comparison.keys.sort.each do |id|
        item = comparison.fetch(id)
        lines << [
          escape(item["display_name"]), escape(item["lifecycle"]), escape(item["status"]),
          escape(item["evidence_source"]), escape(format_value(item["billing"])),
          escape(format_value(item["models"])), escape(format_value(item["stream"])),
          escape(format_value(item["max_verified_concurrency"]))
        ].join(" | ").prepend("| ").concat(" |")
      end
      lines.join("\n") + "\n"
    end

    def self.escape(value)
      value.to_s.gsub("|", "\\|").gsub("\n", " ")
    end

    def self.format_value(value)
      return "unknown" if value.nil? || value == "unknown"
      return value.map { |key, child| "#{key}=#{child}" }.join(", ") if value.is_a?(Hash)

      value.to_s
    end

    private

    def summarize(channel, records)
      return {
        "display_name" => channel["display_name"], "lifecycle" => channel["lifecycle"],
        "status" => "unverified", "evidence_source" => "none", "billing" => "unknown",
        "models" => "unknown", "stream" => "unknown", "max_verified_concurrency" => "unknown"
      } if records.empty?

      metrics = records.sort_by { |run| Time.iso8601(run["recorded_at"]) }.inject({}) do |memo, run|
        deep_merge(memo, run["metrics"] || {})
      end
      concurrency = metrics["concurrency"]
      verified = if concurrency.is_a?(Hash)
                   concurrency.select { |_level, item| item["request_count"] == item["success_count"] }.keys.map(&:to_i).max
                 end
      sources = records.map { |run| run["evidence_source"] || "unknown" }.uniq.sort
      status = records.max_by { |run| status_weight(run["status"]) }["status"]
      {
        "display_name" => channel["display_name"],
        "lifecycle" => channel["lifecycle"],
        "status" => status,
        "evidence_source" => sources.join(","),
        "evidence_sources" => sources,
        "billing" => metrics.fetch("billing", "unknown"),
        "models" => metrics.fetch("models", "unknown"),
        "stream" => metrics.fetch("stream", "unknown"),
        "terms" => metrics.fetch("terms", "unknown"),
        "network" => metrics.fetch("network", "unknown"),
        "max_verified_concurrency" => verified || metrics.fetch("max_verified_concurrency", "unknown"),
        "recorded_at" => records.map { |run| run["recorded_at"] }.max,
        "run_ids" => records.map { |run| run["run_id"] }
      }
    end

    def deep_merge(left, right)
      left.merge(right) do |_key, old_value, new_value|
        if old_value.is_a?(Hash) && new_value.is_a?(Hash)
          deep_merge(old_value, new_value)
        else
          new_value
        end
      end
    end

    def status_weight(status)
      { "passed" => 1, "partial" => 2, "failed" => 3 }.fetch(status, 0)
    end
  end

  class CLI
    ROOT = File.expand_path("..", __dir__).freeze
    DEFAULTS = {
      channels: File.join(ROOT, "config/upstream-benchmarks/channels.yaml"),
      profile: File.join(ROOT, "config/upstream-benchmarks/mvp-text-v1.yaml"),
      runs: File.join(ROOT, "config/upstream-benchmarks/ledger/runs.jsonl"),
      decisions: File.join(ROOT, "config/upstream-benchmarks/ledger/decisions.jsonl")
    }.freeze

    def self.run(argv, out: $stdout, err: $stderr, env: ENV)
      command = argv.shift
      raise ValidationError, "command must be one of: validate, run, import, compare, decide" unless command

      options = DEFAULTS.dup
      parser = option_parser(command, options)
      parser.parse!(argv)
      raise ValidationError, "unexpected arguments: #{argv.join(' ')}" unless argv.empty?

      case command
      when "validate" then validate_command(options, out)
      when "run" then run_command(options, out, env)
      when "import" then import_command(options, out)
      when "compare" then compare_command(options, out)
      when "decide" then decide_command(options, out)
      else raise ValidationError, "unknown command: #{command}"
      end
      0
    rescue ValidationError, OptionParser::ParseError, Errno::ENOENT, Psych::SyntaxError => error
      err.puts("ERROR: #{error.message}")
      2
    end

    def self.option_parser(command, options)
      OptionParser.new do |parser|
        parser.banner = "Usage: upstream-benchmark.rb #{command} [options]"
        parser.on("--channels PATH") { |value| options[:channels] = value }
        parser.on("--profile PATH") { |value| options[:profile] = value }
        parser.on("--runs PATH") { |value| options[:runs] = value }
        parser.on("--decisions PATH") { |value| options[:decisions] = value }
        parser.on("--file PATH") { |value| options[:file] = value }
        parser.on("--channel ID") { |value| options[:channel] = value }
        parser.on("--key-env NAME") { |value| options[:key_env] = value }
        parser.on("--format FORMAT") { |value| options[:format] = value }
        parser.on("--as-of TIME") { |value| options[:as_of] = value }
        parser.on("--dry-run") { options[:dry_run] = true }
      end
    end

    def self.validate_command(options, out)
      registry = Registry.new(load_yaml(options[:channels]))
      profile = Profile.new(load_yaml(options[:profile]))
      ledger = Ledger.new(options[:runs], options[:decisions])
      ledger.validate!
      ledger.runs.each { |record| registry.fetch(record.fetch("channel_id")) }
      out.puts("valid: #{registry.channels.length} channels, profile #{profile['id']}, #{ledger.runs.length} runs, #{ledger.decisions.length} decisions")
    end

    def self.run_command(options, out, env)
      raise ValidationError, "--channel is required" unless options[:channel]
      raise ValidationError, "--key-env is required" unless options[:key_env]

      registry = Registry.new(load_yaml(options[:channels]))
      profile = Profile.new(load_yaml(options[:profile]))
      channel = registry.fetch(options[:channel])
      if options[:dry_run]
        out.puts(JSON.pretty_generate(
          "channel_id" => channel["id"], "profile_id" => profile["id"],
          "models" => profile["models"], "max_requests" => estimated_request_count(profile),
          "key_env" => options[:key_env], "network_sent" => false
        ))
        return
      end

      client = HttpClient.new(
        base_url: channel.fetch("base_url"), api_key: env[options[:key_env]],
        timeout_seconds: profile["timeout_seconds"]
      )
      record = Runner.new(client: client, profile: profile).run(channel_id: channel.fetch("id"))
      Ledger.new(options[:runs], options[:decisions]).append_run(record)
      out.puts(JSON.pretty_generate(Redactor.clean(record)))
    end

    def self.import_command(options, out)
      raise ValidationError, "--file is required" unless options[:file]

      record = Importer.build(load_yaml(options[:file]))
      Ledger.new(options[:runs], options[:decisions]).append_run(record)
      out.puts("imported run #{record['run_id']} for #{record['channel_id']}")
    end

    def self.compare_command(options, out)
      registry = Registry.new(load_yaml(options[:channels]))
      ledger = Ledger.new(options[:runs], options[:decisions])
      comparison = Comparator.new(registry: registry, runs: ledger.runs, as_of: options[:as_of]).build
      format = options[:format] || "markdown"
      case format
      when "markdown" then out.write(Comparator.to_markdown(comparison))
      when "json" then out.puts(JSON.pretty_generate(comparison))
      else raise ValidationError, "format must be markdown or json"
      end
    end

    def self.decide_command(options, out)
      raise ValidationError, "--file is required" unless options[:file]

      record = load_yaml(options[:file])
      Ledger.new(options[:runs], options[:decisions]).append_decision(record)
      out.puts("recorded decision #{record['decision_id']}")
    end

    def self.load_yaml(path)
      document = YAML.safe_load(File.read(path))
      raise ValidationError, "#{path} must contain a mapping" unless document.is_a?(Hash)

      document
    end

    def self.estimated_request_count(profile)
      models = profile["models"].length
      models * 2 + profile["repeat_count"] + profile.concurrency_levels.inject(0, :+)
    end
  end
end

if $PROGRAM_NAME == __FILE__
  exit UpstreamBenchmark::CLI.run(ARGV)
end
