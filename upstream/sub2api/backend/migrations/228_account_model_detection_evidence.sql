-- Migration: 228_account_model_detection_evidence
-- Structured, bounded evidence metadata for the native detector history.

ALTER TABLE account_model_detection_runs
    ADD COLUMN IF NOT EXISTS profile TEXT,
    ADD COLUMN IF NOT EXISTS mode TEXT,
    ADD COLUMN IF NOT EXISTS trigger_reason TEXT,
    ADD COLUMN IF NOT EXISTS planned_requests INTEGER,
    ADD COLUMN IF NOT EXISTS valid_samples INTEGER,
    ADD COLUMN IF NOT EXISTS evidence_state TEXT,
    ADD COLUMN IF NOT EXISTS fingerprint_status TEXT;

CREATE INDEX IF NOT EXISTS account_model_detection_runs_account_created_id_idx
    ON account_model_detection_runs (account_id, created_at DESC, id DESC);
