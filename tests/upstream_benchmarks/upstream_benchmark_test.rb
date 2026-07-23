# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "stringio"
require "tmpdir"
require "yaml"
require_relative "../../ops/upstream-benchmark"

class UpstreamBenchmarkTest < Minitest::Test
  class ScriptedClient
    attr_reader :calls

    def initialize(delay: 0.0, stream_complete: true, status: 200)
      @delay = delay
      @stream_complete = stream_complete
      @status = status
      @calls = []
      @mutex = Mutex.new
    end

    def models
      { "status" => 200, "models" => ["gpt-test"], "duration_ms" => 10.0 }
    end

    def chat(model:, prompt:, max_output_tokens:, stream:)
      sleep @delay
      @mutex.synchronize { @calls << { model: model, stream: stream, prompt: prompt } }
      return { "status" => @status, "duration_ms" => 5.0, "error" => "upstream_http" } unless @status == 200

      {
        "status" => 200,
        "duration_ms" => (@delay * 1000.0) + 5.0,
        "first_event_ms" => stream ? 2.0 : nil,
        "stream_complete" => stream ? @stream_complete : nil,
        "usage" => { "prompt_tokens" => 4, "completion_tokens" => 1, "total_tokens" => 5 }
      }
    end
  end

  def channel_document
    {
      "schema_version" => 1,
      "channels" => [
        {
          "id" => "neko",
          "display_name" => "NekoAPI",
          "base_url" => "https://api.example.com/v1",
          "protocol" => "openai_compatible",
          "advertised_multiplier" => 0.07,
          "resale_permission" => "unknown",
          "lifecycle" => "candidate"
        }
      ]
    }
  end

  def profile_document
    {
      "schema_version" => 1,
      "id" => "mvp-text-v1",
      "endpoint" => "chat_completions",
      "models" => ["gpt-test"],
      "prompt" => "Reply with OK only.",
      "max_output_tokens" => 8,
      "timeout_seconds" => 5,
      "repeat_count" => 2,
      "concurrency_levels" => [2, 3]
    }
  end

  def test_registry_accepts_valid_channel_and_rejects_unknown_channel
    registry = UpstreamBenchmark::Registry.new(channel_document)

    assert_equal "NekoAPI", registry.fetch("neko").fetch("display_name")
    error = assert_raises(UpstreamBenchmark::ValidationError) { registry.fetch("missing") }
    assert_match(/unknown channel/, error.message)
  end

  def test_registry_rejects_plain_http_and_secret_shaped_values
    insecure = channel_document
    insecure["channels"][0]["base_url"] = "http://api.example.com/v1"
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmark::Registry.new(insecure)
    end

    leaked = channel_document
    leaked["channels"][0]["notes"] = "Bearer abcdefghijklmnopqrstuvwxyz123456"
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmark::Registry.new(leaked)
    end
  end

  def test_profile_enforces_bounded_text_workload
    profile = UpstreamBenchmark::Profile.new(profile_document)
    assert_equal [2, 3], profile.concurrency_levels

    invalid = profile_document
    invalid["repeat_count"] = 101
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmark::Profile.new(invalid)
    end
  end

  def test_ledger_appends_records_and_rejects_duplicate_ids
    Dir.mktmpdir do |dir|
      ledger = UpstreamBenchmark::Ledger.new(File.join(dir, "runs.jsonl"), File.join(dir, "decisions.jsonl"))
      record = {
        "schema_version" => 1,
        "run_id" => "run-1",
        "channel_id" => "neko",
        "recorded_at" => "2026-07-18T10:00:00Z",
        "status" => "passed",
        "evidence_source" => "live_direct",
        "metrics" => {}
      }

      ledger.append_run(record)
      assert_equal [record], ledger.runs
      assert_raises(UpstreamBenchmark::ValidationError) { ledger.append_run(record) }
    end
  end

  def test_ledger_rejects_secrets_and_allows_superseding_record
    Dir.mktmpdir do |dir|
      ledger = UpstreamBenchmark::Ledger.new(File.join(dir, "runs.jsonl"), File.join(dir, "decisions.jsonl"))
      first = {
        "schema_version" => 1, "run_id" => "run-1", "channel_id" => "neko",
        "recorded_at" => "2026-07-18T10:00:00Z", "status" => "partial",
        "evidence_source" => "historical_report", "metrics" => {}
      }
      ledger.append_run(first)
      correction = first.merge(
        "run_id" => "run-2", "recorded_at" => "2026-07-18T10:01:00Z",
        "supersedes" => "run-1"
      )
      ledger.append_run(correction)
      assert_equal 2, ledger.runs.length

      leaked = correction.merge("run_id" => "run-3", "api_key" => "secret")
      assert_raises(UpstreamBenchmark::ValidationError) { ledger.append_run(leaked) }
    end
  end

  def test_redactor_removes_authorization_and_response_text
    input = {
      "Authorization" => "Bearer abcdefghijklmnopqrstuvwxyz123456",
      "response" => { "choices" => [{ "message" => { "content" => "private output" } }] },
      "usage" => { "prompt_tokens" => 10, "completion_tokens" => 2 }
    }

    redacted = UpstreamBenchmark::Redactor.clean(input)
    refute_match(/abcdefghijklmnopqrstuvwxyz/, JSON.generate(redacted))
    refute_match(/private output/, JSON.generate(redacted))
    assert_equal 10, redacted.fetch("usage").fetch("prompt_tokens")
  end

  def test_redactor_keeps_non_sensitive_text_model_metadata
    input = {
      "text_models" => ["gpt-5.6-sol"],
      "response_text" => "private output",
      "content" => "private output"
    }

    redacted = UpstreamBenchmark::Redactor.clean(input)

    assert_equal ["gpt-5.6-sol"], redacted.fetch("text_models")
    refute redacted.key?("response_text")
    refute redacted.key?("content")
  end

  def test_metrics_calculates_nearest_rank_percentiles
    assert_equal 20.0, UpstreamBenchmark::Metrics.percentile([10, 20, 30, 40], 0.50)
    assert_equal 40.0, UpstreamBenchmark::Metrics.percentile([10, 20, 30, 40], 0.95)
  end

  def test_runner_records_models_sync_stream_and_usage
    client = ScriptedClient.new
    runner = UpstreamBenchmark::Runner.new(client: client, profile: UpstreamBenchmark::Profile.new(profile_document))

    record = runner.run(channel_id: "neko")

    assert_equal "passed", record.fetch("status")
    assert_equal true, record.dig("metrics", "models", "target_models_present")
    assert_equal 45, record.dig("metrics", "usage", "total_tokens")
    assert_equal true, record.dig("metrics", "stream", "complete")
  end

  def test_runner_marks_incomplete_sse_as_partial
    client = ScriptedClient.new(stream_complete: false)
    runner = UpstreamBenchmark::Runner.new(client: client, profile: UpstreamBenchmark::Profile.new(profile_document))

    record = runner.run(channel_id: "neko")

    assert_equal "partial", record.fetch("status")
    assert_equal false, record.dig("metrics", "stream", "complete")
  end

  def test_runner_classifies_http_failure
    client = ScriptedClient.new(status: 429)
    runner = UpstreamBenchmark::Runner.new(client: client, profile: UpstreamBenchmark::Profile.new(profile_document))

    record = runner.run(channel_id: "neko")

    assert_equal "failed", record.fetch("status")
    assert_includes record.fetch("errors").map { |item| item.fetch("category") }, "rate_limited"
  end

  def test_runner_executes_concurrency_in_parallel
    profile = profile_document
    profile["repeat_count"] = 1
    profile["concurrency_levels"] = [3]
    client = ScriptedClient.new(delay: 0.05)
    runner = UpstreamBenchmark::Runner.new(client: client, profile: UpstreamBenchmark::Profile.new(profile))

    started = Process.clock_gettime(Process::CLOCK_MONOTONIC)
    record = runner.run(channel_id: "neko")
    wall_ms = (Process.clock_gettime(Process::CLOCK_MONOTONIC) - started) * 1000.0

    assert_operator wall_ms, :<, 260.0
    assert_equal 3, record.dig("metrics", "concurrency", "3", "success_count")
  end

  def test_sse_parser_requires_terminal_event_and_collects_usage
    parser = UpstreamBenchmark::SseParser.new
    parser.feed("data: {\"choices\":[{\"delta\":{\"content\":\"OK\"}}]}\n\n")
    refute parser.complete?
    parser.feed("data: {\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1,\"total_tokens\":5}}\n\n")
    parser.feed("data: [DONE]\n\n")

    assert parser.complete?
    assert_equal 5, parser.usage.fetch("total_tokens")
    refute_match(/OK/, JSON.generate(parser.summary))
  end

  def test_importer_preserves_historical_evidence_and_unknowns
    imported = UpstreamBenchmark::Importer.build(
      "channel_id" => "aliu",
      "recorded_at" => "2026-07-17T00:00:00Z",
      "status" => "partial",
      "metrics" => { "billing" => { "actual_multiplier" => 0.04 }, "long_stream" => "unknown" },
      "notes" => ["Existing report evidence"]
    )

    assert_equal "historical_report", imported.fetch("evidence_source")
    assert_equal "unknown", imported.dig("metrics", "long_stream")
    assert imported.fetch("run_id")
  end

  def test_importer_accepts_manual_terms_but_rejects_forged_live_evidence
    terms = UpstreamBenchmark::Importer.build(
      "channel_id" => "neko", "recorded_at" => "2026-07-18T12:00:00Z",
      "status" => "partial", "evidence_source" => "manual_terms",
      "metrics" => { "resale_permission" => "unknown" }
    )
    assert_equal "manual_terms", terms.fetch("evidence_source")

    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmark::Importer.build(
        "channel_id" => "neko", "recorded_at" => "2026-07-18T12:00:00Z",
        "status" => "passed", "evidence_source" => "live_direct", "metrics" => {}
      )
    end
  end

  def test_comparison_uses_latest_record_and_keeps_latest_failure_visible
    registry = UpstreamBenchmark::Registry.new(
      channel_document.merge(
        "channels" => channel_document["channels"] + [
          channel_document["channels"].first.merge("id" => "aliu", "display_name" => "Aliu", "advertised_multiplier" => nil)
        ]
      )
    )
    runs = [
      { "run_id" => "n1", "channel_id" => "neko", "recorded_at" => "2026-07-18T09:00:00Z", "status" => "passed", "metrics" => { "billing" => { "actual_multiplier" => 0.07 } } },
      { "run_id" => "n2", "channel_id" => "neko", "recorded_at" => "2026-07-18T10:00:00Z", "status" => "failed", "metrics" => { "billing" => "unknown" } },
      { "run_id" => "a1", "channel_id" => "aliu", "recorded_at" => "2026-07-17T10:00:00Z", "status" => "partial", "metrics" => { "billing" => { "actual_multiplier" => 0.04 } } }
    ]

    comparison = UpstreamBenchmark::Comparator.new(registry: registry, runs: runs).build

    assert_equal "failed", comparison.fetch("neko").fetch("status")
    assert_equal "unknown", comparison.fetch("neko").fetch("billing")
    assert_equal 0.04, comparison.fetch("aliu").fetch("billing").fetch("actual_multiplier")
    markdown = UpstreamBenchmark::Comparator.to_markdown(comparison)
    assert_match(/NekoAPI/, markdown)
    assert_match(/failed/, markdown)
  end

  def test_comparison_merges_latest_record_per_evidence_source
    registry = UpstreamBenchmark::Registry.new(channel_document)
    runs = [
      {
        "run_id" => "direct-1", "channel_id" => "neko",
        "recorded_at" => "2026-07-18T10:00:00Z", "status" => "passed",
        "evidence_source" => "live_direct",
        "metrics" => { "stream" => { "complete" => true }, "billing" => { "actual_multiplier" => 0.07 } }
      },
      {
        "run_id" => "terms-1", "channel_id" => "neko",
        "recorded_at" => "2026-07-18T11:00:00Z", "status" => "partial",
        "evidence_source" => "manual_terms",
        "metrics" => { "terms" => { "resale_permission" => "unknown" } }
      }
    ]

    item = UpstreamBenchmark::Comparator.new(registry: registry, runs: runs).build.fetch("neko")

    assert_equal true, item.fetch("stream").fetch("complete")
    assert_equal 0.07, item.fetch("billing").fetch("actual_multiplier")
    assert_equal "unknown", item.fetch("terms").fetch("resale_permission")
    assert_equal %w[live_direct manual_terms], item.fetch("evidence_sources").sort
    assert_equal "partial", item.fetch("status")
  end

  def test_comparison_prefers_a_superseding_correction_even_when_timestamp_is_older
    registry = UpstreamBenchmark::Registry.new(channel_document)
    runs = [
      {
        "run_id" => "blocked-1", "channel_id" => "neko",
        "recorded_at" => "2026-07-18T14:05:00Z", "status" => "partial",
        "evidence_source" => "live_gateway",
        "metrics" => { "gateway" => { "status" => "blocked" } }
      },
      {
        "run_id" => "passed-1", "channel_id" => "neko",
        "recorded_at" => "2026-07-18T13:50:00Z", "status" => "passed",
        "evidence_source" => "live_gateway", "supersedes" => "blocked-1",
        "metrics" => { "gateway" => { "status" => "passed" } }
      }
    ]

    item = UpstreamBenchmark::Comparator.new(registry: registry, runs: runs).build.fetch("neko")

    assert_equal "passed", item.fetch("status")
  end

  def test_cli_validates_and_imports_then_compares
    Dir.mktmpdir do |dir|
      channels_path = File.join(dir, "channels.yaml")
      profile_path = File.join(dir, "profile.yaml")
      runs_path = File.join(dir, "runs.jsonl")
      decisions_path = File.join(dir, "decisions.jsonl")
      import_path = File.join(dir, "import.yaml")
      File.write(channels_path, YAML.dump(channel_document))
      File.write(profile_path, YAML.dump(profile_document))
      File.write(runs_path, "")
      File.write(decisions_path, "")
      File.write(import_path, YAML.dump(
        "channel_id" => "neko", "recorded_at" => "2026-07-18T10:00:00Z",
        "status" => "partial", "metrics" => { "billing" => { "actual_multiplier" => 0.07 } }
      ))
      output = StringIO.new
      error = StringIO.new

      assert_equal 0, UpstreamBenchmark::CLI.run([
        "validate", "--channels", channels_path, "--profile", profile_path,
        "--runs", runs_path, "--decisions", decisions_path
      ], out: output, err: error)
      assert_match(/valid/, output.string)

      assert_equal 0, UpstreamBenchmark::CLI.run([
        "import", "--file", import_path, "--runs", runs_path, "--decisions", decisions_path
      ], out: output, err: error)
      assert_equal 1, File.readlines(runs_path).length

      output = StringIO.new
      assert_equal 0, UpstreamBenchmark::CLI.run([
        "compare", "--channels", channels_path, "--runs", runs_path, "--format", "markdown"
      ], out: output, err: error)
      assert_match(/NekoAPI/, output.string)
      assert_empty error.string
    end
  end
end
