-- Preserve the provider-issued request identifier separately from the local
-- usage_logs.request_id idempotency/correlation key. Historical rows remain NULL.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_request_id VARCHAR(255);
