ALTER TABLE relay_ops.upstream_cost_attempts
    ADD COLUMN IF NOT EXISTS site_standard_cost NUMERIC(20, 10) NOT NULL DEFAULT 0
        CHECK (site_standard_cost >= 0);

CREATE INDEX IF NOT EXISTS upstream_cost_attempts_cost_guard_idx
    ON relay_ops.upstream_cost_attempts (account_id, group_id, model, completed_at, id)
    WHERE group_id IS NOT NULL AND site_standard_cost > 0;
