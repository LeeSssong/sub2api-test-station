# frozen_string_literal: true

require "fileutils"
require "minitest/autorun"
require "open3"
require "tmpdir"

ROOT = File.expand_path("../..", __dir__)
WRAPPER = File.join(ROOT, "ops/sub2api-candidate-ssh.sh")
REF = "ghcr.io/leesssong/xingqiao-sub2api@sha256:#{"c" * 64}"
OFFICIAL = "a" * 40
SOURCE = "b" * 40

class Sub2APICandidateSSHTest < Minitest::Test
  def test_valid_command_forwards_exact_arguments_and_stdin
    with_fake_sudo do |fixture|
      command = "prepare #{REF} 0.1.167 #{OFFICIAL} #{SOURCE}"
      stdout, stderr, status = Open3.capture3(
        {
          "SSH_ORIGINAL_COMMAND" => command,
          "SUB2API_CANDIDATE_SUDO" => fixture[:sudo],
          "SUB2API_CANDIDATE_TEST_MODE" => "1",
          "SUB2API_CANDIDATE_TEST_LOG" => fixture[:log]
        },
        WRAPPER,
        stdin_data: "ephemeral-token\n"
      )
      assert status.success?, stdout + stderr
      lines = File.readlines(fixture[:log], chomp: true)
      assert_equal [
        "/usr/local/libexec/sub2api-candidate-loader",
        "prepare", REF, "0.1.167", OFFICIAL, SOURCE,
        "stdin=ephemeral-token"
      ], lines
      refute_includes stdout + stderr, "ephemeral-token"
    end
  end

  def test_rejects_everything_except_the_exact_prepare_grammar
    invalid = [
      "", "bash", "prepare", "prepare #{REF} 0.1.167 #{OFFICIAL}",
      "prepare #{REF} 0.1.167 #{OFFICIAL} #{SOURCE} extra",
      "prepare #{REF};id 0.1.167 #{OFFICIAL} #{SOURCE}",
      "prepare #{REF} 0.1.167 #{OFFICIAL} #{SOURCE}\nid",
      "prepare ghcr.io/other/image@sha256:#{"c" * 64} 0.1.167 #{OFFICIAL} #{SOURCE}"
    ]
    with_fake_sudo do |fixture|
      invalid.each do |command|
        _stdout, _stderr, status = Open3.capture3(
          {
            "SSH_ORIGINAL_COMMAND" => command,
            "SUB2API_CANDIDATE_SUDO" => fixture[:sudo],
            "SUB2API_CANDIDATE_TEST_MODE" => "1",
            "SUB2API_CANDIDATE_TEST_LOG" => fixture[:log]
          },
          WRAPPER,
          stdin_data: "token\n"
        )
        refute status.success?, "accepted #{command.inspect}"
      end
      refute File.exist?(fixture[:log]), "sudo ran for invalid input"
    end
  end

  private

  def with_fake_sudo
    Dir.mktmpdir("candidate-ssh") do |dir|
      sudo = File.join(dir, "sudo")
      log = File.join(dir, "arguments")
      File.write(sudo, <<~SH)
        #!/bin/sh
        for argument in "$@"; do printf '%s\n' "$argument"; done > "$SUB2API_CANDIDATE_TEST_LOG"
        IFS= read -r token
        printf 'stdin=%s\n' "$token" >> "$SUB2API_CANDIDATE_TEST_LOG"
      SH
      File.chmod(0o755, sudo)
      yield sudo: sudo, log: log
    end
  end
end
