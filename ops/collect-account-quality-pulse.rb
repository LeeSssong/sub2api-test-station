#!/usr/bin/env ruby
# frozen_string_literal: true

require "digest"
require "json"
require "net/http"
require "optparse"
require "tempfile"
require "time"
require "uri"

catalog_path = File.expand_path("upstream-benchmark-v2.rb", __dir__)
catalog_path = "/app/ops/upstream-benchmark-v2.rb" unless File.file?(catalog_path)
require catalog_path

module AccountQualityPulse
  PAGE_SIZE = 100
  MAX_RESPONSE_BYTES = 2 * 1024 * 1024
  HISTORY_WINDOW_SECONDS = 24 * 60 * 60
  PROMPT = "Reply with OK only."
  BALANCE_PATTERNS = [
    /\binsufficient (?:balance|credit|quota)\b/i,
    /\b(?:balance|credit) (?:is )?exhausted\b/i,
    /\bquota (?:has been )?exhausted\b/i
  ].freeze

  class ValidationError < StandardError; end
  class StopStream < StandardError; end
  ProbeResult = Struct.new(:result, :ttft_ms, :error_code, keyword_init: true)

  module_function

  def classify_error(message)
    return "balance_exhausted" if BALANCE_PATTERNS.any? { |pattern| pattern.match?(message.to_s) }

    "account_test_error"
  end

  class ModelSelector
    GPT_FAMILY = /\Agpt-(\d+)\.(\d+)(?:[-._].*)?\z/i

    def self.select(models)
      catalog = UpstreamBenchmarkV2::ModelCatalog.discover(models)
      text_models = catalog.values.select { |model| model.fetch("testable") }.map { |model| model.fetch("id") }
      gpt_models = text_models.map do |model_id|
        match = GPT_FAMILY.match(model_id)
        match && [match[1].to_i, match[2].to_i, model_id]
      end.compact
      return gpt_models.sort_by { |major, minor, model_id| [-major, -minor, model_id] }.first&.last unless gpt_models.empty?

      text_models.sort.first
    end
  end

  class SSEParser
    def initialize
      @buffer = +""
      @first_content = false
      @error_code = nil
      @saw_event = false
      @malformed = false
    end

    def feed(chunk)
      @buffer << chunk.to_s
      while (boundary = @buffer.index("\n\n"))
        event = @buffer.slice!(0, boundary + 2)
        parse_event(event)
      end
      self
    end

    def finish
      parse_event(@buffer) unless @buffer.empty?
      @buffer.clear
      self
    end

    def first_content?
      @first_content
    end

    def error_code
      @error_code
    end

    def malformed?
      @malformed || !@saw_event
    end

    def to_h
      { "first_content" => @first_content, "error_code" => @error_code, "malformed" => malformed? }
    end

    private

    def parse_event(frame)
      data = frame.each_line.map do |line|
        line.start_with?("data:") ? line.delete_prefix("data:").strip : nil
      end.compact.join("\n")
      return if data.empty?

      payload = JSON.parse(data)
      unless payload.is_a?(Hash) && payload["type"].is_a?(String)
        @malformed = true
        return
      end

      @saw_event = true
      case payload.fetch("type")
      when "content"
        @first_content = payload["text"].is_a?(String) && !payload.fetch("text").strip.empty?
      when "error"
        @error_code = AccountQualityPulse.classify_error(payload["error"])
      end
    rescue JSON::ParserError
      @malformed = true
    end
  end

  class NativeClient
    def initialize(base_url:, admin_key_file:)
      @base_url = validate_base_url(base_url)
      @admin_key = read_admin_key(admin_key_file)
    end

    def accounts
      data = json_request("GET", "/api/v1/admin/accounts?page=1&page_size=#{PAGE_SIZE}")
      items = data.fetch("items")
      total = Integer(data.fetch("total"))
      unless items.is_a?(Array) && total.between?(0, PAGE_SIZE) && items.length == total
        raise ValidationError, "native account list is invalid"
      end

      items
    rescue KeyError, ArgumentError, TypeError
      raise ValidationError, "native account list is invalid"
    end

    def models(account_id)
      data = json_request("GET", "/api/v1/admin/accounts/#{positive_id(account_id)}/models")
      raise ValidationError, "native account models are invalid" unless data.is_a?(Array)

      data
    end

    def probe(account_id:, model_id:, prompt:)
      payload = JSON.generate("model_id" => model_id, "prompt" => prompt)
      request_stream(account_id, payload)
    end

    private

    def json_request(method, path)
      response = request(method, path)
      raise ValidationError, "native request failed" unless response.code.to_i.between?(200, 299)
      raise ValidationError, "native response exceeds limit" if response.body.to_s.bytesize > MAX_RESPONSE_BYTES

      envelope = JSON.parse(response.body)
      envelope.fetch("data", envelope)
    rescue JSON::ParserError, SocketError, SystemCallError, Net::OpenTimeout, Net::ReadTimeout, Timeout::Error, URI::InvalidURIError
      raise ValidationError, "native request failed"
    end

    def request_stream(account_id, payload)
      parser = SSEParser.new
      started = Process.clock_gettime(Process::CLOCK_MONOTONIC)
      bytes_read = 0
      response_code = nil
      response_body = +""
      uri = uri_for("/api/v1/admin/accounts/#{positive_id(account_id)}/test")
      request = Net::HTTP::Post.new(uri)
      request["Accept"] = "text/event-stream"
      request["Content-Type"] = "application/json"
      request["x-api-key"] = @admin_key
      request.body = payload

      with_http(uri) do |http|
        http.request(request) do |response|
          response_code = response.code.to_i
          response.read_body do |chunk|
            bytes_read += chunk.bytesize
            raise ValidationError, "native response exceeds limit" if bytes_read > MAX_RESPONSE_BYTES

            if response_code.between?(200, 299)
              parser.feed(chunk)
              raise StopStream if parser.first_content? || parser.error_code
            else
              response_body << chunk
            end
          end
        end
      end

      unless response_code&.between?(200, 299)
        error_code = AccountQualityPulse.classify_error(response_body)
        error_code = "http_error" unless error_code == "balance_exhausted"
        return ProbeResult.new(result: error_code, ttft_ms: nil, error_code: error_code)
      end
      return ProbeResult.new(result: "passed", ttft_ms: elapsed_ms(started), error_code: "") if parser.first_content?
      return ProbeResult.new(result: parser.error_code, ttft_ms: nil, error_code: parser.error_code) if parser.error_code

      parser.finish
      return ProbeResult.new(result: parser.error_code, ttft_ms: nil, error_code: parser.error_code) if parser.error_code

      ProbeResult.new(result: parser.malformed? ? "malformed_stream" : "account_test_error", ttft_ms: nil,
                      error_code: parser.malformed? ? "malformed_stream" : "account_test_error")
    rescue StopStream
      return ProbeResult.new(result: "passed", ttft_ms: elapsed_ms(started), error_code: "") if parser.first_content?
      return ProbeResult.new(result: parser.error_code, ttft_ms: nil, error_code: parser.error_code) if parser.error_code

      ProbeResult.new(result: "malformed_stream", ttft_ms: nil, error_code: "malformed_stream")
    rescue Net::OpenTimeout, Net::ReadTimeout, Timeout::Error
      ProbeResult.new(result: "timeout", ttft_ms: nil, error_code: "timeout")
    rescue ValidationError
      ProbeResult.new(result: "http_error", ttft_ms: nil, error_code: "http_error")
    rescue SocketError, SystemCallError, URI::InvalidURIError
      ProbeResult.new(result: "http_error", ttft_ms: nil, error_code: "http_error")
    end

    def request(method, path)
      uri = uri_for(path)
      request = method == "POST" ? Net::HTTP::Post.new(uri) : Net::HTTP::Get.new(uri)
      request["Accept"] = "application/json"
      request["x-api-key"] = @admin_key
      with_http(uri) { |http| http.request(request) }
    end

    def with_http(uri)
      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = uri.scheme == "https"
      http.open_timeout = 5
      http.read_timeout = 20
      yield http
    end

    def uri_for(path)
      URI.parse(@base_url + path)
    end

    def validate_base_url(value)
      uri = URI.parse(value.to_s.strip.sub(%r{/+\z}, ""))
      valid = %w[http https].include?(uri.scheme) && uri.host && uri.userinfo.nil? && uri.query.nil? && uri.fragment.nil? &&
        (uri.path.nil? || uri.path.empty?)
      raise ValidationError, "Sub2API base URL is invalid" unless valid

      uri.to_s
    rescue URI::InvalidURIError
      raise ValidationError, "Sub2API base URL is invalid"
    end

    def read_admin_key(path)
      raise ValidationError, "Sub2API Admin key file is invalid" unless File.absolute_path(path.to_s) == path.to_s

      stat = File.stat(path)
      valid_mode = [0o600, 0o640].include?(stat.mode & 0o777)
      raise ValidationError, "Sub2API Admin key file is invalid" unless stat.file? && valid_mode

      value = File.read(path, 4096).strip
      raise ValidationError, "Sub2API Admin key file is invalid" if value.empty?

      value
    rescue Errno::ENOENT, Errno::EACCES
      raise ValidationError, "Sub2API Admin key file is invalid"
    end

    def positive_id(value)
      id = Integer(value)
      raise ValidationError, "account ID is invalid" unless id.positive?

      id
    rescue ArgumentError, TypeError
      raise ValidationError, "account ID is invalid"
    end

    def elapsed_ms(started)
      ((Process.clock_gettime(Process::CLOCK_MONOTONIC) - started) * 1000).round(3)
    end
  end

  class Collector
    def initialize(client:, now: Time.now.utc)
      @client = client
      @now = now.utc
    end

    def collect(history:)
      accounts = eligible_accounts
      samples = prune_history(history)
      records = accounts.map do |account|
        account_id = Integer(account.fetch("id"))
        collect_account(account_id, samples)
      rescue KeyError, ArgumentError, TypeError
        raise ValidationError, "native account list is invalid"
      end
      records.each { |record| samples << record.fetch("sample") }

      result_accounts = records.map { |record| summarize(record.fetch("sample"), samples) }.sort_by { |record| record.fetch("account_id") }
      {
        "result" => {
          "schema_version" => 1,
          "snapshot_id" => "ACCOUNT-QUALITY-#{@now.strftime('%Y%m%dT%H%M%SZ')}",
          "observed_at" => @now.iso8601,
          "account_set_sha256" => Digest::SHA256.hexdigest(JSON.generate(accounts.map { |account| Integer(account.fetch("id")) })),
          "accounts" => result_accounts
        },
        "history" => samples
      }
    end

    private

    def eligible_accounts
      raw = @client.accounts
      raise ValidationError, "native account list is invalid" unless raw.is_a?(Array)

      selected = raw.select { |account| account.is_a?(Hash) && account["status"] == "active" && account["schedulable"] == true }
      ids = selected.map { |account| Integer(account.fetch("id")) }
      raise ValidationError, "native account list is invalid" unless ids.all?(&:positive?) && ids.uniq.length == ids.length

      selected.sort_by { |account| Integer(account.fetch("id")) }
    rescue KeyError, ArgumentError, TypeError
      raise ValidationError, "native account list is invalid"
    end

    def collect_account(account_id, history)
      model_id = ModelSelector.select(@client.models(account_id))
      probe = if model_id
                @client.probe(account_id: account_id, model_id: model_id, prompt: PROMPT)
              else
                ProbeResult.new(result: "model_unavailable", ttft_ms: nil, error_code: "model_unavailable")
              end
      sample = {
        "account_id" => account_id,
        "model_id" => model_id.to_s,
        "result" => approved_result(probe.result),
        "error_code" => approved_error_code(probe.error_code, probe.result),
        "ttft_ms" => valid_ttft(probe.ttft_ms),
        "observed_at" => @now.iso8601
      }
      { "sample" => sample }
    rescue StandardError
      { "sample" => failure_sample(account_id) }
    end

    def failure_sample(account_id)
      {
        "account_id" => account_id, "model_id" => "",
        "result" => "account_test_error", "error_code" => "account_test_error", "ttft_ms" => nil,
        "observed_at" => @now.iso8601
      }
    end

    def prune_history(history)
      cutoff = @now - HISTORY_WINDOW_SECONDS
      Array(history).select do |sample|
        next false unless sample.is_a?(Hash)

        Time.iso8601(sample.fetch("observed_at")) >= cutoff
      rescue KeyError, ArgumentError
        false
      end
    end

    def summarize(current, samples)
      account_samples = samples.select { |sample| sample.fetch("account_id") == current.fetch("account_id") }
      successful = account_samples.select { |sample| sample.fetch("result") == "passed" }
      ttfts = successful.map { |sample| sample["ttft_ms"] }.compact.sort
      {
        "account_id" => current.fetch("account_id"),
        "model_id" => current.fetch("model_id"),
        "sample_count" => account_samples.length,
        "success_count" => successful.length,
        "success_rate" => (successful.length.to_f / account_samples.length).round(6),
        "ttft_p50_ms" => percentile(ttfts, 0.50),
        "ttft_p95_ms" => percentile(ttfts, 0.95),
        "last_result" => current.fetch("result"),
        "last_error_code" => current.fetch("error_code"),
        "last_observed_at" => current.fetch("observed_at")
      }
    end

    def percentile(values, quantile)
      return nil if values.empty?

      values[(values.length * quantile).ceil - 1].to_f
    end

    def approved_result(value)
      %w[passed balance_exhausted account_test_error http_error timeout malformed_stream model_unavailable].include?(value) ? value : "account_test_error"
    end

    def approved_error_code(value, result)
      return "" if result == "passed"

      approved_result(value)
    end

    def valid_ttft(value)
      return nil if value.nil?

      numeric = Float(value)
      return nil unless numeric.finite? && numeric >= 0

      numeric
    rescue ArgumentError, TypeError
      nil
    end
  end

  class Publisher
    class PublicationError < ValidationError; end

    def self.publish(result_path:, history_path:, result:, history:, rename: File.method(:rename))
      [result_path, history_path].each do |path|
        raise ValidationError, "output path must be absolute" unless File.absolute_path(path) == path
      end

      originals = {}
      [result_path, history_path].each do |path|
        originals[path] = File.file?(path) ? [File.binread(path), File.stat(path).mode & 0o777] : nil
      end
      staged = {
        result_path => stage(result_path, JSON.pretty_generate(result)),
        history_path => stage(history_path, JSON.pretty_generate(history))
      }
      backups = {}
      originals.each do |path, original|
        next unless original

        backup = Tempfile.new([".account-quality-backup-", ".json"], File.dirname(path))
        backup.close
        File.unlink(backup.path)
        rename.call(path, backup.path)
        backups[path] = backup.path
      end
      staged.each { |path, temp| rename.call(temp, path) }
      File.open(File.dirname(result_path), "r") { |directory| directory.fsync }
      [result_path, history_path].each { |path| raise PublicationError, "published evidence readback failed" unless File.file?(path) && JSON.parse(File.read(path)) }
      backups.each_value { |path| File.delete(path) if File.exist?(path) }
    rescue StandardError => error
      staged&.each_value { |path| File.delete(path) if path && File.exist?(path) }
      backups&.each do |path, backup|
        File.delete(path) if File.exist?(path)
        rename.call(backup, path) if File.exist?(backup)
      end
      originals&.each { |path, original| File.delete(path) if original.nil? && File.exist?(path) }
      raise(error.is_a?(PublicationError) ? error : PublicationError.new("evidence publication failed"))
    end

    def self.stage(path, content)
      file = Tempfile.new([".account-quality-", ".json"], File.dirname(path))
      file.chmod(0o600)
      file.write(content)
      file.flush
      file.fsync
      file.close
      file.path
    rescue SystemCallError
      file&.close!
      raise PublicationError, "evidence publication failed"
    end
    private_class_method :stage
  end

  class CLI
    def self.run(argv, out: $stdout, err: $stderr)
      command = argv.shift
      options = {}
      OptionParser.new do |parser|
        parser.on("--base-url URL") { |value| options[:base_url] = value }
        parser.on("--admin-key-file PATH") { |value| options[:admin_key_file] = value }
        parser.on("--output PATH") { |value| options[:output] = value }
      end.parse!(argv)
      raise ValidationError, "command must be collect" unless command == "collect"
      raise ValidationError, "unexpected arguments" unless argv.empty?
      %i[base_url admin_key_file output].each { |key| raise ValidationError, "missing required option" unless options[key] }
      raise ValidationError, "output path must be absolute" unless File.absolute_path(options[:output]) == options[:output]

      history_path = File.join(File.dirname(options[:output]), "account-quality-history.json")
      history = File.file?(history_path) ? JSON.parse(File.read(history_path, MAX_RESPONSE_BYTES)) : []
      output = Collector.new(client: NativeClient.new(base_url: options[:base_url], admin_key_file: options[:admin_key_file])).collect(history: history)
      Publisher.publish(result_path: options[:output], history_path: history_path, result: output.fetch("result"), history: output.fetch("history"))
      out.puts(JSON.generate("snapshot_id" => output.dig("result", "snapshot_id"), "account_set_sha256" => output.dig("result", "account_set_sha256")))
      0
    rescue Publisher::PublicationError
      err.puts("ERROR: account quality evidence publication rejected")
      46
    rescue ValidationError, JSON::ParserError, OptionParser::ParseError, Errno::ENOENT, Errno::EACCES, ArgumentError
      err.puts("ERROR: account quality pulse collection rejected")
      2
    end
  end
end

if $PROGRAM_NAME == __FILE__
  exit AccountQualityPulse::CLI.run(ARGV)
end
