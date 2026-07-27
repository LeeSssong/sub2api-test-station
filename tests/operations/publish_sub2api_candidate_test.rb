# frozen_string_literal: true

require "fileutils"
require "json"
require "minitest/autorun"
require "open3"
require "tmpdir"

ROOT = File.expand_path("../..", __dir__)
PUBLISHER = File.join(ROOT, "ops/publish-sub2api-candidate.sh")
VERSION = "0.1.167"
OFFICIAL_COMMIT = "a" * 40
DIGEST = "b" * 64
IMAGE_ID = "sha256:#{"c" * 64}"

class PublishSub2APICandidateTest < Minitest::Test
  def setup
    assert File.file?(PUBLISHER), "publisher script is missing"
  end

  def test_reuses_matching_existing_content_without_push
    with_fixture("matching") do |fixture|
      status, output = publish(fixture)

      assert status.success?, output
      result = JSON.parse(File.read(fixture[:output]))
      assert_equal "ghcr.io/leesssong/xingqiao-sub2api@sha256:#{DIGEST}", result.fetch("reference")
      assert_equal "automation/sub2api-upstream-#{VERSION}", result.fetch("audit_branch")
      refute_includes File.read(fixture[:docker_log]), " push "
      assert_equal fixture[:candidate], git(fixture[:remote], "rev-parse", "refs/heads/#{result.fetch("audit_branch")}").strip
    end
  end

  def test_refuses_mismatched_existing_content_without_overwriting_tag
    with_fixture("mismatch") do |fixture|
      status, = publish(fixture)

      refute status.success?
      refute File.exist?(fixture[:output])
      log = File.read(fixture[:docker_log])
      refute_includes log, " push "
      refute_includes log, " tag "
      refute status.success?
    end
  end

  def test_pushes_new_content_once_and_returns_immutable_digest
    with_fixture("new") do |fixture|
      status, output = publish(fixture)

      assert status.success?, output
      assert_equal 1, File.readlines(fixture[:docker_log]).count { |line| line.include?(" push ") }
      result = JSON.parse(File.read(fixture[:output]))
      assert_equal "ghcr.io/leesssong/xingqiao-sub2api@sha256:#{DIGEST}", result.fetch("reference")
      assert_equal 0o600, File.stat(fixture[:output]).mode & 0o777
    end
  end

  def test_registry_inspection_failure_never_falls_through_to_push
    with_fixture("registry-error") do |fixture|
      status, = publish(fixture)

      refute status.success?
      refute File.exist?(fixture[:output])
      refute_includes File.read(fixture[:docker_log]), " push "
    end
  end

  private

  def with_fixture(scenario)
    Dir.mktmpdir("publish-sub2api-candidate") do |dir|
      repository = File.join(dir, "repository")
      remote = File.join(dir, "remote.git")
      FileUtils.mkdir_p(repository)
      git(repository, "init", "-q")
      configure_git(repository)
      File.write(File.join(repository, "README.md"), "base\n")
      git(repository, "add", ".")
      git(repository, "commit", "-q", "-m", "base")
      base = git(repository, "rev-parse", "HEAD").strip
      File.write(File.join(repository, "candidate.txt"), "candidate\n")
      git(repository, "add", ".")
      git(repository, "commit", "-q", "-m", "candidate")
      candidate = git(repository, "rev-parse", "HEAD").strip
      git(repository, "branch", "candidate-artifact", candidate)
      bundle = File.join(dir, "candidate.bundle")
      git(repository, "bundle", "create", bundle, "candidate-artifact")
      FileUtils.mkdir_p(remote)
      git(remote, "init", "--bare", "-q")
      git(repository, "remote", "add", "fixture-origin", remote)
      git(repository, "push", "-q", "fixture-origin", "#{base}:refs/heads/main")

      metadata = File.join(dir, "metadata.json")
      report = File.join(dir, "report.json")
      archive = File.join(dir, "candidate.tar")
      output = File.join(dir, "publish.json")
      File.write(metadata, JSON.generate(
        "version" => VERSION, "official_commit" => OFFICIAL_COMMIT, "base_sha" => base
      ))
      File.write(report, JSON.generate(
        "version" => VERSION, "official_commit" => OFFICIAL_COMMIT,
        "candidate_commit" => candidate, "base_sha" => base
      ))
      File.write(archive, "docker archive fixture")

      fake_bin = File.join(dir, "bin")
      FileUtils.mkdir_p(fake_bin)
      docker_log = File.join(dir, "docker.log")
      write_fake_docker(File.join(fake_bin, "docker"))
      yield(
        dir: dir, archive: archive, metadata: metadata, report: report,
        bundle: bundle, remote: remote, output: output, fake_bin: fake_bin,
        docker_log: docker_log, scenario: scenario, candidate: candidate
      )
    end
  end

  def publish(fixture)
    env = {
      "PATH" => "#{fixture[:fake_bin]}:#{ENV.fetch("PATH")}",
      "FAKE_DOCKER_LOG" => fixture[:docker_log],
      "FAKE_DOCKER_SCENARIO" => fixture[:scenario],
      "FAKE_DOCKER_VERSION" => VERSION,
      "FAKE_DOCKER_OFFICIAL" => OFFICIAL_COMMIT,
      "FAKE_DOCKER_SOURCE" => fixture[:candidate],
      "FAKE_DOCKER_DIGEST" => DIGEST,
      "FAKE_DOCKER_IMAGE_ID" => IMAGE_ID
    }
    Open3.capture3(
      env, "bash", PUBLISHER,
      "--archive", fixture[:archive],
      "--metadata", fixture[:metadata],
      "--report", fixture[:report],
      "--bundle", fixture[:bundle],
      "--remote", fixture[:remote],
      "--output", fixture[:output]
    ).then { |stdout, stderr, status| [status, stdout + stderr] }
  end

  def write_fake_docker(path)
    File.write(path, <<~'SH')
      #!/usr/bin/env bash
      set -euo pipefail
      printf 'docker %s \n' "$*" >>"$FAKE_DOCKER_LOG"
      repository=ghcr.io/leesssong/xingqiao-sub2api
      local_ref=xingqiao-sub2api:upstream-"$FAKE_DOCKER_VERSION"
      target="$repository:upstream-$FAKE_DOCKER_VERSION"
      labels=$(printf '{"com.xingqiao.sub2api.qualified":"true","com.xingqiao.sub2api.upstream.version":"%s","com.xingqiao.sub2api.upstream.commit":"%s","com.xingqiao.sub2api.source.commit":"%s"}' "$FAKE_DOCKER_VERSION" "$FAKE_DOCKER_OFFICIAL" "$FAKE_DOCKER_SOURCE")
      if [[ "$1 $2" == "manifest inspect" ]]; then
        if [[ "$FAKE_DOCKER_SCENARIO" == new ]]; then
          printf 'manifest unknown\n' >&2
          exit 1
        fi
        if [[ "$FAKE_DOCKER_SCENARIO" == registry-error ]]; then
          printf 'registry connection failed\n' >&2
          exit 1
        fi
        exit 0
      fi
      if [[ "$1 $2" == "image inspect" ]]; then
        reference=${@: -1}
        image_id=$FAKE_DOCKER_IMAGE_ID
        if [[ "$reference" == "$target" && "$FAKE_DOCKER_SCENARIO" == mismatch ]]; then
          image_id=sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd
        fi
        printf '{"Id":"%s","Os":"linux","Architecture":"amd64","RepoDigests":["%s@sha256:%s"],"Config":{"Labels":%s}}\n' "$image_id" "$repository" "$FAKE_DOCKER_DIGEST" "$labels"
        exit
      fi
      case "$1" in
        load|pull|tag|push) exit 0 ;;
      esac
      exit 1
    SH
    File.chmod(0o755, path)
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
