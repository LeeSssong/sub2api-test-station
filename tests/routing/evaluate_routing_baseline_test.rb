# frozen_string_literal: true

require "json"
require "minitest/autorun"
require "open3"
require "tempfile"
require "yaml"
require_relative "../../ops/evaluate-routing-baseline"

class EvaluateRoutingBaselineTest < Minitest::Test
  def valid_config
    {
      "schema_version" => 1,
      "routing_id" => "ROUTE01",
      "status" => "fictional",
      "reviewed_at" => "2026-07-15",
      "external_actions_deferred" => true,
      "traffic_stage" => "private_test",
      "policy" => {
        "weights" => {
          "success_rate" => 0.30,
          "cost" => 0.25,
          "ttft" => 0.20,
          "capacity_headroom" => 0.15,
          "support_response" => 0.10
        },
        "thresholds" => {
          "min_observations" => 20,
          "min_balance_days" => 2.0,
          "min_concurrency_headroom_ratio" => 0.10,
          "max_rate_limit_ratio" => 0.15,
          "switch_margin_points" => 8.0,
          "switch_confirmation_windows" => 3
        },
        "normalization" => {
          "success_rate_bad" => 0.90,
          "success_rate_good" => 0.995,
          "cost_usd_per_million_low" => 1.5,
          "cost_usd_per_million_high" => 4.0,
          "ttft_ms_good" => 500,
          "ttft_ms_bad" => 2500,
          "headroom_ratio_low" => 0.10,
          "headroom_ratio_high" => 0.80,
          "support_minutes_good" => 30,
          "support_minutes_bad" => 240
        }
      },
      "retry_policy" => {
        "max_additional_attempts" => 1,
        "retryable_failure_types" => %w[connect_error tls_error timeout],
        "retryable_status_codes" => [408, 429, 500, 502, 503, 504, 529],
        "never_retry_status_codes" => [400, 401, 402, 403, 404, 409, 422],
        "require_response_not_started" => true,
        "require_charge_not_observed" => true
      },
      "circuit_breaker" => {
        "window_size" => 20,
        "minimum_samples" => 20,
        "consecutive_failures_to_open" => 5,
        "failure_ratio_to_open" => 0.50,
        "base_cooldown_seconds" => 60,
        "max_cooldown_seconds" => 900,
        "half_open_probe_interval_seconds" => 30,
        "half_open_successes_to_close" => 2,
        "rate_limit_fallback_cooldown_seconds" => 30,
        "overload_529_cooldown_seconds" => 600
      },
      "capacity_policy" => {
        "vertical" => {
          "oom_restarts_24h" => 1,
          "mem_available_mib_below" => 300,
          "swap_used_mib_above" => 512,
          "cpu_percent_above" => 70,
          "postgres_connection_ratio_above" => 0.80,
          "sustained_minutes" => 15
        },
        "network" => {
          "minimum_samples_per_grid" => 20,
          "entry_latency_p95_ms_above" => 220,
          "entry_packet_loss_percent_above" => 2.0
        },
        "second_node" => {
          "minimum_memory_gib" => 4,
          "entry_availability_7d_below" => 99.5,
          "sustained_sse_concurrency_above" => 30,
          "loss_to_cost_ratio" => 1.5
        }
      },
      "upstreams" => [
        upstream("UP01", terms_status: "unknown", success_rate: 0.98,
                 cost: 2.0, ttft: 1300, headroom: 0.70, support_minutes: 30),
        upstream("UP02", terms_status: "confirmed", success_rate: 0.995,
                 cost: 3.0, ttft: 900, headroom: 0.60, support_minutes: 90),
        upstream("UP03", terms_status: "confirmed", manual_disabled: true),
        upstream("UP04", terms_status: "confirmed", circuit_state: "half_open")
      ],
      "network_measurements" => [],
      "evidence" => {
        "sub2api_version" => "v0.1.155",
        "sub2api_commit" => "41cec0db059ffb82d0efdcfcf07a24ab51fbfe97",
        "notes" => "Fictional offline routing baseline. No real traffic or purchase exists."
      }
    }
  end

  def upstream(id, terms_status:, success_rate: 0.98, cost: 2.4, ttft: 1200,
               headroom: 0.5, support_minutes: 60, rate_limit_ratio: 0.02,
               manual_disabled: false,
               circuit_state: "closed")
    {
      "upstream_id" => id,
      "status" => "fictional",
      "enabled" => true,
      "manual_disabled" => manual_disabled,
      "terms_status" => terms_status,
      "models" => ["gpt-test"],
      "circuit_state" => circuit_state,
      "metrics" => {
        "observations" => 50,
        "success_rate" => success_rate,
        "unit_cost_usd_per_million_tokens" => cost,
        "ttft_p95_ms" => ttft,
        "rate_limit_ratio" => rate_limit_ratio,
        "balance_days_remaining" => 10.0,
        "concurrency_headroom_ratio" => headroom,
        "support_response_minutes" => support_minutes
      },
      "secret_ref" => "sub2api-admin://upstreams/#{id}"
    }
  end

  def config_errors(document = valid_config)
    RoutingBaseline::ConfigValidator.new(document).errors
  end

  def test_accepts_complete_fictional_configuration
    assert_empty config_errors
  end

  def test_rejects_bad_weights_unsafe_retry_count_and_real_samples_in_fictional_config
    document = valid_config
    document["policy"]["weights"]["cost"] = 0.50
    document["retry_policy"]["max_additional_attempts"] = 2
    document["network_measurements"] << {
      "carrier" => "china_telecom",
      "period" => "peak",
      "segment" => "client_to_entry",
      "sample_count" => 20
    }

    errors = config_errors(document)

    assert_includes errors, "policy.weights: values must sum to 1.0"
    assert_includes errors, "retry_policy.max_additional_attempts: must be 0 or 1"
    assert_includes errors, "network_measurements: must be empty while status is fictional"
  end

  def test_rejects_credential_fields_and_secret_like_values
    document = valid_config
    document["upstreams"].first["api_key"] = "not-a-real-key"
    document["evidence"]["notes"] = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456"

    errors = config_errors(document)

    assert_includes errors, "upstreams[0].api_key: credential fields are forbidden; use secret_ref"
    assert_includes errors, "evidence.notes: value looks like a secret"
  end

  def test_rejects_missing_circuit_and_capacity_fields_before_runtime
    document = valid_config
    document["circuit_breaker"].delete("failure_ratio_to_open")
    document["capacity_policy"]["vertical"].delete("sustained_minutes")

    errors = config_errors(document)

    assert_includes errors, "circuit_breaker.failure_ratio_to_open: is required"
    assert_includes errors, "capacity_policy.vertical.sustained_minutes: is required"
  end

  def test_ranks_eligible_channels_and_separates_probe_only_and_excluded
    result = RoutingBaseline::Ranker.new(valid_config).rank("gpt-test")

    assert_equal "UP01", result.fetch("recommended_primary")
    assert_equal %w[UP01 UP02], result.fetch("eligible").map { |item| item.fetch("upstream_id") }
    assert_equal ["UP04"], result.fetch("probe_only").map { |item| item.fetch("upstream_id") }
    assert_equal ["UP03"], result.fetch("excluded").map { |item| item.fetch("upstream_id") }
    assert_operator result.dig("eligible", 0, "score"), :>, result.dig("eligible", 1, "score")
    assert_equal %w[capacity_headroom cost success_rate support_response ttft],
                 result.dig("eligible", 0, "components").keys.sort
  end

  def test_paid_public_stage_excludes_unknown_resale_terms
    document = valid_config
    document["traffic_stage"] = "paid_public"

    result = RoutingBaseline::Ranker.new(document).rank("gpt-test")

    assert_equal ["UP02"], result.fetch("eligible").map { |item| item.fetch("upstream_id") }
    unknown = result.fetch("excluded").find { |item| item.fetch("upstream_id") == "UP01" }
    assert_includes unknown.fetch("reasons"), "terms_not_confirmed_for_paid_public"
  end

  def test_excludes_channel_when_rate_limit_ratio_exceeds_gate
    document = valid_config
    document["upstreams"][1]["metrics"]["rate_limit_ratio"] = 0.20

    result = RoutingBaseline::Ranker.new(document).rank("gpt-test")

    assert_equal ["UP01"], result.fetch("eligible").map { |item| item.fetch("upstream_id") }
    limited = result.fetch("excluded").find { |item| item.fetch("upstream_id") == "UP02" }
    assert_includes limited.fetch("reasons"), "rate_limit_ratio_too_high"
  end

  def test_allows_only_pre_write_or_confirmed_non_billed_retry
    policy = RoutingBaseline::RetryPolicy.new(valid_config.fetch("retry_policy"))

    connect = policy.decide(
      "attempts_used" => 0,
      "failure_type" => "connect_error",
      "status_code" => nil,
      "response_started" => false,
      "request_body_sent" => false,
      "charge_state" => "not_charged",
      "idempotency_supported" => false,
      "different_eligible_target" => true
    )
    rejected_429 = policy.decide(connect_event("http_error", 429, "unknown"))
    accepted_429 = policy.decide(connect_event("http_error", 429, "not_charged"))

    assert_equal true, connect.fetch("retry")
    assert_equal false, rejected_429.fetch("retry")
    assert_equal "charge_state_unknown", rejected_429.fetch("reason")
    assert_equal true, accepted_429.fetch("retry")
  end

  def test_rejects_retry_after_output_user_error_or_attempt_exhaustion
    policy = RoutingBaseline::RetryPolicy.new(valid_config.fetch("retry_policy"))

    started = connect_event("connect_error", nil, "not_charged").merge("response_started" => true)
    user_error = connect_event("http_error", 400, "not_charged")
    exhausted = connect_event("connect_error", nil, "not_charged").merge("attempts_used" => 1)

    assert_equal "response_already_started", policy.decide(started).fetch("reason")
    assert_equal "status_never_retry", policy.decide(user_error).fetch("reason")
    assert_equal "attempt_limit_reached", policy.decide(exhausted).fetch("reason")
  end

  def connect_event(failure_type, status_code, charge_state)
    {
      "attempts_used" => 0,
      "failure_type" => failure_type,
      "status_code" => status_code,
      "response_started" => false,
      "request_body_sent" => failure_type != "connect_error",
      "charge_state" => charge_state,
      "idempotency_supported" => false,
      "different_eligible_target" => true
    }
  end

  def test_circuit_opens_then_half_opens_and_closes_after_probes
    breaker = RoutingBaseline::CircuitBreaker.new(valid_config.fetch("circuit_breaker"))
    state = breaker.initial_state
    now = Time.utc(2026, 7, 15, 0, 0, 0)

    5.times { |index| state = breaker.apply(state, "failure", now: now + index) }
    assert_equal "open", state.fetch("state")
    assert_equal 60, state.fetch("cooldown_seconds")

    state = breaker.apply(state, "tick", now: now + 63)
    assert_equal "open", state.fetch("state")
    state = breaker.apply(state, "tick", now: now + 64)
    assert_equal "half_open", state.fetch("state")

    state = breaker.apply(state, "success", now: now + 65)
    assert_equal "half_open", state.fetch("state")
    state = breaker.apply(state, "success", now: now + 66)
    assert_equal "closed", state.fetch("state")
  end

  def test_half_open_failure_reopens_with_backoff
    breaker = RoutingBaseline::CircuitBreaker.new(valid_config.fetch("circuit_breaker"))
    now = Time.utc(2026, 7, 15, 0, 0, 0)
    state = breaker.initial_state.merge(
      "state" => "half_open",
      "reopen_count" => 1,
      "half_open_successes" => 0
    )

    state = breaker.apply(state, "failure", now: now)

    assert_equal "open", state.fetch("state")
    assert_equal 2, state.fetch("reopen_count")
    assert_equal 120, state.fetch("cooldown_seconds")
  end

  def test_half_open_allows_only_one_probe_per_interval
    breaker = RoutingBaseline::CircuitBreaker.new(valid_config.fetch("circuit_breaker"))
    now = Time.utc(2026, 7, 15, 0, 0, 0)
    state = breaker.initial_state.merge("state" => "half_open")

    first = breaker.apply(state, "probe", now: now)
    too_soon = breaker.apply(first, "probe", now: now + 29)
    next_probe = breaker.apply(too_soon, "probe", now: now + 30)

    assert_equal "probe_allowed", first.fetch("last_action")
    assert_equal "probe_rejected_interval", too_soon.fetch("last_action")
    assert_equal "probe_allowed", next_probe.fetch("last_action")
  end

  def test_manual_disable_never_auto_recovers
    breaker = RoutingBaseline::CircuitBreaker.new(valid_config.fetch("circuit_breaker"))
    now = Time.utc(2026, 7, 15, 0, 0, 0)
    state = breaker.apply(breaker.initial_state, "manual_disable", now: now)

    assert_equal "manual_open", state.fetch("state")
    state = breaker.apply(state, "tick", now: now + 86_400)
    assert_equal "manual_open", state.fetch("state")
    state = breaker.apply(state, "manual_enable", now: now + 86_401)
    assert_equal "closed", state.fetch("state")
  end

  def test_capacity_advice_requires_observation_and_economic_gates
    advisor = RoutingBaseline::CapacityAdvisor.new(valid_config.fetch("capacity_policy"))

    quiet = advisor.recommend(capacity_metrics)
    vertical = advisor.recommend(capacity_metrics.merge(
      "mem_available_mib" => 250,
      "resource_pressure_minutes" => 15
    ))
    all_gates = advisor.recommend(capacity_metrics.merge(
      "current_memory_gib" => 4,
      "vertical_upgrade_completed" => true,
      "grid_samples_complete" => true,
      "entry_is_primary_bottleneck" => true,
      "entry_latency_p95_ms" => 260,
      "entry_availability_7d_percent" => 99.2,
      "sse_concurrency" => 35,
      "pressure_minutes" => 15,
      "expected_monthly_incident_loss_cny" => 180,
      "second_node_monthly_cost_cny" => 100
    ))

    assert_equal false, quiet.fetch("vertical_upgrade")
    assert_equal true, vertical.fetch("vertical_upgrade")
    assert_equal false, vertical.fetch("optimized_line")
    assert_equal false, vertical.fetch("second_node")
    assert_equal true, all_gates.fetch("optimized_line")
    assert_equal true, all_gates.fetch("second_node")
  end

  def capacity_metrics
    {
      "current_memory_gib" => 2,
      "vertical_upgrade_completed" => false,
      "oom_restarts_24h" => 0,
      "mem_available_mib" => 800,
      "swap_used_mib" => 0,
      "cpu_percent" => 20,
      "postgres_connection_ratio" => 0.20,
      "resource_pressure_minutes" => 0,
      "grid_samples_complete" => false,
      "entry_is_primary_bottleneck" => false,
      "entry_latency_p95_ms" => 80,
      "entry_packet_loss_percent" => 0.0,
      "entry_availability_7d_percent" => 100.0,
      "sse_concurrency" => 2,
      "pressure_minutes" => 0,
      "expected_monthly_incident_loss_cny" => 0,
      "second_node_monthly_cost_cny" => 100
    }
  end

  def test_validate_cli_is_non_sensitive_and_offline
    Tempfile.create(["route01", ".yaml"]) do |file|
      file.write(YAML.dump(valid_config))
      file.flush

      stdout, stderr, status = Open3.capture3(
        "ruby", "ops/evaluate-routing-baseline.rb", "validate", file.path
      )

      assert status.success?, stderr
      payload = JSON.parse(stdout)
      assert_equal true, payload.fetch("valid")
      assert_equal true, payload.fetch("offline_simulation")
      assert_equal false, payload.fetch("real_traffic_sent")
      refute_match(/api_key|Bearer|Cookie|Authorization/i, stdout)
    end
  end
end
