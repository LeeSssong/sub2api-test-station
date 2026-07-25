#!/usr/bin/env ruby
# frozen_string_literal: true

require "json"
require "open3"
require "tempfile"
require "tmpdir"
require "minitest/autorun"

class AnalyzeAccountMonitorTest < Minitest::Test
  ROOT = File.expand_path("../..", __dir__)
  SCRIPT = File.join(ROOT, "ops", "analyze-account-monitor.rb")

  def test_emits_deterministic_candidate_recommendation
    input = projection(
      account(11, "账号 A", 0.70, 450, 1600, 0.12, 100, 8),
      account(12, "账号 B", 0.96, 120, 500, 0.08, 40, 0)
    )

    output = run_analyzer(input)

    assert output.fetch(:status).success?, output
    result = JSON.parse(output.fetch(:file))
    group = result.fetch("groups").first
    assert_equal "candidate_better", group.fetch("decision")
    assert_includes group.fetch("reasons").join(" "), "稳定性更高"
    assert_operator group.fetch("score_delta"), :>=, 0.05
    assert_equal output.fetch(:file), run_analyzer(input).fetch(:file)
  end

  def test_rejects_secret_shaped_input
    input = projection(account(11, "账号 A", 0.9, 100, 400, 0.1, 10, 0))
    input["api_key"] = "must-not-leak"

    output = run_analyzer(input)

    refute output.fetch(:status).success?
  end

  def test_suppresses_recommendation_when_required_metrics_are_missing
    input = projection(
      account(11, "账号 A", 0.70, 450, 1_600, 0.12, 100, 8),
      account(12, "账号 B", 0.99, 120, 500, 0.08, 40, 0)
    )
    input.fetch("accounts")[1]["ttft_p95_ms"] = nil

    output = run_analyzer(input)
    assert output.fetch(:status).success?, output
    group = JSON.parse(output.fetch(:file)).fetch("groups").first
    refute_equal "candidate_better", group.fetch("decision")
    assert_equal "missing_metrics", group.fetch("evidence_state")
  end

  private

  def run_analyzer(document)
    Dir.mktmpdir do |dir|
      input_path = File.join(dir, "input.json")
      output_path = File.join(dir, "output.json")
      File.write(input_path, JSON.generate(document))
      stdout, stderr, status = Open3.capture3(
        "ruby", SCRIPT, "analyze", "--input", input_path, "--output", output_path
      )
      return { stdout: stdout, stderr: stderr, status: status, file: File.exist?(output_path) ? File.read(output_path) : "" }
    end
  end

  def projection(*accounts)
    {
      "schema_version" => 1,
      "observed_at" => "2026-07-25T07:00:00Z",
      "stale" => false,
      "settings" => { "interval_seconds" => 300 },
      "accounts" => accounts
    }
  end

  def account(id, name, success, ttft, latency, multiplier, requests, errors)
    {
      "account_id" => id, "name" => name, "platform" => "openai", "status" => "active",
      "schedulable" => true, "group_ids" => [3], "group_names" => ["GPT-Pro"],
      "model_id" => "gpt-5.6-sol", "latest_status" => "passed", "error_code" => "",
      "sample_count" => 4, "success_rate" => success, "ttft_p50_ms" => ttft / 2.0,
      "ttft_p95_ms" => ttft, "latency_p95_ms" => latency, "multiplier" => multiplier,
      "request_count" => requests, "error_count" => errors,
      "usage_windows" => [{ "name" => "daily", "utilization" => 0.2, "requests" => 1, "tokens" => 2 }],
      "checked_at" => "2026-07-25T06:59:00Z", "stale" => false
    }
  end
end
