ALTER TABLE relay_ops.incidents
    ADD COLUMN IF NOT EXISTS occurrence_no BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS acknowledged_occurrence BIGINT,
    ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS acknowledged_by BIGINT,
    ADD COLUMN IF NOT EXISTS escalation_level INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_escalation_at TIMESTAMPTZ;

ALTER TABLE relay_ops.notification_deliveries
    ADD COLUMN IF NOT EXISTS occurrence_no BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS transition TEXT NOT NULL DEFAULT 'confirmed',
    ADD COLUMN IF NOT EXISTS message_payload JSONB,
    ADD COLUMN IF NOT EXISTS message_id TEXT,
    ADD COLUMN IF NOT EXISTS urgent_status TEXT,
    ADD COLUMN IF NOT EXISTS urgent_response_code INTEGER;

CREATE INDEX IF NOT EXISTS incidents_escalation_due_idx
    ON relay_ops.incidents (next_escalation_at, id)
    WHERE next_escalation_at IS NOT NULL;
