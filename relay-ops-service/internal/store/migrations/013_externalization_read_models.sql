CREATE SCHEMA IF NOT EXISTS relay_ops;

CREATE TABLE IF NOT EXISTS relay_ops.externalization_events (
    event_id TEXT PRIMARY KEY,
    source_version TEXT NOT NULL,
    event_type TEXT NOT NULL,
    contract_version INTEGER NOT NULL CHECK (contract_version > 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    status TEXT NOT NULL CHECK (status IN ('processing', 'processed', 'dead')),
    attempts INTEGER NOT NULL DEFAULT 1 CHECK (attempts > 0),
    lease_until TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_externalization_events_resume
    ON relay_ops.externalization_events (status, lease_until, occurred_at, event_id);

CREATE TABLE IF NOT EXISTS relay_ops.externalization_watermarks (
    source TEXT PRIMARY KEY,
    last_event_id TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL,
    completeness TEXT NOT NULL CHECK (completeness IN ('empty', 'partial', 'complete'))
);

CREATE TABLE IF NOT EXISTS relay_ops.externalization_dead_letters (
    event_id TEXT PRIMARY KEY REFERENCES relay_ops.externalization_events(event_id),
    source_version TEXT NOT NULL,
    event_type TEXT NOT NULL,
    contract_version INTEGER NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    error TEXT NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS relay_ops.account_read_models (
    account_id BIGINT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'unknown',
    balance NUMERIC,
    currency TEXT,
    observed_at TIMESTAMPTZ,
    health_occurred_at TIMESTAMPTZ,
    health_event_id TEXT,
    balance_occurred_at TIMESTAMPTZ,
    balance_event_id TEXT,
    generated_at TIMESTAMPTZ NOT NULL,
    source_watermark TEXT NOT NULL,
    freshness_seconds BIGINT NOT NULL CHECK (freshness_seconds >= 0),
    completeness TEXT NOT NULL,
    calculation_version TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS relay_ops.profitability_read_models (
    account_id BIGINT PRIMARY KEY,
    requests BIGINT NOT NULL CHECK (requests >= 0),
    revenue NUMERIC NOT NULL,
    cost NUMERIC NOT NULL,
    profit NUMERIC NOT NULL,
    margin NUMERIC NOT NULL,
    rank INTEGER NOT NULL CHECK (rank > 0),
    source_occurred_at TIMESTAMPTZ NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    source_watermark TEXT NOT NULL,
    freshness_seconds BIGINT NOT NULL CHECK (freshness_seconds >= 0),
    completeness TEXT NOT NULL,
    calculation_version TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_profitability_read_models_rank
    ON relay_ops.profitability_read_models (rank, account_id);

CREATE TABLE IF NOT EXISTS relay_ops.accounting_read_models (
    scope TEXT PRIMARY KEY,
    requests BIGINT NOT NULL CHECK (requests >= 0),
    revenue NUMERIC NOT NULL,
    cost NUMERIC NOT NULL,
    source_occurred_at TIMESTAMPTZ NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    source_watermark TEXT NOT NULL,
    freshness_seconds BIGINT NOT NULL CHECK (freshness_seconds >= 0),
    completeness TEXT NOT NULL,
    calculation_version TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS relay_ops.reconciliation_read_models (
    scope TEXT PRIMARY KEY,
    matched BIGINT NOT NULL CHECK (matched >= 0),
    exceptions BIGINT NOT NULL CHECK (exceptions >= 0),
    source_occurred_at TIMESTAMPTZ NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    source_watermark TEXT NOT NULL,
    freshness_seconds BIGINT NOT NULL CHECK (freshness_seconds >= 0),
    completeness TEXT NOT NULL,
    calculation_version TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS relay_ops.externalization_rebuild_jobs (
    job_id BIGSERIAL PRIMARY KEY,
    projection TEXT NOT NULL,
    snapshot_watermark TEXT,
    target_watermark TEXT,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
