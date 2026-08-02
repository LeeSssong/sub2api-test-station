ALTER TABLE relay_ops.auth_sessions
    ADD COLUMN IF NOT EXISTS billing_account_id BIGINT;

CREATE INDEX IF NOT EXISTS auth_sessions_billing_account_idx
    ON relay_ops.auth_sessions (billing_account_id)
    WHERE billing_account_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS auth_sessions_active_billing_account_unique
    ON relay_ops.auth_sessions (billing_account_id)
    WHERE billing_account_id IS NOT NULL
      AND status = 'active'
      AND scope = 'billing_read';

CREATE INDEX IF NOT EXISTS upstream_cost_attempts_account_window_idx
    ON relay_ops.upstream_cost_attempts (account_id, completed_at, id);
