CREATE TABLE IF NOT EXISTS relay_ops.quality_reports (
    report_id TEXT PRIMARY KEY,
    report_hash TEXT NOT NULL CHECK (report_hash ~ '^[0-9a-f]{64}$'),
    upstream_id BIGINT NOT NULL REFERENCES relay_ops.upstreams(id),
    upstream_name TEXT NOT NULL,
    job_kind TEXT NOT NULL CHECK (job_kind IN ('health_pulse', 'catalog_quick', 'capacity_check')),
    status TEXT NOT NULL CHECK (status IN ('blocked', 'needs_evidence', 'not_better', 'review_recommended', 'eligible_for_manual_switch')),
    quality_score INTEGER NOT NULL CHECK (quality_score BETWEEN 0 AND 90),
    total_score INTEGER NOT NULL CHECK (total_score BETWEEN 0 AND 100),
    direct_summary TEXT NOT NULL,
    gateway_summary TEXT NOT NULL,
    models_summary TEXT NOT NULL,
    pricing_summary TEXT NOT NULL,
    capacity_summary TEXT NOT NULL,
    record JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS quality_reports_recorded_idx
    ON relay_ops.quality_reports (recorded_at DESC, report_id DESC);
