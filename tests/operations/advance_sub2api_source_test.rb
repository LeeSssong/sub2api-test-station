# frozen_string_literal: true

require "fileutils"
require "json"
require "minitest/autorun"
require "open3"
require "tmpdir"

ROOT = File.expand_path("../..", __dir__)
ADVANCER = File.join(ROOT, "ops/advance-sub2api-source.sh")

class AdvanceSub2APISourceTest < Minitest::Test
  def setup
    assert File.file?(ADVANCER), "source advancer script is missing"
  end

  def test_fast_forwards_main_only_from_the_exact_base
    with_fixture do |fixture|
      status, output = advance(fixture)

      assert status.success?, output
      assert_equal fixture[:candidate], git(fixture[:remote], "rev-parse", "refs/heads/main").strip
      result = JSON.parse(File.read(fixture[:output]))
      assert_equal fixture[:base], result.fetch("previous_main")
      assert_equal fixture[:candidate], result.fetch("current_main")
      assert_equal 0o600, File.stat(fixture[:output]).mode & 0o777
    end
  end

  def test_exact_candidate_is_an_idempotent_success
    with_fixture do |fixture|
      first_status, first_output = advance(fixture)
      assert first_status.success?, first_output
      FileUtils.rm_f(fixture[:output])

      status, output = advance(fixture)

      assert status.success?, output
      result = JSON.parse(File.read(fixture[:output]))
      assert_equal fixture[:candidate], result.fetch("previous_main")
      assert_equal fixture[:candidate], result.fetch("current_main")
      assert_equal fixture[:candidate], git(fixture[:remote], "rev-parse", "refs/heads/main").strip
    end
  end

  def test_concurrent_main_change_fails_without_force_or_overwrite
    with_fixture do |fixture|
      concurrent = File.join(fixture[:dir], "concurrent")
      system("git", "clone", "-q", fixture[:remote], concurrent) or flunk "clone failed"
      configure_git(concurrent)
      File.write(File.join(concurrent, "concurrent.txt"), "human change\n")
      git(concurrent, "add", ".")
      git(concurrent, "commit", "-q", "-m", "concurrent")
      concurrent_sha = git(concurrent, "rev-parse", "HEAD").strip
      git(concurrent, "push", "-q", "origin", "main")

      status, = advance(fixture)

      refute status.success?
      refute File.exist?(fixture[:output])
      assert_equal concurrent_sha, git(fixture[:remote], "rev-parse", "refs/heads/main").strip
    end
  end

  private

  def with_fixture
    Dir.mktmpdir("advance-sub2api-source") do |dir|
      source = File.join(dir, "source")
      remote = File.join(dir, "remote.git")
      FileUtils.mkdir_p(source)
      git(source, "init", "-q")
      configure_git(source)
      File.write(File.join(source, "README.md"), "base\n")
      git(source, "add", ".")
      git(source, "commit", "-q", "-m", "base")
      base = git(source, "rev-parse", "HEAD").strip
      File.write(File.join(source, "candidate.txt"), "candidate\n")
      git(source, "add", ".")
      git(source, "commit", "-q", "-m", "candidate")
      candidate = git(source, "rev-parse", "HEAD").strip
      git(source, "branch", "candidate-artifact", candidate)
      bundle = File.join(dir, "candidate.bundle")
      git(source, "bundle", "create", bundle, "candidate-artifact")

      FileUtils.mkdir_p(remote)
      git(remote, "init", "--bare", "-q")
      git(source, "remote", "add", "origin", remote)
      git(source, "push", "-q", "origin", "#{base}:refs/heads/main")
      git(remote, "symbolic-ref", "HEAD", "refs/heads/main")
      yield(
        dir: dir, remote: remote, bundle: bundle, base: base,
        candidate: candidate, output: File.join(dir, "advance.json")
      )
    end
  end

  def advance(fixture)
    Open3.capture3(
      "bash", ADVANCER,
      "--bundle", fixture[:bundle],
      "--base-sha", fixture[:base],
      "--candidate-sha", fixture[:candidate],
      "--remote", fixture[:remote],
      "--output", fixture[:output]
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
