ALTER TABLE relay_ops.upstream_cost_attempts
    ADD COLUMN IF NOT EXISTS group_id BIGINT NULL;

CREATE INDEX IF NOT EXISTS upstream_cost_attempts_group_window_idx
    ON relay_ops.upstream_cost_attempts (group_id, completed_at, id)
    WHERE group_id IS NOT NULL;
