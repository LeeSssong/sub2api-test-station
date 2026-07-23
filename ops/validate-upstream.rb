#!/usr/bin/env ruby
# frozen_string_literal: true

require "optparse"
require "uri"
require "yaml"

class UpstreamConfigValidator
  ROOT_KEYS = %w[
    schema_version upstream_id display_name readiness reviewed_at connection
    sub2api models limits rate_limit balance commercial evidence
  ].freeze
  READINESS_VALUES = %w[draft ready_for_live_test accepted].freeze
  PROTOCOL_VALUES = %w[
    openai_compatible anthropic_compatible gemini_native other_https
  ].freeze
  AUTH_SCHEME_VALUES = %w[bearer x_api_key custom_header].freeze
  PLATFORM_VALUES = %w[openai anthropic gemini grok antigravity].freeze
  CAPABILITY_VALUES = %w[
    models chat_completions responses messages streaming tools embeddings images
  ].freeze
  RESALE_VALUES = %w[unknown allowed conditional prohibited].freeze
  SECRET_REFERENCE = %r{\A(?:sub2api-admin|env|password-manager)://[A-Za-z0-9._/-]+\z}.freeze
  FORBIDDEN_CREDENTIAL_KEY = /(?:\A|_)(?:
    api_?key|access_?token|refresh_?token|id_?token|bearer_?token|password|passwd|
    cookie|cookies|session_?key|private_?key|secret_?key|client_?secret|authorization|
    credential|credentials
  )(?:\z|_)/ix.freeze
  SECRET_VALUE_PATTERNS = [
    /-----BEGIN [A-Z ]*PRIVATE KEY-----/,
    /\bBearer\s+[A-Za-z0-9._~+\/=:-]{16,}/i,
    /\bsk[-_](?:live[-_]|test[-_]|proj[-_])?[A-Za-z0-9_-]{16,}\b/i,
    /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/,
    /\bAKIA[A-Z0-9]{16}\b/,
    /\bAIza[A-Za-z0-9_-]{20,}\b/,
    /\bxox[baprs]-[A-Za-z0-9-]{16,}\b/i,
    /\bgh[pousr]_[A-Za-z0-9]{20,}\b/i
  ].freeze

  def initialize(document, live_ready: false)
    @document = document
    @live_ready = live_ready
    @errors = []
    @validated = false
  end

  def errors
    return @errors if @validated

    @validated = true
    validate_document
    @errors
  end

  private

  def validate_document
    unless @document.is_a?(Hash)
      add("root", "must be a mapping")
      return
    end

    ROOT_KEYS.each { |key| require_key(@document, key, key) }
    (@document.keys.map(&:to_s) - ROOT_KEYS).sort.each do |key|
      add(key, "unknown top-level field")
    end

    validate_scalar_fields
    validate_connection
    validate_sub2api
    validate_models
    validate_limits
    validate_rate_limit
    validate_balance
    validate_commercial
    validate_evidence
    scan_for_secrets(@document)
    validate_cross_field_rules
    validate_live_readiness if @live_ready
  end

  def validate_scalar_fields
    integer(@document["schema_version"], "schema_version", minimum: 1, maximum: 1)
    string(@document["upstream_id"], "upstream_id", pattern: /\AUP[0-9]{2,}\z/)
    string(@document["display_name"], "display_name")
    enum(@document["readiness"], "readiness", READINESS_VALUES)
    string(@document["reviewed_at"], "reviewed_at", pattern: /\A\d{4}-\d{2}-\d{2}\z/)
  end

  def validate_connection
    connection = mapping(@document["connection"], "connection")
    return unless connection

    %w[protocol base_url allowlist_host auth_scheme secret_ref].each do |key|
      require_key(connection, key, "connection.#{key}")
    end
    enum(connection["protocol"], "connection.protocol", PROTOCOL_VALUES)
    enum(connection["auth_scheme"], "connection.auth_scheme", AUTH_SCHEME_VALUES)
    string(connection["allowlist_host"], "connection.allowlist_host",
           pattern: /\A(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)*[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\z/)
    validate_base_url(connection)
    string(connection["secret_ref"], "connection.secret_ref", pattern: SECRET_REFERENCE,
           pattern_message: "must be a symbolic secret location")
  end

  def validate_base_url(connection)
    raw = connection["base_url"]
    return unless string(raw, "connection.base_url")

    begin
      uri = URI.parse(raw)
    rescue URI::InvalidURIError
      add("connection.base_url", "must be a valid absolute URL")
      return
    end

    add("connection.base_url", "must use https") unless uri.scheme == "https"
    add("connection.base_url", "must include a host") if blank?(uri.host)
    add("connection.base_url", "must not include user information") if uri.userinfo
    add("connection.base_url", "must not include a query string") if uri.query
    add("connection.base_url", "must not include a fragment") if uri.fragment

    return if blank?(uri.host) || blank?(connection["allowlist_host"])

    expected = uri.host.downcase
    actual = connection["allowlist_host"].to_s.downcase
    unless actual == expected
      add("connection.allowlist_host", "must exactly match connection.base_url host #{expected}")
    end
  end

  def validate_sub2api
    config = mapping(@document["sub2api"], "sub2api")
    return unless config

    %w[
      platform account_type account_name group_name priority concurrency
      rate_multiplier pool_mode
    ].each { |key| require_key(config, key, "sub2api.#{key}") }
    enum(config["platform"], "sub2api.platform", PLATFORM_VALUES)
    enum(config["account_type"], "sub2api.account_type", ["apikey"])
    string(config["account_name"], "sub2api.account_name")
    string(config["group_name"], "sub2api.group_name")
    integer(config["priority"], "sub2api.priority", minimum: 0)
    integer(config["concurrency"], "sub2api.concurrency", minimum: 1)
    number(config["rate_multiplier"], "sub2api.rate_multiplier", minimum: 0)

    pool = mapping(config["pool_mode"], "sub2api.pool_mode")
    return unless pool

    %w[enabled retry_count retry_status_codes].each do |key|
      require_key(pool, key, "sub2api.pool_mode.#{key}")
    end
    boolean(pool["enabled"], "sub2api.pool_mode.enabled")
    integer(pool["retry_count"], "sub2api.pool_mode.retry_count", minimum: 0, maximum: 10)
    codes = array(pool["retry_status_codes"], "sub2api.pool_mode.retry_status_codes")
    return unless codes

    add("sub2api.pool_mode.retry_status_codes", "must not be empty") if codes.empty?
    codes.each_with_index do |code, index|
      integer(code, "sub2api.pool_mode.retry_status_codes[#{index}]", minimum: 400, maximum: 599)
    end
  end

  def validate_models
    models = array(@document["models"], "models")
    return unless models

    add("models", "must contain at least one model") if models.empty?
    seen = {}
    enabled_count = 0
    models.each_with_index do |raw_model, index|
      path = "models[#{index}]"
      model = mapping(raw_model, path)
      next unless model

      %w[public_name upstream_name enabled capabilities pricing].each do |key|
        require_key(model, key, "#{path}.#{key}")
      end
      public_name = model["public_name"]
      string(public_name, "#{path}.public_name", pattern: /\A\S{1,200}\z/)
      string(model["upstream_name"], "#{path}.upstream_name", pattern: /\A\S{1,200}\z/)
      boolean(model["enabled"], "#{path}.enabled")
      enabled_count += 1 if model["enabled"] == true

      if public_name.is_a?(String) && !public_name.empty?
        if seen.key?(public_name)
          add("#{path}.public_name", "duplicates #{public_name}")
        else
          seen[public_name] = true
        end
      end

      capabilities = array(model["capabilities"], "#{path}.capabilities")
      if capabilities
        add("#{path}.capabilities", "must not be empty") if capabilities.empty?
        capabilities.each_with_index do |capability, capability_index|
          enum(capability, "#{path}.capabilities[#{capability_index}]", CAPABILITY_VALUES)
        end
      end
      validate_pricing(model["pricing"], "#{path}.pricing")
    end
    add("models", "must contain at least one enabled model") if enabled_count.zero?
  end

  def validate_pricing(raw_pricing, path)
    pricing = mapping(raw_pricing, path)
    return unless pricing

    %w[currency unit input output cached_input cache_write].each do |key|
      require_key(pricing, key, "#{path}.#{key}")
    end
    string(pricing["currency"], "#{path}.currency", pattern: /\A[A-Z]{3}\z/)
    enum(pricing["unit"], "#{path}.unit", ["per_1m_tokens"])
    number(pricing["input"], "#{path}.input", minimum: 0)
    number(pricing["output"], "#{path}.output", minimum: 0)
    nullable_number(pricing["cached_input"], "#{path}.cached_input", minimum: 0)
    nullable_number(pricing["cache_write"], "#{path}.cache_write", minimum: 0)
  end

  def validate_limits
    limits = mapping(@document["limits"], "limits")
    return unless limits

    %w[max_concurrency rpm tpm request_timeout_seconds daily_cost_cap].each do |key|
      require_key(limits, key, "limits.#{key}")
    end
    integer(limits["max_concurrency"], "limits.max_concurrency", minimum: 1)
    integer(limits["rpm"], "limits.rpm", minimum: 1)
    integer(limits["tpm"], "limits.tpm", minimum: 1)
    integer(limits["request_timeout_seconds"], "limits.request_timeout_seconds", minimum: 1, maximum: 1800)
    validate_money(limits["daily_cost_cap"], "limits.daily_cost_cap", minimum: 0.01)
  end

  def validate_rate_limit
    policy = mapping(@document["rate_limit"], "rate_limit")
    return unless policy

    %w[status_code retry_after retryable max_attempts reset_behavior].each do |key|
      require_key(policy, key, "rate_limit.#{key}")
    end
    integer(policy["status_code"], "rate_limit.status_code", minimum: 429, maximum: 429)
    enum(policy["retry_after"], "rate_limit.retry_after", %w[honor_when_present ignore unavailable])
    boolean(policy["retryable"], "rate_limit.retryable")
    integer(policy["max_attempts"], "rate_limit.max_attempts", minimum: 0, maximum: 3)
    string(policy["reset_behavior"], "rate_limit.reset_behavior")
  end

  def validate_balance
    balance = mapping(@document["balance"], "balance")
    return unless balance

    %w[query_method query_reference minimum_top_up low_balance_alert].each do |key|
      require_key(balance, key, "balance.#{key}")
    end
    enum(balance["query_method"], "balance.query_method", %w[dashboard api support unavailable])
    string(balance["query_reference"], "balance.query_reference")
    validate_money(balance["minimum_top_up"], "balance.minimum_top_up", minimum: 0)
    validate_money(balance["low_balance_alert"], "balance.low_balance_alert", minimum: 0)
  end

  def validate_commercial
    commercial = mapping(@document["commercial"], "commercial")
    return unless commercial

    %w[
      resale_permission terms_reference refund_policy support_reference
      risk_acknowledged
    ].each { |key| require_key(commercial, key, "commercial.#{key}") }
    enum(commercial["resale_permission"], "commercial.resale_permission", RESALE_VALUES)
    string(commercial["terms_reference"], "commercial.terms_reference")
    string(commercial["refund_policy"], "commercial.refund_policy")
    string(commercial["support_reference"], "commercial.support_reference")
    boolean(commercial["risk_acknowledged"], "commercial.risk_acknowledged")
  end

  def validate_evidence
    evidence = mapping(@document["evidence"], "evidence")
    return unless evidence

    %w[checked_by checked_at notes].each do |key|
      require_key(evidence, key, "evidence.#{key}")
    end
    string(evidence["checked_by"], "evidence.checked_by")
    string(evidence["checked_at"], "evidence.checked_at",
           pattern: /\A\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z\z/)
    string(evidence["notes"], "evidence.notes")
  end

  def validate_money(raw_money, path, minimum:)
    money = mapping(raw_money, path)
    return unless money

    %w[currency amount].each { |key| require_key(money, key, "#{path}.#{key}") }
    string(money["currency"], "#{path}.currency", pattern: /\A[A-Z]{3}\z/)
    number(money["amount"], "#{path}.amount", minimum: minimum)
  end

  def validate_cross_field_rules
    sub2api = @document["sub2api"]
    limits = @document["limits"]
    return unless sub2api.is_a?(Hash) && limits.is_a?(Hash)

    configured = sub2api["concurrency"]
    supplier_max = limits["max_concurrency"]
    return unless configured.is_a?(Integer) && supplier_max.is_a?(Integer)
    return unless configured > supplier_max

    add("sub2api.concurrency", "must not exceed limits.max_concurrency (#{supplier_max})")
  end

  def validate_live_readiness
    unless @document["readiness"] == "ready_for_live_test"
      add("readiness", "must be ready_for_live_test in --live-ready mode")
    end
    commercial = @document["commercial"]
    if commercial.is_a?(Hash) && commercial["resale_permission"] == "prohibited"
      add("commercial.resale_permission", "must not be prohibited for a commercial-channel live test")
    end
    if commercial.is_a?(Hash) && commercial["risk_acknowledged"] != true
      add("commercial.risk_acknowledged", "must be true in --live-ready mode")
    end
  end

  def scan_for_secrets(value, path = nil)
    case value
    when Hash
      value.each do |raw_key, child|
        key = raw_key.to_s
        child_path = path ? "#{path}.#{key}" : key
        if key != "secret_ref" && key.downcase.match?(FORBIDDEN_CREDENTIAL_KEY)
          add(child_path, "credential fields are forbidden; use connection.secret_ref")
        end
        scan_for_secrets(child, child_path)
      end
    when Array
      value.each_with_index { |child, index| scan_for_secrets(child, "#{path}[#{index}]") }
    when String
      return if path == "connection.secret_ref"
      add(path, "value looks like a secret") if SECRET_VALUE_PATTERNS.any? { |pattern| value.match?(pattern) }
    end
  end

  def require_key(mapping_value, key, path)
    add(path, "is required") unless mapping_value.is_a?(Hash) && mapping_value.key?(key)
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

  def string(value, path, pattern: nil, pattern_message: "has an invalid format")
    unless value.is_a?(String) && !value.strip.empty?
      add(path, "must be a non-empty string") unless value.nil?
      return false
    end
    if pattern && !value.match?(pattern)
      add(path, pattern_message)
      return false
    end
    true
  end

  def enum(value, path, allowed)
    return true if allowed.include?(value)

    add(path, "must be one of: #{allowed.join(', ')}") unless value.nil?
    false
  end

  def boolean(value, path)
    return true if value == true || value == false

    add(path, "must be true or false") unless value.nil?
    false
  end

  def integer(value, path, minimum: nil, maximum: nil)
    unless value.is_a?(Integer)
      add(path, "must be an integer") unless value.nil?
      return false
    end
    add(path, "must be at least #{minimum}") if minimum && value < minimum
    add(path, "must be at most #{maximum}") if maximum && value > maximum
    true
  end

  def number(value, path, minimum: nil)
    unless value.is_a?(Numeric) && value.finite?
      add(path, "must be a finite number") unless value.nil?
      return false
    end
    add(path, "must be at least #{minimum}") if minimum && value < minimum
    true
  end

  def nullable_number(value, path, minimum: nil)
    return true if value.nil?

    number(value, path, minimum: minimum)
  end

  def blank?(value)
    value.nil? || value.to_s.strip.empty?
  end

  def add(path, message)
    @errors << "#{path}: #{message}"
  end
end

def load_upstream_yaml(path)
  YAML.safe_load(
    File.read(path),
    permitted_classes: [],
    permitted_symbols: [],
    aliases: false,
    filename: path
  )
end

if $PROGRAM_NAME == __FILE__
  options = { live_ready: false }
  parser = OptionParser.new do |opts|
    opts.banner = "Usage: ruby ops/validate-upstream.rb [--live-ready] FILE..."
    opts.on("--live-ready", "Require readiness for a bounded live test") do
      options[:live_ready] = true
    end
  end

  begin
    parser.parse!
  rescue OptionParser::ParseError => e
    warn e.message
    warn parser
    exit 2
  end

  if ARGV.empty?
    warn parser
    exit 2
  end

  all_valid = true
  ARGV.each do |path|
    begin
      document = load_upstream_yaml(path)
      validation_errors = UpstreamConfigValidator.new(
        document,
        live_ready: options[:live_ready]
      ).errors
      if validation_errors.empty?
        puts "#{path}: OK"
      else
        all_valid = false
        warn "#{path}: INVALID"
        validation_errors.each { |error| warn "  - #{error}" }
      end
    rescue Errno::ENOENT, Errno::EACCES, Psych::Exception => e
      all_valid = false
      warn "#{path}: ERROR: #{e.message}"
    end
  end

  exit(all_valid ? 0 : 1)
end
