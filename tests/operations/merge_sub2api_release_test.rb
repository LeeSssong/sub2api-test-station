# frozen_string_literal: true

require "fileutils"
require "json"
require "minitest/autorun"
require "open3"
require "tmpdir"

PROJECT_ROOT = File.expand_path("../..", __dir__)
MERGER = File.join(PROJECT_ROOT, "ops/merge-sub2api-release.sh")

class MergeSub2APIReleaseTest < Minitest::Test
  def test_merges_official_delta_with_custom_snapshot_and_exports_bundle
    with_repositories do |fixture|
      status, output = run_merge(fixture)

      assert status.success?, output
      assert_equal "official\n", File.read(File.join(fixture[:root], "upstream/sub2api/app.txt"))
      assert_equal "custom\n", File.read(File.join(fixture[:root], "upstream/sub2api/custom.txt"))
      assert_equal "new\n", File.read(File.join(fixture[:root], "upstream/sub2api/new.txt"))
      refute Dir.exist?(File.join(fixture[:root], "upstream/sub2api/.git"))
      assert File.file?(fixture[:bundle])
      assert_equal 0o600, File.stat(fixture[:report]).mode & 0o777

      report = JSON.parse(File.read(fixture[:report]))
      assert_match(/\A[0-9a-f]{40}\z/, report.fetch("candidate_commit"))
      assert_equal fixture[:root_base], git(fixture[:root], "rev-parse", "HEAD^").strip

      imported = File.join(fixture[:dir], "imported")
      system("git", "clone", "-q", fixture[:root], imported) or flunk "clone failed"
      system("git", "-C", imported, "fetch", "-q", fixture[:bundle], "candidate-artifact:candidate-artifact") or flunk "bundle import failed"
      assert_equal report.fetch("candidate_commit"), git(imported, "rev-parse", "candidate-artifact").strip
    end
  end

  def test_conflict_fails_before_changing_root_head_or_files
    with_repositories(conflict: true) do |fixture|
      before_head = git(fixture[:root], "rev-parse", "HEAD")
      before_tree = git(fixture[:root], "status", "--porcelain=v1")

      status, = run_merge(fixture)

      refute status.success?
      assert_equal before_head, git(fixture[:root], "rev-parse", "HEAD")
      assert_equal before_tree, git(fixture[:root], "status", "--porcelain=v1")
      refute File.exist?(fixture[:bundle])
      refute File.exist?(fixture[:report])
    end
  end

  private

  def with_repositories(conflict: false)
    Dir.mktmpdir("merge-sub2api-release") do |dir|
      official = File.join(dir, "official")
      root = File.join(dir, "root")
      FileUtils.mkdir_p([official, root])
      git(official, "init", "-q")
      configure_git(official)
      File.write(File.join(official, "app.txt"), "base\n")
      File.write(File.join(official, "common.txt"), "base\n")
      git(official, "add", ".")
      git(official, "commit", "-q", "-m", "base")
      base_commit = git(official, "rev-parse", "HEAD").strip
      File.write(File.join(official, "app.txt"), "official\n")
      File.write(File.join(official, "new.txt"), "new\n")
      git(official, "add", ".")
      git(official, "commit", "-q", "-m", "target")
      target_commit = git(official, "rev-parse", "HEAD").strip
      git(official, "tag", "-a", "v0.1.167", "-m", "release", target_commit)

      git(root, "init", "-q")
      configure_git(root)
      upstream = File.join(root, "upstream/sub2api")
      FileUtils.mkdir_p(upstream)
      File.write(File.join(upstream, "app.txt"), conflict ? "custom-conflict\n" : "base\n")
      File.write(File.join(upstream, "common.txt"), "base\n")
      File.write(File.join(upstream, "custom.txt"), "custom\n")
      File.write(File.join(upstream, "XINGQIAO_UPSTREAM.md"), <<~MD)
        # Xingqiao Upstream Source
        - Repository: `https://github.com/Wei-Shaw/sub2api.git`
        - Release tag: `v0.1.166`
        - Source commit: `#{base_commit}`
        - Annotated tag object: `#{"1" * 40}`
        - Imported: `2026-07-27`
      MD
      git(root, "add", ".")
      git(root, "commit", "-q", "-m", "root base")
      root_base = git(root, "rev-parse", "HEAD").strip

      metadata = File.join(dir, "metadata.json")
      File.write(metadata, JSON.generate(
        "has_update" => true,
        "base_sha" => root_base,
        "base_version" => "0.1.166",
        "base_commit" => base_commit,
        "version" => "0.1.167",
        "tag" => "v0.1.167",
        "official_commit" => target_commit,
        "published_at" => "2026-07-28T01:02:03Z"
      ))
      yield(
        dir: dir, official: official, root: root, root_base: root_base,
        metadata: metadata, bundle: File.join(dir, "candidate.bundle"),
        report: File.join(dir, "report.json")
      )
    end
  end

  def run_merge(fixture)
    Open3.capture3(
      "bash", MERGER,
      "--root", fixture[:root],
      "--metadata", fixture[:metadata],
      "--official-repository", fixture[:official],
      "--bundle", fixture[:bundle],
      "--report", fixture[:report]
    ).then { |stdout, stderr, status| [status, stdout + stderr] }
  end

  def configure_git(path)
    git(path, "config", "user.name", "Test")
    git(path, "config", "user.email", "test@example.invalid")
  end

  def git(path, *arguments)
    stdout, stderr, status = Open3.capture3("git", "-C", path, *arguments)
    raise "git #{arguments.join(" ")}: #{stderr}" unless status.success?

    stdout
  end
end
