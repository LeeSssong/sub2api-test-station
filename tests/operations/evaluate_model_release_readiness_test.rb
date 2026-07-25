# frozen_string_literal: true

require "digest"
require "json"
require "minitest/autorun"
require "stringio"
require "tmpdir"
require "time"

EVALUATOR_PATH = File.expand_path("../../ops/evaluate-model-release-readiness.rb", __dir__)
load EVALUATOR_PATH if File.file?(EVALUATOR_PATH)

class EvaluateModelReleaseReadinessTest < Minitest::Test
  NOW = Time.iso8601("2026-07-22T12:10:00Z")
  MODELS = %w[gpt-5.5 gpt-5.6 gpt-5.6-luna gpt-5.6-sol gpt-5.6-terra].freeze

  def evaluator
    return ModelRelease::Evaluator.new(policy: policy, now: NOW) if defined?(ModelRelease::Evaluator)

    flunk "ModelRelease::Evaluator is not implemented"
  end

  def policy
    ModelRelease::Policy.load(File.expand_path("../../config/operations/model-release-policy-v1.yaml", __dir__))
  end

  def healthy_snapshot
    snapshot = {
      "schema_version" => 1,
      "snapshot_id" => "MODEL-RELEASE-20260722-1",
      "captured_at" => "2026-07-22T12:05:00Z",
      "modes" => {
        "relay_ops_mode" => "read_only",
        "feishu_command_mode" => "dry_run"
      },
      "published" => { "families" => [], "models" => [] },
      "public_groups" => [
        { "group_id" => 2, "name" => "GPT-Plus" },
        { "group_id" => 3, "name" => "GPT-Pro" }
      ],
      "accounts" => [
        account(10, [2], MODELS.first(3)),
        account(11, [3], MODELS),
        account(12, [2], MODELS.last(2))
      ],
      "pricing" => MODELS.map { |model_id| { "model_id" => model_id, "complete" => true } },
      "base_configuration" => {
        "accounts" => [
          { "account_id" => 10, "model_mapping" => MODELS.first(3).to_h { |model| [model, model] } },
          { "account_id" => 11, "model_mapping" => MODELS.to_h { |model| [model, model] } },
          { "account_id" => 12, "model_mapping" => MODELS.last(2).to_h { |model| [model, model] } }
        ],
        "groups" => [
          { "group_id" => 2, "models_list_config" => { "enabled" => false, "models" => [] } },
          { "group_id" => 3, "models_list_config" => { "enabled" => false, "models" => [] } }
        ]
      }
    }
    snapshot["account_set_sha256"] = sha256(snapshot.fetch("accounts").map do |account|
      account.slice("account_id", "status", "schedulable", "group_ids")
    end)
    snapshot["base_config_sha256"] = sha256(snapshot.fetch("base_configuration"))
    snapshot
  end

  def account(account_id, group_ids, qualified_models)
    {
      "account_id" => account_id,
      "status" => "active",
      "schedulable" => true,
      "group_ids" => group_ids,
      "discovery_recorded_at" => "2026-07-22T12:05:00Z",
      "discovered_models" => MODELS,
      "balance_usd" => 5.0,
      "financial_recorded_at" => "2026-07-22T12:05:00Z",
      "quality_source" => "sub2api_account_attributed_natural_traffic",
      "quality_recorded_at" => "2026-07-22T12:05:00Z",
      "sample_count" => 20,
      "success_rate" => 0.95,
      "error_rate" => 0.05,
      "ttft_p95_ms" => 5000,
      "total_latency_p95_ms" => 45_000,
      "qualifications" => qualified_models.to_h do |model_id|
        [model_id, {
          "sync_attempts" => 3,
          "sync_successes" => 3,
          "sse_attempts" => 3,
          "sse_successes" => 3,
          "sse_terminal_events" => 3
        }]
      end
    }
  end

  def test_complete_bootstrap_snapshot_is_upgradeable_with_group_union_coverage
    result = evaluator.evaluate(healthy_snapshot)

    assert_equal "可升级", result.fetch("status")
    assert_empty result.fetch("blockers")
    assert_equal MODELS, result.dig("candidate", "models")
    assert result.fetch("groups").all? { |group| group.fetch("covered") }
    assert_equal MODELS.first(3), result.fetch("accounts").find { |item| item.fetch("account_id") == 10 }.fetch("qualified_models")
    assert_match(/\A[0-9a-f]{64}\z/, result.fetch("proposal_id"))
  end

  def test_one_account_subset_is_allowed_but_missing_group_union_blocks
    snapshot = healthy_snapshot
    snapshot.fetch("accounts").last.fetch("qualifications").delete("gpt-5.6-terra")

    result = evaluator.evaluate(snapshot)

    assert_equal "测试未通过", result.fetch("status")
    assert_includes result.fetch("blockers"), "group_model_coverage_incomplete"
    plus = result.fetch("groups").find { |group| group.fetch("group_id") == 2 }
    assert_equal ["gpt-5.6-terra"], plus.fetch("missing_models")
  end

  def test_untested_bootstrap_remains_pending_instead_of_failed
    snapshot = healthy_snapshot
    snapshot.fetch("accounts").each do |account|
      account["qualifications"] = {}
      %w[balance_usd financial_recorded_at quality_source quality_recorded_at sample_count success_rate error_rate ttft_p95_ms total_latency_p95_ms].each do |field|
        account[field] = nil
      end
    end

    result = evaluator.evaluate(snapshot)

    assert_equal "待测试", result.fetch("status")
    assert_includes result.fetch("blockers"), "bootstrap_qualification_required"
    assert_includes result.fetch("blockers"), "financial_evidence_missing"
    assert_includes result.fetch("blockers"), "quality_evidence_missing"
    assert_includes result.fetch("blockers"), "group_model_coverage_incomplete"
  end

  def test_balance_quality_pricing_freshness_and_modes_fail_closed
    cases = {
      "balance_below_minimum" => ->(snapshot) { snapshot.fetch("accounts").first["balance_usd"] = 4.99 },
      "financial_evidence_missing" => lambda do |snapshot|
        snapshot.fetch("accounts").first["balance_usd"] = nil
        snapshot.fetch("accounts").first["financial_recorded_at"] = nil
      end,
      "quality_evidence_stale" => ->(snapshot) { snapshot.fetch("accounts").first["quality_recorded_at"] = "2026-07-22T11:49:59Z" },
      "quality_evidence_missing" => lambda do |snapshot|
        account = snapshot.fetch("accounts").first
        %w[quality_source quality_recorded_at sample_count success_rate error_rate ttft_p95_ms total_latency_p95_ms].each do |field|
          account[field] = nil
        end
      end,
      "quality_samples_insufficient" => ->(snapshot) { snapshot.fetch("accounts").first["sample_count"] = 19 },
      "quality_success_rate_low" => ->(snapshot) { snapshot.fetch("accounts").first["success_rate"] = 0.9499 },
      "quality_error_rate_high" => ->(snapshot) { snapshot.fetch("accounts").first["error_rate"] = 0.0501 },
      "quality_ttft_p95_high" => ->(snapshot) { snapshot.fetch("accounts").first["ttft_p95_ms"] = 5001 },
      "quality_total_latency_p95_high" => ->(snapshot) { snapshot.fetch("accounts").first["total_latency_p95_ms"] = 45_001 },
      "model_pricing_incomplete" => ->(snapshot) { snapshot.fetch("pricing").last["complete"] = false },
      "unsafe_operating_mode" => ->(snapshot) { snapshot.dig("modes")["feishu_command_mode"] = "enabled" }
    }

    cases.each do |reason, mutate|
      snapshot = healthy_snapshot
      mutate.call(snapshot)
      result = evaluator.evaluate(snapshot)
      assert_includes result.fetch("blockers"), reason
      refute_equal "可升级", result.fetch("status")
    end
  end

  def test_no_new_family_does_not_require_candidate_quality_evidence
    snapshot = healthy_snapshot
    snapshot["published"] = { "families" => %w[5.5 5.6], "models" => MODELS }
    snapshot["pricing"] = []
    snapshot.fetch("accounts").each do |account|
      %w[balance_usd financial_recorded_at quality_source quality_recorded_at sample_count success_rate error_rate ttft_p95_ms total_latency_p95_ms].each do |field|
        account[field] = nil
      end
      account["qualifications"] = {}
    end

    result = evaluator.evaluate(snapshot)

    assert_equal "未发现更新", result.fetch("status")
    assert_empty result.fetch("candidate").fetch("models")
    assert_empty result.fetch("blockers")
  end

  def test_hashes_are_order_independent_but_mismatches_block
    snapshot = healthy_snapshot
    snapshot["accounts"] = snapshot.fetch("accounts").reverse
    snapshot.fetch("accounts").each { |account| account["group_ids"] = account.fetch("group_ids").reverse }
    snapshot.fetch("base_configuration")["accounts"] = snapshot.dig("base_configuration", "accounts").reverse

    assert_equal "可升级", evaluator.evaluate(snapshot).fetch("status")

    snapshot["account_set_sha256"] = "0" * 64
    result = evaluator.evaluate(snapshot)
    assert_includes result.fetch("blockers"), "account_set_hash_mismatch"
  end

  def test_future_timestamp_and_secret_or_model_output_are_rejected
    snapshot = healthy_snapshot
    snapshot["captured_at"] = "2026-07-22T12:10:01Z"
    assert_raises(ModelRelease::ValidationError) { evaluator.evaluate(snapshot) }

    snapshot = healthy_snapshot
    snapshot["api_key"] = "sk-must-not-appear"
    assert_raises(ModelRelease::ValidationError) { evaluator.evaluate(snapshot) }

    snapshot = healthy_snapshot
    snapshot.fetch("accounts").first["model_output"] = "private response"
    assert_raises(ModelRelease::ValidationError) { evaluator.evaluate(snapshot) }
  end

  def test_cli_atomically_writes_secret_free_result_without_external_contact
    Dir.mktmpdir do |dir|
      snapshot_path = File.join(dir, "snapshot.json")
      output_path = File.join(dir, "result.json")
      File.write(snapshot_path, JSON.generate(healthy_snapshot))
      out = StringIO.new
      err = StringIO.new

      status = ModelRelease::ReadinessCLI.run([
        "evaluate",
        "--policy", File.expand_path("../../config/operations/model-release-policy-v1.yaml", __dir__),
        "--snapshot", snapshot_path,
        "--output", output_path,
        "--now", NOW.iso8601
      ], out: out, err: err)

      assert_equal 0, status
      result = JSON.parse(File.read(output_path))
      assert_equal "可升级", result.fetch("status")
      assert_equal result.fetch("proposal_id"), JSON.parse(out.string).fetch("proposal_id")
      assert_empty err.string
      refute_match(/api[_-]?key|authorization|password|model_output/i, File.read(output_path))
    end
  end

  private

  def sha256(value)
    Digest::SHA256.hexdigest(JSON.generate(canonical(value)))
  end

  def canonical(value)
    case value
    when Hash
      value.keys.sort.to_h { |key| [key, canonical(value.fetch(key))] }
    when Array
      items = value.map { |item| canonical(item) }
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
