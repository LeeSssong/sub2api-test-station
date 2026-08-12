ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_actual_cost DECIMAL(20, 10),
    ADD COLUMN IF NOT EXISTS upstream_cost_status VARCHAR(16),
    ADD COLUMN IF NOT EXISTS upstream_cost_reason VARCHAR(64),
    ADD COLUMN IF NOT EXISTS profit DECIMAL(20, 10),
    ADD COLUMN IF NOT EXISTS upstream_cost_recorded_at TIMESTAMPTZ;
