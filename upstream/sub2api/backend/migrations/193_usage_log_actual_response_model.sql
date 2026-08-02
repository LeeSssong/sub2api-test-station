ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS actual_response_model VARCHAR(100);
