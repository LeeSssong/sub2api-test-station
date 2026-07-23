#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "time"
require "uri"
require "yaml"

module PaymentControl
  class ValidationError < StandardError
    attr_reader :errors

    def initialize(errors)
      @errors = errors
      super(errors.join("; "))
    end
  end

  class ConfigValidator
    ROOT_KEYS = %w[
      schema_version payment_id status reviewed_at merchant_context decision settings
      providers evidence
    ].freeze
    STATUS_VALUES = %w[fictional draft ready_for_merchant_review live_accepted].freeze
    PROVIDER_KEYS = %w[alipay wxpay stripe easypay].freeze
    READINESS_VALUES = %w[recommended_conditional deferred_due_diligence live_accepted rejected].freeze
    ROLES = %w[mainland_primary mainland_secondary overseas_primary aggregation_fallback].freeze
    CALLBACK_PATHS = {
      "alipay" => "/api/v1/payment/webhook/alipay",
      "wxpay" => "/api/v1/payment/webhook/wxpay",
      "stripe" => "/api/v1/payment/webhook/stripe",
      "easypay" => "/api/v1/payment/webhook/easypay"
    }.freeze
    CAPABILITIES = %w[signature_verification active_query refund reconciliation].freeze
    SECRET_REFERENCE = %r{\A(?:sub2api-admin|password-manager)://[A-Za-z0-9._/-]+\z}.freeze
    FORBIDDEN_SECRET_KEYS = %w[
      api_key api_v3_key pkey merchant_key secret_key webhook_secret private_key
      public_key certificate certificate_private_key access_token refresh_token
      password card_number payment_card cvv credentials
    ].freeze
    SECRET_VALUE_PATTERNS = [
      /-----BEGIN [A-Z ]*PRIVATE KEY-----/,
      /\bBearer\s+[A-Za-z0-9._~+\/=:-]{16,}/i,
      /\bsk_(?:live|test)_[A-Za-z0-9_-]{16,}\b/i,
      /\bpk_(?:live|test)_[A-Za-z0-9_-]{16,}\b/i,
      /\bwhsec_[A-Za-z0-9_-]{16,}\b/i
    ].freeze

    SECTION_KEYS = {
      "merchant_context" => %w[
        current_state assumed_primary_market settlement_currency external_actions_deferred
      ],
      "decision" => %w[
        current_mode mainland_primary mainland_secondary overseas_primary
        aggregation_fallback public_registration
      ],
      "settings" => %w[
        payment_enabled enabled_payment_types payment_min_amount_cny
        payment_max_amount_cny payment_daily_limit_cny payment_order_timeout_minutes
        payment_max_pending_orders cny_per_usd
        balance_recharge_multiplier_usd_per_cny payment_recharge_fee_rate
        payment_load_balance_strategy cancel_rate_limit
      ],
      "evidence" => %w[sub2api_version sub2api_commit source_reference notes]
    }.freeze

    def initialize(document)
      @document = document
      @errors = []
      @validated = false
    end

    def errors
      return @errors if @validated

      @validated = true
      validate
      @errors.uniq
    end

    private

    def validate
      unless @document.is_a?(Hash)
        add("root", "must be a mapping")
        return
      end

      ROOT_KEYS.each { |key| require_key(@document, key, key) }
      (@document.keys.map(&:to_s) - ROOT_KEYS).sort.each do |key|
        add(key, "unknown top-level field")
      end

      integer(@document["schema_version"], "schema_version", minimum: 1, maximum: 1)
      string(@document["payment_id"], "payment_id", pattern: /\APAY[0-9]{2,}\z/)
      enum(@document["status"], "status", STATUS_VALUES)
      string(@document["reviewed_at"], "reviewed_at", pattern: /\A\d{4}-\d{2}-\d{2}\z/)

      SECTION_KEYS.each do |section_name, keys|
        section = mapping(@document[section_name], section_name)
        next unless section

        keys.each { |key| require_key(section, key, "#{section_name}.#{key}") }
      end
      require_key(@document, "providers", "providers")

      validate_merchant_context
      validate_decision
      validate_settings
      validate_providers
      validate_evidence
      validate_cross_field_rules
      scan_for_secrets(@document)
    end

    def validate_merchant_context
      section = @document["merchant_context"]
      return unless section.is_a?(Hash)

      enum(section["current_state"], "merchant_context.current_state",
           %w[unknown mainland_qualified overseas_qualified both_qualified])
      enum(section["assumed_primary_market"], "merchant_context.assumed_primary_market",
           %w[mainland_china international mixed])
      enum(section["settlement_currency"], "merchant_context.settlement_currency", %w[CNY USD])
      boolean(section["external_actions_deferred"], "merchant_context.external_actions_deferred")
    end

    def validate_decision
      section = @document["decision"]
      return unless section.is_a?(Hash)

      enum(section["current_mode"], "decision.current_mode",
           %w[manual_ledger_simulation automatic_payment])
      enum(section["mainland_primary"], "decision.mainland_primary", %w[alipay])
      enum(section["mainland_secondary"], "decision.mainland_secondary", %w[wxpay])
      enum(section["overseas_primary"], "decision.overseas_primary", %w[stripe])
      enum(section["aggregation_fallback"], "decision.aggregation_fallback", %w[easypay])
      enum(section["public_registration"], "decision.public_registration",
           %w[invitation_only public])
    end

    def validate_settings
      section = @document["settings"]
      return unless section.is_a?(Hash)

      boolean(section["payment_enabled"], "settings.payment_enabled")
      types = array(section["enabled_payment_types"], "settings.enabled_payment_types")
      types&.each_with_index do |type, index|
        enum(type, "settings.enabled_payment_types[#{index}]", PROVIDER_KEYS)
      end
      %w[
        payment_min_amount_cny payment_max_amount_cny payment_daily_limit_cny
        cny_per_usd balance_recharge_multiplier_usd_per_cny payment_recharge_fee_rate
      ].each do |key|
        number(section[key], "settings.#{key}", minimum: 0)
      end
      integer(section["payment_order_timeout_minutes"],
              "settings.payment_order_timeout_minutes", minimum: 1)
      integer(section["payment_max_pending_orders"],
              "settings.payment_max_pending_orders", minimum: 1)
      enum(section["payment_load_balance_strategy"],
           "settings.payment_load_balance_strategy", %w[round_robin least_amount])

      limit = mapping(section["cancel_rate_limit"], "settings.cancel_rate_limit")
      return unless limit

      %w[enabled max_cancels window unit mode].each do |key|
        require_key(limit, key, "settings.cancel_rate_limit.#{key}")
      end
      boolean(limit["enabled"], "settings.cancel_rate_limit.enabled")
      integer(limit["max_cancels"], "settings.cancel_rate_limit.max_cancels", minimum: 1)
      integer(limit["window"], "settings.cancel_rate_limit.window", minimum: 1)
      enum(limit["unit"], "settings.cancel_rate_limit.unit", %w[minute hour day])
      enum(limit["mode"], "settings.cancel_rate_limit.mode", %w[rolling fixed])
    end

    def validate_providers
      providers = array(@document["providers"], "providers")
      return unless providers

      add("providers", "must contain at least one provider") if providers.empty?
      seen = {}
      providers.each_with_index do |raw_provider, index|
        path = "providers[#{index}]"
        provider = mapping(raw_provider, path)
        next unless provider

        %w[
          provider_key role readiness required_condition callback_url capabilities
          secret_ref enabled
        ].each { |key| require_key(provider, key, "#{path}.#{key}") }

        key = provider["provider_key"]
        enum(key, "#{path}.provider_key", PROVIDER_KEYS)
        if key.is_a?(String)
          add("#{path}.provider_key", "duplicates #{key}") if seen[key]
          seen[key] = true
        end
        enum(provider["role"], "#{path}.role", ROLES)
        enum(provider["readiness"], "#{path}.readiness", READINESS_VALUES)
        string(provider["required_condition"], "#{path}.required_condition")
        validate_callback(provider, path)
        string(provider["secret_ref"], "#{path}.secret_ref", pattern: SECRET_REFERENCE,
               pattern_message: "must be a symbolic secret location")
        boolean(provider["enabled"], "#{path}.enabled")
        validate_capabilities(provider, path)
        validate_easypay_due_diligence(provider, path) if key == "easypay"
      end
    end

    def validate_callback(provider, path)
      raw = provider["callback_url"]
      return unless string(raw, "#{path}.callback_url")

      begin
        uri = URI.parse(raw)
      rescue URI::InvalidURIError
        add("#{path}.callback_url", "must be a valid absolute URL")
        return
      end

      add("#{path}.callback_url", "must use https") unless uri.scheme == "https"
      add("#{path}.callback_url", "must include a host") if blank?(uri.host)
      add("#{path}.callback_url", "must not include user information") if uri.userinfo
      add("#{path}.callback_url", "must not include a query string") if uri.query
      add("#{path}.callback_url", "must not include a fragment") if uri.fragment

      expected = CALLBACK_PATHS[provider["provider_key"]]
      if expected && uri.path != expected
        add("#{path}.callback_url", "path must be #{expected}")
      end
    end

    def validate_capabilities(provider, path)
      capabilities = mapping(provider["capabilities"], "#{path}.capabilities")
      return unless capabilities

      CAPABILITIES.each do |capability|
        require_key(capabilities, capability, "#{path}.capabilities.#{capability}")
        boolean(capabilities[capability], "#{path}.capabilities.#{capability}")
      end
      return unless %w[recommended_conditional live_accepted].include?(provider["readiness"])

      CAPABILITIES.each do |capability|
        unless capabilities[capability] == true
          add("#{path}.capabilities.#{capability}",
              "recommended providers must support this capability")
        end
      end
    end

    def validate_easypay_due_diligence(provider, path)
      due_diligence = mapping(provider["due_diligence"], "#{path}.due_diligence")
      unless due_diligence
        add("#{path}.due_diligence", "is required for EasyPay")
        return
      end

      keys = %w[
        funds_flow_verified settlement_verified freeze_and_refund_verified
        reconciliation_verified continuity_verified
      ]
      keys.each do |key|
        require_key(due_diligence, key, "#{path}.due_diligence.#{key}")
        boolean(due_diligence[key], "#{path}.due_diligence.#{key}")
      end

      eligible = provider["enabled"] || provider["readiness"] != "deferred_due_diligence"
      if eligible && !keys.all? { |key| due_diligence[key] == true }
        add("#{path}.due_diligence",
            "all checks must pass before EasyPay can be recommended or enabled")
      end
    end

    def validate_evidence
      section = @document["evidence"]
      return unless section.is_a?(Hash)

      %w[sub2api_version sub2api_commit source_reference notes].each do |key|
        string(section[key], "evidence.#{key}")
      end
    end

    def validate_cross_field_rules
      settings = @document["settings"]
      providers = @document["providers"]
      return unless settings.is_a?(Hash) && providers.is_a?(Array)

      min = settings["payment_min_amount_cny"]
      max = settings["payment_max_amount_cny"]
      daily = settings["payment_daily_limit_cny"]
      if min.is_a?(Numeric) && max.is_a?(Numeric) && min > max
        add("settings.payment_min_amount_cny", "must not exceed payment_max_amount_cny")
      end
      if daily.is_a?(Numeric) && max.is_a?(Numeric) && daily < max
        add("settings.payment_daily_limit_cny", "must be at least payment_max_amount_cny")
      end

      cny_per_usd = settings["cny_per_usd"]
      multiplier = settings["balance_recharge_multiplier_usd_per_cny"]
      if cny_per_usd.is_a?(Numeric) && cny_per_usd.positive? && multiplier.is_a?(Numeric)
        expected = 1.0 / cny_per_usd
        if (multiplier - expected).abs > 1e-12
          add("settings.balance_recharge_multiplier_usd_per_cny",
              "must equal 1 / settings.cny_per_usd")
        end
      end

      enabled_providers = providers.select { |provider| provider.is_a?(Hash) && provider["enabled"] }
      live_enabled = enabled_providers.any? { |provider| provider["readiness"] == "live_accepted" }
      if settings["payment_enabled"] && !live_enabled
        add("settings.payment_enabled", "requires an enabled live_accepted provider")
      end
      enabled_providers.each_with_index do |provider, _enabled_index|
        next if provider["readiness"] == "live_accepted"

        index = providers.index(provider)
        add("providers[#{index}].enabled", "provider must be live_accepted before enabling")
      end

      types = settings["enabled_payment_types"]
      if settings["payment_enabled"] == false && types.is_a?(Array) && !types.empty?
        add("settings.enabled_payment_types", "must be empty while payment is disabled")
      end
      if settings["payment_enabled"] == false
        enabled_providers.each do |provider|
          index = providers.index(provider)
          add("providers[#{index}].enabled", "must be false while payment is disabled")
        end
      end
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

    def string(value, path, pattern: nil, pattern_message: "has an invalid format")
      unless value.is_a?(String) && !value.strip.empty?
        add(path, "must be a non-empty string") unless value.nil?
        return false
      end
      add(path, pattern_message) if pattern && !value.match?(pattern)
      true
    end

    def enum(value, path, allowed)
      return if value.nil? || allowed.include?(value)

      add(path, "must be one of: #{allowed.join(', ')}")
    end

    def boolean(value, path)
      return if value.nil? || value == true || value == false

      add(path, "must be true or false")
    end

    def integer(value, path, minimum: nil, maximum: nil)
      return if value.nil?
      unless value.is_a?(Integer)
        add(path, "must be an integer")
        return
      end
      add(path, "must be at least #{minimum}") if minimum && value < minimum
      add(path, "must be at most #{maximum}") if maximum && value > maximum
    end

    def number(value, path, minimum: nil)
      return if value.nil?
      unless value.is_a?(Numeric) && value.finite?
        add(path, "must be a finite number")
        return
      end
      add(path, "must be at least #{minimum}") if minimum && value < minimum
    end

    def scan_for_secrets(value, path = nil)
      case value
      when Hash
        value.each do |key, child|
          child_path = path ? "#{path}.#{key}" : key.to_s
          if FORBIDDEN_SECRET_KEYS.include?(key.to_s.downcase)
            add(child_path, "credential fields are forbidden; use secret_ref")
          end
          scan_for_secrets(child, child_path)
        end
      when Array
        value.each_with_index { |child, index| scan_for_secrets(child, "#{path}[#{index}]") }
      when String
        add(path, "value looks like a secret") if SECRET_VALUE_PATTERNS.any? { |pattern| value.match?(pattern) }
      end
    end

    def blank?(value)
      value.nil? || value.to_s.strip.empty?
    end

    def add(path, message)
      @errors << "#{path}: #{message}"
    end
  end

  module OrderSimulator
    EVENT_TYPES = %w[payment_succeeded payment_failed refund_requested refund_succeeded].freeze
    PROVIDER_EVENT_TYPES = %w[payment_succeeded payment_failed refund_succeeded].freeze
    INTERNAL_EVENT_TYPES = %w[refund_requested].freeze
    ORDER_STATUSES = %w[
      PENDING COMPLETED FAILED EXPIRED REFUND_REQUESTED REFUNDED
    ].freeze

    module_function

    def create_order(out_trade_no:, amount_cny:, balance_recharge_multiplier:)
      errors = []
      errors << "order.out_trade_no: must be a non-empty string" if blank?(out_trade_no)
      unless amount_cny.is_a?(Numeric) && amount_cny.finite? && amount_cny.positive?
        errors << "order.amount_cny: must be a positive finite number"
      end
      unless balance_recharge_multiplier.is_a?(Numeric) &&
             balance_recharge_multiplier.finite? && balance_recharge_multiplier.positive?
        errors << "order.balance_recharge_multiplier: must be a positive finite number"
      end
      raise ValidationError, errors unless errors.empty?

      {
        "out_trade_no" => out_trade_no,
        "amount_cny" => amount_cny.to_f,
        "currency" => "CNY",
        "expected_credit_usd" => amount_cny.to_f * balance_recharge_multiplier.to_f,
        "status" => "PENDING",
        "credit_count" => 0,
        "credited_balance_usd" => 0.0,
        "reversal_count" => 0,
        "reversed_balance_usd" => 0.0,
        "processed_events" => {}
      }
    end

    def apply(order, event)
      errors = validate_order(order) + validate_event(order, event)
      raise ValidationError, errors unless errors.empty?

      processed = order.fetch("processed_events")
      event_id = event.fetch("event_id")
      digest = event.fetch("raw_body_sha256")
      if processed.key?(event_id)
        if processed[event_id] != digest
          raise ValidationError,
                ["event.event_id: was already used with a different raw body digest"]
        end
        return { "action" => "duplicate_event_noop", "order" => deep_copy(order) }
      end

      next_order = deep_copy(order)
      next_order.fetch("processed_events")[event_id] = digest
      action = transition(next_order, event.fetch("event_type"))
      { "action" => action, "order" => next_order }
    end

    def validate_order(order)
      return ["order: must be a mapping"] unless order.is_a?(Hash)

      errors = []
      errors << "order.status: is invalid" unless ORDER_STATUSES.include?(order["status"])
      errors << "order.processed_events: must be a mapping" unless order["processed_events"].is_a?(Hash)
      errors
    end

    def validate_event(order, event)
      return ["event: must be a mapping"] unless event.is_a?(Hash)

      errors = []
      required = %w[
        event_id event_type source out_trade_no amount_cny currency occurred_at
        raw_body_sha256 signature_verified
      ]
      required.each do |key|
        errors << "event.#{key}: is required" unless event.key?(key)
      end
      return errors unless errors.empty?

      errors << "event.event_id: must be a non-empty string" if blank?(event["event_id"])
      unless EVENT_TYPES.include?(event["event_type"])
        errors << "event.event_type: is invalid"
      end
      unless %w[provider internal].include?(event["source"])
        errors << "event.source: must be provider or internal"
      end
      if event["source"] == "provider"
        unless PROVIDER_EVENT_TYPES.include?(event["event_type"])
          errors << "event.event_type: is not allowed for provider events"
        end
        unless event["signature_verified"] == true
          errors << "event.signature_verified: provider event was not verified"
        end
      elsif event["source"] == "internal" && !INTERNAL_EVENT_TYPES.include?(event["event_type"])
        errors << "event.event_type: is not allowed for internal events"
      end

      errors << "event.out_trade_no: does not match order" if event["out_trade_no"] != order["out_trade_no"]
      unless event["amount_cny"].is_a?(Numeric) &&
             (event["amount_cny"].to_f - order["amount_cny"].to_f).abs <= 1e-12
        errors << "event.amount_cny: does not match order amount"
      end
      if event["currency"] != order["currency"]
        errors << "event.currency: does not match order currency #{order['currency']}"
      end
      unless event["raw_body_sha256"].is_a?(String) &&
             event["raw_body_sha256"].match?(/\A[0-9a-f]{64}\z/)
        errors << "event.raw_body_sha256: must be a lowercase SHA-256 digest"
      end
      begin
        Time.iso8601(event["occurred_at"].to_s)
      rescue ArgumentError
        errors << "event.occurred_at: must be an ISO 8601 timestamp"
      end
      errors
    end

    def transition(order, event_type)
      case event_type
      when "payment_succeeded"
        complete_payment(order)
      when "payment_failed"
        fail_payment(order)
      when "refund_requested"
        request_refund(order)
      when "refund_succeeded"
        complete_refund(order)
      end
    end

    def complete_payment(order)
      if %w[PENDING FAILED].include?(order["status"])
        order["status"] = "COMPLETED"
        order["credit_count"] += 1
        order["credited_balance_usd"] += order["expected_credit_usd"]
        "payment_completed"
      else
        "already_fulfilled_noop"
      end
    end

    def fail_payment(order)
      if order["status"] == "PENDING"
        order["status"] = "FAILED"
        "payment_failed"
      else
        "out_of_order_event_noop"
      end
    end

    def request_refund(order)
      if order["status"] == "COMPLETED"
        order["status"] = "REFUND_REQUESTED"
        "refund_requested"
      elsif %w[REFUND_REQUESTED REFUNDED].include?(order["status"])
        "refund_request_noop"
      else
        "out_of_order_event_noop"
      end
    end

    def complete_refund(order)
      if order["status"] == "REFUND_REQUESTED"
        order["status"] = "REFUNDED"
        order["reversal_count"] += 1
        order["reversed_balance_usd"] += order["credited_balance_usd"]
        "refund_completed"
      elsif order["status"] == "REFUNDED"
        "already_refunded_noop"
      else
        "out_of_order_event_noop"
      end
    end

    def deep_copy(value)
      Marshal.load(Marshal.dump(value))
    end

    def blank?(value)
      value.nil? || value.to_s.strip.empty?
    end
  end

  module CLI
    module_function

    def run(argv, stdout: $stdout, stderr: $stderr)
      command = argv.shift
      path = argv.shift
      unless %w[validate demo].include?(command) && path && argv.empty?
        stderr.puts "Usage: ruby ops/payment-control-simulator.rb validate|demo PAY01.yaml"
        return 64
      end

      document = YAML.safe_load(File.read(path), aliases: false, filename: path)
      errors = ConfigValidator.new(document).errors
      if command == "validate"
        stdout.puts JSON.pretty_generate(validation_result(document, errors))
        return errors.empty? ? 0 : 1
      end
      unless errors.empty?
        stderr.puts errors.join("\n")
        return 1
      end

      stdout.puts JSON.pretty_generate(demo(document))
      0
    rescue Errno::ENOENT, Psych::Exception => e
      stderr.puts "payment input error: #{e.message}"
      65
    end

    def validation_result(document, errors)
      enabled = document.is_a?(Hash) && document.dig("settings", "payment_enabled") == true
      {
        "payment_id" => document.is_a?(Hash) ? document["payment_id"] : nil,
        "valid" => errors.empty?,
        "errors" => errors,
        "activation_state" => enabled ? "live_configured" : "disabled_simulation_only"
      }
    end

    def demo(document)
      multiplier = document.dig("settings", "balance_recharge_multiplier_usd_per_cny")
      order = OrderSimulator.create_order(
        out_trade_no: "PAY01-SIM-ORDER-0001",
        amount_cny: 72.0,
        balance_recharge_multiplier: multiplier
      )
      events = demo_events
      actions = events.map do |event|
        result = OrderSimulator.apply(order, event)
        order = result.fetch("order")
        result.fetch("action")
      end
      {
        "mode" => "offline_simulation",
        "real_payment_sent" => false,
        "actions" => actions,
        "final_order" => order.reject { |key, _value| key == "processed_events" }
      }
    end

    def demo_events
      base = {
        "out_trade_no" => "PAY01-SIM-ORDER-0001",
        "amount_cny" => 72.0,
        "currency" => "CNY",
        "occurred_at" => "2026-07-15T00:01:00Z"
      }
      [
        base.merge(
          "event_id" => "EVT-PAY-SUCCESS-0001",
          "event_type" => "payment_succeeded",
          "source" => "provider",
          "raw_body_sha256" => "a" * 64,
          "signature_verified" => true
        ),
        base.merge(
          "event_id" => "EVT-PAY-SUCCESS-0001",
          "event_type" => "payment_succeeded",
          "source" => "provider",
          "raw_body_sha256" => "a" * 64,
          "signature_verified" => true
        ),
        base.merge(
          "event_id" => "EVT-REFUND-REQUEST-0001",
          "event_type" => "refund_requested",
          "source" => "internal",
          "raw_body_sha256" => "b" * 64,
          "signature_verified" => false
        ),
        base.merge(
          "event_id" => "EVT-REFUND-SUCCESS-0001",
          "event_type" => "refund_succeeded",
          "source" => "provider",
          "raw_body_sha256" => "c" * 64,
          "signature_verified" => true
        ),
        base.merge(
          "event_id" => "EVT-REFUND-SUCCESS-0001",
          "event_type" => "refund_succeeded",
          "source" => "provider",
          "raw_body_sha256" => "c" * 64,
          "signature_verified" => true
        )
      ]
    end
  end
end

exit PaymentControl::CLI.run(ARGV) if __FILE__ == $PROGRAM_NAME
