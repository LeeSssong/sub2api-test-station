# frozen_string_literal: true

require "digest"
require "json"
require "time"
require_relative "model-release-policy"

module ModelRelease
  class Evaluator
    MAX_AGE_SECONDS = 20 * 60
    REQUIRED_MODES = {
      "relay_ops_mode" => "read_only",
      "feishu_command_mode" => "dry_run",
      "d04_mode" => "read_only",
      "registration_open" => false
    }.freeze
    FORBIDDEN_KEY = /\A(?:api[_-]?key|token|cookie|authorization|password|secret|credentials?|model[_-]?output)\z/i
    ROOT_KEYS = %w[
      account_set_sha256
      accounts
      base_config_sha256
      base_configuration
      captured_at
      modes
      pricing
      public_groups
      published
      schema_version
      snapshot_id
    ].freeze
    ACCOUNT_KEYS = %w[
      account_id
      balance_usd
      discovered_models
      discovery_recorded_at
      financial_recorded_at
      group_ids
      qualifications
      quality_recorded_at
      quality_source
      error_rate
      sample_count
      schedulable
      status
      success_rate
      total_latency_p95_ms
      ttft_p95_ms
    ].freeze
    QUALIFICATION_KEYS = %w[
      sse_attempts
      sse_successes
      sse_terminal_events
      sync_attempts
      sync_successes
    ].freeze

    def initialize(policy:, now: Time.now.utc)
      @policy = policy
      @now = now.utc
    end

    def evaluate(snapshot)
      validate_snapshot!(snapshot)
      accounts = snapshot.fetch("accounts").sort_by { |account| account.fetch("account_id") }
      published = snapshot.fetch("published")
      decision = @policy.candidate_set(
        discovered_models: accounts.flat_map { |account| account.fetch("discovered_models") },
        published_models: published.fetch("models")
      )
      candidate_models = decision.candidate_models
      blockers = []

      blockers << "account_set_hash_mismatch" unless secure_equal(
        snapshot.fetch("account_set_sha256"), account_set_hash(accounts)
      )
      blockers << "base_config_hash_mismatch" unless secure_equal(
        snapshot.fetch("base_config_sha256"), Canonical.sha256(snapshot.fetch("base_configuration"))
      )
      blockers << "unsafe_operating_mode" unless snapshot.fetch("modes") == REQUIRED_MODES

      account_views = accounts.map do |account|
        account_blockers(account).each { |reason| blockers << reason }
        qualified_models = candidate_models.select { |model_id| qualified?(account, model_id) }
        {
          "account_id" => account.fetch("account_id"),
          "group_ids" => account.fetch("group_ids").sort,
          "qualified_models" => qualified_models
        }
      end
      group_views = snapshot.fetch("public_groups").sort_by { |group| group.fetch("group_id") }.map do |group|
        covered = account_views.select { |account| account.fetch("group_ids").include?(group.fetch("group_id")) }
                               .flat_map { |account| account.fetch("qualified_models") }.uniq.sort
        missing = candidate_models - covered
        blockers << "group_model_coverage_incomplete" unless missing.empty?
        {
          "group_id" => group.fetch("group_id"),
          "name" => group.fetch("name"),
          "covered" => missing.empty?,
          "covered_models" => covered,
          "missing_models" => missing
        }
      end

      priced = snapshot.fetch("pricing").select { |item| item.fetch("complete") }.map { |item| item.fetch("model_id") }
      blockers << "model_pricing_incomplete" unless (candidate_models - priced).empty?
      blockers.concat(decision.reason_codes) if decision.status == "待确认"
      blockers = blockers.uniq.sort

      result = {
        "schema_version" => 1,
        "proposal_id" => "",
        "snapshot_id" => snapshot.fetch("snapshot_id"),
        "evaluated_at" => @now.iso8601,
        "status" => result_status(decision, blockers),
        "account_set_sha256" => snapshot.fetch("account_set_sha256"),
        "base_config_sha256" => snapshot.fetch("base_config_sha256"),
        "published" => {
          "families" => published.fetch("families").sort_by { |family| family.split(".").map(&:to_i) },
          "models" => published.fetch("models").sort
        },
        "candidate" => {
          "family" => decision.candidate_family,
          "families" => candidate_models.map { |model| @policy.classify(model).family }.uniq.sort,
          "models" => candidate_models,
          "review_models" => decision.review_models
        },
        "groups" => group_views,
        "accounts" => account_views,
        "evidence" => {
          "captured_at" => snapshot.fetch("captured_at"),
          "freshness_minutes" => 20,
          "balance_min_usd" => 5.0,
          "quality_samples_min" => 20
        },
        "blockers" => blockers
      }
      result["proposal_id"] = Canonical.sha256(result.reject { |key, _| key == "proposal_id" })
      result
    end

    private

    def validate_snapshot!(snapshot)
      reject_forbidden_keys!(snapshot)
      exact_keys!(snapshot, ROOT_KEYS, "snapshot")
      raise ValidationError, "schema_version must be 1" unless snapshot["schema_version"] == 1
      text!(snapshot["snapshot_id"], "snapshot_id")
      timestamp!(snapshot["captured_at"], "captured_at")
      hash64!(snapshot["account_set_sha256"], "account_set_sha256")
      hash64!(snapshot["base_config_sha256"], "base_config_sha256")
      exact_keys!(snapshot["modes"], REQUIRED_MODES.keys, "modes")
      validate_published!(snapshot["published"])
      validate_groups!(snapshot["public_groups"])
      validate_accounts!(snapshot["accounts"])
      validate_pricing!(snapshot["pricing"])
      validate_base_configuration!(snapshot["base_configuration"])
    end

    def validate_published!(published)
      exact_keys!(published, %w[families models], "published")
      string_list!(published["families"], "published.families", allow_empty: true)
      string_list!(published["models"], "published.models", allow_empty: true)
    end

    def validate_groups!(groups)
      array!(groups, "public_groups", allow_empty: false, max: 100)
      ids = groups.map.with_index do |group, index|
        exact_keys!(group, %w[group_id name], "public_groups[#{index}]")
        positive_integer!(group["group_id"], "public_groups[#{index}].group_id")
        text!(group["name"], "public_groups[#{index}].name")
        group["group_id"]
      end
      raise ValidationError, "public_groups contains duplicate IDs" unless ids.uniq.length == ids.length
    end

    def validate_accounts!(accounts)
      array!(accounts, "accounts", allow_empty: false, max: 2000)
      ids = accounts.map.with_index do |account, index|
        path = "accounts[#{index}]"
        exact_keys!(account, ACCOUNT_KEYS, path)
        positive_integer!(account["account_id"], "#{path}.account_id")
        raise ValidationError, "#{path} is not active and schedulable" unless account["status"] == "active" && account["schedulable"] == true
        integer_list!(account["group_ids"], "#{path}.group_ids")
        string_list!(account["discovered_models"], "#{path}.discovered_models", allow_empty: false)
        %w[discovery_recorded_at financial_recorded_at quality_recorded_at].each do |field|
          timestamp!(account[field], "#{path}.#{field}")
        end
        raise ValidationError, "#{path}.quality_source is invalid" unless account["quality_source"] == "sub2api_account_attributed_natural_traffic"
        number!(account["balance_usd"], "#{path}.balance_usd")
        positive_integer!(account["sample_count"], "#{path}.sample_count")
        %w[success_rate error_rate ttft_p95_ms total_latency_p95_ms].each { |field| number!(account[field], "#{path}.#{field}") }
        validate_qualifications!(account["qualifications"], "#{path}.qualifications")
        account["account_id"]
      end
      raise ValidationError, "accounts contains duplicate IDs" unless ids.uniq.length == ids.length
    end

    def validate_qualifications!(qualifications, path)
      raise ValidationError, "#{path} must be a mapping" unless qualifications.is_a?(Hash)
      qualifications.each do |model_id, evidence|
        text!(model_id, "#{path}.model_id")
        exact_keys!(evidence, QUALIFICATION_KEYS, "#{path}.#{model_id}")
        QUALIFICATION_KEYS.each { |field| positive_integer!(evidence[field], "#{path}.#{model_id}.#{field}") }
      end
    end

    def validate_pricing!(pricing)
      array!(pricing, "pricing", allow_empty: false, max: 512)
      models = pricing.map.with_index do |item, index|
        exact_keys!(item, %w[complete model_id], "pricing[#{index}]")
        text!(item["model_id"], "pricing[#{index}].model_id")
        raise ValidationError, "pricing[#{index}].complete must be boolean" unless [true, false].include?(item["complete"])
        item["model_id"]
      end
      raise ValidationError, "pricing contains duplicate models" unless models.uniq.length == models.length
    end

    def validate_base_configuration!(configuration)
      exact_keys!(configuration, %w[accounts channels], "base_configuration")
      array!(configuration["accounts"], "base_configuration.accounts", allow_empty: false, max: 2000)
      array!(configuration["channels"], "base_configuration.channels", allow_empty: false, max: 100)
    end

    def account_blockers(account)
      reasons = []
      reasons << "discovery_evidence_stale" if stale?(account.fetch("discovery_recorded_at"))
      reasons << "financial_evidence_stale" if stale?(account.fetch("financial_recorded_at"))
      reasons << "quality_evidence_stale" if stale?(account.fetch("quality_recorded_at"))
      reasons << "balance_below_minimum" if account.fetch("balance_usd") < 5.0
      reasons << "quality_samples_insufficient" if account.fetch("sample_count") < 20
      reasons << "quality_success_rate_low" if account.fetch("success_rate") < 0.95
      reasons << "quality_error_rate_high" if account.fetch("error_rate") > 0.05
      reasons << "quality_ttft_p95_high" if account.fetch("ttft_p95_ms") > 5000
      reasons << "quality_total_latency_p95_high" if account.fetch("total_latency_p95_ms") > 45_000
      reasons
    end

    def qualified?(account, model_id)
      return false unless account.fetch("discovered_models").include?(model_id)

      evidence = account.fetch("qualifications")[model_id]
      return false unless evidence

      evidence.fetch("sync_attempts") == 3 && evidence.fetch("sync_successes") == 3 &&
        evidence.fetch("sse_attempts") == 3 && evidence.fetch("sse_successes") == 3 &&
        evidence.fetch("sse_terminal_events") == 3
    end

    def result_status(decision, blockers)
      return "待确认" if decision.status == "待确认"
      return "未发现更新" if decision.status == "未发现更新" && blockers.empty?
      return "可升级" if blockers.empty?

      "测试未通过"
    end

    def account_set_hash(accounts)
      Canonical.sha256(accounts.map do |account|
        {
          "account_id" => account.fetch("account_id"),
          "status" => account.fetch("status"),
          "schedulable" => account.fetch("schedulable"),
          "group_ids" => account.fetch("group_ids")
        }
      end)
    end

    def stale?(value)
      (@now - Time.iso8601(value)) > MAX_AGE_SECONDS
    end

    def timestamp!(value, path)
      parsed = Time.iso8601(value)
      raise ValidationError, "#{path} is in the future" if parsed > @now
    rescue ArgumentError, TypeError
      raise ValidationError, "#{path} must be an ISO8601 timestamp"
    end

    def reject_forbidden_keys!(value, path = "snapshot")
      case value
      when Hash
        value.each do |key, child|
          raise ValidationError, "#{path}.#{key} is forbidden" if key.to_s.match?(FORBIDDEN_KEY)

          reject_forbidden_keys!(child, "#{path}.#{key}")
        end
      when Array
        value.each_with_index { |child, index| reject_forbidden_keys!(child, "#{path}[#{index}]") }
      end
    end

    def exact_keys!(value, expected, path)
      raise ValidationError, "#{path} must be a mapping" unless value.is_a?(Hash)

      actual = value.keys.map(&:to_s).sort
      return if actual == expected.sort

      raise ValidationError, "#{path} keys are invalid"
    end

    def array!(value, path, allow_empty:, max:)
      unless value.is_a?(Array) && (allow_empty || !value.empty?) && value.length <= max
        raise ValidationError, "#{path} must be a bounded list"
      end
    end

    def string_list!(value, path, allow_empty:)
      array!(value, path, allow_empty: allow_empty, max: 4096)
      raise ValidationError, "#{path} must contain unique strings" unless value.all? { |item| item.is_a?(String) && !item.empty? } && value.uniq.length == value.length
    end

    def integer_list!(value, path)
      array!(value, path, allow_empty: false, max: 100)
      raise ValidationError, "#{path} must contain unique positive integers" unless value.all? { |item| item.is_a?(Integer) && item.positive? } && value.uniq.length == value.length
    end

    def positive_integer!(value, path)
      raise ValidationError, "#{path} must be a positive integer" unless value.is_a?(Integer) && value.positive?
    end

    def number!(value, path)
      raise ValidationError, "#{path} must be a finite non-negative number" unless value.is_a?(Numeric) && value.finite? && value >= 0
    end

    def text!(value, path)
      raise ValidationError, "#{path} must be non-empty text" unless value.is_a?(String) && !value.empty? && value.length <= 256
    end

    def hash64!(value, path)
      raise ValidationError, "#{path} must be lowercase SHA-256" unless value.is_a?(String) && value.match?(/\A[0-9a-f]{64}\z/)
    end

    def secure_equal(left, right)
      left == right
    end
  end

  module Canonical
    module_function

    def sha256(value)
      Digest::SHA256.hexdigest(JSON.generate(normalize(value)))
    end

    def normalize(value)
      case value
      when Hash
        value.keys.map(&:to_s).sort.to_h { |key| [key, normalize(value.fetch(key))] }
      when Array
        items = value.map { |item| normalize(item) }
        if items.all? { |item| item.is_a?(Hash) && item.key?("account_id") }
          items.sort_by { |item| item.fetch("account_id") }
        elsif items.all? { |item| item.is_a?(Hash) && item.key?("channel_id") }
          items.sort_by { |item| item.fetch("channel_id") }
        elsif items.all? { |item| item.is_a?(String) || item.is_a?(Numeric) }
          items.sort
        else
          items
        end
      else
        value
      end
    end
  end
end
