CREATE TABLE IF NOT EXISTS account_monitor_v4_snapshots (
    "window" TEXT NOT NULL CHECK ("window" IN ('24h', '7d', '30d')),
    group_id BIGINT NOT NULL CHECK (group_id > 0),
    snapshot_id UUID NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    contract_version TEXT NOT NULL,
    success_rate DOUBLE PRECISION CHECK (success_rate IS NULL OR success_rate >= 0),
    request_count INTEGER NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    success_count INTEGER NOT NULL DEFAULT 0 CHECK (success_count >= 0),
    real_request_count INTEGER NOT NULL DEFAULT 0 CHECK (real_request_count >= 0),
    real_success_count INTEGER NOT NULL DEFAULT 0 CHECK (real_success_count >= 0),
    probe_fallback_bucket_count INTEGER NOT NULL DEFAULT 0 CHECK (probe_fallback_bucket_count >= 0),
    probe_fallback_request_count INTEGER NOT NULL DEFAULT 0 CHECK (probe_fallback_request_count >= 0),
    missing_probe_terminal_count INTEGER NOT NULL DEFAULT 0 CHECK (missing_probe_terminal_count >= 0),
    ttft_p95_ms DOUBLE PRECISION,
    ttft_sample_count INTEGER NOT NULL DEFAULT 0 CHECK (ttft_sample_count >= 0),
    latency_p95_ms DOUBLE PRECISION,
    latency_sample_count INTEGER NOT NULL DEFAULT 0 CHECK (latency_sample_count >= 0),
    cache_hit_rate DOUBLE PRECISION,
    source_updated_at TIMESTAMPTZ,
    current_operational BOOLEAN NOT NULL,
    CHECK (window_start < window_end),
    UNIQUE ("window", group_id)
);

CREATE INDEX IF NOT EXISTS account_monitor_v4_snapshots_window_generated_idx
    ON account_monitor_v4_snapshots ("window", generated_at DESC);
