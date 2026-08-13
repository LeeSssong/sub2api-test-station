-- T03-R1 independent financial evidence and account-day storage.
-- Expand-only: no historical backfill and no changes to usage_logs.

CREATE TABLE IF NOT EXISTS usage_upstream_cost_evidence (
    id                    BIGSERIAL PRIMARY KEY,
    usage_log_id          BIGINT NOT NULL UNIQUE REFERENCES usage_logs(id) ON DELETE CASCADE,
    source                VARCHAR(20) NOT NULL CHECK (source IN ('sub', 'newapi')),
    upstream_request_id   VARCHAR(255),
    upstream_billing_time TIMESTAMPTZ,
    upstream_model        VARCHAR(255),
    sub_actual_cost       NUMERIC(20,10),
    newapi_quota          NUMERIC(20,10),
    newapi_quota_per_unit NUMERIC(20,10),
    normalized_cost_cny   NUMERIC(20,10),
    profit_cny            NUMERIC(20,10),
    evidence_status       VARCHAR(20) NOT NULL CHECK (evidence_status IN ('confirmed', 'confirmed_zero', 'unavailable')),
    reason_code           VARCHAR(64),
    recorded_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_upstream_cost_evidence_status_usage_log_id
    ON usage_upstream_cost_evidence (evidence_status, usage_log_id);

CREATE TABLE IF NOT EXISTS usage_cost_reviews (
    id               BIGSERIAL PRIMARY KEY,
    usage_log_id     BIGINT NOT NULL UNIQUE REFERENCES usage_logs(id) ON DELETE CASCADE,
    review_status    VARCHAR(20) NOT NULL DEFAULT 'reviewed' CHECK (review_status = 'reviewed'),
    manual_cost_cny  NUMERIC(20,10) NOT NULL DEFAULT 0,
    manual_profit_cny NUMERIC(20,10) NOT NULL,
    reviewed_by      BIGINT NOT NULL,
    reviewed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_cost_reviews_usage_log_id
    ON usage_cost_reviews (usage_log_id);

CREATE TABLE IF NOT EXISTS account_daily_financial_values (
    id                         BIGSERIAL PRIMARY KEY,
    account_id                 BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    business_date              DATE NOT NULL,
    oauth_cost_cny             NUMERIC(20,10),
    revenue_override_cny       NUMERIC(20,10),
    revenue_override_at        TIMESTAMPTZ,
    revenue_evidence_cutoff_id BIGINT,
    revenue_review_cutoff_id   BIGINT,
    cost_override_cny          NUMERIC(20,10),
    cost_override_at           TIMESTAMPTZ,
    cost_evidence_cutoff_id    BIGINT,
    cost_review_cutoff_id      BIGINT,
    updated_by                 BIGINT NOT NULL,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_daily_financial_values_account_date_key UNIQUE (account_id, business_date)
);

CREATE INDEX IF NOT EXISTS idx_account_daily_financial_values_account_id_business_date
    ON account_daily_financial_values (account_id, business_date);

CREATE TABLE IF NOT EXISTS account_financial_settings (
    id         BIGSERIAL PRIMARY KEY,
    key        VARCHAR(100) NOT NULL UNIQUE DEFAULT 't03_r1_account_financial',
    enabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
