CREATE TABLE IF NOT EXISTS relay_ops.upstream_cost_attempts (
    id BIGSERIAL PRIMARY KEY,
    attempt_id TEXT NOT NULL UNIQUE,
    local_request_id TEXT NOT NULL,
    account_id BIGINT NOT NULL,
    adapter_type TEXT NOT NULL CHECK (adapter_type IN ('sub2api', 'newapi')),
    upstream_request_id TEXT,
    model TEXT NOT NULL DEFAULT '',
    input_tokens BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    user_charge NUMERIC(20, 10) NOT NULL DEFAULT 0,
    currency CHAR(3) NOT NULL DEFAULT 'USD',
    request_status TEXT NOT NULL CHECK (request_status IN ('success', 'failed')),
    reconcile_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (reconcile_status IN ('pending', 'matched', 'exception', 'manual', 'conflict')),
    completed_at TIMESTAMPTZ NOT NULL,
    matched_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS upstream_cost_attempts_pending_idx
    ON relay_ops.upstream_cost_attempts (account_id, completed_at, id)
    WHERE reconcile_status IN ('pending', 'exception');

CREATE INDEX IF NOT EXISTS upstream_cost_attempts_request_idx
    ON relay_ops.upstream_cost_attempts (local_request_id, id);

CREATE INDEX IF NOT EXISTS upstream_cost_attempts_upstream_request_idx
    ON relay_ops.upstream_cost_attempts (account_id, upstream_request_id)
    WHERE upstream_request_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS relay_ops.upstream_cost_transactions (
    id BIGSERIAL PRIMARY KEY,
    attempt_id BIGINT REFERENCES relay_ops.upstream_cost_attempts(id),
    account_id BIGINT NOT NULL,
    source_type TEXT NOT NULL
        CHECK (source_type IN ('automatic_charge', 'automatic_refund', 'manual_adjustment', 'manual_reversal')),
    source_record_id TEXT,
    amount NUMERIC(20, 10) NOT NULL CHECK (amount <> 0),
    currency CHAR(3) NOT NULL DEFAULT 'USD',
    effective BOOLEAN NOT NULL DEFAULT TRUE,
    occurred_at TIMESTAMPTZ NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    notes TEXT NOT NULL DEFAULT '',
    created_by_user_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (source_type IN ('automatic_charge', 'manual_adjustment') AND amount > 0)
        OR (source_type IN ('automatic_refund', 'manual_reversal') AND amount < 0)
    ),
    CHECK (
        (source_type IN ('manual_adjustment', 'manual_reversal') AND created_by_user_id IS NOT NULL)
        OR (source_type IN ('automatic_charge', 'automatic_refund') AND created_by_user_id IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS upstream_cost_transactions_attempt_idx
    ON relay_ops.upstream_cost_transactions (attempt_id, occurred_at, id);

CREATE INDEX IF NOT EXISTS upstream_cost_transactions_account_idx
    ON relay_ops.upstream_cost_transactions (account_id, occurred_at, id);

CREATE TABLE IF NOT EXISTS relay_ops.upstream_cost_snapshots (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL,
    adapter_type TEXT NOT NULL CHECK (adapter_type IN ('sub2api', 'newapi')),
    cumulative_amount NUMERIC(20, 10) NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'USD',
    source_cursor TEXT,
    observed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, observed_at)
);

CREATE TABLE IF NOT EXISTS relay_ops.upstream_reconciliation_runs (
    id BIGSERIAL PRIMARY KEY,
    trigger_type TEXT NOT NULL
        CHECK (trigger_type IN ('request_event', 'periodic_sweep', 'admin_refresh', 'daily_close')),
    account_id BIGINT,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'partial', 'failed')),
    scanned_count BIGINT NOT NULL DEFAULT 0,
    matched_count BIGINT NOT NULL DEFAULT 0,
    pending_count BIGINT NOT NULL DEFAULT 0,
    conflict_count BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS upstream_reconciliation_runs_recent_idx
    ON relay_ops.upstream_reconciliation_runs (started_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS relay_ops.upstream_reconciliation_exceptions (
    id BIGSERIAL PRIMARY KEY,
    attempt_id BIGINT NOT NULL REFERENCES relay_ops.upstream_cost_attempts(id),
    reason_code TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    first_detected_at TIMESTAMPTZ NOT NULL,
    last_checked_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    resolution_type TEXT CHECK (resolution_type IN ('automatic', 'manual', 'reversed')),
    UNIQUE (attempt_id)
);

ALTER TABLE relay_ops.accounting_daily_snapshots
    ADD COLUMN IF NOT EXISTS reconciliation_status TEXT NOT NULL DEFAULT 'closing'
        CHECK (reconciliation_status IN ('closing', 'closed', 'exception')),
    ADD COLUMN IF NOT EXISTS cost_coverage_ratio NUMERIC(9, 8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pending_cost_count BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upstream_actual_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upstream_cost_currency CHAR(3) NOT NULL DEFAULT 'USD';
