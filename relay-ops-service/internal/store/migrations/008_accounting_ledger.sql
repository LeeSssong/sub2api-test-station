CREATE TABLE IF NOT EXISTS relay_ops.accounting_cash_events (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL CHECK (event_type IN ('account_purchase', 'upstream_topup', 'refund', 'fee')),
    paid_at TIMESTAMPTZ NOT NULL,
    amount_cny NUMERIC(20, 8) NOT NULL,
    source_kind TEXT NOT NULL CHECK (source_kind IN ('owned_oauth', 'upstream_apikey')),
    account_id BIGINT,
    notes TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL UNIQUE,
    created_by_user_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (event_type = 'refund' AND amount_cny < 0)
        OR (event_type <> 'refund' AND amount_cny > 0)
    )
);

CREATE INDEX IF NOT EXISTS accounting_cash_events_paid_at_idx
    ON relay_ops.accounting_cash_events (paid_at, id);

CREATE TABLE IF NOT EXISTS relay_ops.accounting_daily_snapshots (
    report_date DATE PRIMARY KEY,
    external_revenue_cny NUMERIC(20, 8) NOT NULL,
    external_requests BIGINT NOT NULL,
    internal_requests BIGINT NOT NULL,
    customer_resource_cost_cny NUMERIC(20, 8) NOT NULL,
    internal_resource_cost_cny NUMERIC(20, 8) NOT NULL,
    resource_cost_cny NUMERIC(20, 8) NOT NULL,
    operating_gross_profit_cny NUMERIC(20, 8) NOT NULL,
    cash_outflow_cny NUMERIC(20, 8) NOT NULL,
    cash_net_result_cny NUMERIC(20, 8) NOT NULL,
    unlinked_cash_outflow_cny NUMERIC(20, 8) NOT NULL,
    cash_event_count BIGINT NOT NULL,
    owned_oauth_cost_cny NUMERIC(20, 8) NOT NULL,
    upstream_apikey_cost_cny NUMERIC(20, 8) NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
