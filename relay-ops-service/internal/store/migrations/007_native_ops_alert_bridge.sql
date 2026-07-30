CREATE TABLE IF NOT EXISTS relay_ops.native_ops_alert_sync_state (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    before_fired_at TIMESTAMPTZ,
    before_id BIGINT,
    initialized_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK ((before_fired_at IS NULL) = (before_id IS NULL)),
    CHECK (before_id IS NULL OR before_id > 0)
);

CREATE TABLE IF NOT EXISTS relay_ops.native_ops_alert_events (
    source_event_id BIGINT PRIMARY KEY CHECK (source_event_id > 0),
    rule_id BIGINT NOT NULL CHECK (rule_id > 0),
    incident_key TEXT NOT NULL CHECK (btrim(incident_key) <> ''),
    severity TEXT NOT NULL CHECK (severity IN ('P0', 'P1')),
    source_status TEXT NOT NULL CHECK (source_status IN ('firing', 'resolved', 'manual_resolved')),
    fired_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    silenced BOOLEAN NOT NULL DEFAULT FALSE,
    dimensions_hash TEXT NOT NULL CHECK (dimensions_hash ~ '^[0-9a-f]{64}$'),
    last_seen_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (
        (source_status = 'firing' AND resolved_at IS NULL)
        OR
        (source_status IN ('resolved', 'manual_resolved') AND resolved_at IS NOT NULL)
    ),
    CHECK (last_seen_at >= fired_at),
    CHECK (resolved_at IS NULL OR resolved_at >= fired_at)
);

CREATE INDEX IF NOT EXISTS native_ops_alert_events_firing_idx
ON relay_ops.native_ops_alert_events (last_seen_at, source_event_id)
WHERE source_status='firing';
