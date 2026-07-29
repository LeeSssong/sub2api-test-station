ALTER TABLE relay_ops.notification_deliveries
    ADD COLUMN IF NOT EXISTS attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ;

ALTER TABLE relay_ops.incidents
    ADD COLUMN IF NOT EXISTS escalation_claim_token TEXT,
    ADD COLUMN IF NOT EXISTS escalation_claimed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS notification_delivery_retry_due_idx
    ON relay_ops.notification_deliveries (next_attempt_at, created_at, id)
    WHERE delivery_status IN ('failed', 'reserved');
