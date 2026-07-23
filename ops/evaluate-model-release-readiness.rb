# frozen_string_literal: true

require "digest"
require "json"
require "optparse"
require "tempfile"
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
    MODEL_ID = /\A[a-z0-9][a-z0-9._-]{0,127}\z/
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
        account_blockers(account).each { |reason| blockers << reason } unless candidate_models.empty?
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
      qualification_attempted = accounts.any? do |account|
        !(account.fetch("qualifications").keys & candidate_models).empty?
      end
      if decision.status == "待确认" || (decision.status == "待测试" && !qualification_attempted)
        blockers.concat(decision.reason_codes)
      end
      blockers = blockers.uniq.sort

      result = {
        "schema_version" => 1,
        "proposal_id" => "",
        "snapshot_id" => snapshot.fetch("snapshot_id"),
        "evaluated_at" => @now.iso8601,
        "status" => result_status(decision, blockers, qualification_attempted: qualification_attempted),
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
        timestamp!(account["discovery_recorded_at"], "#{path}.discovery_recorded_at")
        optional_timestamp!(account["financial_recorded_at"], "#{path}.financial_recorded_at")
        optional_number!(account["balance_usd"], "#{path}.balance_usd")
        validate_optional_quality!(account, path)
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
      array!(pricing, "pricing", allow_empty: true, max: 512)
      models = pricing.map.with_index do |item, index|
        exact_keys!(item, %w[complete model_id], "pricing[#{index}]")
        text!(item["model_id"], "pricing[#{index}].model_id")
        raise ValidationError, "pricing[#{index}].complete must be boolean" unless [true, false].include?(item["complete"])
        item["model_id"]
      end
      raise ValidationError, "pricing contains duplicate models" unless models.uniq.length == models.length
    end

    def validate_base_configuration!(configuration)
      exact_keys!(configuration, %w[accounts groups], "base_configuration")
      array!(configuration["accounts"], "base_configuration.accounts", allow_empty: false, max: 2000)
      account_ids = configuration["accounts"].map.with_index do |account, index|
        path = "base_configuration.accounts[#{index}]"
        exact_keys!(account, %w[account_id model_mapping], path)
        positive_integer!(account["account_id"], "#{path}.account_id")
        model_mapping!(account["model_mapping"], "#{path}.model_mapping")
        account["account_id"]
      end
      raise ValidationError, "base_configuration.accounts contains duplicate IDs" unless account_ids.uniq.length == account_ids.length

      array!(configuration["groups"], "base_configuration.groups", allow_empty: false, max: 100)
      group_ids = configuration["groups"].map.with_index do |group, index|
        path = "base_configuration.groups[#{index}]"
        exact_keys!(group, %w[group_id models_list_config], path)
        positive_integer!(group["group_id"], "#{path}.group_id")
        config = group["models_list_config"]
        exact_keys!(config, %w[enabled models], "#{path}.models_list_config")
        unless [true, false].include?(config["enabled"])
          raise ValidationError, "#{path}.models_list_config.enabled must be boolean"
        end
        string_list!(config["models"], "#{path}.models_list_config.models", allow_empty: true)
        unless config["models"].all? { |model_id| model_id.match?(MODEL_ID) }
          raise ValidationError, "#{path}.models_list_config.models contains invalid model IDs"
        end
        group["group_id"]
      end
      raise ValidationError, "base_configuration.groups contains duplicate IDs" unless group_ids.uniq.length == group_ids.length
    end

    def model_mapping!(value, path)
      unless value.is_a?(Hash) && value.length <= 4096 && value.all? do |source, target|
               source.is_a?(String) && source.match?(MODEL_ID) && target.is_a?(String) && target.match?(MODEL_ID)
             end
        raise ValidationError, "#{path} is invalid"
      end
    end

    def account_blockers(account)
      reasons = []
      reasons << "discovery_evidence_stale" if stale?(account.fetch("discovery_recorded_at"))
      if account["balance_usd"].nil? || account["financial_recorded_at"].nil?
        reasons << "financial_evidence_missing"
      else
        reasons << "financial_evidence_stale" if stale?(account.fetch("financial_recorded_at"))
        reasons << "balance_below_minimum" if account.fetch("balance_usd") < 5.0
      end
      if account["quality_recorded_at"].nil?
        reasons << "quality_evidence_missing"
      else
        reasons << "quality_evidence_stale" if stale?(account.fetch("quality_recorded_at"))
        reasons << "quality_samples_insufficient" if account.fetch("sample_count") < 20
        reasons << "quality_success_rate_low" if account.fetch("success_rate") < 0.95
        reasons << "quality_error_rate_high" if account.fetch("error_rate") > 0.05
        reasons << "quality_ttft_p95_high" if account.fetch("ttft_p95_ms") > 5000
        reasons << "quality_total_latency_p95_high" if account.fetch("total_latency_p95_ms") > 45_000
      end
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

    def result_status(decision, blockers, qualification_attempted:)
      return "待确认" if decision.status == "待确认"
      return "未发现更新" if decision.status == "未发现更新" && blockers.empty?
      return "待测试" if decision.status == "待测试" && !qualification_attempted
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

    def optional_timestamp!(value, path)
      timestamp!(value, path) unless value.nil?
    end

    def validate_optional_quality!(account, path)
      fields = %w[quality_source quality_recorded_at sample_count success_rate error_rate ttft_p95_ms total_latency_p95_ms]
      return if fields.all? { |field| account[field].nil? }

      raise ValidationError, "#{path}.quality evidence is incomplete" if fields.any? { |field| account[field].nil? }
      unless account["quality_source"] == "sub2api_account_attributed_natural_traffic"
        raise ValidationError, "#{path}.quality_source is invalid"
      end
      timestamp!(account["quality_recorded_at"], "#{path}.quality_recorded_at")
      non_negative_integer!(account["sample_count"], "#{path}.sample_count")
      %w[success_rate error_rate ttft_p95_ms total_latency_p95_ms].each do |field|
        number!(account[field], "#{path}.#{field}")
      end
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

    def non_negative_integer!(value, path)
      raise ValidationError, "#{path} must be a non-negative integer" unless value.is_a?(Integer) && value >= 0
    end

    def number!(value, path)
      raise ValidationError, "#{path} must be a finite non-negative number" unless value.is_a?(Numeric) && value.finite? && value >= 0
    end

    def optional_number!(value, path)
      number!(value, path) unless value.nil?
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
        elsif items.all? { |item| item.is_a?(Hash) && item.key?("group_id") }
          items.sort_by { |item| item.fetch("group_id") }
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

  class ReadinessCLI
    def self.run(argv, out: $stdout, err: $stderr)
      command = argv.shift
      options = {}
      OptionParser.new do |parser|
        parser.on("--policy PATH") { |value| options[:policy] = value }
        parser.on("--snapshot PATH") { |value| options[:snapshot] = value }
        parser.on("--output PATH") { |value| options[:output] = value }
        parser.on("--now TIME") { |value| options[:now] = value }
      end.parse!(argv)
      raise ValidationError, "command must be evaluate" unless command == "evaluate"
      raise ValidationError, "unexpected arguments" unless argv.empty?
      %i[policy snapshot output].each { |key| raise ValidationError, "missing required option" unless options[key] }
      raise ValidationError, "output path must be absolute" unless File.absolute_path(options[:output]) == options[:output]
      raise ValidationError, "snapshot is too large" if File.size(options[:snapshot]) > 2 << 20

      snapshot = JSON.parse(File.read(options[:snapshot], 2 << 20))
      now = options[:now] ? Time.iso8601(options[:now]).utc : Time.now.utc
      result = Evaluator.new(policy: Policy.load(options[:policy]), now: now).evaluate(snapshot)
      write_atomic(options[:output], JSON.pretty_generate(result))
      out.puts(JSON.generate("status" => result.fetch("status"), "proposal_id" => result.fetch("proposal_id")))
      0
    rescue ValidationError, JSON::ParserError, OptionParser::ParseError, Errno::ENOENT, Errno::EACCES, ArgumentError
      err.puts("ERROR: model release readiness evaluation rejected")
      2
    end

    def self.write_atomic(path, content)
      directory = File.dirname(path)
      Tempfile.create([".model-release-result-", ".json"], directory) do |file|
        file.chmod(0o640)
        file.write(content)
        file.flush
        file.fsync
        File.rename(file.path, path)
      end
    end
    private_class_method :write_atomic
  end
end

if $PROGRAM_NAME == __FILE__
  exit ModelRelease::ReadinessCLI.run(ARGV)
end
