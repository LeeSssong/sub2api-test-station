# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "tmpdir"

ROOT = File.expand_path("../..", __dir__)
CLI = File.join(ROOT, "ops/sub2api-release-metadata.rb")
COMMIT_166 = "a" * 40
COMMIT_167 = "b" * 40
BASE_SHA = "c" * 40

class Sub2APIReleaseMetadataTest < Minitest::Test
  def test_discovers_a_new_stable_release_without_shell_interpreting_body
    with_fixture do |fixture|
      release = stable_release(body: "fix $(touch /tmp/must-not-run)\nsecond line")
      File.write(fixture[:release], JSON.generate(release))

      status, output = discover(fixture)

      assert status.success?, output
      result = JSON.parse(File.read(fixture[:output]))
      assert_equal true, result.fetch("has_update")
      assert_equal "0.1.167", result.fetch("version")
      assert_equal COMMIT_167, result.fetch("official_commit")
      assert_equal release.fetch("body"), result.fetch("body")
      assert_equal BASE_SHA, result.fetch("base_sha")
      assert_equal 0o600, File.stat(fixture[:output]).mode & 0o777
      refute File.exist?("/tmp/must-not-run")
    end
  end

  def test_same_release_is_an_idempotent_noop
    with_fixture(base_version: "0.1.167", base_commit: COMMIT_167) do |fixture|
      File.write(fixture[:release], JSON.generate(stable_release))

      status, output = discover(fixture)

      assert status.success?, output
      assert_equal false, JSON.parse(File.read(fixture[:output])).fetch("has_update")
    end
  end

  def test_force_rebuild_marks_the_same_qualified_release_as_an_update
    with_fixture(base_version: "0.1.167", base_commit: COMMIT_167) do |fixture|
      File.write(fixture[:release], JSON.generate(stable_release))

      stdout, stderr, status = Open3.capture3(
        "ruby", CLI, "discover",
        "--release", fixture[:release],
        "--provenance", fixture[:provenance],
        "--base-sha", BASE_SHA,
        "--official-commit", COMMIT_167,
        "--output", fixture[:output],
        "--force-rebuild"
      )

      assert status.success?, stdout + stderr
      assert_equal true, JSON.parse(File.read(fixture[:output])).fetch("has_update")
    end
  end

  def test_rejects_unstable_malformed_and_untrusted_release_metadata
    cases = [
      stable_release.merge("draft" => true),
      stable_release.merge("prerelease" => true),
      stable_release.merge("tag_name" => "v0.1.167;rm"),
      stable_release.merge("published_at" => "tomorrow"),
      stable_release.merge("html_url" => "https://evil.example/release"),
      stable_release.merge("tag_name" => "v0.1.165")
    ]

    cases.each do |release|
      with_fixture do |fixture|
        File.write(fixture[:release], JSON.generate(release))
        status, = discover(fixture)
        refute status.success?, "accepted #{release.inspect}"
        refute File.exist?(fixture[:output])
      end
    end
  end

  def test_rejects_a_tag_resolution_that_is_not_an_immutable_commit
    with_fixture do |fixture|
      File.write(fixture[:release], JSON.generate(stable_release))
      _stdout, _stderr, status = Open3.capture3(
        "ruby", CLI, "discover",
        "--release", fixture[:release],
        "--provenance", fixture[:provenance],
        "--base-sha", BASE_SHA,
        "--official-commit", "main",
        "--output", fixture[:output]
      )
      refute status.success?
      refute File.exist?(fixture[:output])
    end
  end

  def test_advances_only_the_provenance_fields_atomically
    with_fixture do |fixture|
      File.write(fixture[:release], JSON.generate(stable_release))
      status, output = discover(fixture)
      assert status.success?, output

      stdout, stderr, status = Open3.capture3(
        "ruby", CLI, "advance-provenance",
        "--metadata", fixture[:output],
        "--provenance", fixture[:provenance],
        "--imported", "2026-07-28",
        "--annotated-tag", "d" * 40
      )

      assert status.success?, stdout + stderr
      text = File.read(fixture[:provenance])
      assert_includes text, "- Release tag: `v0.1.167`"
      assert_includes text, "- Source commit: `#{COMMIT_167}`"
      assert_includes text, "- Annotated tag object: `#{"d" * 40}`"
      assert_includes text, "- Imported: `2026-07-28`"
      assert_includes text, "Preserve this line."
      assert_equal 0o600, File.stat(fixture[:provenance]).mode & 0o777
    end
  end

  private

  def stable_release(body: "Official fixes")
    {
      "tag_name" => "v0.1.167",
      "name" => "v0.1.167",
      "body" => body,
      "published_at" => "2026-07-28T01:02:03Z",
      "html_url" => "https://github.com/Wei-Shaw/sub2api/releases/tag/v0.1.167",
      "draft" => false,
      "prerelease" => false,
      "target_commitish" => "main"
    }
  end

  def with_fixture(base_version: "0.1.166", base_commit: COMMIT_166)
    Dir.mktmpdir("sub2api-release-metadata") do |dir|
      paths = {
        release: File.join(dir, "release.json"),
        provenance: File.join(dir, "XINGQIAO_UPSTREAM.md"),
        output: File.join(dir, "metadata.json")
      }
      File.write(paths[:provenance], <<~MD)
        # Xingqiao Upstream Source
        - Repository: `https://github.com/Wei-Shaw/sub2api.git`
        - Release tag: `v#{base_version}`
        - Source commit: `#{base_commit}`
        - Annotated tag object: `#{"e" * 40}`
        - Imported: `2026-07-27`

        Preserve this line.
      MD
      yield paths
    end
  end

  def discover(fixture)
    Open3.capture3(
      "ruby", CLI, "discover",
      "--release", fixture[:release],
      "--provenance", fixture[:provenance],
      "--base-sha", BASE_SHA,
      "--official-commit", COMMIT_167,
      "--output", fixture[:output]
    ).then { |stdout, stderr, status| [status, stdout + stderr] }
  end
end
