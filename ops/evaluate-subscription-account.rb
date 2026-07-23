#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "yaml"

class SubscriptionAccountEvaluator
  ROOT_KEYS = %w[
    schema_version candidate_id status reviewed_at seller listing entitlement
    control authorization operations evidence
  ].freeze
  STATUS_VALUES = %w[fictional draft candidate_verified].freeze
  PLATFORM_VALUES = %w[openai anthropic gemini antigravity grok unknown].freeze
  OWNERSHIP_VALUES = %w[independent shared managed unknown].freeze
  ORIGIN_VALUES = %w[new transferred invited unknown].freeze
  ACCOUNT_TYPE_VALUES = %w[oauth setup-token].freeze
  DELIVERY_VALUES = %w[
    account_control oauth_handoff setup_token_flow token_only cookie_only
    browser_profile unknown
  ].freeze
  REPLACEMENT_VALUES = %w[refund_or_replace replace_only refund_only none unknown].freeze
  SUPPORTED_ACCOUNT_TYPES = {
    "openai" => %w[oauth],
    "anthropic" => %w[oauth setup-token],
    "gemini" => %w[oauth],
    "antigravity" => %w[oauth],
    "grok" => %w[oauth]
  }.freeze
  SAFE_DELIVERY_VALUES = %w[account_control oauth_handoff setup_token_flow].freeze
  FORBIDDEN_CREDENTIAL_KEYS = %w[
    api_key access_token refresh_token id_token bearer_token token password passwd
    cookie cookies session_key private_key secret_key client_secret
    credential credentials two_factor_recovery_codes 2fa_recovery_codes card_number
    payment_card cvv
  ].freeze
  SECRET_VALUE_PATTERNS = [
    /-----BEGIN [A-Z ]*PRIVATE KEY-----/,
    /\bBearer\s+[A-Za-z0-9._~+\/=:-]{16,}/i,
    /\bsk[-_](?:live[-_]|test[-_]|proj[-_])?[A-Za-z0-9_-]{16,}\b/i,
    /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/,
    /\bAKIA[A-Z0-9]{16}\b/,
    /\bAIza[A-Za-z0-9_-]{20,}\b/
  ].freeze

  SECTION_KEYS = {
    "seller" => %w[public_name storefront_reference support_days replacement_policy],
    "listing" => %w[public_label price_cny currency quantity remaining_days],
    "entitlement" => %w[
      platform official_product official_tier ownership_mode account_origin
      organization_managed
    ],
    "control" => %w[
      buyer_controls_primary_email buyer_controls_recovery buyer_controls_password
      buyer_controls_2fa seller_retains_recovery
    ],
    "authorization" => %w[
      sub2api_platform sub2api_account_type normal_authorization_only delivery_mode
      requires_credential_extraction requires_bypass
    ],
    "operations" => %w[
      isolated_group individually_disableable cost_traceable auto_pause_on_expiry
      initial_concurrency
    ],
    "evidence" => %w[listing_snapshot_ref terms_snapshot_ref notes]
  }.freeze

  def initialize(document)
    @document = document
    @errors = []
  end

  def evaluate
    validate
    return invalid_result unless @errors.empty?

    rejections = hard_rejections
    return result(decision: "rejected", hard_rejections: rejections) unless rejections.empty?

    breakdown = score_breakdown
    score = breakdown.values.sum
    decision = if score >= 85
                 "recommended"
               elsif score >= 70
                 "conditional"
               else
                 "not_recommended"
               end

    result(
      decision: decision,
      hard_rejections: [],
      score: score,
      score_breakdown: breakdown
    )
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
    string(@document["candidate_id"], "candidate_id", pattern: /\AACC-CANDIDATE-[A-Z0-9-]+\z/)
    enum(@document["status"], "status", STATUS_VALUES)
    string(@document["reviewed_at"], "reviewed_at", pattern: /\A\d{4}-\d{2}-\d{2}\z/)

    SECTION_KEYS.each do |section_name, keys|
      section = mapping(@document[section_name], section_name)
      next unless section

      keys.each { |key| require_key(section, key, "#{section_name}.#{key}") }
    end

    validate_seller
    validate_listing
    validate_entitlement
    validate_control
    validate_authorization
    validate_operations
    validate_evidence
    scan_for_secrets(@document)
  end

  def validate_seller
    section = @document["seller"]
    return unless section.is_a?(Hash)

    string(section["public_name"], "seller.public_name")
    string(section["storefront_reference"], "seller.storefront_reference")
    integer(section["support_days"], "seller.support_days", minimum: 0)
    enum(section["replacement_policy"], "seller.replacement_policy", REPLACEMENT_VALUES)
  end

  def validate_listing
    section = @document["listing"]
    return unless section.is_a?(Hash)

    string(section["public_label"], "listing.public_label")
    number(section["price_cny"], "listing.price_cny", minimum: 0)
    enum(section["currency"], "listing.currency", %w[CNY])
    integer(section["quantity"], "listing.quantity", minimum: 1)
    integer(section["remaining_days"], "listing.remaining_days", minimum: 0)
  end

  def validate_entitlement
    section = @document["entitlement"]
    return unless section.is_a?(Hash)

    enum(section["platform"], "entitlement.platform", PLATFORM_VALUES)
    string(section["official_product"], "entitlement.official_product")
    string(section["official_tier"], "entitlement.official_tier")
    enum(section["ownership_mode"], "entitlement.ownership_mode", OWNERSHIP_VALUES)
    enum(section["account_origin"], "entitlement.account_origin", ORIGIN_VALUES)
    boolean(section["organization_managed"], "entitlement.organization_managed")
  end

  def validate_control
    section = @document["control"]
    return unless section.is_a?(Hash)

    SECTION_KEYS.fetch("control").each do |key|
      boolean(section[key], "control.#{key}")
    end
  end

  def validate_authorization
    section = @document["authorization"]
    return unless section.is_a?(Hash)

    enum(section["sub2api_platform"], "authorization.sub2api_platform", PLATFORM_VALUES - ["unknown"])
    enum(section["sub2api_account_type"], "authorization.sub2api_account_type", ACCOUNT_TYPE_VALUES)
    boolean(section["normal_authorization_only"], "authorization.normal_authorization_only")
    enum(section["delivery_mode"], "authorization.delivery_mode", DELIVERY_VALUES)
    boolean(section["requires_credential_extraction"], "authorization.requires_credential_extraction")
    boolean(section["requires_bypass"], "authorization.requires_bypass")
  end

  def validate_operations
    section = @document["operations"]
    return unless section.is_a?(Hash)

    %w[isolated_group individually_disableable cost_traceable auto_pause_on_expiry].each do |key|
      boolean(section[key], "operations.#{key}")
    end
    integer(section["initial_concurrency"], "operations.initial_concurrency", minimum: 1)
  end

  def validate_evidence
    section = @document["evidence"]
    return unless section.is_a?(Hash)

    %w[listing_snapshot_ref terms_snapshot_ref notes].each do |key|
      string(section[key], "evidence.#{key}")
    end
  end

  def hard_rejections
    entitlement = @document.fetch("entitlement")
    control = @document.fetch("control")
    authorization = @document.fetch("authorization")
    listing = @document.fetch("listing")
    rejections = []

    unresolved = entitlement["platform"] == "unknown" ||
                 unresolved_label?(entitlement["official_product"]) ||
                 unresolved_label?(entitlement["official_tier"])
    rejections << "unresolved_entitlement" if unresolved
    rejections << "shared_or_managed_account" unless entitlement["ownership_mode"] == "independent"
    rejections << "organization_managed_account" if entitlement["organization_managed"]

    buyer_control_keys = %w[
      buyer_controls_primary_email buyer_controls_recovery buyer_controls_password
      buyer_controls_2fa
    ]
    rejections << "incomplete_buyer_control" unless buyer_control_keys.all? { |key| control[key] }
    rejections << "seller_retains_recovery" if control["seller_retains_recovery"]

    safe_delivery = SAFE_DELIVERY_VALUES.include?(authorization["delivery_mode"]) &&
                    authorization["normal_authorization_only"]
    rejections << "unsupported_delivery_mode" unless safe_delivery
    if authorization["requires_credential_extraction"]
      rejections << "credential_extraction_required"
    end
    rejections << "bypass_required" if authorization["requires_bypass"]
    rejections << "unsupported_sub2api_mapping" unless supported_mapping?(entitlement, authorization)
    rejections << "sample_budget_exceeded" if listing["price_cny"] > 300
    rejections << "bulk_first_purchase_required" unless listing["quantity"] == 1
    rejections
  end

  def supported_mapping?(entitlement, authorization)
    platform = entitlement["platform"]
    platform == authorization["sub2api_platform"] &&
      SUPPORTED_ACCOUNT_TYPES.fetch(platform, []).include?(authorization["sub2api_account_type"]) &&
      authorization["normal_authorization_only"]
  end

  def score_breakdown
    {
      "account_control" => 30,
      "authorization_compatibility" => 25,
      "support_and_traceability" => support_score,
      "sample_economics" => economics_score,
      "operational_isolation" => operations_score
    }
  end

  def support_score
    seller = @document.fetch("seller")
    evidence = @document.fetch("evidence")
    score = if seller["support_days"] >= 14
              10
            elsif seller["support_days"] >= 7
              7
            elsif seller["support_days"].positive?
              3
            else
              0
            end
    score += case seller["replacement_policy"]
             when "refund_or_replace" then 5
             when "replace_only", "refund_only" then 3
             else 0
             end
    if evidence_reference?(evidence["listing_snapshot_ref"]) &&
       evidence_reference?(evidence["terms_snapshot_ref"])
      score += 5
    end
    score
  end

  def economics_score
    listing = @document.fetch("listing")
    score = listing["quantity"] == 1 ? 5 : 0
    score += if listing["price_cny"] <= 200
               5
             elsif listing["price_cny"] <= 300
               3
             else
               0
             end
    score += if listing["remaining_days"] >= 30
               5
             elsif listing["remaining_days"] >= 14
               3
             else
               0
             end
    score
  end

  def operations_score
    operations = @document.fetch("operations")
    score = 0
    score += 3 if operations["isolated_group"]
    score += 3 if operations["individually_disableable"]
    score += 2 if operations["cost_traceable"]
    score += 1 if operations["auto_pause_on_expiry"]
    score += 1 if operations["initial_concurrency"] == 1
    score
  end

  def invalid_result
    result(decision: "invalid", valid: false, hard_rejections: [])
  end

  def result(decision:, hard_rejections:, valid: true, score: nil, score_breakdown: nil)
    {
      "candidate_id" => candidate_id,
      "status" => @document.is_a?(Hash) ? @document["status"] : nil,
      "asset_state" => "assumed_not_purchased",
      "valid" => valid,
      "errors" => @errors.uniq,
      "hard_rejections" => hard_rejections.uniq,
      "score" => score,
      "score_breakdown" => score_breakdown,
      "decision" => decision
    }
  end

  def candidate_id
    return nil unless @document.is_a?(Hash)

    @document["candidate_id"]
  end

  def require_key(mapping, key, path)
    add(path, "is required") unless mapping.key?(key)
  end

  def mapping(value, path)
    return value if value.is_a?(Hash)

    add(path, "must be a mapping") unless value.nil?
    nil
  end

  def string(value, path, pattern: nil)
    unless value.is_a?(String) && !value.strip.empty?
      add(path, "must be a non-empty string") unless value.nil?
      return false
    end
    add(path, "has an invalid format") if pattern && !value.match?(pattern)
    true
  end

  def enum(value, path, allowed)
    return if value.nil?
    return if allowed.include?(value)

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
        if FORBIDDEN_CREDENTIAL_KEYS.include?(key.to_s.downcase)
          add(child_path, "credential fields are forbidden")
        end
        scan_for_secrets(child, child_path)
      end
    when Array
      value.each_with_index { |child, index| scan_for_secrets(child, "#{path}[#{index}]") }
    when String
      add(path, "value looks like a secret") if SECRET_VALUE_PATTERNS.any? { |pattern| value.match?(pattern) }
    end
  end

  def unresolved_label?(value)
    %w[unknown unverified k12].include?(value.to_s.strip.downcase)
  end

  def evidence_reference?(value)
    value.is_a?(String) && !value.strip.empty? && value.strip.downcase != "unknown"
  end

  def add(path, message)
    @errors << "#{path}: #{message}"
  end
end

module SubscriptionAccountCLI
  module_function

  def run(argv, stdout: $stdout, stderr: $stderr)
    command = argv.shift
    paths = argv
    unless command == "compare" && !paths.empty?
      stderr.puts "Usage: ruby ops/evaluate-subscription-account.rb compare CANDIDATE.yaml [...]"
      return 64
    end

    candidates = paths.map do |path|
      document = YAML.safe_load(File.read(path), aliases: false, filename: path)
      SubscriptionAccountEvaluator.new(document).evaluate.merge("source_file" => File.basename(path))
    end
    decisions = candidates.group_by { |candidate| candidate.fetch("decision") }
    summary = %w[recommended conditional not_recommended rejected invalid].to_h do |decision|
      [decision, decisions.fetch(decision, []).length]
    end

    stdout.puts JSON.pretty_generate(
      "asset_state" => "assumed_not_purchased",
      "candidates" => candidates,
      "summary" => summary
    )
    0
  rescue Errno::ENOENT, Psych::Exception => e
    stderr.puts "candidate input error: #{e.message}"
    65
  end
end

exit SubscriptionAccountCLI.run(ARGV) if __FILE__ == $PROGRAM_NAME
