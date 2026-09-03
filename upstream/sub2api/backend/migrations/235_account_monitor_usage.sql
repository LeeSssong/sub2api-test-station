-- T128: retain provider usage observed by active account probes so group
-- monitor projections can use the same cache-rate denominator as native Sub.
ALTER TABLE account_monitor_results
    ADD COLUMN IF NOT EXISTS input_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS usage_completeness TEXT NOT NULL DEFAULT 'unknown';

ALTER TABLE account_monitor_bucket_terminals
    ADD COLUMN IF NOT EXISTS input_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS usage_completeness TEXT NOT NULL DEFAULT 'unknown';
