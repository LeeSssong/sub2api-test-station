CREATE TABLE IF NOT EXISTS relay_ops.feishu_command_events (
    event_id VARCHAR(256) PRIMARY KEY,
    message_id VARCHAR(256) NOT NULL,
    chat_id VARCHAR(256) NOT NULL,
    sender_open_id VARCHAR(256) NOT NULL,
    command_text VARCHAR(128),
    action_kind VARCHAR(16),
    group_name VARCHAR(64),
    target_role VARCHAR(16),
    status VARCHAR(16) NOT NULL CHECK (status IN ('received', 'running', 'succeeded', 'no_op', 'partial', 'failed', 'rejected')),
    before_state JSONB,
    after_state JSONB,
    error_code VARCHAR(64),
    duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    lease_expires_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    reply_attempts INTEGER NOT NULL DEFAULT 0 CHECK (reply_attempts >= 0 AND reply_attempts <= 3),
    reply_delivered BOOLEAN NOT NULL DEFAULT FALSE,
    reply_message_id VARCHAR(256),
    reply_error_code VARCHAR(64),
    received_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (target_role IS NULL OR target_role IN ('primary', 'backup'))
);

CREATE INDEX IF NOT EXISTS idx_feishu_command_events_pending
    ON relay_ops.feishu_command_events (status, received_at);

CREATE INDEX IF NOT EXISTS idx_feishu_command_events_lease
    ON relay_ops.feishu_command_events (lease_expires_at)
    WHERE status = 'running';
