ALTER TABLE account_monitor_group_score_weights
    ADD COLUMN IF NOT EXISTS ttft_target_ms INTEGER NOT NULL DEFAULT 1000,
    ADD COLUMN IF NOT EXISTS ttft_limit_ms INTEGER NOT NULL DEFAULT 5000,
    ADD COLUMN IF NOT EXISTS latency_target_ms INTEGER NOT NULL DEFAULT 10000,
    ADD COLUMN IF NOT EXISTS latency_limit_ms INTEGER NOT NULL DEFAULT 60000;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'account_monitor_score_ttft_range_check'
    ) THEN
        ALTER TABLE account_monitor_group_score_weights
            ADD CONSTRAINT account_monitor_score_ttft_range_check
            CHECK (ttft_target_ms >= 0 AND ttft_limit_ms > ttft_target_ms);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'account_monitor_score_latency_range_check'
    ) THEN
        ALTER TABLE account_monitor_group_score_weights
            ADD CONSTRAINT account_monitor_score_latency_range_check
            CHECK (latency_target_ms >= 0 AND latency_limit_ms > latency_target_ms);
    END IF;
END $$;
