CREATE SCHEMA IF NOT EXISTS relay_ops;

CREATE TABLE IF NOT EXISTS relay_ops.upstreams (
    id BIGSERIAL PRIMARY KEY,
    display_name TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL CHECK (role IN ('production', 'candidate', 'paused', 'rejected')),
    base_url TEXT NOT NULL UNIQUE,
    site_url TEXT,
    pricing_url TEXT,
    usage_url TEXT,
    performance_url TEXT,
    adapter_type TEXT NOT NULL,
    candidate_probe_secret_ref TEXT,
    sub2api_channel_monitor_id BIGINT,
    advertised_multiplier_bps BIGINT,
    billing_evidence_status TEXT NOT NULL DEFAULT 'not_requested',
    monitor_status TEXT NOT NULL DEFAULT 'unknown',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_success_at TIMESTAMPTZ,
    last_error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS relay_ops.secret_refs (
    secret_ref TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    owner_scope TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    last_four TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS secret_refs_candidate_probe_fingerprint_uidx
    ON relay_ops.secret_refs (fingerprint)
    WHERE kind = 'candidate_probe_key';

CREATE TABLE IF NOT EXISTS relay_ops.public_groups (
    group_id BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL,
    customer_visible BOOLEAN NOT NULL,
    user_multiplier_bps BIGINT NOT NULL,
    model_allowlist JSONB NOT NULL DEFAULT '[]'::JSONB,
    upstream_route_refs JSONB NOT NULL DEFAULT '[]'::JSONB,
    sub2api_channel_monitor_ids JSONB NOT NULL DEFAULT '[]'::JSONB,
    qualification_run_id TEXT,
    qualified_at TIMESTAMPTZ,
    health_gate TEXT NOT NULL DEFAULT 'pending',
    source_revision TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS relay_ops.upstream_public_groups (
    upstream_id BIGINT NOT NULL REFERENCES relay_ops.upstreams(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES relay_ops.public_groups(group_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (upstream_id, group_id)
);

CREATE TABLE IF NOT EXISTS relay_ops.pricing_snapshots (
    id BIGSERIAL PRIMARY KEY,
    upstream_id BIGINT NOT NULL REFERENCES relay_ops.upstreams(id),
    source_url TEXT NOT NULL,
    source_type TEXT NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    content_hash TEXT NOT NULL,
    normalized_payload JSONB NOT NULL,
    diff_summary JSONB,
    evidence_level TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS pricing_snapshots_upstream_fetched_idx ON relay_ops.pricing_snapshots(upstream_id, fetched_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS relay_ops.probe_runs (
    id BIGSERIAL PRIMARY KEY,
    run_id TEXT NOT NULL UNIQUE,
    upstream_id BIGINT NOT NULL REFERENCES relay_ops.upstreams(id),
    group_id BIGINT,
    model_id TEXT,
    probe_kind TEXT NOT NULL,
    status TEXT NOT NULL,
    http_status INTEGER,
    error_class TEXT,
    input_tokens BIGINT,
    output_tokens BIGINT,
    cache_tokens BIGINT,
    ttft_ms BIGINT,
    total_latency_ms BIGINT,
    tps DOUBLE PRECISION,
    sse_done BOOLEAN,
    estimated_standard_cost_microusd BIGINT,
    estimated_upstream_cost_microusd BIGINT,
    actual_upstream_cost_microusd BIGINT,
    evidence_level TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    redaction_version TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS relay_ops.metric_refs (
    id BIGSERIAL PRIMARY KEY,
    source_kind TEXT NOT NULL,
    external_id TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    payload_hash TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_kind, external_id, window_start, window_end, payload_hash)
);

CREATE TABLE IF NOT EXISTS relay_ops.comparison_windows (
    id BIGSERIAL PRIMARY KEY,
    upstream_id BIGINT REFERENCES relay_ops.upstreams(id),
    group_id BIGINT,
    model_id TEXT,
    source_kind TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    sample_count BIGINT NOT NULL,
    metrics JSONB NOT NULL,
    schema_version TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS relay_ops.cost_observations (
    id BIGSERIAL PRIMARY KEY,
    upstream_id BIGINT NOT NULL REFERENCES relay_ops.upstreams(id),
    source TEXT NOT NULL,
    standard_cost_microusd BIGINT,
    actual_cost_microusd BIGINT,
    effective_multiplier_bps BIGINT,
    model_id TEXT,
    group_id BIGINT,
    confidence TEXT NOT NULL,
    comparison_note TEXT,
    observed_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS relay_ops.candidate_comparisons (
    id BIGSERIAL PRIMARY KEY,
    upstream_id BIGINT NOT NULL REFERENCES relay_ops.upstreams(id),
    group_id BIGINT NOT NULL,
    model_id TEXT NOT NULL,
    more_stable BOOLEAN NOT NULL,
    faster_ttft BOOLEAN NOT NULL,
    cheaper BOOLEAN NOT NULL,
    overall_better BOOLEAN NOT NULL,
    evidence_window JSONB NOT NULL,
    sample_count BIGINT NOT NULL,
    comparison_status TEXT NOT NULL,
    recommended_action TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS relay_ops.incidents (
    id BIGSERIAL PRIMARY KEY,
    incident_key TEXT NOT NULL UNIQUE,
    severity TEXT NOT NULL CHECK (severity IN ('P0', 'P1', 'P2')),
    state TEXT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_notified_at TIMESTAMPTZ,
    next_review_at TIMESTAMPTZ,
    baseline_value TEXT,
    current_value TEXT,
    sample_count BIGINT NOT NULL DEFAULT 1,
    evidence_refs JSONB NOT NULL DEFAULT '[]'::JSONB,
    recommended_action TEXT,
    human_decision TEXT,
    closed_by BIGINT
);

CREATE TABLE IF NOT EXISTS relay_ops.notification_deliveries (
    id BIGSERIAL PRIMARY KEY,
    incident_id BIGINT NOT NULL REFERENCES relay_ops.incidents(id),
    dedup_key TEXT NOT NULL UNIQUE,
    message_hash TEXT NOT NULL,
    delivery_status TEXT NOT NULL,
    response_code INTEGER,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS relay_ops.auth_sessions (
    upstream_id BIGINT PRIMARY KEY REFERENCES relay_ops.upstreams(id),
    secret_ref TEXT NOT NULL,
    auth_mode TEXT NOT NULL,
    status TEXT NOT NULL,
    login_url TEXT NOT NULL,
    expires_at TIMESTAMPTZ,
    last_refresh_at TIMESTAMPTZ,
    last_success_at TIMESTAMPTZ,
    last_failure_reason TEXT,
    last_notified_at TIMESTAMPTZ,
    scope TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS relay_ops.agent_analyses (
    id BIGSERIAL PRIMARY KEY,
    analysis_id TEXT NOT NULL UNIQUE,
    incident_id BIGINT NOT NULL REFERENCES relay_ops.incidents(id),
    model_provider TEXT NOT NULL,
    prompt_contract_version TEXT NOT NULL,
    result JSONB NOT NULL,
    confidence DOUBLE PRECISION,
    requires_human_approval BOOLEAN NOT NULL,
    delivery_status TEXT NOT NULL,
    output_hash TEXT NOT NULL,
    estimated_cost_microusd BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS relay_ops.audit_events (
    id BIGSERIAL PRIMARY KEY,
    actor_user_id BIGINT,
    action TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    before_summary JSONB,
    after_summary JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS relay_ops.scheduler_jobs (
    job_key TEXT PRIMARY KEY,
    next_due_at TIMESTAMPTZ NOT NULL,
    last_started_at TIMESTAMPTZ,
    last_finished_at TIMESTAMPTZ,
    last_status TEXT,
    last_error_code TEXT
);
