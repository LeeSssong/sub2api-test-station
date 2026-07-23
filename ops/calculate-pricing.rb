#!/usr/bin/env ruby
# frozen_string_literal: true

require "bigdecimal"
require "json"
require "optparse"
require "yaml"
require_relative "validate-upstream"

class PricingCalculator
  CATEGORIES = %w[input output cached_input cache_write].freeze
  CHANNEL_FIELDS = {
    "input" => "input_price",
    "output" => "output_price",
    "cached_input" => "cache_read_price",
    "cache_write" => "cache_write_price"
  }.freeze

  class ValidationError < StandardError
    attr_reader :errors

    def initialize(errors)
      @errors = errors
      super(errors.join("; "))
    end
  end

  def initialize(upstream:, scenario:)
    @upstream = upstream
    @scenario = scenario
    @errors = []
  end

  def calculate
    validate
    raise ValidationError, @errors unless @errors.empty?

    build_result
  end

  private

  def validate
    upstream_errors = UpstreamConfigValidator.new(@upstream).errors
    @errors.concat(upstream_errors.map { |error| "upstream.#{error}" })
    unless @scenario.is_a?(Hash)
      add("scenario", "must be a mapping")
      return
    end

    validate_scenario_header
    validate_currencies
    validate_assumptions
    validate_fixed_costs
    validate_model_mix
  end

  def validate_scenario_header
    %w[
      schema_version scenario_id status source_upstream_id currencies assumptions
      monthly_fixed_costs model_mix
    ].each { |key| required(@scenario, key, key) }
    integer(@scenario["schema_version"], "schema_version", minimum: 1, maximum: 1)
    string(@scenario["scenario_id"], "scenario_id")
    enum(@scenario["status"], "status", %w[fictional draft ready_for_decision])
    string(@scenario["source_upstream_id"], "source_upstream_id")

    if @upstream.is_a?(Hash) && @scenario["source_upstream_id"] != @upstream["upstream_id"]
      add("source_upstream_id", "must match upstream_id #{@upstream['upstream_id']}")
    end
  end

  def validate_currencies
    currencies = mapping(@scenario["currencies"], "currencies")
    return unless currencies

    %w[public sub2api_balance cny_per_usd].each do |key|
      required(currencies, key, "currencies.#{key}")
    end
    enum(currencies["public"], "currencies.public", ["CNY"])
    enum(currencies["sub2api_balance"], "currencies.sub2api_balance", ["USD"])
    number(currencies["cny_per_usd"], "currencies.cny_per_usd", minimum: 0.000001)
  end

  def validate_assumptions
    assumptions = mapping(@scenario["assumptions"], "assumptions")
    return unless assumptions

    %w[
      target_fully_loaded_margin_rate payment_fee_rate failure_compensation_rate
      rounding_increment_cny_per_1m
    ].each { |key| required(assumptions, key, "assumptions.#{key}") }

    margin = assumptions["target_fully_loaded_margin_rate"]
    fee = assumptions["payment_fee_rate"]
    number(margin, "assumptions.target_fully_loaded_margin_rate", minimum: 0.2, maximum: 0.95)
    number(fee, "assumptions.payment_fee_rate", minimum: 0, maximum: 0.5)
    number(assumptions["failure_compensation_rate"],
           "assumptions.failure_compensation_rate", minimum: 0, maximum: 1)
    number(assumptions["rounding_increment_cny_per_1m"],
           "assumptions.rounding_increment_cny_per_1m", minimum: 0.000001)

    if margin.is_a?(Numeric) && fee.is_a?(Numeric) && margin + fee >= 1
      add("assumptions", "target margin plus payment fee must be less than 1")
    end
  end

  def validate_fixed_costs
    costs = array(@scenario["monthly_fixed_costs"], "monthly_fixed_costs")
    return unless costs

    add("monthly_fixed_costs", "must contain at least one item") if costs.empty?
    seen = {}
    costs.each_with_index do |raw_cost, index|
      path = "monthly_fixed_costs[#{index}]"
      cost = mapping(raw_cost, path)
      next unless cost

      %w[id amount_cny].each { |key| required(cost, key, "#{path}.#{key}") }
      identifier = cost["id"]
      string(identifier, "#{path}.id")
      number(cost["amount_cny"], "#{path}.amount_cny", minimum: 0)
      if identifier.is_a?(String)
        if seen[identifier]
          add("#{path}.id", "duplicates #{identifier}")
        else
          seen[identifier] = true
        end
      end
    end
  end

  def validate_model_mix
    mixes = array(@scenario["model_mix"], "model_mix")
    return unless mixes

    add("model_mix", "must contain at least one model") if mixes.empty?
    models = enabled_models
    seen = {}
    total_tokens = 0

    mixes.each_with_index do |raw_mix, index|
      path = "model_mix[#{index}]"
      mix = mapping(raw_mix, path)
      next unless mix

      %w[public_name monthly_tokens].each { |key| required(mix, key, "#{path}.#{key}") }
      name = mix["public_name"]
      string(name, "#{path}.public_name")
      if name.is_a?(String)
        if seen[name]
          add("#{path}.public_name", "duplicates #{name}")
        else
          seen[name] = true
        end
      end
      model = models[name]
      add("#{path}.public_name", "is not an enabled model in #{@upstream['upstream_id']}") unless model

      tokens = mapping(mix["monthly_tokens"], "#{path}.monthly_tokens")
      next unless tokens

      CATEGORIES.each do |category|
        token_path = "#{path}.monthly_tokens.#{category}"
        required(tokens, category, token_path)
        count = tokens[category]
        integer(count, token_path, minimum: 0)
        total_tokens += count if count.is_a?(Integer) && count >= 0
        next unless model && count.is_a?(Integer) && count.positive?

        if model.dig("pricing", category).nil?
          add(token_path, "forecast is nonzero but upstream price is unknown")
        end
      end
    end
    add("model_mix", "forecast token total must be greater than zero") if total_tokens.zero?
  end

  def enabled_models
    return {} unless @upstream.is_a?(Hash) && @upstream["models"].is_a?(Array)

    @upstream["models"].each_with_object({}) do |model, result|
      next unless model.is_a?(Hash) && model["enabled"] == true

      currency = model.dig("pricing", "currency")
      unless %w[CNY USD].include?(currency)
        add("upstream.models.#{model['public_name']}.pricing.currency", "must be CNY or USD")
      end
      result[model["public_name"]] = model
    end
  end

  def build_result
    fx = decimal(@scenario.dig("currencies", "cny_per_usd"))
    assumptions = @scenario["assumptions"]
    margin = decimal(assumptions["target_fully_loaded_margin_rate"])
    payment_fee = decimal(assumptions["payment_fee_rate"])
    compensation = decimal(assumptions["failure_compensation_rate"])
    increment = decimal(assumptions["rounding_increment_cny_per_1m"])
    fixed_cost = @scenario["monthly_fixed_costs"].sum do |item|
      decimal(item["amount_cny"])
    end
    total_tokens = @scenario["model_mix"].sum do |mix|
      CATEGORIES.sum { |category| mix.dig("monthly_tokens", category) }
    end
    fixed_per_million = fixed_cost / decimal(total_tokens) * decimal(1_000_000)
    denominator = decimal(1) - margin - payment_fee
    models = enabled_models

    total_revenue = decimal(0)
    total_upstream_cost = decimal(0)
    output_models = @scenario["model_mix"].map do |mix|
      upstream_model = models.fetch(mix["public_name"])
      pricing = upstream_model.fetch("pricing")
      source_prices = {}
      public_prices = {}
      channel_prices = {}

      CATEGORIES.each do |category|
        source_value = pricing[category]
        source_cny = source_value.nil? ? nil : price_to_cny(source_value, pricing["currency"], fx)
        public_price = if source_cny.nil?
                         nil
                       else
                         loaded = source_cny * (decimal(1) + compensation) + fixed_per_million
                         ceil_to_increment(loaded / denominator, increment)
                       end
        source_prices[category] = decimal_to_number(source_cny)
        public_prices[category] = decimal_to_number(public_price)
        channel_prices[CHANNEL_FIELDS.fetch(category)] = if public_price.nil?
                                                           nil
                                                         else
                                                           decimal_to_number(public_price / fx / decimal(1_000_000))
                                                         end

        token_count = mix.dig("monthly_tokens", category)
        units = decimal(token_count) / decimal(1_000_000)
        total_upstream_cost += source_cny * units if source_cny
        total_revenue += public_price * units if public_price
      end

      {
        "public_name" => mix["public_name"],
        "upstream_name" => upstream_model["upstream_name"],
        "monthly_tokens" => mix["monthly_tokens"],
        "upstream_cost_cny_per_1m" => source_prices,
        "public_price_cny_per_1m" => public_prices,
        "sub2api_usd_per_token" => channel_prices
      }
    end

    compensation_cost = total_upstream_cost * compensation
    payment_cost = total_revenue * payment_fee
    profit = total_revenue - total_upstream_cost - compensation_cost - payment_cost - fixed_cost
    actual_margin = total_revenue.zero? ? decimal(0) : profit / total_revenue

    {
      "schema_version" => 1,
      "scenario_id" => @scenario["scenario_id"],
      "status" => @scenario["status"],
      "source_upstream_id" => @upstream["upstream_id"],
      "fixed_cost_cny_per_1m_tokens" => decimal_to_number(fixed_per_million),
      "models" => output_models,
      "forecast_summary" => {
        "monthly_tokens" => total_tokens,
        "monthly_revenue_cny" => decimal_to_number(total_revenue),
        "monthly_upstream_cost_cny" => decimal_to_number(total_upstream_cost),
        "monthly_compensation_reserve_cny" => decimal_to_number(compensation_cost),
        "monthly_payment_fee_cny" => decimal_to_number(payment_cost),
        "monthly_fixed_cost_cny" => decimal_to_number(fixed_cost),
        "monthly_fully_loaded_profit_cny" => decimal_to_number(profit),
        "fully_loaded_margin_rate" => decimal_to_number(actual_margin),
        "target_fully_loaded_margin_rate" => assumptions["target_fully_loaded_margin_rate"]
      },
      "sub2api_recommendation" => build_sub2api_recommendation(output_models, fx)
    }
  end

  def build_sub2api_recommendation(models, fx)
    platform = @upstream.dig("sub2api", "platform")
    mappings = {}
    pricing = models.map do |model|
      mappings[model["public_name"]] = model["upstream_name"]
      prices = model["sub2api_usd_per_token"]
      {
        "platform" => platform,
        "models" => [model["public_name"]],
        "billing_mode" => "token",
        "input_price" => prices["input_price"],
        "output_price" => prices["output_price"],
        "cache_write_price" => prices["cache_write_price"],
        "cache_read_price" => prices["cache_read_price"],
        "image_output_price" => nil,
        "per_request_price" => nil,
        "intervals" => []
      }
    end

    {
      "balance_recharge_multiplier_usd_per_cny" => decimal_to_number(decimal(1) / fx),
      "group_rate_multiplier" => 1.0,
      "account_rate_multiplier" => 1.0,
      "channel" => {
        "billing_model_source" => "requested",
        "restrict_models" => true,
        "model_mapping" => { platform => mappings },
        "model_pricing" => pricing
      }
    }
  end

  def price_to_cny(value, currency, fx)
    price = decimal(value)
    currency == "USD" ? price * fx : price
  end

  def ceil_to_increment(value, increment)
    (value / increment).ceil * increment
  end

  def decimal(value)
    BigDecimal(value.to_s)
  end

  def decimal_to_number(value)
    value.nil? ? nil : value.to_f
  end

  def required(hash, key, path)
    add(path, "is required") unless hash.is_a?(Hash) && hash.key?(key)
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

  def enum(value, path, allowed)
    return true if allowed.include?(value)

    add(path, "must be one of: #{allowed.join(', ')}") unless value.nil?
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

  def number(value, path, minimum: nil, maximum: nil)
    unless value.is_a?(Numeric) && value.finite?
      add(path, "must be a finite number") unless value.nil?
      return false
    end
    add(path, "must be at least #{minimum}") if minimum && value < minimum
    add(path, "must be at most #{maximum}") if maximum && value > maximum
    true
  end

  def add(path, message)
    @errors << "#{path}: #{message}"
  end
