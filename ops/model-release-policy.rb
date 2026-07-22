# frozen_string_literal: true

require "yaml"

module ModelRelease
  class ValidationError < StandardError; end

  Classification = Struct.new(:model_id, :family, :state, :reason_code, keyword_init: true) do
    def initialize(**arguments)
      super
      freeze
    end
  end

  CandidateDecision = Struct.new(
    :published_families,
    :candidate_family,
    :candidate_models,
    :review_models,
    :status,
    :reason_codes,
    keyword_init: true
  ) do
    def initialize(**arguments)
      arguments.each_value(&:freeze)
      super
      freeze
    end
  end

  class Policy
    REQUIRED_KEYS = %w[
      approved_suffixes
      bootstrap_models
      excluded_markers
      published_family_limit
      schema_version
    ].freeze
    MODEL_PATTERN = /\Agpt-(\d+)\.(\d+)(?:-([a-z0-9.-]+))?\z/
    DATED_ALIAS_PATTERN = /(?:\A|-)\d{4}-\d{2}-\d{2}(?:\z|-)/
    BOOTSTRAP_FAMILIES = %w[5.5 5.6].freeze

    attr_reader :bootstrap_models, :published_family_limit

    def self.load(path)
      document = YAML.safe_load(File.read(path), permitted_classes: [], permitted_symbols: [], aliases: false)
      new(document)
    rescue Psych::Exception => error
      raise ValidationError, "invalid policy YAML: #{error.message}"
    end

    def initialize(document)
      raise ValidationError, "policy must be a mapping" unless document.is_a?(Hash)

      keys = document.keys.map(&:to_s).sort
      unknown = keys - REQUIRED_KEYS
      missing = REQUIRED_KEYS - keys
      raise ValidationError, "unknown keys: #{unknown.join(', ')}" unless unknown.empty?
      raise ValidationError, "missing keys: #{missing.join(', ')}" unless missing.empty?
      raise ValidationError, "schema_version must be 1" unless document["schema_version"] == 1
      raise ValidationError, "published_family_limit must be 2" unless document["published_family_limit"] == 2

      @bootstrap_models = string_list(document["bootstrap_models"], "bootstrap_models")
      @approved_suffixes = string_list(document["approved_suffixes"], "approved_suffixes")
      @excluded_markers = string_list(document["excluded_markers"], "excluded_markers")
      @published_family_limit = document["published_family_limit"]

      bootstrap_classifications = @bootstrap_models.map { |model_id| classify(model_id) }
      unless bootstrap_classifications.all? { |item| item.state == "candidate" }
        raise ValidationError, "bootstrap_models must contain only supported GPT text models"
      end
      families = bootstrap_classifications.map(&:family).uniq.sort_by { |family| family_tuple(family) }
      unless families == BOOTSTRAP_FAMILIES
        raise ValidationError, "bootstrap families must be exactly 5.5, 5.6"
      end

      @bootstrap_models.freeze
      @approved_suffixes.freeze
      @excluded_markers.freeze
      freeze
    end

    def classify(model_id)
      return classification(model_id, nil, "excluded", "unsupported_model_id") unless model_id.is_a?(String)

      match = MODEL_PATTERN.match(model_id)
      return classification(model_id, nil, "excluded", "unsupported_model_id") unless match

      family = "#{match[1]}.#{match[2]}"
      suffix = match[3]
      return classification(model_id, family, "candidate", nil) if suffix.nil?

      tokens = suffix.split(/[.-]/)
      if DATED_ALIAS_PATTERN.match?(suffix) || !(tokens & @excluded_markers).empty?
        return classification(model_id, family, "excluded", "special_purpose_model")
      end
      return classification(model_id, family, "candidate", nil) if @approved_suffixes.include?(suffix)

      classification(model_id, family, "review", "unknown_model_suffix")
    end

    def candidate_set(discovered_models:, published_models:)
      discovered = normalized_models(discovered_models, "discovered_models")
      published = normalized_models(published_models, "published_models")

      if published.empty?
        return decision(
          published_families: BOOTSTRAP_FAMILIES,
          candidate_models: @bootstrap_models,
          status: "待测试",
          reason_codes: ["bootstrap_qualification_required"]
        )
      end

      published_classifications = published.map { |model_id| classify(model_id) }
      invalid_published = published_classifications.reject { |item| item.state == "candidate" }
      unless invalid_published.empty?
        raise ValidationError, "published_models contains unsupported model IDs"
      end
      published_families = published_classifications.map(&:family).uniq
                                                     .sort_by { |family| family_tuple(family) }
                                                     .last(@published_family_limit)
      latest_published = family_tuple(published_families.last)
      discovered_classifications = discovered.map { |model_id| classify(model_id) }
      newer = discovered_classifications.select do |item|
        item.family && (family_tuple(item.family) <=> latest_published) == 1 && item.state != "excluded"
      end
      if newer.empty?
        return decision(published_families: published_families, status: "未发现更新")
      end

      candidate_family = newer.map(&:family).uniq.max_by { |family| family_tuple(family) }
      family_models = newer.select { |item| item.family == candidate_family }
      candidate_models = family_models.select { |item| item.state == "candidate" }.map(&:model_id).sort
      review_models = family_models.select { |item| item.state == "review" }.map(&:model_id).sort
      reason_codes = review_models.empty? ? ["candidate_qualification_required"] : ["unknown_model_suffix"]

      decision(
        published_families: published_families,
        candidate_family: candidate_family,
        candidate_models: candidate_models,
        review_models: review_models,
        status: review_models.empty? ? "待测试" : "待确认",
        reason_codes: reason_codes
      )
    end

    private

    def string_list(value, field)
      unless value.is_a?(Array) && !value.empty? && value.all? { |item| item.is_a?(String) && !item.empty? }
        raise ValidationError, "#{field} must be a non-empty string list"
      end
      raise ValidationError, "#{field} contains duplicates" unless value.uniq.length == value.length

      value.dup
    end

    def normalized_models(value, field)
      unless value.is_a?(Array) && value.all? { |item| item.is_a?(String) && !item.empty? }
        raise ValidationError, "#{field} must be a string list"
      end

      value.uniq.sort
    end

    def classification(model_id, family, state, reason_code)
      Classification.new(model_id: model_id, family: family, state: state, reason_code: reason_code)
    end

    def decision(published_families:, candidate_family: nil, candidate_models: [], review_models: [], status:, reason_codes: [])
      CandidateDecision.new(
        published_families: published_families.dup,
        candidate_family: candidate_family,
        candidate_models: candidate_models.dup,
        review_models: review_models.dup,
        status: status,
        reason_codes: reason_codes.dup
      )
    end

    def family_tuple(family)
      family.split(".", 2).map(&:to_i)
    end
  end
end
