-- Migration: 224_account_probe_cost_logs
-- Isolated append-only ledger for native operational probe costs.
CREATE TABLE IF NOT EXISTS account_probe_cost_logs (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    group_id BIGINT,
    probe_run_id VARCHAR(128) NOT NULL,
    probe_kind VARCHAR(16) NOT NULL CHECK (probe_kind IN ('monitor', 'scheduled', 'manual')),
    model VARCHAR(100) NOT NULL,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    account_cost DECIMAL(20,10),
    usage_completeness VARCHAR(16) NOT NULL CHECK (usage_completeness IN ('complete', 'partial', 'unknown')),
    probe_outcome VARCHAR(16) NOT NULL CHECK (probe_outcome IN ('success', 'failure')),
    error_code VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_probe_cost_logs_probe_run_id_key UNIQUE (probe_run_id)
);

CREATE INDEX IF NOT EXISTS idx_account_probe_cost_logs_created_at
    ON account_probe_cost_logs (created_at);
CREATE INDEX IF NOT EXISTS idx_account_probe_cost_logs_account_created_at
    ON account_probe_cost_logs (account_id, created_at);
CREATE INDEX IF NOT EXISTS idx_account_probe_cost_logs_group_created_at
    ON account_probe_cost_logs (group_id, created_at);
