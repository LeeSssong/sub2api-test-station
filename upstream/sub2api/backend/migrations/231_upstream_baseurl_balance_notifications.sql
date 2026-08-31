SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS scope_type VARCHAR(32),
    ADD COLUMN IF NOT EXISTS scope_key TEXT,
    ADD COLUMN IF NOT EXISTS notification_state VARCHAR(16),
    ADD COLUMN IF NOT EXISTS last_observed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_delivered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_generation BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS delivery_attempt_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_lease_token VARCHAR(64),
    ADD COLUMN IF NOT EXISTS delivery_lease_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_delivery_error_code VARCHAR(64);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ops_alert_events_active_scope_unique
    ON ops_alert_events (rule_id, scope_type, scope_key)
    WHERE status = 'firing';

CREATE INDEX IF NOT EXISTS idx_ops_alert_events_balance_due
    ON ops_alert_events (rule_id, next_attempt_at, last_delivered_at)
    WHERE status = 'firing' AND scope_type = 'base_url';

INSERT INTO ops_alert_rules (
    name,
    description,
    enabled,
    severity,
    metric_type,
    operator,
    threshold,
    window_minutes,
    sustained_minutes,
    cooldown_minutes,
    notify_email,
    created_at,
    updated_at
) VALUES (
    'upstream_baseurl_balance_usd_v1',
    'Native BaseURL-scoped upstream balance notification ledger',
    true,
    'P2',
    'upstream_baseurl_balance_usd_v1',
    '<',
    5,
    1,
    1,
    30,
    false,
    NOW(),
    NOW()
)
ON CONFLICT (name) DO NOTHING;
