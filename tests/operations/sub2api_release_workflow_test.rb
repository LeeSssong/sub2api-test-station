# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "yaml"

ROOT = File.expand_path("../..", __dir__)
WORKFLOW_PATH = File.join(ROOT, ".github/workflows/sub2api-release-preparation.yml")

class Sub2APIReleaseWorkflowTest < Minitest::Test
  ACTION_SHA = /\A[^@\s]+@[0-9a-f]{40}\z/
  PINNED_ACTIONS = {
    "actions/checkout" => "11bd71901bbe5b1630ceea73d27597364c9af683",
    "actions/upload-artifact" => "ea165f8d65b6e75b540449e92b4886f43607fa02",
    "actions/download-artifact" => "d3f86a106a0bac45b974a628896c90dbdf5c8093",
    "actions/setup-go" => "d35c59abb061a4a6fb18e82ac0862c26744d6ab5",
    "actions/setup-node" => "49933ea5288caeca8642d1e84afbd3f7d6820020",
    "actions/cache" => "5a3ec84eff668545956fd18022155c47e93e2684",
    "docker/setup-buildx-action" => "e468171a9de216ec08956ac3ada2f0791b6bd435"
  }.freeze

  def setup
    @workflow = YAML.safe_load(File.read(WORKFLOW_PATH), [], [], true)
    @jobs = @workflow.fetch("jobs")
  end

  def test_schedule_concurrency_and_job_order_are_fixed
    triggers = @workflow["on"] || @workflow[true]
    assert_equal [{ "cron" => "17 */6 * * *" }], triggers.fetch("schedule")
    assert triggers.key?("workflow_dispatch")
    assert_equal false, @workflow.dig("concurrency", "cancel-in-progress")
    assert_equal %w[discover prepare publish stage-production advance-source notify], @jobs.keys
    assert_equal "discover", @jobs.dig("prepare", "needs")
    assert_equal "prepare", @jobs.dig("publish", "needs")
    assert_equal "publish", @jobs.dig("stage-production", "needs")
    assert_equal "stage-production", @jobs.dig("advance-source", "needs")
    assert_equal %w[discover prepare publish stage-production advance-source], @jobs.dig("notify", "needs")
  end

  def test_stale_queued_run_is_suppressed_before_preparation
    discover_runs = @jobs.fetch("discover").fetch("steps").map { |step| step["run"] }.compact.join("\n")

    assert_includes discover_runs, 'https://api.github.com/repos/$GITHUB_REPOSITORY/commits/main'
    assert_includes discover_runs, '"stale_run"'
    assert_includes discover_runs, 'metadata["has_update"] = false'
  end

  def test_permissions_and_secret_boundaries_are_least_privilege
    assert_equal({ "contents" => "read" }, @jobs.dig("discover", "permissions"))
    assert_equal({ "contents" => "read" }, @jobs.dig("prepare", "permissions"))
    assert_equal({ "contents" => "write", "packages" => "write" }, @jobs.dig("publish", "permissions"))
    assert_equal({ "contents" => "read", "packages" => "read" }, @jobs.dig("stage-production", "permissions"))
    assert_equal({ "contents" => "write" }, @jobs.dig("advance-source", "permissions"))
    assert_equal({ "contents" => "read" }, @jobs.dig("notify", "permissions"))

    prepare = @jobs.fetch("prepare").to_s
    refute_match(/secrets\.|packages:\s*write|contents:\s*write|SUB2API_PREP|FEISHU/i, prepare)
    publish = @jobs.fetch("publish").to_s
    refute_match(/SUB2API_PREP_SSH|FEISHU/i, publish)
    stage = @jobs.fetch("stage-production").to_s
    refute_match(/FEISHU/i, stage)
    notify = @jobs.fetch("notify").to_s
    refute_match(/SUB2API_PREP_SSH|packages:\s*(read|write)/i, notify)
  end

  def test_actions_are_pinned_and_trusted_checkout_is_used_for_notification
    uses = @jobs.values.flat_map { |job| job.fetch("steps").map { |step| step["uses"] }.compact }
    refute_empty uses
    uses.each do |reference|
      assert_match ACTION_SHA, reference
      name, sha = reference.split("@", 2)
      assert_equal PINNED_ACTIONS.fetch(name), sha
    end

    notify_checkout = @jobs.fetch("notify").fetch("steps").find { |step| step["uses"]&.start_with?("actions/checkout@") }
    assert_equal "${{ github.sha }}", notify_checkout.dig("with", "ref")
  end

  def test_production_token_uses_stdin_and_workflow_contains_no_runtime_mutation
    stage_steps = @jobs.fetch("stage-production").fetch("steps")
    stage_runs = stage_steps.map { |step| step["run"] }.compact.join("\n")
    assert_match(/\|\s*ssh\b/, stage_runs)
    staging = stage_steps.find { |step| step["name"] == "Stage and verify production candidate" }
    assert_equal "${{ github.token }}", staging.dig("env", "GHCR_TOKEN")
    refute_match(/ssh[^\n]*(?:github\.token|GHCR_TOKEN)/, stage_runs)

    all_runs = @jobs.values.flat_map { |job| job.fetch("steps").map { |step| step["run"] }.compact }.join("\n")
    refute_match(/docker\s+compose|\/system\/update|psql\b|redis-cli|\bdocker\s+(?:restart|stop|kill|prune)\b|docker\s+system\s+prune/i, all_runs)
  end

  def test_hidden_artifacts_are_uploaded_and_downloaded_at_their_real_roots
    upload_steps = @jobs.values.flat_map do |job|
      job.fetch("steps").select { |step| step["uses"]&.start_with?("actions/upload-artifact@") }
    end
    upload_steps.each do |step|
      path = step.dig("with", "path").to_s
      assert_equal true, step.dig("with", "include-hidden-files"), step.fetch("name") if path.include?(".release")
    end

    publish_runs = @jobs.fetch("publish").fetch("steps").map { |step| step["run"] }.compact.join("\n")
    assert_includes publish_runs, ".release/preparation/candidate-image.tar"
    assert_includes publish_runs, ".release/discovery/metadata.json"
    refute_includes publish_runs, ".release/.release/"

    stage_runs = @jobs.fetch("stage-production").fetch("steps").map { |step| step["run"] }.compact.join("\n")
    assert_includes stage_runs, ".release/preparation/discovery/metadata.json"
    assert_includes stage_runs, ".release/preparation/preparation/report.json"
    refute_includes stage_runs, ".release/preparation/.release/"
  end

  def test_hyphenated_context_keys_use_bracket_access
    source = File.read(WORKFLOW_PATH)
    refute_match(/\b(?:needs|steps)\.[a-z0-9_]+-[a-z0-9_-]+/i, source)
  end

  def test_runtime_release_artifacts_do_not_dirty_the_checkout
    _stdout, _stderr, status = Open3.capture3(
      "git", "-C", ROOT, "check-ignore", "-q", ".release/discovery/metadata.json"
    )
    assert status.success?, ".release runtime artifacts are not ignored"
  end
end
