-- Native administrator account monitoring projection.
-- The monitor stores only redacted probe metadata and bounded latency history.

CREATE TABLE IF NOT EXISTS account_monitor_settings (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    interval_seconds INTEGER NOT NULL CHECK (interval_seconds BETWEEN 15 AND 3600),
    updated_by BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO account_monitor_settings (id, interval_seconds, updated_by)
VALUES (1, 300, 0)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS account_monitor_results (
    id BIGSERIAL PRIMARY KEY,
    run_id UUID NOT NULL,
    account_id BIGINT NOT NULL,
    model_id TEXT NOT NULL,
    status TEXT NOT NULL,
    error_code TEXT NOT NULL DEFAULT '',
    http_status INTEGER,
    ttft_ms DOUBLE PRECISION,
    latency_ms DOUBLE PRECISION,
    checked_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_monitor_results_status_check
        CHECK (status IN ('success', 'failed', 'unknown'))
);

CREATE INDEX IF NOT EXISTS account_monitor_results_account_checked_idx
    ON account_monitor_results (account_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS account_monitor_results_checked_idx
    ON account_monitor_results (checked_at);
