CREATE SCHEMA IF NOT EXISTS relay_ops;
CREATE TABLE IF NOT EXISTS relay_ops.externalization_events (
 event_id TEXT PRIMARY KEY, source_version TEXT NOT NULL, event_type TEXT NOT NULL,
 occurred_at TIMESTAMPTZ NOT NULL, processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), payload JSONB NOT NULL
);
CREATE TABLE IF NOT EXISTS relay_ops.externalization_watermarks (
 source TEXT PRIMARY KEY, last_event_id TEXT NOT NULL, occurred_at TIMESTAMPTZ NOT NULL,
 processed_at TIMESTAMPTZ NOT NULL, completeness TEXT NOT NULL, calculation_version TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS relay_ops.externalization_dead_letters (
 event_id TEXT PRIMARY KEY, error TEXT NOT NULL, payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS relay_ops.account_read_models (
 account_id BIGINT PRIMARY KEY, status TEXT NOT NULL DEFAULT 'unknown', balance NUMERIC, currency TEXT,
 observed_at TIMESTAMPTZ, generated_at TIMESTAMPTZ NOT NULL, source_watermark TEXT NOT NULL,
 freshness_seconds BIGINT NOT NULL DEFAULT 0, completeness TEXT NOT NULL, calculation_version TEXT NOT NULL
);
