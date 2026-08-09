-- Keep this migration metadata-only: usage_logs is partitioned in production,
-- so avoid a full-table backfill, constraint rebuild, or type rewrite.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS logical_request_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS attempt_id VARCHAR(160),
    ADD COLUMN IF NOT EXISTS usage_completeness VARCHAR(16) NOT NULL DEFAULT 'complete',
    ADD COLUMN IF NOT EXISTS reconciliation_required BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS unsafe_to_replay BOOLEAN NOT NULL DEFAULT FALSE;

-- Existing rows keep a nil logical ID; callers use request_id as their legacy
-- fallback. New rows are validated without scanning/relocking all partitions.
ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_usage_completeness_check
    CHECK (usage_completeness IN ('complete', 'partial', 'unknown')) NOT VALID;

CREATE INDEX IF NOT EXISTS idx_usage_logs_logical_request_id
    ON usage_logs (logical_request_id)
    WHERE logical_request_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_reconciliation_pending
    ON usage_logs (created_at, id)
    WHERE reconciliation_required = TRUE;
