ALTER TABLE account_monitor_v4_snapshots
    ADD COLUMN IF NOT EXISTS ttft_p50_ms double precision,
    ADD COLUMN IF NOT EXISTS latency_p50_ms double precision;

-- Old operational flags did not enforce freshness/first-token thresholds.
UPDATE account_monitor_v4_snapshots SET current_operational = FALSE;
