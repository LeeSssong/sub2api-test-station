-- OpenAI scheduler decision events are an independent, best-effort
-- observability stream. They intentionally do not alter usage_logs.
CREATE TABLE IF NOT EXISTS openai_scheduler_logs (
    id BIGSERIAL PRIMARY KEY,
    event_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    platform VARCHAR(32) NOT NULL DEFAULT 'openai',
    group_id BIGINT NULL,
    logical_request_id VARCHAR(160) NOT NULL,
    attempt_id VARCHAR(160) NULL,
    attempt_number INTEGER NOT NULL DEFAULT 0,
    event_name VARCHAR(96) NOT NULL,
    account_id BIGINT NULL,
    canonical_model VARCHAR(160) NULL,
    outcome VARCHAR(48) NULL,
    final_outcome VARCHAR(48) NULL,
    selection_layer VARCHAR(96) NULL,
    algorithm_version VARCHAR(64) NOT NULL DEFAULT 'openai-multi-window-quality-v1',
    decision JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_openai_scheduler_logs_event_at
    ON openai_scheduler_logs (event_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_openai_scheduler_logs_logical_request
    ON openai_scheduler_logs (logical_request_id, event_at ASC, id ASC);
CREATE INDEX IF NOT EXISTS idx_openai_scheduler_logs_account_event_at
    ON openai_scheduler_logs (account_id, event_at DESC) WHERE account_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_openai_scheduler_logs_group_event_at
    ON openai_scheduler_logs (group_id, event_at DESC) WHERE group_id IS NOT NULL;
