# frozen_string_literal: true

require "fileutils"
require "minitest/autorun"
require "open3"
require "tmpdir"

ROOT = File.expand_path("../..", __dir__)
WRAPPER = File.join(ROOT, "ops/run-account-quality-monitor.sh")
SERVICE = File.join(ROOT, "infra/systemd/sub2api-account-quality-monitor.service")
TIMER = File.join(ROOT, "infra/systemd/sub2api-account-quality-monitor.timer")
ENVIRONMENT = File.join(ROOT, "infra/systemd/account-quality-monitor.env.example")

class AccountQualityMonitorTest < Minitest::Test
  def test_wrapper_runs_only_one_hardened_read_only_collector
    with_fixture do |fixture|
      status, output = run_wrapper(fixture)

      assert status.success?, output
      arguments = File.read(fixture.fetch(:arguments))
      %w[--rm --network sub2api_default --user 10002:10002 --read-only --cap-drop ALL
         --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.25
         --tmpfs /tmp:rw,nosuid,nodev,noexec,size=16m].each do |token|
        assert_includes arguments, token
      end
      assert_includes arguments, "collect-account-quality-pulse.rb collect"
      refute_match(/upstream-benchmark|promote-model-release|sync-upstream|capacity|probe|curl|wget/, arguments)
      assert_equal "account_quality_monitor status=started\naccount_quality_monitor status=succeeded\n", output
      refute_includes output, fixture.fetch(:secret)
    end
  end

  def test_wrapper_fails_closed_without_echoing_sensitive_fixture_value
    with_fixture(exit_code: 7) do |fixture|
      status, output = run_wrapper(fixture)

      refute status.success?
      assert_equal "account_quality_monitor status=started\naccount_quality_monitor status=failed\n", output
      refute_includes output, fixture.fetch(:secret)
    end
  end

  def test_wrapper_rejects_missing_or_relative_sensitive_paths
    with_fixture do |fixture|
      status, output = run_wrapper(fixture.merge(key: "relative-key-file"))

      refute status.success?
      assert_equal "account_quality_monitor status=failed\n", output
      refute_includes output, fixture.fetch(:secret)
    end
  end

  def test_systemd_units_and_environment_are_restricted_and_secret_free
    [SERVICE, TIMER, ENVIRONMENT].each { |path| assert File.file?(path), "missing template: #{path}" }
    service = File.read(SERVICE)
    timer = File.read(TIMER)
    environment = File.read(ENVIRONMENT)

    assert_includes service, "User=ubuntu"
    assert_includes service, "Group=ubuntu"
    assert_includes service, "EnvironmentFile=/etc/sub2api/account-quality-monitor.env"
    assert_includes service, "ExecStart=/opt/sub2api/production/ops/account-quality/run-account-quality-monitor.sh"
    assert_includes service, "ConditionPathExists=/opt/sub2api/production/ops/account-quality/run-account-quality-monitor.sh"
    assert_includes service, "RuntimeDirectory=account-quality-monitor"
    assert_includes service, "Environment=DOCKER_CONFIG=/run/account-quality-monitor"
    %w[
      NoNewPrivileges=true
      PrivateTmp=true
      ProtectHome=true
      ProtectSystem=full
      ProtectKernelTunables=true
      ProtectControlGroups=true
      RestrictSUIDSGID=true
    ].each { |setting| assert_includes service, setting }
    assert_includes timer, "OnUnitActiveSec=15m"
    assert_includes timer, "RandomizedDelaySec=2m"
    assert_includes timer, "Persistent=true"
    assert_includes timer, "Unit=sub2api-account-quality-monitor.service"
    assert_includes environment, "ACCOUNT_QUALITY_ROOT=/opt/sub2api/production/ops/account-quality"
    assert_includes environment, "ACCOUNT_QUALITY_ADMIN_KEY_FILE=/opt/sub2api/production/secrets/sub2api-admin-api-key"
    assert_includes environment, "ACCOUNT_QUALITY_EVIDENCE_DIR=/opt/sub2api/production/evidence/account-quality"
    assert_includes environment, "ACCOUNT_QUALITY_RUNNER_IMAGE=sub2api-relay-ops:account-quality-monitor-v1"
    assert_includes environment, "ACCOUNT_QUALITY_DOCKER_NETWORK=sub2api_default"
    refute_match(/(?:api[_-]?key|token|secret|password)\s*=\s*[^\s#]+/i, environment)
  end

  private

  def with_fixture(exit_code: 0)
    Dir.mktmpdir("account-quality-monitor") do |dir|
      root = File.join(dir, "tools")
      evidence = File.join(dir, "evidence")
      key = File.join(dir, "admin-key")
      arguments = File.join(dir, "docker-arguments")
      docker = File.join(dir, "docker")
      secret = "fixture-secret-must-not-appear"

      FileUtils.mkdir_p(root)
      FileUtils.mkdir_p(evidence)
      File.write(File.join(root, "collect-account-quality-pulse.rb"), "# fixture\n")
      File.write(key, secret)
      File.chmod(0o600, key)
      File.write(docker, <<~SH)
        #!/bin/sh
        printf '%s\n' "$@" > "$ACCOUNT_QUALITY_TEST_ARGUMENTS"
        printf '%s\n' "$ACCOUNT_QUALITY_TEST_SECRET"
        printf '%s\n' "$ACCOUNT_QUALITY_TEST_SECRET" >&2
        exit "$ACCOUNT_QUALITY_TEST_EXIT"
      SH
      File.chmod(0o755, docker)

      yield(
        root: root, evidence: evidence, key: key, docker: docker, arguments: arguments,
        exit_code: exit_code, secret: secret
      )
    end
  end

  def run_wrapper(fixture)
    env = {
      "ACCOUNT_QUALITY_ROOT" => fixture.fetch(:root),
      "ACCOUNT_QUALITY_ADMIN_KEY_FILE" => fixture.fetch(:key),
      "ACCOUNT_QUALITY_EVIDENCE_DIR" => fixture.fetch(:evidence),
      "ACCOUNT_QUALITY_RUNNER_IMAGE" => "sub2api-relay-ops:test",
      "ACCOUNT_QUALITY_DOCKER_NETWORK" => "sub2api_default",
      "ACCOUNT_QUALITY_DOCKER_BIN" => fixture.fetch(:docker),
      "ACCOUNT_QUALITY_TEST_ARGUMENTS" => fixture.fetch(:arguments),
      "ACCOUNT_QUALITY_TEST_EXIT" => fixture.fetch(:exit_code).to_s,
      "ACCOUNT_QUALITY_TEST_SECRET" => fixture.fetch(:secret)
    }
    stdout, stderr, status = Open3.capture3(env, WRAPPER)
    [status, stdout + stderr]
  rescue Errno::ENOENT
    flunk "wrapper is missing: #{WRAPPER}"
  end
end
