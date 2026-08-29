-- T85: one idempotent active-probe terminal per group and five-minute bucket.
-- This is an observation ledger only; real request facts remain in usage_logs
-- and ops_error_logs, and account-level attempts remain in
-- account_monitor_results.
CREATE TABLE IF NOT EXISTS account_monitor_bucket_terminals (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL,
    bucket_start TIMESTAMPTZ NOT NULL,
    run_id UUID NOT NULL,
    status TEXT NOT NULL,
    ttft_ms DOUBLE PRECISION,
    latency_ms DOUBLE PRECISION,
    checked_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_monitor_bucket_terminals_status_check
        CHECK (status IN ('success', 'failed')),
    CONSTRAINT account_monitor_bucket_terminals_group_bucket_key
        UNIQUE (group_id, bucket_start)
);

CREATE INDEX IF NOT EXISTS account_monitor_bucket_terminals_checked_idx
    ON account_monitor_bucket_terminals (checked_at DESC);
