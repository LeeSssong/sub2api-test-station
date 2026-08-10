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
    claim_token TEXT,
    claim_generation BIGINT NOT NULL DEFAULT 0,
    lease_until TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Upgrade the placeholder Task 3 schema in place. These are expand-only and
-- retain legacy rows as already processed facts.
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS contract_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'processed';
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 1;
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS claim_token TEXT;
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS claim_generation BIGINT NOT NULL DEFAULT 0;
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS last_error TEXT;
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE relay_ops.externalization_events ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_externalization_events_resume
    ON relay_ops.externalization_events (status, lease_until, occurred_at, event_id);

CREATE TABLE IF NOT EXISTS relay_ops.externalization_watermarks (
    source TEXT PRIMARY KEY,
    last_event_id TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL,
    completeness TEXT NOT NULL CHECK (completeness IN ('empty', 'partial', 'complete')),
    calculation_version TEXT NOT NULL DEFAULT 'event-consumer-v1'
);

ALTER TABLE relay_ops.externalization_watermarks ADD COLUMN IF NOT EXISTS calculation_version TEXT NOT NULL DEFAULT 'event-consumer-v1';

CREATE TABLE IF NOT EXISTS relay_ops.externalization_dead_letters (
    event_id TEXT PRIMARY KEY,
    source_version TEXT NOT NULL,
    event_type TEXT NOT NULL,
    contract_version INTEGER NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL,
    error TEXT NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE relay_ops.externalization_dead_letters ADD COLUMN IF NOT EXISTS source_version TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE relay_ops.externalization_dead_letters ADD COLUMN IF NOT EXISTS event_type TEXT NOT NULL DEFAULT 'unknown';
ALTER TABLE relay_ops.externalization_dead_letters ADD COLUMN IF NOT EXISTS contract_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE relay_ops.externalization_dead_letters ADD COLUMN IF NOT EXISTS occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE relay_ops.externalization_dead_letters ADD COLUMN IF NOT EXISTS failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE relay_ops.externalization_dead_letters ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

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

ALTER TABLE relay_ops.account_read_models ADD COLUMN IF NOT EXISTS health_occurred_at TIMESTAMPTZ;
ALTER TABLE relay_ops.account_read_models ADD COLUMN IF NOT EXISTS health_event_id TEXT;
ALTER TABLE relay_ops.account_read_models ADD COLUMN IF NOT EXISTS balance_occurred_at TIMESTAMPTZ;
ALTER TABLE relay_ops.account_read_models ADD COLUMN IF NOT EXISTS balance_event_id TEXT;

UPDATE relay_ops.account_read_models
SET health_occurred_at = COALESCE(health_occurred_at, observed_at),
    health_event_id = COALESCE(NULLIF(health_event_id, ''), source_watermark)
WHERE observed_at IS NOT NULL
  AND NULLIF(source_watermark, '') IS NOT NULL
  AND NULLIF(status, '') IS NOT NULL
  AND (health_occurred_at IS NULL OR NULLIF(health_event_id, '') IS NULL);

UPDATE relay_ops.account_read_models
SET balance_occurred_at = COALESCE(balance_occurred_at, observed_at),
    balance_event_id = COALESCE(NULLIF(balance_event_id, ''), source_watermark)
WHERE observed_at IS NOT NULL
  AND NULLIF(source_watermark, '') IS NOT NULL
  AND (balance IS NOT NULL OR NULLIF(currency, '') IS NOT NULL)
  AND (balance_occurred_at IS NULL OR NULLIF(balance_event_id, '') IS NULL);

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
