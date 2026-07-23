# frozen_string_literal: true

require "minitest/autorun"
require "stringio"
require "tmpdir"
require "yaml"
require_relative "../../ops/upstream-benchmark-nonfunctional"
require_relative "../../ops/upstream-benchmark-v2"

class UpstreamBenchmarkNonfunctionalTest < Minitest::Test
  def profile_document
    {
      "schema_version" => 3,
      "id" => "bounded-text-capacity-v3",
      "protocol" => "responses",
      "models_path" => "/models",
      "generate_path" => "/responses",
      "terminal_events" => ["response.completed", "[DONE]"],
      "prompt" => "Reply with OK only.",
      "max_output_tokens" => 8,
      "request_timeout_seconds" => 45,
      "capacity" => {
        "sync_concurrency_levels" => [1, 2, 3, 5, 8, 10],
        "sse_concurrency_levels" => [1, 2, 3, 5, 8, 10],
        "rpm_levels" => [6, 12, 20, 30],
        "rpm_window_seconds" => 10,
        "waves_per_level" => 1
      },
      "metrics" => {
        "percentile_method" => "nearest_rank",
        "percentiles" => [50, 95],
        "record_queue_signal" => true
      }
    }
  end

  def test_profile_exposes_independent_sync_and_sse_ladders
    profile = UpstreamBenchmarkNonfunctional::Profile.new(profile_document)

    assert_equal [1, 2, 3, 5, 8, 10], profile.concurrency_levels("sync")
    assert_equal [1, 2, 3, 5, 8, 10], profile.concurrency_levels("sse")
    assert_equal [6, 12, 20, 30], profile.rpm_levels
    assert_equal 1, profile.waves_per_level
    assert_match(/\A[0-9a-f]{64}\z/, profile.profile_hash)
  end

  def test_profile_rejects_duplicate_or_unbounded_ladders
    duplicate = Marshal.load(Marshal.dump(profile_document))
    duplicate["capacity"]["sse_concurrency_levels"] = [1, 2, 2]
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkNonfunctional::Profile.new(duplicate)
    end

    unbounded = Marshal.load(Marshal.dump(profile_document))
    unbounded["capacity"]["sync_concurrency_levels"] = [1, 11]
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkNonfunctional::Profile.new(unbounded)
    end
  end

  def test_profile_rejects_unknown_percentile_method_and_secret_fields
    invalid_method = Marshal.load(Marshal.dump(profile_document))
    invalid_method["metrics"]["percentile_method"] = "interpolated"
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkNonfunctional::Profile.new(invalid_method)
    end

    secret = Marshal.load(Marshal.dump(profile_document))
    secret["api_key"] = "sk-must-not-be-stored"
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkNonfunctional::Profile.new(secret)
    end
  end

  def test_profile_rejects_terminal_events_that_do_not_complete_the_protocol
    invalid = Marshal.load(Marshal.dump(profile_document))
    invalid["terminal_events"] = ["[DONE]"]

    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkNonfunctional::Profile.new(invalid)
    end
  end

  def test_request_budget_matches_the_approved_formula
    profile = UpstreamBenchmarkNonfunctional::Profile.new(profile_document)
    result = UpstreamBenchmarkNonfunctional::RequestBudget.new(profile: profile).calculate(
      model_count: 3,
      include_discovery: true,
      topology_verification_requests: 4
    )

    assert_equal 81, result.fetch("maximum_http_requests")
    assert_equal 80, result.fetch("maximum_generation_requests")
    assert_equal 6, result.dig("phases", "compatibility")
    assert_equal 29, result.dig("phases", "sync_capacity")
    assert_equal 29, result.dig("phases", "sse_capacity")
    assert_equal 12, result.dig("phases", "rpm_capacity")
    assert_equal 4, result.dig("phases", "topology_verification")
  end

  def test_request_budget_rejects_negative_counts
    profile = UpstreamBenchmarkNonfunctional::Profile.new(profile_document)

    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkNonfunctional::RequestBudget.new(profile: profile).calculate(
        model_count: -1,
        include_discovery: false,
        topology_verification_requests: 0
      )
    end
  end

  def evidence_identity
    {
      "channel_id" => "channel-a",
      "role" => "gateway_primary",
      "group" => "group-a",
      "account_evidence_ref" => "sha256:account-a",
      "model_id" => "model-a",
      "profile_id" => "bounded-text-capacity-v3",
      "profile_hash" => "a" * 64,
      "measurement_location" => "production-edge",
      "run_id" => "run-20260721-a",
      "recorded_at" => "2026-07-21T00:00:00Z"
    }
  end

  def compact_profile
    document = Marshal.load(Marshal.dump(profile_document))
    document["capacity"]["sync_concurrency_levels"] = [1, 2]
    document["capacity"]["sse_concurrency_levels"] = [1, 2]
    document["capacity"]["rpm_levels"] = [6]
    UpstreamBenchmarkNonfunctional::Profile.new(document)
  end

  def test_sample_normalizes_identity_and_drops_response_content
    sample = UpstreamBenchmarkNonfunctional::Sample.normalize(
      {
        "status" => 200,
        "duration_ms" => 120,
        "first_event_ms" => 25,
        "stream_complete" => true,
        "usage" => { "input_tokens" => 2, "output_tokens" => 1, "total_tokens" => 3 },
        "content" => "must not persist"
      },
      request_kind: "sse",
      identity: evidence_identity
    )

    assert_equal "sse", sample.fetch("request_kind")
    assert_equal "channel-a", sample.dig("identity", "channel_id")
    assert_equal "run-20260721-a", sample.dig("identity", "run_id")
    assert_equal "2026-07-21T00:00:00Z", sample.dig("identity", "recorded_at")
    assert_equal 25.0, sample.fetch("ttft_ms")
    assert_equal true, sample.fetch("stream_completed")
    refute sample.key?("content")
    refute_includes JSON.generate(sample), "must not persist"
  end

  def test_sample_rejects_incomplete_identity
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkNonfunctional::Sample.normalize(
        { "status" => 200, "duration_ms" => 1 },
        request_kind: "sync",
        identity: evidence_identity.reject { |key, _| key == "role" }
      )
    end
  end

  def test_sample_fails_closed_for_unknown_explicit_error
    sample = UpstreamBenchmarkNonfunctional::Sample.normalize(
      { "status" => 200, "duration_ms" => 1, "error" => "provider-specific detail" },
      request_kind: "sync",
      identity: evidence_identity
    )

    assert_equal "unknown", sample.fetch("error_category")
    refute UpstreamBenchmarkNonfunctional::Sample.success?(sample)
    refute_includes JSON.generate(sample), "provider-specific detail"
  end

  def test_sync_and_sse_capacity_are_independent_and_measure_overlap
    observed = []
    mutex = Mutex.new
    invoke = lambda do |request_kind:|
      mutex.synchronize { observed << request_kind }
      sleep 0.01
      {
        "status" => 200,
        "duration_ms" => request_kind == "sse" ? 20 : 10,
        "first_event_ms" => request_kind == "sse" ? 4 : nil,
        "stream_complete" => request_kind == "sse"
      }
    end

    sync = UpstreamBenchmarkNonfunctional::CapacityProbe.new(
      invoke: invoke, profile: compact_profile, request_kind: "sync", identity: evidence_identity
    ).run
    sse = UpstreamBenchmarkNonfunctional::CapacityProbe.new(
      invoke: invoke, profile: compact_profile, request_kind: "sse", identity: evidence_identity
    ).run

    assert_equal "sync", sync.fetch("request_kind")
    assert_equal "sse", sse.fetch("request_kind")
    assert_equal 2, sync.fetch("last_stable")
    assert_equal 2, sse.fetch("last_stable")
    assert_operator sync.dig("levels", "2", "achieved_overlap"), :>=, 2
    assert_operator sse.dig("levels", "2", "achieved_overlap"), :>=, 2
    assert_includes observed, "sync"
    assert_includes observed, "sse"
  end

  def test_sse_capacity_stops_on_missing_terminal_event
    probe = UpstreamBenchmarkNonfunctional::CapacityProbe.new(
      invoke: ->(request_kind:) {
        { "status" => 200, "duration_ms" => 10, "first_event_ms" => 2,
          "stream_complete" => request_kind == "sync" }
      },
      profile: compact_profile,
      request_kind: "sse",
      identity: evidence_identity
    )

    result = probe.run

    assert_nil result.fetch("last_stable")
    assert_equal "stream_incomplete", result.fetch("stop_reason")
    refute result.fetch("levels").key?("2")
  end

  def test_capacity_reports_nearest_rank_percentiles_and_unknown_queue
    durations = [10, 20, 30]
    index = -1
    document = Marshal.load(Marshal.dump(profile_document))
    document["capacity"]["sync_concurrency_levels"] = [3]
    profile = UpstreamBenchmarkNonfunctional::Profile.new(document)
    probe = UpstreamBenchmarkNonfunctional::CapacityProbe.new(
      invoke: ->(request_kind:) {
        value = durations[(index += 1) % durations.length]
        sleep 0.005
        { "status" => 200, "duration_ms" => value, "first_event_ms" => nil,
          "stream_complete" => request_kind == "sse" }
      },
      profile: profile,
      request_kind: "sync",
      identity: evidence_identity
    )

    level = probe.run.dig("levels", "3")

    assert_equal({ "p50" => 20.0, "p95" => 30.0, "n" => 3 }, level.fetch("total_duration_ms"))
    assert_equal "unknown", level.fetch("queue_wait_ms")
  end

  def test_rpm_capacity_is_separate_and_stops_after_first_failure
    document = Marshal.load(Marshal.dump(profile_document))
    document["capacity"]["rpm_levels"] = [60, 120]
    document["capacity"]["rpm_window_seconds"] = 1
    profile = UpstreamBenchmarkNonfunctional::Profile.new(document)
    calls = 0
    probe = UpstreamBenchmarkNonfunctional::RpmProbe.new(
      invoke: ->(request_kind:) {
        calls += 1
        { "status" => calls == 3 ? 429 : 200, "duration_ms" => 2,
          "stream_complete" => request_kind == "sse" }
      },
      profile: profile,
      request_kind: "sync",
      identity: evidence_identity,
      sleeper: ->(_seconds) {}
    )

    result = probe.run

    assert_equal "rpm", result.fetch("mode")
    assert_equal "sync", result.fetch("request_kind")
    assert_equal 60, result.fetch("last_stable")
    assert_equal "rate_limited", result.fetch("stop_reason")
    assert_equal 1, result.dig("levels", "60", "request_count")
    assert_equal 2, result.dig("levels", "120", "planned_request_count")
    assert_equal 3, calls
  end

  def test_execution_budget_stops_before_request_limit_and_after_token_or_cost_limit
    request_budget = UpstreamBenchmarkNonfunctional::ExecutionBudget.new(
      max_requests: 1, max_tokens: 100, max_cost_usd: 1.0, max_wall_seconds: 60
    )
    calls = 0
    probe = UpstreamBenchmarkNonfunctional::CapacityProbe.new(
      invoke: ->(request_kind:) {
        calls += 1
        { "status" => 200, "duration_ms" => 10, "stream_complete" => request_kind == "sse",
          "usage" => { "total_tokens" => 1 }, "actual_cost_usd" => 0.01 }
      }, profile: compact_profile, request_kind: "sync", identity: evidence_identity,
      budget: request_budget
    )

    result = probe.run

    assert_equal 1, calls
    assert_equal "budget_exhausted", result.fetch("stop_reason")
    assert_equal 1, result.dig("budget", "requests_used")

    token_budget = UpstreamBenchmarkNonfunctional::ExecutionBudget.new(
      max_requests: 10, max_tokens: 1, max_cost_usd: 1.0, max_wall_seconds: 60
    )
    result = UpstreamBenchmarkNonfunctional::RpmProbe.new(
      invoke: ->(request_kind:) {
        { "status" => 200, "duration_ms" => 10, "stream_complete" => request_kind == "sse",
          "usage" => { "total_tokens" => 2 }, "actual_cost_usd" => 0.01 }
      }, profile: compact_profile, request_kind: "sync", identity: evidence_identity,
      budget: token_budget, sleeper: ->(_seconds) {}
    ).run
    assert_equal "budget_exhausted", result.fetch("stop_reason")
    assert_equal 2, result.dig("budget", "tokens_used")
  end

  def test_execution_budget_stops_on_latency_and_unknown_billing
    budget = UpstreamBenchmarkNonfunctional::ExecutionBudget.new(
      max_requests: 10, max_tokens: 100, max_cost_usd: 1.0, max_wall_seconds: 60,
      max_total_duration_ms: 50, max_queue_wait_ms: 10
    )
    sample = UpstreamBenchmarkNonfunctional::Sample.normalize(
      { "status" => 200, "duration_ms" => 60, "queue_wait_ms" => 2,
        "stream_complete" => false, "usage" => { "total_tokens" => 1 } },
      request_kind: "sync", identity: evidence_identity
    )

    assert_equal "billing_unknown", budget.observe(sample)

    costed = UpstreamBenchmarkNonfunctional::ExecutionBudget.new(
      max_requests: 10, max_tokens: 100, max_cost_usd: 1.0, max_wall_seconds: 60,
      max_total_duration_ms: 50, max_queue_wait_ms: 10
    )
    sample["actual_cost_usd"] = 0.01
    assert_equal "total_duration_threshold", costed.observe(sample)
  end

  def test_rpm_paces_request_starts_against_the_window_clock
    document = Marshal.load(Marshal.dump(profile_document))
    document["capacity"]["rpm_levels"] = [120]
    document["capacity"]["rpm_window_seconds"] = 1
    profile = UpstreamBenchmarkNonfunctional::Profile.new(document)
    now = 0.0
    starts = []
    probe = UpstreamBenchmarkNonfunctional::RpmProbe.new(
      invoke: ->(request_kind:) {
        starts << now
        now += 0.2
        { "status" => 200, "duration_ms" => 200, "stream_complete" => request_kind == "sse" }
      }, profile: profile, request_kind: "sync", identity: evidence_identity,
      clock: -> { now }, sleeper: ->(seconds) { now += seconds }
    )

    result = probe.run

    assert_equal [0.0, 0.5], starts
    assert_equal 0.0, result.dig("levels", "120", "launch_lag_ms", "p95")
  end

  def test_rpm_stops_when_target_start_rate_is_not_demonstrated
    document = Marshal.load(Marshal.dump(profile_document))
    document["capacity"]["rpm_levels"] = [120]
    document["capacity"]["rpm_window_seconds"] = 1
    now = 0.0
    result = UpstreamBenchmarkNonfunctional::RpmProbe.new(
      invoke: ->(request_kind:) {
        now += 0.8
        { "status" => 200, "duration_ms" => 800, "stream_complete" => request_kind == "sse" }
      }, profile: UpstreamBenchmarkNonfunctional::Profile.new(document), request_kind: "sync",
      identity: evidence_identity, clock: -> { now }, sleeper: ->(seconds) { now += seconds }
    ).run

    assert_equal "target_rate_not_demonstrated", result.fetch("stop_reason")
    assert_nil result.fetch("last_stable")
  end

  def test_metrics_include_error_categories_timestamps_and_cost
    samples = [
      UpstreamBenchmarkNonfunctional::Sample.normalize(
        { "status" => 200, "started_at" => "2026-07-21T00:00:00Z", "completed_at" => "2026-07-21T00:00:01Z",
          "duration_ms" => 100, "usage" => { "total_tokens" => 3 }, "actual_cost_usd" => 0.01 },
        request_kind: "sync", identity: evidence_identity
      ),
      UpstreamBenchmarkNonfunctional::Sample.normalize(
        { "status" => 429, "started_at" => "2026-07-21T00:00:02Z", "completed_at" => "2026-07-21T00:00:03Z",
          "duration_ms" => 100, "actual_cost_usd" => 0.0 },
        request_kind: "sync", identity: evidence_identity
      )
    ]

    summary = UpstreamBenchmarkNonfunctional::Metrics.summarize(samples, achieved_overlap: 1)

    assert_equal({ "rate_limited" => 1 }, summary.fetch("error_categories"))
    assert_equal "2026-07-21T00:00:00Z", summary.fetch("first_started_at")
    assert_equal "2026-07-21T00:00:03Z", summary.fetch("last_completed_at")
    assert_in_delta 0.01, summary.dig("cost_usd", "actual"), 0.0001
  end

  def topology_document
    {
      "schema_version" => 3,
      "id" => "two-group-shared-backup-v3",
      "roles" => [
        { "group" => "group-a", "role" => "gateway_primary", "channel" => "primary-a",
          "account_evidence_ref" => "sha256:primary-a", "model_id" => "model-a",
          "required_request_kinds" => %w[sync sse], "profile_id" => "bounded-text-capacity-v3",
          "profile_hash" => "a" * 64, "measurement_location" => "production-edge" },
        { "group" => "group-a", "role" => "gateway_backup", "channel" => "backup-channel",
          "account_evidence_ref" => "sha256:shared-backup", "model_id" => "model-a",
          "required_request_kinds" => %w[sync sse], "profile_id" => "bounded-text-capacity-v3",
          "profile_hash" => "a" * 64, "measurement_location" => "production-edge" },
        { "group" => "group-b", "role" => "gateway_primary", "channel" => "primary-b",
          "account_evidence_ref" => "sha256:primary-b", "model_id" => "model-b",
          "required_request_kinds" => %w[sync sse], "profile_id" => "bounded-text-capacity-v3",
          "profile_hash" => "a" * 64, "measurement_location" => "production-edge" },
        { "group" => "group-b", "role" => "gateway_backup", "channel" => "backup-channel",
          "account_evidence_ref" => "sha256:shared-backup", "model_id" => "model-b",
          "required_request_kinds" => %w[sync sse], "profile_id" => "bounded-text-capacity-v3",
          "profile_hash" => "a" * 64, "measurement_location" => "production-edge" }
      ],
      "shared_capacity_pools" => [{
        "id" => "shared-backup-pool-1",
        "members" => [
          { "group" => "group-a", "role" => "gateway_backup", "channel" => "backup-channel",
            "requested_concurrency" => 1 },
          { "group" => "group-b", "role" => "gateway_backup", "channel" => "backup-channel",
            "requested_concurrency" => 1 }
        ],
        "aggregate_concurrency_limit" => 2,
        "allocation_policy" => "equal_demand"
      }]
    }
  end

  def approved_thresholds
    {
      "approved" => true,
      "shared_pool_share_deviation_max" => 0.20,
      "sustained_observation_hours_min" => 24,
      "failover_rto_seconds_max" => 60,
      "failback_rto_seconds_max" => 120,
      "primary_recovery_window_seconds_min" => 60
    }
  end

  def test_topology_rejects_primary_account_reuse_and_accepts_explicit_shared_backup
    scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document)
    assert_match(/\A[0-9a-f]{64}\z/, scenario.scenario_hash)
    assert_equal "shared-backup-pool-1", scenario.shared_capacity_pools.first.fetch("id")

    duplicate = Marshal.load(Marshal.dump(topology_document))
    duplicate["roles"][2]["account_evidence_ref"] = "sha256:primary-a"
    assert_raises(UpstreamBenchmark::ValidationError) do
      UpstreamBenchmarkNonfunctional::TopologyScenario.new(duplicate)
    end
  end

  def role_sample(group:, role_name:, request_kind:, phase: nil, wave_id: nil,
                  started_at: "2026-07-21T00:00:00Z", completed_at: "2026-07-21T00:00:01Z",
                  status: 200, error_category: nil, identity_overrides: {})
    role = topology_document.fetch("roles").find do |item|
      item["group"] == group && item["role"] == role_name
    end
    identity = evidence_identity.merge(
      "channel_id" => role.fetch("channel"),
      "role" => role.fetch("role"),
      "group" => role.fetch("group"),
      "account_evidence_ref" => role.fetch("account_evidence_ref"),
      "model_id" => role.fetch("model_id"),
      "profile_id" => role.fetch("profile_id"),
      "profile_hash" => role.fetch("profile_hash"),
      "measurement_location" => role.fetch("measurement_location")
    ).merge(identity_overrides)
    UpstreamBenchmarkNonfunctional::Sample.normalize(
      { "status" => status, "duration_ms" => 100,
        "first_event_ms" => request_kind == "sse" ? 20 : nil,
        "stream_complete" => request_kind == "sse",
        "error_category" => error_category, "topology_phase" => phase,
        "wave_id" => wave_id, "started_at" => started_at, "completed_at" => completed_at },
      request_kind: request_kind,
      identity: identity
    )
  end

  def pool_sample(**arguments)
    role_sample(role_name: "gateway_backup", **arguments)
  end

  def valid_pool_samples
    samples = []
    %w[group-a group-b].each do |group|
      %w[sync sse].each do |kind|
        samples << pool_sample(group: group, request_kind: kind, phase: "isolated",
                              wave_id: "isolated-#{group}-#{kind}")
      end
    end
    %w[equal_demand approved_mix].each do |phase|
      %w[sync sse].each do |kind|
        %w[group-a group-b].each do |group|
          samples << pool_sample(group: group, request_kind: kind, phase: phase,
                                wave_id: "#{phase}-#{kind}")
        end
      end
    end
    samples
  end

  def test_shared_pool_reports_aggregate_and_per_member_fairness
    scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document)
    samples = valid_pool_samples
    result = UpstreamBenchmarkNonfunctional::SharedCapacityPoolEvaluator.new(
      scenario: scenario, thresholds: approved_thresholds
    ).evaluate(pool_id: "shared-backup-pool-1", samples: samples)

    assert_equal "passed", result.fetch("status")
    assert_equal 12, result.dig("aggregate", "request_count")
    assert_equal 2, result.dig("phases", "equal_demand", "sync", "achieved_overlap")
    assert_in_delta 0.5, result.dig("members", "group-a:gateway_backup", "completed_share"), 0.001
    assert_in_delta 0.5, result.dig("members", "group-b:gateway_backup", "completed_share"), 0.001
  end

  def test_shared_pool_cannot_hide_missing_member_and_unapproved_thresholds
    scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document)
    evaluator = UpstreamBenchmarkNonfunctional::SharedCapacityPoolEvaluator.new(
      scenario: scenario, thresholds: approved_thresholds
    )
    missing = evaluator.evaluate(pool_id: "shared-backup-pool-1", samples: valid_pool_samples.reject { |sample| sample.dig("identity", "group") == "group-b" })
    assert_equal "failed", missing.fetch("status")
    assert_includes missing.fetch("reasons"), "missing_member_evidence:group-b:gateway_backup"

    pending = UpstreamBenchmarkNonfunctional::SharedCapacityPoolEvaluator.new(
      scenario: scenario, thresholds: approved_thresholds.merge("approved" => false)
    ).evaluate(pool_id: "shared-backup-pool-1", samples: valid_pool_samples)
    assert_equal "pending_threshold_approval", pending.fetch("status")

    missing_metrics = valid_pool_samples.map do |sample|
      copy = Marshal.load(Marshal.dump(sample))
      copy.delete("ttft_ms")
      copy.delete("total_duration_ms")
      copy
    end
    result = evaluator.evaluate(pool_id: "shared-backup-pool-1", samples: missing_metrics)
    assert_equal "failed", result.fetch("status")
    assert result.fetch("reasons").any? { |reason| reason.include?("missing_ttft_p95") }
    assert result.fetch("reasons").any? { |reason| reason.include?("missing_total_duration_p95") }
  end

  def test_shared_pool_rejects_wrong_role_identity_missing_kind_and_sequential_combined_load
    scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document)
    evaluator = UpstreamBenchmarkNonfunctional::SharedCapacityPoolEvaluator.new(
      scenario: scenario, thresholds: approved_thresholds
    )

    wrong_account = valid_pool_samples
    wrong_account[0] = pool_sample(
      group: "group-a", request_kind: "sync", phase: "isolated", wave_id: "isolated-group-a-sync",
      identity_overrides: { "account_evidence_ref" => "sha256:other" }
    )
    result = evaluator.evaluate(pool_id: "shared-backup-pool-1", samples: wrong_account)
    assert_equal "failed", result.fetch("status")
    assert_includes result.fetch("reasons"), "unexpected_sample_identity"

    missing_sync = valid_pool_samples.reject { |sample| sample["request_kind"] == "sync" }
    result = evaluator.evaluate(pool_id: "shared-backup-pool-1", samples: missing_sync)
    assert_equal "failed", result.fetch("status")
    assert result.fetch("reasons").any? { |reason| reason.include?("missing_request_kind:sync") }

    sequential = valid_pool_samples.map.with_index do |sample, index|
      next sample unless %w[equal_demand approved_mix].include?(sample["topology_phase"])

      copy = Marshal.load(Marshal.dump(sample))
      copy["started_at"] = (Time.utc(2026, 7, 21) + index * 2).iso8601
      copy["completed_at"] = (Time.utc(2026, 7, 21) + index * 2 + 1).iso8601
      copy
    end
    result = evaluator.evaluate(pool_id: "shared-backup-pool-1", samples: sequential)
    assert_equal "failed", result.fetch("status")
    assert result.fetch("reasons").any? { |reason| reason.include?("aggregate_overlap_not_demonstrated") }

    unknown_phase = valid_pool_samples
    unknown_phase[0] = pool_sample(group: "group-a", request_kind: "sync", phase: "unapproved", wave_id: "unknown")
    result = evaluator.evaluate(pool_id: "shared-backup-pool-1", samples: unknown_phase)
    assert_equal "failed", result.fetch("status")
    assert_includes result.fetch("reasons"), "unexpected_topology_phase:unapproved"

    missing_wave = valid_pool_samples
    missing_wave[0] = pool_sample(group: "group-a", request_kind: "sync", phase: "isolated", wave_id: nil)
    result = evaluator.evaluate(pool_id: "shared-backup-pool-1", samples: missing_wave)
    assert_equal "failed", result.fetch("status")
    assert_includes result.fetch("reasons"), "missing_wave_id:isolated:sync"
  end

  def test_observation_requires_every_approved_hour
    scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document)
    evaluator = UpstreamBenchmarkNonfunctional::ObservationEvaluator.new(
      scenario: scenario, thresholds: approved_thresholds
    )
    start = Time.utc(2026, 7, 21, 0, 0, 0)
    windows = scenario.roles.flat_map do |role|
      Array.new(24) do |index|
        { "hour" => (start + index * 3600).iso8601, "sample_count" => 2,
          "success_count" => 2, "rate_limited_count" => 0, "upstream_5xx_count" => 0,
          "ttft_p95_ms" => 100, "total_duration_p95_ms" => 300,
          "stream_interruption_ratio" => 0.0, "request_kind_counts" => { "sync" => 1, "sse" => 1 },
          "missing" => false, "identity" => evidence_identity.merge(
            "channel_id" => role.fetch("channel"), "role" => role.fetch("role"),
            "group" => role.fetch("group"), "account_evidence_ref" => role.fetch("account_evidence_ref"),
            "model_id" => role.fetch("model_id"), "profile_id" => role.fetch("profile_id"),
            "profile_hash" => role.fetch("profile_hash"),
            "measurement_location" => role.fetch("measurement_location")
          ) }
      end
    end

    assert_equal "passed", evaluator.evaluate(windows: windows).fetch("status")
    incomplete = evaluator.evaluate(windows: windows.reject.with_index { |_item, index| index == 12 })
    assert_equal "failed", incomplete.fetch("status")
    assert incomplete.fetch("reasons").any? { |reason| reason.include?("insufficient_observation_hours") }

    unhealthy = Marshal.load(Marshal.dump(windows))
    unhealthy.each { |window| window["success_count"] = 0 }
    result = evaluator.evaluate(windows: unhealthy)
    assert_equal "failed", result.fetch("status")
    assert result.fetch("reasons").any? { |reason| reason.include?("success_rate_below_gate") }

    missing_metrics = Marshal.load(Marshal.dump(windows))
    missing_metrics.each do |window|
      window.delete("ttft_p95_ms")
      window.delete("total_duration_p95_ms")
      window["request_kind_counts"]["sse"] = 1
      window.delete("stream_interruption_ratio")
    end
    result = evaluator.evaluate(windows: missing_metrics)
    assert_equal "failed", result.fetch("status")
    assert result.fetch("reasons").any? { |reason| reason.include?("missing_ttft_p95") }
    assert result.fetch("reasons").any? { |reason| reason.include?("missing_total_duration_p95") }
    assert result.fetch("reasons").any? { |reason| reason.include?("missing_stream_interruption_ratio") }
  end

  def test_drill_calculates_complete_timeline_and_preserves_unknown
    scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document)
    evaluator = UpstreamBenchmarkNonfunctional::DrillEvaluator.new(
      scenario: scenario, thresholds: approved_thresholds
    )
    timeline = {
      "t_fault_observed" => "2026-07-21T00:00:00Z",
      "t_detection_confirmed" => "2026-07-21T00:00:05Z",
      "t_change_requested" => "2026-07-21T00:00:10Z",
      "t_change_accepted" => "2026-07-21T00:00:12Z",
      "t_route_converged" => "2026-07-21T00:00:20Z",
      "t_first_backup_success" => "2026-07-21T00:00:40Z",
      "t_primary_recovery_confirmed" => "2026-07-21T00:10:00Z",
      "t_failback_requested" => "2026-07-21T00:10:10Z",
      "t_failback_converged" => "2026-07-21T00:10:20Z",
      "t_first_primary_success" => "2026-07-21T00:10:40Z",
      "group" => "group-a",
      "route_evidence_after_failover" => { "state" => "backup", "canonical_hash" => "b" * 64 },
      "route_evidence_after_failback" => { "state" => "primary", "canonical_hash" => "c" * 64 },
      "backup_verification_samples" => %w[sync sse].map { |kind| role_sample(group: "group-a", role_name: "gateway_backup", request_kind: kind) },
      "primary_verification_samples" => %w[sync sse].map { |kind| role_sample(group: "group-a", role_name: "gateway_primary", request_kind: kind) },
      "primary_recovery_window" => { "started_at" => "2026-07-21T00:09:00Z", "completed_at" => "2026-07-21T00:10:00Z" }
    }
    result = evaluator.evaluate(timeline: timeline)

    assert_equal "passed", result.fetch("status")
    assert_equal 40.0, result.dig("durations_seconds", "service_failover_rto")
    assert_equal 10.0, result.dig("durations_seconds", "control_failover_time")
    assert_equal 30.0, result.dig("durations_seconds", "service_failback_rto")

    unknown = evaluator.evaluate(timeline: timeline.reject { |key, _| key == "t_fault_observed" })
    assert_equal "unknown", unknown.dig("durations_seconds", "service_failover_rto")
    assert_equal "partial", unknown.fetch("status")
  end

  def test_drill_rejects_non_monotonic_timeline
    scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document)
    evaluator = UpstreamBenchmarkNonfunctional::DrillEvaluator.new(scenario: scenario, thresholds: approved_thresholds)
    timeline = valid_drill_timeline.merge("t_change_requested" => "2026-07-21T00:00:20Z",
                                          "t_route_converged" => "2026-07-21T00:00:10Z")

    result = evaluator.evaluate(timeline: timeline)

    assert_equal "failed", result.fetch("status")
    assert_includes result.fetch("reasons"), "non_monotonic_timeline"
  end

  def valid_drill_timeline
    {
      "t_fault_observed" => "2026-07-21T00:00:00Z", "t_detection_confirmed" => "2026-07-21T00:00:05Z",
      "t_change_requested" => "2026-07-21T00:00:10Z", "t_change_accepted" => "2026-07-21T00:00:12Z",
      "t_route_converged" => "2026-07-21T00:00:20Z", "t_first_backup_success" => "2026-07-21T00:00:40Z",
      "t_primary_recovery_confirmed" => "2026-07-21T00:10:00Z", "t_failback_requested" => "2026-07-21T00:10:10Z",
      "t_failback_converged" => "2026-07-21T00:10:20Z", "t_first_primary_success" => "2026-07-21T00:10:40Z",
      "group" => "group-a",
      "route_evidence_after_failover" => { "state" => "backup", "canonical_hash" => "b" * 64 },
      "route_evidence_after_failback" => { "state" => "primary", "canonical_hash" => "c" * 64 },
      "backup_verification_samples" => %w[sync sse].map { |kind| role_sample(group: "group-a", role_name: "gateway_backup", request_kind: kind) },
      "primary_verification_samples" => %w[sync sse].map { |kind| role_sample(group: "group-a", role_name: "gateway_primary", request_kind: kind) },
      "primary_recovery_window" => { "started_at" => "2026-07-21T00:09:00Z", "completed_at" => "2026-07-21T00:10:00Z" }
    }
  end

  def test_drill_rejects_route_strings_without_read_after_write_and_protocol_proof
    scenario = UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document)
    evaluator = UpstreamBenchmarkNonfunctional::DrillEvaluator.new(scenario: scenario, thresholds: approved_thresholds)
    legacy = valid_drill_timeline.reject do |key, _|
      %w[route_evidence_after_failover route_evidence_after_failback backup_verification_samples primary_verification_samples primary_recovery_window].include?(key)
    end.merge("route_state_after_failover" => "backup", "route_state_after_failback" => "primary")

    result = evaluator.evaluate(timeline: legacy)

    assert_equal "failed", result.fetch("status")
    assert_includes result.fetch("reasons"), "failover_route_state_unproved"
    assert_includes result.fetch("reasons"), "backup_sync_sse_unproved"
    assert_includes result.fetch("reasons"), "primary_recovery_window_unproved"
  end

  def test_cli_capacity_dry_run_is_exact_and_sends_no_network
    Dir.mktmpdir do |dir|
      profile_path = File.join(dir, "profile.yaml")
      File.write(profile_path, YAML.dump(profile_document))
      output = StringIO.new
      error = StringIO.new

      code = UpstreamBenchmarkV2::CLI.run([
        "capacity-dry-run", "--profile", profile_path, "--model-count", "3",
        "--include-discovery", "--topology-verification-requests", "4"
      ], out: output, err: error)
      result = JSON.parse(output.string)

      assert_equal 0, code
      assert_equal 81, result.fetch("maximum_http_requests")
      assert_equal 0, result.fetch("requests_sent")
      assert_equal false, result.fetch("network_sent")
      assert_empty error.string
    end
  end

  def test_cli_capacity_dry_run_rejects_missing_model_count
    Dir.mktmpdir do |dir|
      profile_path = File.join(dir, "profile.yaml")
      File.write(profile_path, YAML.dump(profile_document))
      output = StringIO.new
      error = StringIO.new

      code = UpstreamBenchmarkV2::CLI.run([
        "capacity-dry-run", "--profile", profile_path
      ], out: output, err: error)

      assert_equal 2, code
      assert_empty output.string
      assert_match(/--model-count is required/, error.string)
    end
  end

  def test_cli_capacity_dry_run_defaults_to_v3_profile
    output = StringIO.new
    error = StringIO.new

    code = UpstreamBenchmarkV2::CLI.run(["capacity-dry-run", "--model-count", "1"], out: output, err: error)
    result = JSON.parse(output.string)

    assert_equal 0, code
    assert_equal "bounded-text-capacity-v3", result.fetch("profile_id")
    assert_equal 73, result.fetch("maximum_http_requests")
    assert_empty error.string
  end

  def topology_evidence_document
    {
      "schema_version" => 3,
      "scenario_id" => "two-group-shared-backup-v3",
      "scenario_hash" => UpstreamBenchmarkNonfunctional::TopologyScenario.new(topology_document).scenario_hash,
      "thresholds" => approved_thresholds.merge("approved" => false),
      "shared_capacity_samples" => {
        "shared-backup-pool-1" => valid_pool_samples
      },
      "observation_windows" => [],
      "drill_timeline" => {}
    }
  end

  def test_cli_topology_dry_run_evaluates_offline_evidence_only
    Dir.mktmpdir do |dir|
      scenario_path = File.join(dir, "scenario.yaml")
      evidence_path = File.join(dir, "evidence.json")
      File.write(scenario_path, YAML.dump(topology_document))
      File.write(evidence_path, JSON.generate(topology_evidence_document))
      output = StringIO.new
      error = StringIO.new

      code = UpstreamBenchmarkV2::CLI.run([
        "topology-dry-run", "--scenario", scenario_path, "--evidence", evidence_path
      ], out: output, err: error)
      result = JSON.parse(output.string)

      assert_equal 0, code
      assert_equal "pending_threshold_approval", result.dig("shared_capacity_pools", "shared-backup-pool-1", "status")
      assert_equal "not_evaluated", result.dig("observation", "status")
      assert_equal "not_evaluated", result.dig("drill", "status")
      assert_equal 0, result.fetch("requests_sent")
      assert_equal false, result.fetch("network_sent")
      assert_empty error.string
    end
  end

  def test_cli_topology_dry_run_rejects_secret_shaped_evidence
    Dir.mktmpdir do |dir|
      scenario_path = File.join(dir, "scenario.yaml")
      evidence_path = File.join(dir, "evidence.json")
      evidence = topology_evidence_document.merge("api_key" => "sk-sensitive-temporary-value")
      File.write(scenario_path, YAML.dump(topology_document))
      File.write(evidence_path, JSON.generate(evidence))
      output = StringIO.new
      error = StringIO.new

      code = UpstreamBenchmarkV2::CLI.run([
        "topology-dry-run", "--scenario", scenario_path, "--evidence", evidence_path
      ], out: output, err: error)

      assert_equal 2, code
      assert_empty output.string
      assert_match(/credential fields are forbidden/, error.string)
      refute_includes error.string, "sk-sensitive-temporary-value"
    end
  end

  def test_topology_dry_run_rejects_evidence_bound_to_another_scenario
    Dir.mktmpdir do |dir|
      scenario_path = File.join(dir, "scenario.yaml")
      evidence_path = File.join(dir, "evidence.json")
      evidence = topology_evidence_document.merge("scenario_id" => "other-scenario")
      File.write(scenario_path, YAML.dump(topology_document))
      File.write(evidence_path, JSON.generate(evidence))
      output = StringIO.new
      error = StringIO.new

      code = UpstreamBenchmarkV2::CLI.run([
        "topology-dry-run", "--scenario", scenario_path, "--evidence", evidence_path
      ], out: output, err: error)

      assert_equal 2, code
      assert_empty output.string
      assert_match(/scenario_id does not match/, error.string)
    end
  end
end
