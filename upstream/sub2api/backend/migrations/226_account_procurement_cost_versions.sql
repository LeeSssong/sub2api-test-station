-- T23: append-only, versioned procurement cost ledger.
CREATE TABLE IF NOT EXISTS account_procurement_cost_versions (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    version_no INTEGER NOT NULL,
    cost_cny NUMERIC(14,2),
    estimated_usable_quota_usd NUMERIC(14,2),
    effective_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    settled_at TIMESTAMPTZ,
    loss_cny NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (loss_cny >= 0),
    status VARCHAR(32) NOT NULL DEFAULT 'active' CHECK (status IN ('active','ended','settled','cost_pending')),
    actor_user_id BIGINT,
    request_id VARCHAR(128),
    settlement_request_id VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_procurement_cost_versions_account_version_key UNIQUE (account_id, version_no),
    CONSTRAINT account_procurement_cost_versions_dates CHECK (ended_at IS NULL OR ended_at >= effective_at),
    CONSTRAINT account_procurement_cost_versions_values CHECK (
      (status = 'cost_pending' AND cost_cny IS NULL AND estimated_usable_quota_usd IS NULL)
      OR (status <> 'cost_pending' AND cost_cny >= 0 AND estimated_usable_quota_usd > 0)
    )
);
CREATE UNIQUE INDEX IF NOT EXISTS account_procurement_cost_versions_active_key
    ON account_procurement_cost_versions(account_id) WHERE ended_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS account_procurement_cost_versions_request_key
    ON account_procurement_cost_versions(request_id) WHERE request_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS account_procurement_cost_versions_settlement_request_key
    ON account_procurement_cost_versions(settlement_request_id) WHERE settlement_request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS account_procurement_cost_versions_effective_idx
    ON account_procurement_cost_versions(account_id, effective_at, ended_at);
