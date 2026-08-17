-- Migration: 225_account_model_detection
-- Native Sub-owned scheduling facts for an external, execution-only detector.

CREATE TABLE IF NOT EXISTS account_model_detection_settings (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    connection_probe_model TEXT NOT NULL DEFAULT '',
    model_detection_model TEXT NOT NULL DEFAULT '',
    updated_by BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS account_model_detection_runs (
    id UUID PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    slot_key TEXT,
    trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('manual', 'scheduled')),
    model_id TEXT NOT NULL,
    claimed_model TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'normal', 'abnormal', 'insufficient', 'failed')),
    juice_status TEXT,
    juice_summary JSONB,
    fingerprint_candidate TEXT,
    fingerprint_similarity JSONB,
    detector_version TEXT,
    error_code TEXT,
    error_message TEXT,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT account_model_detection_runs_account_slot_key UNIQUE (account_id, slot_key)
);

CREATE INDEX IF NOT EXISTS account_model_detection_runs_account_created_idx
    ON account_model_detection_runs (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS account_model_detection_runs_status_idx
    ON account_model_detection_runs (status, queued_at);
CREATE UNIQUE INDEX IF NOT EXISTS account_model_detection_runs_active_account_idx
    ON account_model_detection_runs (account_id)
    WHERE status IN ('queued', 'running');
