SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS silenced_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS action_token_hash VARCHAR(64);

CREATE INDEX IF NOT EXISTS idx_ops_alert_events_balance_silence
    ON ops_alert_events (rule_id, scope_key, silenced_until)
    WHERE scope_type = 'base_url';
