# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "tmpdir"
require "fileutils"

ROOT = File.expand_path("../..", __dir__)
WRAPPER = File.join(ROOT, "ops/run-model-release-monitor.sh")
SERVICE = File.join(ROOT, "infra/systemd/sub2api-model-release-monitor.service")
TIMER = File.join(ROOT, "infra/systemd/sub2api-model-release-monitor.timer")
ENVIRONMENT = File.join(ROOT, "infra/systemd/model-release-monitor.env.example")

class ModelReleaseMonitorTest < Minitest::Test
  def test_wrapper_runs_only_hardened_read_only_collection_and_evaluation
    with_fixture do |fixture|
      status, output = run_wrapper(fixture)

      assert status.success?, output
      arguments = File.read(fixture.fetch(:arguments))
      %w[--rm --read-only --network sub2api_default --user 10002:10002 --cap-drop ALL
         --security-opt no-new-privileges --pids-limit 64 --memory 128m --cpus 0.25].each do |token|
        assert_includes arguments, token
      end
      assert_includes arguments, "collect-model-release-snapshot.rb collect"
      assert_includes arguments, "evaluate-model-release-readiness.rb evaluate"
      refute_match(/upstream-benchmark|promote-model-release|capacity|probe|curl|wget/, arguments)
      assert_equal "model_release_monitor status=started\nmodel_release_monitor status=succeeded\n", output
      refute_includes output, fixture.fetch(:secret)
    end
  end

  def test_wrapper_fails_closed_without_echoing_sensitive_fixture_value
    with_fixture(exit_code: 7) do |fixture|
      status, output = run_wrapper(fixture)

      refute status.success?
      assert_equal "model_release_monitor status=started\nmodel_release_monitor status=failed\n", output
      refute_includes output, fixture.fetch(:secret)
    end
  end

  def test_wrapper_rejects_missing_or_relative_sensitive_paths
    with_fixture do |fixture|
      relative = fixture.merge(key: "relative-key-file")
      status, output = run_wrapper(relative)

      refute status.success?
      assert_equal "model_release_monitor status=failed\n", output
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
    assert_includes service, "EnvironmentFile=/etc/sub2api/model-release-monitor.env"
    assert_includes service, "ExecStart=/opt/sub2api/production/ops/model-release/run-model-release-monitor.sh"
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
    assert_includes timer, "Unit=sub2api-model-release-monitor.service"
    assert_includes environment, "MODEL_RELEASE_ROOT=/opt/sub2api/production/ops/model-release"
    assert_includes environment, "MODEL_RELEASE_ADMIN_KEY_FILE=/opt/sub2api/production/secrets/sub2api-admin-api-key"
    assert_includes environment, "MODEL_RELEASE_EVIDENCE_DIR=/opt/sub2api/production/evidence/model-release-20260722"
    assert_includes environment, "MODEL_RELEASE_RUNNER_IMAGE=sub2api-relay-ops:model-release-read-only-20260722-v1"
    assert_includes environment, "MODEL_RELEASE_DOCKER_NETWORK=sub2api_default"
    refute_match(/(?:api[_-]?key|token|secret|password)\s*=\s*[^\s#]+/i, environment)
  end

  private

  def with_fixture(exit_code: 0)
    Dir.mktmpdir("model-release-monitor") do |dir|
      root = File.join(dir, "tools")
      evidence = File.join(dir, "evidence")
      key = File.join(dir, "admin-key")
      arguments = File.join(dir, "docker-arguments")
      docker = File.join(dir, "docker")
      secret = "fixture-secret-must-not-appear"

      FileUtils.mkdir_p(root)
      FileUtils.mkdir_p(evidence)
      %w[
        collect-model-release-snapshot.rb
        evaluate-model-release-readiness.rb
        model-release-policy.rb
        model-release-policy-v1.yaml
      ].each { |name| File.write(File.join(root, name), "# fixture\n") }
      File.write(key, secret)
      File.chmod(0o600, key)
      File.write(docker, <<~SH)
        #!/bin/sh
        printf '%s\n' "$@" > "$MODEL_RELEASE_TEST_ARGUMENTS"
        exit "$MODEL_RELEASE_TEST_EXIT"
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
      "MODEL_RELEASE_ROOT" => fixture.fetch(:root),
      "MODEL_RELEASE_ADMIN_KEY_FILE" => fixture.fetch(:key),
      "MODEL_RELEASE_EVIDENCE_DIR" => fixture.fetch(:evidence),
      "MODEL_RELEASE_RUNNER_IMAGE" => "sub2api-relay-ops:test",
      "MODEL_RELEASE_DOCKER_NETWORK" => "sub2api_default",
      "MODEL_RELEASE_DOCKER_BIN" => fixture.fetch(:docker),
      "MODEL_RELEASE_TEST_ARGUMENTS" => fixture.fetch(:arguments),
      "MODEL_RELEASE_TEST_EXIT" => fixture.fetch(:exit_code).to_s
    }
    stdout, stderr, status = Open3.capture3(env, WRAPPER)
    [status, stdout + stderr]
  rescue Errno::ENOENT
    flunk "wrapper is missing: #{WRAPPER}"
  end
end
