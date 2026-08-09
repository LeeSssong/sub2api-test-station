ALTER TABLE usage_logs
    ALTER COLUMN request_id TYPE VARCHAR(160),
    ADD COLUMN IF NOT EXISTS logical_request_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS attempt_id VARCHAR(160),
    ADD COLUMN IF NOT EXISTS usage_completeness VARCHAR(16) NOT NULL DEFAULT 'complete',
    ADD COLUMN IF NOT EXISTS reconciliation_required BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS unsafe_to_replay BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE usage_logs
SET logical_request_id = request_id
WHERE logical_request_id IS NULL AND request_id IS NOT NULL;

ALTER TABLE usage_logs
    DROP CONSTRAINT IF EXISTS usage_logs_usage_completeness_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_usage_completeness_check
    CHECK (usage_completeness IN ('complete', 'partial', 'unknown'));

CREATE INDEX IF NOT EXISTS idx_usage_logs_logical_request_id
    ON usage_logs (logical_request_id)
    WHERE logical_request_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_logs_reconciliation_pending
    ON usage_logs (created_at, id)
    WHERE reconciliation_required = TRUE;
