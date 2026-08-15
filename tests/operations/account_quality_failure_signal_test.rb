# frozen_string_literal: true

require "minitest/autorun"
require "open3"
require "tmpdir"

ROOT = File.expand_path("../..", __dir__)
HELPER = File.join(ROOT, "ops/account-quality-failure-signal.sh")

class AccountQualityFailureSignalTest < Minitest::Test
  def test_emits_allowlisted_redacted_deduplicable_payload
    Dir.mktmpdir do |dir|
      logger = File.join(dir, "logger")
      capture = File.join(dir, "payload")
      File.write(logger, "#!/bin/sh\nprintf '%s\\n' \"$*\" > #{capture}\n")
      File.chmod(0o755, logger)
      env = {"PATH" => "#{dir}:/usr/bin:/bin:/sbin", "T10_FAILURE_PHASE" => "collector", "T10_REASON_CODE" => "collector_failed",
             "SYSTEMD_RESULT" => "exit-code", "SYSTEMD_EXEC_MAIN_STATUS" => "1",
             "T10_UNIT_NAME" => "secret/path.service"}
      _stdout, _stderr, status = Open3.capture3(env, HELPER, chdir: ROOT)
      assert status.success?
      payload = File.read(capture)
      assert_match(/t10\.failure\.v1 phase=collector reason=collector_failed/, payload)
      assert_match(/dedupe=[0-9a-f]{64}/, payload)
      refute_includes payload, "secret/path"
    end
  end

  def test_unknown_values_are_redacted
    Dir.mktmpdir do |dir|
      logger = File.join(dir, "logger")
      capture = File.join(dir, "payload")
      File.write(logger, "#!/bin/sh\nprintf '%s\\n' \"$*\" > #{capture}\n")
      File.chmod(0o755, logger)
      env = {"PATH" => "#{dir}:/usr/bin:/bin:/sbin", "T10_FAILURE_PHASE" => "/raw/path", "T10_REASON_CODE" => "raw stderr",
             "SYSTEMD_RESULT" => "raw", "SYSTEMD_EXEC_MAIN_STATUS" => "raw"}
      _stdout, _stderr, status = Open3.capture3(env, HELPER, chdir: ROOT)
      assert status.success?
      payload = File.read(capture)
      assert_equal 4, payload.scan(/unknown/).length
      refute_includes payload, "raw"
    end
  end
end
