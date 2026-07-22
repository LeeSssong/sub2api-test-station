# frozen_string_literal: true

require "minitest/autorun"
require "tempfile"
require "yaml"

POLICY_IMPLEMENTATION = File.expand_path("../../ops/model-release-policy.rb", __dir__)
load POLICY_IMPLEMENTATION if File.file?(POLICY_IMPLEMENTATION)

class ModelReleasePolicyTest < Minitest::Test
  POLICY_PATH = File.expand_path("../../config/operations/model-release-policy-v1.yaml", __dir__)
  BOOTSTRAP_MODELS = %w[
    gpt-5.5
    gpt-5.6
    gpt-5.6-luna
    gpt-5.6-sol
    gpt-5.6-terra
  ].freeze

  def policy
    return ModelRelease::Policy.load(POLICY_PATH) if defined?(ModelRelease::Policy)

    flunk "ModelRelease::Policy is not implemented"
  end

  def test_bootstrap_catalog_is_the_approved_55_and_56_text_set
    decision = policy.candidate_set(
      discovered_models: BOOTSTRAP_MODELS + %w[gpt-5.6-codex gpt-5.6-mini],
      published_models: []
    )

    assert_equal %w[5.5 5.6], decision.published_families
    assert_nil decision.candidate_family
    assert_equal BOOTSTRAP_MODELS, decision.candidate_models
    assert_equal "待测试", decision.status
  end

  def test_new_minor_proposes_only_the_new_family_without_publishing_it
    decision = policy.candidate_set(
      discovered_models: %w[gpt-5.6 gpt-5.7-terra gpt-5.7 gpt-5.7-sol],
      published_models: %w[gpt-5.5 gpt-5.6 gpt-5.6-sol]
    )

    assert_equal %w[5.5 5.6], decision.published_families
    assert_equal "5.7", decision.candidate_family
    assert_equal %w[gpt-5.7 gpt-5.7-sol gpt-5.7-terra], decision.candidate_models
    assert_equal "待测试", decision.status
    assert_empty decision.review_models
  end

  def test_no_newer_family_reports_no_update
    decision = policy.candidate_set(
      discovered_models: %w[gpt-5.5 gpt-5.6 gpt-5.6-terra],
      published_models: %w[gpt-5.5 gpt-5.6]
    )

    assert_equal "未发现更新", decision.status
    assert_nil decision.candidate_family
    assert_empty decision.candidate_models
  end

  def test_known_special_purpose_and_dated_aliases_are_excluded
    %w[
      gpt-5.7-codex
      gpt-5.7-mini
      gpt-5.7-image
      gpt-5.7-realtime-preview
      gpt-5.7-2026-07-22
    ].each do |model_id|
      classification = policy.classify(model_id)
      assert_equal "excluded", classification.state, model_id
      assert_equal "special_purpose_model", classification.reason_code, model_id
    end
  end

  def test_unknown_suffix_is_review_only_and_blocks_candidate_selection
    classification = policy.classify("gpt-5.7-orbit")

    assert_equal "5.7", classification.family
    assert_equal "review", classification.state
    assert_equal "unknown_model_suffix", classification.reason_code

    decision = policy.candidate_set(
      discovered_models: %w[gpt-5.7 gpt-5.7-orbit],
      published_models: %w[gpt-5.5 gpt-5.6]
    )
    assert_equal "待确认", decision.status
    assert_equal ["gpt-5.7-orbit"], decision.review_models
    assert_includes decision.reason_codes, "unknown_model_suffix"
  end

  def test_malformed_or_non_gpt_model_is_excluded_without_becoming_review_work
    ["GPT-5.7", "gpt-5", "claude-4", "gpt-5.7-", " gpt-5.7"].each do |model_id|
      classification = policy.classify(model_id)
      assert_equal "excluded", classification.state, model_id
      assert_equal "unsupported_model_id", classification.reason_code, model_id
    end
  end

  def test_candidate_selection_is_deterministic_and_deduplicated
    first = policy.candidate_set(
      discovered_models: %w[gpt-5.7-sol gpt-5.7 gpt-5.7-sol gpt-5.6],
      published_models: %w[gpt-5.6 gpt-5.5 gpt-5.6]
    )
    second = policy.candidate_set(
      discovered_models: %w[gpt-5.6 gpt-5.7 gpt-5.7-sol],
      published_models: %w[gpt-5.5 gpt-5.6]
    )

    assert_equal first.to_h, second.to_h
  end

  def test_policy_rejects_unknown_keys_and_invalid_bootstrap_families
    document = YAML.safe_load(File.read(POLICY_PATH))
    document["unexpected"] = true
    assert_policy_error(document, /unknown keys: unexpected/)

    document = YAML.safe_load(File.read(POLICY_PATH))
    document["bootstrap_models"] << "gpt-5.7"
    assert_policy_error(document, /bootstrap families must be exactly 5.5, 5.6/)
  end

  private

  def assert_policy_error(document, pattern)
    Tempfile.create(["model-release-policy", ".yaml"]) do |file|
      file.write(YAML.dump(document))
      file.flush
      error = assert_raises(ModelRelease::ValidationError) { ModelRelease::Policy.load(file.path) }
      assert_match pattern, error.message
    end
  end
end
