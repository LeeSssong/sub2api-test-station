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
end