end

def load_pricing_yaml(path)
  YAML.safe_load(
    File.read(path),
    permitted_classes: [],
    permitted_symbols: [],
    aliases: false,
    filename: path
  )
end

def pricing_markdown(result)
  summary = result.fetch("forecast_summary")
  lines = []
  lines << "# Pricing Simulation: #{result['scenario_id']}"
  lines << ""
  lines << "> Status: `#{result['status']}`. This output is an offline simulation, not an approved public price."
  lines << ""
  lines << "| Model | Input CNY/1M | Output CNY/1M | Cache read CNY/1M | Cache write CNY/1M |"
  lines << "|---|---:|---:|---:|---:|"
  result.fetch("models").each do |model|
    price = model.fetch("public_price_cny_per_1m")
    values = %w[input output cached_input cache_write].map do |key|
      price[key].nil? ? "unpriced" : format("%.6f", price[key])
    end
    lines << "| #{model['public_name']} | #{values.join(' | ')} |"
  end
  lines << ""
  lines << "- Forecast monthly revenue: CNY #{format('%.4f', summary['monthly_revenue_cny'])}"
  lines << "- Forecast fully loaded profit: CNY #{format('%.4f', summary['monthly_fully_loaded_profit_cny'])}"
  lines << "- Forecast fully loaded margin: #{format('%.2f%%', summary['fully_loaded_margin_rate'] * 100)}"
  lines << "- Target margin: #{format('%.2f%%', summary['target_fully_loaded_margin_rate'] * 100)}"
  lines << "- Future CNY recharge multiplier: 1 CNY = #{format('%.8f', result.dig('sub2api_recommendation', 'balance_recharge_multiplier_usd_per_cny'))} USD balance"
  lines << "- Channel billing model source: `requested`; exact channel pricing and both multipliers use 1.0"
  lines.join("\n") + "\n"
end

if $PROGRAM_NAME == __FILE__
  options = { format: "markdown" }
  parser = OptionParser.new do |opts|
    opts.banner = "Usage: ruby ops/calculate-pricing.rb --upstream FILE --scenario FILE [--format markdown|json]"
    opts.on("--upstream FILE", "Validated upstream YAML") { |value| options[:upstream] = value }
    opts.on("--scenario FILE", "Pricing scenario YAML") { |value| options[:scenario] = value }
    opts.on("--format FORMAT", %w[markdown json], "Output format") { |value| options[:format] = value }
  end

  begin
    parser.parse!
    unless options[:upstream] && options[:scenario]
      warn parser
      exit 2
    end
    result = PricingCalculator.new(
      upstream: load_upstream_yaml(options[:upstream]),
      scenario: load_pricing_yaml(options[:scenario])
    ).calculate
    puts(options[:format] == "json" ? JSON.pretty_generate(result) : pricing_markdown(result))
  rescue OptionParser::ParseError, Errno::ENOENT, Errno::EACCES, Psych::Exception => e
    warn e.message
    exit 2
  rescue PricingCalculator::ValidationError => e
    warn "pricing input is invalid"
    e.errors.each { |error| warn "  - #{error}" }
    exit 1
  end
end
