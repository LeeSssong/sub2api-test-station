ALTER TABLE account_model_detection_runs
    ADD COLUMN IF NOT EXISTS trigger_evidence JSONB;
