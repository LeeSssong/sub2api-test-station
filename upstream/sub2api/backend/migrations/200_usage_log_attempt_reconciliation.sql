-- Keep this migration metadata-only: usage_logs is partitioned in production,
-- so avoid a full-table backfill or constraint validation scan. Widening the
-- request ID is metadata-only and is required for durable attempt identities.
ALTER TABLE usage_logs
    ALTER COLUMN request_id TYPE VARCHAR(160),
    ADD COLUMN IF NOT EXISTS logical_request_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS attempt_id VARCHAR(160),
    ADD COLUMN IF NOT EXISTS usage_completeness VARCHAR(16) NOT NULL DEFAULT 'complete',
    ADD COLUMN IF NOT EXISTS reconciliation_required BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS unsafe_to_replay BOOLEAN NOT NULL DEFAULT FALSE;

-- Existing rows keep a nil logical ID; callers use request_id as their legacy
-- fallback. New rows are validated without scanning/relocking all partitions.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'usage_logs'::regclass
          AND conname = 'usage_logs_usage_completeness_check'
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_usage_completeness_check
            CHECK (usage_completeness IN ('complete', 'partial', 'unknown')) NOT VALID;
    END IF;
END $$;

-- Optional reporting indexes are intentionally deferred. Creating an index on
-- a partitioned parent recursively builds every child while this migration is
-- transactional, blocking production writes. They must be provisioned by a
-- separate, reviewed online-index operation.
