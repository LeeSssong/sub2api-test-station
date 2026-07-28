ALTER TABLE relay_ops.incidents
    ADD COLUMN IF NOT EXISTS family TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS policy_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS source_kind TEXT NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS recovery_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS material_hash TEXT,
    ADD COLUMN IF NOT EXISTS latest_payload JSONB;

CREATE TABLE IF NOT EXISTS relay_ops.group_impact_signals (
    id BIGSERIAL PRIMARY KEY,
    group_name TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    source_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    source_observed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_name, source_kind, source_key)
);

CREATE INDEX IF NOT EXISTS group_impact_signals_fresh_idx
    ON relay_ops.group_impact_signals (group_name, expires_at, source_kind, source_key);

CREATE TABLE IF NOT EXISTS relay_ops.notification_messages (
    id BIGSERIAL PRIMARY KEY,
    notification_key TEXT NOT NULL UNIQUE,
    family TEXT NOT NULL,
    policy_version INTEGER NOT NULL,
    source_kind TEXT NOT NULL,
    dedup_key TEXT NOT NULL UNIQUE,
    message_hash TEXT NOT NULL,
    message_payload JSONB NOT NULL,
    delivery_status TEXT NOT NULL,
    response_code INTEGER,
    delivered_at TIMESTAMPTZ,
    message_id TEXT,
    urgent_status TEXT,
    urgent_response_code INTEGER,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS notification_messages_retry_due_idx
    ON relay_ops.notification_messages (next_attempt_at, created_at, id)
    WHERE delivery_status IN ('failed', 'reserved');

CREATE TABLE IF NOT EXISTS relay_ops.notification_decisions (
    id BIGSERIAL PRIMARY KEY,
    decision_key TEXT NOT NULL UNIQUE,
    family TEXT NOT NULL,
    policy_version INTEGER NOT NULL,
    source_kind TEXT NOT NULL,
    decision TEXT NOT NULL,
    reason TEXT NOT NULL,
    details JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    observation_count BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS notification_decisions_family_seen_idx
    ON relay_ops.notification_decisions (family, last_seen_at DESC);

CREATE TABLE IF NOT EXISTS relay_ops.operational_baselines (
    baseline_key TEXT PRIMARY KEY,
    current_value TEXT NOT NULL,
    evidence_hash TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
