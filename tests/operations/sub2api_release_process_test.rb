# frozen_string_literal: true

require "minitest/autorun"

ROOT = File.expand_path("../..", __dir__)

class Sub2APIReleaseProcessTest < Minitest::Test
  def test_github_actions_release_workflow_is_retired
    workflow = File.join(ROOT, ".github/workflows/sub2api-release-preparation.yml")
    refute File.exist?(workflow), "release preparation must not depend on GitHub Actions"
  end

  def test_local_release_chain_is_available
    %w[
      ops/sub2api-release-metadata.rb
      ops/merge-sub2api-release.sh
      ops/publish-sub2api-candidate.sh
      ops/advance-sub2api-source.sh
      ops/deploy-sub2api-blue-green-host.sh
    ].each do |relative_path|
      path = File.join(ROOT, relative_path)
      assert File.file?(path), "missing local release step: #{relative_path}"
    end
  end

  def test_current_candidate_source_version_matches_recorded_release_tag
    provenance = File.binread(File.join(ROOT, "upstream/sub2api/XINGQIAO_UPSTREAM.md"))
    target_version = provenance.match(/^- Release tag: `v([0-9]+(?:\.[0-9]+){1,2})`$/)&.captures&.first
    refute_nil target_version, "upstream provenance must record a release tag"

    candidate_version = File.binread(
      File.join(ROOT, "upstream/sub2api/backend/cmd/server/VERSION")
    ).strip
    assert_equal target_version, candidate_version
  end
end
