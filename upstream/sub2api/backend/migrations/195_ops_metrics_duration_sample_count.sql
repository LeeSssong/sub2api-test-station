ALTER TABLE ops_metrics_hourly
    ADD COLUMN IF NOT EXISTS duration_sample_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE ops_metrics_daily
    ADD COLUMN IF NOT EXISTS duration_sample_count BIGINT NOT NULL DEFAULT 0;

-- Rows created before this column was introduced already contain duration
-- percentiles/averages but cannot recover the exact number of non-null raw
-- durations. Use the historical success count as a conservative compatibility
-- weight whenever a duration statistic exists; newly aggregated rows write the
-- exact COUNT(duration_ms) value above. This prevents old dashboard windows
-- from becoming blank immediately after the migration.
UPDATE ops_metrics_hourly
SET duration_sample_count = success_count
WHERE duration_sample_count = 0
  AND (duration_p50_ms IS NOT NULL
       OR duration_p90_ms IS NOT NULL
       OR duration_avg_ms IS NOT NULL
       OR duration_max_ms IS NOT NULL);

UPDATE ops_metrics_daily
SET duration_sample_count = success_count
WHERE duration_sample_count = 0
  AND (duration_p50_ms IS NOT NULL
       OR duration_p90_ms IS NOT NULL
       OR duration_avg_ms IS NOT NULL
       OR duration_max_ms IS NOT NULL);
