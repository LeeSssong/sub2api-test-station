# frozen_string_literal: true

require "minitest/autorun"
require "time"

COLLECTOR_PATH = File.expand_path("../../ops/collect-model-release-snapshot.rb", __dir__)
load COLLECTOR_PATH if File.file?(COLLECTOR_PATH)

class CollectModelReleaseSnapshotTest < Minitest::Test
  NOW = Time.iso8601("2026-07-22T16:15:00Z")
  MODELS = %w[gpt-5.5 gpt-5.6 gpt-5.6-luna gpt-5.6-sol gpt-5.6-terra].freeze

  class FakeReader
    attr_reader :pricing_requests

    def initialize(groups:)
      @groups = groups
      @pricing_requests = []
    end

    def active_accounts
      [
        { "account_id" => 10, "status" => "active", "schedulable" => true, "group_ids" => [6] },
        { "account_id" => 11, "status" => "active", "schedulable" => true, "group_ids" => [2] }
      ]
    end

    def public_groups
      @groups
    end

    def account_mapping(account_id)
      models = account_id == 10 ? MODELS : MODELS.reverse
      models.to_h { |model_id| [model_id, model_id] }
    end

    def sync_upstream_models(account_id)
      account_id == 10 ? MODELS : MODELS.reverse
    end

    def pricing_complete?(model_id)
      @pricing_requests << model_id
      true
    end
  end

  def policy
    ModelRelease::Policy.load(File.expand_path("../../config/operations/model-release-policy-v1.yaml", __dir__))
  end

  def disabled_groups
    [2, 6].map do |group_id|
      {
        "group_id" => group_id, "name" => "Group #{group_id}",
        "models_list_config" => { "enabled" => false, "models" => MODELS }
      }
    end
  end

  def test_collects_secret_free_bootstrap_snapshot_from_native_state
    reader = FakeReader.new(groups: disabled_groups)
    snapshot = ModelRelease::SnapshotCollector.new(reader: reader, policy: policy, now: NOW).collect(
      snapshot_id: "MODEL-RELEASE-1"
    )

    assert_equal({ "families" => [], "models" => [] }, snapshot.fetch("published"))
    assert_equal MODELS, reader.pricing_requests
    assert_equal [10, 11], snapshot.fetch("accounts").map { |account| account.fetch("account_id") }
    assert snapshot.fetch("accounts").all? { |account| account.fetch("qualifications").empty? }
    assert snapshot.fetch("accounts").all? { |account| account.fetch("balance_usd").nil? }
    assert_equal [2, 6], snapshot.dig("base_configuration", "groups").map { |group| group.fetch("group_id") }
    assert_match(/\A[0-9a-f]{64}\z/, snapshot.fetch("account_set_sha256"))
    assert_match(/\A[0-9a-f]{64}\z/, snapshot.fetch("base_config_sha256"))
    refute_match(/api[_-]?key|token|authorization|password|secret/i, snapshot.to_s)
  end

  def test_published_native_group_catalog_skips_candidate_pricing_when_no_update
    groups = disabled_groups.map do |group|
      group.merge("models_list_config" => { "enabled" => true, "models" => MODELS })
    end
    reader = FakeReader.new(groups: groups)

    snapshot = ModelRelease::SnapshotCollector.new(reader: reader, policy: policy, now: NOW).collect(
      snapshot_id: "MODEL-RELEASE-2"
    )

    assert_equal MODELS, snapshot.dig("published", "models")
    assert_equal %w[5.5 5.6], snapshot.dig("published", "families")
    assert_empty snapshot.fetch("pricing")
    assert_empty reader.pricing_requests
  end

  def test_rejects_different_public_group_catalogs
    groups = disabled_groups
    groups[0] = groups[0].merge("models_list_config" => { "enabled" => true, "models" => MODELS })
    groups[1] = groups[1].merge("models_list_config" => { "enabled" => true, "models" => MODELS.first(2) })

    error = assert_raises(ModelRelease::ValidationError) do
      ModelRelease::SnapshotCollector.new(reader: FakeReader.new(groups: groups), policy: policy, now: NOW).collect(
        snapshot_id: "MODEL-RELEASE-3"
      )
    end
    assert_match(/public group model catalogs differ/, error.message)
  end
end
