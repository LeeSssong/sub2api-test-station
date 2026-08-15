# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "tmpdir"

ROOT = File.expand_path("../..", __dir__)
HELPER = File.join(ROOT, "ops/account-quality-failure-signal.sh")

class AccountQualityFailureSignalTest < Minitest::Test
  MAPPINGS = {
    "203" => %w[systemd systemd_exec_203], "40" => %w[preflight path_or_mode_preflight],
    "41" => %w[evidence uid10002_evidence_write], "42" => %w[credentials admin_key_read],
    "43" => %w[runtime docker_start_or_runtime], "44" => %w[collector collector_nonzero],
    "45" => %w[resource timeout_or_resource], "46" => %w[publish evidence_publish]
  }.freeze

  def run_signal(status_value, extra = {})
    Dir.mktmpdir do |dir|
      logger = File.join(dir, "logger")
      capture = File.join(dir, "payload")
      File.write(logger, "#!/bin/sh\nprintf '%s\\n' \"$*\" > #{capture}\n")
      File.chmod(0o755, logger)
      env = {"PATH" => "#{dir}:/usr/bin:/bin:/sbin", "MONITOR_UNIT" => "sub2api-account-quality-monitor.service",
             "MONITOR_SERVICE_RESULT" => "exit-code", "MONITOR_EXIT_CODE" => "exited",
             "MONITOR_EXIT_STATUS" => status_value.to_s}.merge(extra)
      _out, _err, status = Open3.capture3(env, HELPER, chdir: ROOT)
      raise "signal helper failed" unless status.success?
      File.read(capture)
    end
  end

  def test_every_allowlisted_status_has_exact_phase_and_reason
    MAPPINGS.each do |status, (phase, reason)|
      payload = run_signal(status)
      assert_includes payload, "schema_version=t10.failure.v1"
      assert_includes payload, "failure_phase=#{phase}"
      assert_includes payload, "reason_code=#{reason}"
      assert_match(/dedupe_key=[0-9a-f]{64}/, payload)
    end
  end

  def test_unknown_metadata_is_redacted_and_stable
    first = run_signal("raw-status", "MONITOR_UNIT" => "/secret/path.service", "MONITOR_SERVICE_RESULT" => "raw stderr", "MONITOR_EXIT_CODE" => "raw")
    second = run_signal("raw-status", "MONITOR_UNIT" => "/secret/path.service", "MONITOR_SERVICE_RESULT" => "raw stderr", "MONITOR_EXIT_CODE" => "raw")
    assert_includes first, "unit=unknown"
    assert_includes first, "service_result=unknown"
    assert_includes first, "exit_code=unknown"
    assert_includes first, "exit_status=unknown"
    refute_includes first, "secret"
    assert_equal first, second
  end
end
