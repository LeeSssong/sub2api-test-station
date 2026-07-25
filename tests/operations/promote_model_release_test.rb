# frozen_string_literal: true

require "digest"
require "json"
require "minitest/autorun"
require "tmpdir"
require "time"

PROMOTER_PATH = File.expand_path("../../ops/promote-model-release.rb", __dir__)
load PROMOTER_PATH if File.file?(PROMOTER_PATH)

class PromoteModelReleaseTest < Minitest::Test
  NOW = Time.iso8601("2026-07-22T12:10:00Z")

  class FakeClient
    attr_reader :writes
    attr_accessor :fail_group_once

    def initialize(state)
      @state = Marshal.load(Marshal.dump(state))
      @writes = []
      @fail_group_once = false
    end

    def active_accounts
      @state.fetch("active_accounts")
    end

    def account_mapping(account_id)
      Marshal.load(Marshal.dump(@state.fetch("accounts").fetch(account_id)))
    end

    def group_config(group_id)
      Marshal.load(Marshal.dump(@state.fetch("groups").fetch(group_id)))
    end

    def update_account_mapping(account_id, mapping)
      @writes << ["account", account_id, Marshal.load(Marshal.dump(mapping))]
      @state.fetch("accounts")[account_id] = Marshal.load(Marshal.dump(mapping))
    end

    def update_group_config(group_id, config)
      @writes << ["group", group_id, Marshal.load(Marshal.dump(config))]
      if @fail_group_once
        @fail_group_once = false
        raise ModelRelease::PromotionError, "native group update failed"
      end
      @state.fetch("groups")[group_id] = Marshal.load(Marshal.dump(config))
    end
  end

  def test_preflight_rejects_stale_or_hash_mismatched_proposal_with_zero_writes
    client = FakeClient.new(native_state)
    proposal = valid_proposal
    proposal["evaluated_at"] = "2026-07-22T11:49:59Z"
    rehash!(proposal)

    assert_raises(ModelRelease::PromotionError) do
      promoter(client).preflight(proposal)
    end
    assert_empty client.writes

    proposal = valid_proposal
    proposal["account_set_sha256"] = "0" * 64
    rehash!(proposal)
    assert_raises(ModelRelease::PromotionError) { promoter(client).preflight(proposal) }
    assert_empty client.writes
  end

  def test_apply_writes_only_model_mappings_and_group_models_list
    client = FakeClient.new(native_state)
    proposal = valid_proposal

    Dir.mktmpdir do |dir|
      result = promoter(client).apply(proposal, snapshot_dir: dir)

      assert_equal "published", result.fetch("status")
      assert_equal [
        ["account", 10, { "gpt-5.6" => "gpt-5.6", "gpt-5.7" => "gpt-5.7" }],
        ["account", 11, { "gpt-5.6" => "gpt-5.6", "gpt-5.7-sol" => "gpt-5.7-sol" }],
        ["group", 2, after_group]
      ], client.writes
      snapshot_path = result.fetch("snapshot_path")
      assert_equal dir, File.dirname(snapshot_path)
      assert_equal 0o600, File.stat(snapshot_path).mode & 0o777
      snapshot = JSON.parse(File.read(snapshot_path))
      assert_equal proposal.fetch("proposal_id"), snapshot.fetch("proposal_id")
      refute_match(/"(?:api[_-]?key|access[_-]?token|authorization|password|secret)"\s*:/i, File.read(snapshot_path))
    end
  end

  def test_partial_failure_restores_prior_native_configuration
    client = FakeClient.new(native_state)
    client.fail_group_once = true

    Dir.mktmpdir do |dir|
      error = assert_raises(ModelRelease::PromotionError) do
        promoter(client).apply(valid_proposal, snapshot_dir: dir)
      end
      assert_match(/rolled back/, error.message)
    end

    assert_equal before_account_10, client.account_mapping(10)
    assert_equal before_account_11, client.account_mapping(11)
    assert_equal before_group, client.group_config(2)
    assert_equal ["account", "account", "group", "account", "account"], client.writes.map(&:first)
  end

  def test_bootstrap_skips_unchanged_account_mappings
    client = FakeClient.new(native_state)
    proposal = valid_proposal
    proposal.fetch("accounts").each { |account| account["after_mapping"] = account.fetch("before_mapping") }
    target = {
      "accounts" => proposal.fetch("accounts").map do |account|
        { "account_id" => account.fetch("account_id"), "model_mapping" => account.fetch("after_mapping") }
      end,
      "groups" => [{ "group_id" => 2 }.merge(after_group)]
    }
    proposal["target_config_sha256"] = sha256(target)
    rehash!(proposal)

    Dir.mktmpdir { |dir| promoter(client).apply(proposal, snapshot_dir: dir) }

    assert_equal [["group", 2, after_group]], client.writes
  end

  def test_native_client_uses_only_official_admin_write_shapes
    requests = []
    transport = lambda do |method:, path:, body:, headers:|
      requests << { method: method, path: path, body: body, headers: headers }
      case path
      when "/api/v1/admin/accounts/bulk-update"
        { "success" => 1, "failed" => 0, "success_ids" => [10], "results" => [] }
      when "/api/v1/admin/groups/2"
        after_group.merge("id" => 2)
      else
        raise "unexpected request"
      end
    end

    Dir.mktmpdir do |dir|
      key_path = File.join(dir, "admin-key")
      File.write(key_path, "admin-test-key", mode: "w", perm: 0o600)
      client = ModelRelease::NativeClient.new(
        base_url: "https://sub2api.example.test", admin_key_file: key_path, transport: transport
      )
      client.update_account_mapping(10, { "gpt-5.7" => "gpt-5.7" })
      client.update_group_config(2, after_group)
    end

    assert_equal({
      "account_ids" => [10],
      "credentials" => { "model_mapping" => { "gpt-5.7" => "gpt-5.7" } }
    }, requests[0].fetch(:body))
    assert_equal after_group, requests[1].fetch(:body)
    assert requests.all? { |request| request.fetch(:headers).fetch("x-api-key") == "admin-test-key" }
    assert_equal %w[POST PUT], requests.map { |request| request.fetch(:method) }
  end

  private

  def promoter(client)
    return ModelRelease::Promoter.new(client: client, now: NOW) if defined?(ModelRelease::Promoter)

    flunk "ModelRelease::Promoter is not implemented"
  end

  def before_account_10
    { "gpt-5.5" => "gpt-5.5", "gpt-5.6" => "gpt-5.6" }
  end

  def before_account_11
    { "gpt-5.5" => "gpt-5.5", "gpt-5.6" => "gpt-5.6" }
  end

  def before_group
    {
      "models_list_config" => { "enabled" => true, "models" => %w[gpt-5.5 gpt-5.6] }
    }
  end

  def after_group
    {
      "models_list_config" => { "enabled" => true, "models" => %w[gpt-5.6 gpt-5.7 gpt-5.7-sol] }
    }
  end

  def native_state
    {
      "active_accounts" => [
        { "account_id" => 10, "status" => "active", "schedulable" => true, "group_ids" => [2] },
        { "account_id" => 11, "status" => "active", "schedulable" => true, "group_ids" => [3] }
      ],
      "accounts" => { 10 => before_account_10, 11 => before_account_11 },
      "groups" => { 2 => before_group }
    }
  end

  def valid_proposal
    document = {
      "schema_version" => 1,
      "proposal_id" => "",
      "evaluated_at" => "2026-07-22T12:05:00Z",
      "status" => "可升级",
      "modes" => {
        "relay_ops_mode" => "read_only", "feishu_command_mode" => "dry_run",
      },
      "account_set_sha256" => sha256(native_state.fetch("active_accounts")),
      "base_config_sha256" => sha256(before_configuration),
      "target_config_sha256" => sha256(after_configuration),
      "accounts" => [
        { "account_id" => 10, "before_mapping" => before_account_10, "after_mapping" => { "gpt-5.6" => "gpt-5.6", "gpt-5.7" => "gpt-5.7" } },
        { "account_id" => 11, "before_mapping" => before_account_11, "after_mapping" => { "gpt-5.6" => "gpt-5.6", "gpt-5.7-sol" => "gpt-5.7-sol" } }
      ],
      "groups" => [{ "group_id" => 2, "before" => before_group, "after" => after_group }]
    }
    rehash!(document)
  end

  def before_configuration
    {
      "accounts" => [{ "account_id" => 10, "model_mapping" => before_account_10 }, { "account_id" => 11, "model_mapping" => before_account_11 }],
      "groups" => [{ "group_id" => 2 }.merge(before_group)]
    }
  end

  def after_configuration
    proposal_accounts = [
      { "account_id" => 10, "model_mapping" => { "gpt-5.6" => "gpt-5.6", "gpt-5.7" => "gpt-5.7" } },
      { "account_id" => 11, "model_mapping" => { "gpt-5.6" => "gpt-5.6", "gpt-5.7-sol" => "gpt-5.7-sol" } }
    ]
    { "accounts" => proposal_accounts, "groups" => [{ "group_id" => 2 }.merge(after_group)] }
  end

  def rehash!(document)
    document["proposal_id"] = sha256(document.reject { |key, _| key == "proposal_id" })
    document
  end

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
      elsif items.all? { |item| item.is_a?(Hash) && item.key?("group_id") }
        items.sort_by { |item| item.fetch("group_id") }
      else
        items
      end
    else
      value
    end
  end
end
