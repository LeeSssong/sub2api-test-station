DO $migration$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_attribute
               WHERE attrelid = 'channel_monitor_v2_config'::regclass
                 AND attname = 'exclude_sync_requests' AND NOT attisdropped) THEN
        RETURN;
    END IF;
-- Preserve request type in derived monitor facts so V2 can filter consistently.
-- -1 marks legacy mixed rows, not unknown raw requests (0). They remain visible
-- to unfiltered readers until the bounded worker replaces them from source logs.
ALTER TABLE channel_monitor_v2_config
    ADD COLUMN exclude_sync_requests BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE channel_monitor_v2_metrics_1m
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT -1,
    DROP CONSTRAINT channel_monitor_v2_metrics_1m_pkey,
    ADD PRIMARY KEY (bucket_start, platform, group_id, model, request_type);
ALTER TABLE channel_monitor_v2_metrics_1m ALTER COLUMN request_type SET DEFAULT 0;

ALTER TABLE channel_monitor_v2_user_metrics_1m
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT -1,
    DROP CONSTRAINT channel_monitor_v2_user_metrics_1m_pkey,
    ADD PRIMARY KEY (bucket_start, platform, group_id, model, user_id, request_type);
ALTER TABLE channel_monitor_v2_user_metrics_1m ALTER COLUMN request_type SET DEFAULT 0;

ALTER TABLE channel_monitor_v2_error_metrics_1m
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT -1,
    DROP CONSTRAINT channel_monitor_v2_error_metrics_1m_pkey,
    ADD PRIMARY KEY (bucket_start, platform, group_id, model, error_category, taxonomy_version, request_type);
ALTER TABLE channel_monitor_v2_error_metrics_1m ALTER COLUMN request_type SET DEFAULT 0;

ALTER TABLE channel_monitor_v2_latency_histograms_1m
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT -1,
    DROP CONSTRAINT channel_monitor_v2_latency_histograms_1m_pkey,
    ADD PRIMARY KEY (bucket_start, platform, group_id, model, user_id, metric, upper_bound_ms, request_type);
ALTER TABLE channel_monitor_v2_latency_histograms_1m ALTER COLUMN request_type SET DEFAULT 0;

ALTER TABLE channel_monitor_v2_metrics_rollup
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT -1,
    DROP CONSTRAINT channel_monitor_v2_metrics_rollup_pkey,
    ADD PRIMARY KEY (bucket_seconds, bucket_start, platform, group_id, model, request_type);
ALTER TABLE channel_monitor_v2_metrics_rollup ALTER COLUMN request_type SET DEFAULT 0;

ALTER TABLE channel_monitor_v2_user_metrics_rollup
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT -1,
    DROP CONSTRAINT channel_monitor_v2_user_metrics_rollup_pkey,
    ADD PRIMARY KEY (bucket_seconds, bucket_start, platform, group_id, model, user_id, request_type);
ALTER TABLE channel_monitor_v2_user_metrics_rollup ALTER COLUMN request_type SET DEFAULT 0;

ALTER TABLE channel_monitor_v2_error_metrics_rollup
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT -1,
    DROP CONSTRAINT channel_monitor_v2_error_metrics_rollup_pkey,
    ADD PRIMARY KEY (bucket_seconds, bucket_start, platform, group_id, model, error_category, taxonomy_version, request_type);
ALTER TABLE channel_monitor_v2_error_metrics_rollup ALTER COLUMN request_type SET DEFAULT 0;

ALTER TABLE channel_monitor_v2_latency_histograms_rollup
    ADD COLUMN request_type SMALLINT NOT NULL DEFAULT -1,
    DROP CONSTRAINT channel_monitor_v2_latency_histograms_rollup_pkey,
    ADD PRIMARY KEY (bucket_seconds, bucket_start, platform, group_id, model, user_id, metric, upper_bound_ms, request_type);
ALTER TABLE channel_monitor_v2_latency_histograms_rollup ALTER COLUMN request_type SET DEFAULT 0;

-- Logs can have shorter retention than rollups. Never erase old mixed facts
-- whose source rows may already have been cleaned. Round up to the first whole
-- minute for which both raw sources can be scanned. An empty source starts
-- classification at deployment time, without blocking future fresh traffic.
ALTER TABLE channel_monitor_v2_watermarks
    ADD COLUMN request_type_rebuild_from TIMESTAMPTZ;
UPDATE channel_monitor_v2_watermarks
SET request_type_rebuild_from = GREATEST(
    date_trunc('minute', NOW()) - INTERVAL '90 days',
    date_trunc('minute', GREATEST(
        COALESCE((SELECT MIN(created_at) FROM usage_logs), NOW()),
        COALESCE((SELECT MIN(created_at) FROM ops_error_logs), NOW())
    )) + INTERVAL '1 minute'
) WHERE id = 1;

-- Restart classification backfill after the worker is stopped for migration.
-- Keep the existing unfiltered coverage and facts for V3 and the default scope.
UPDATE channel_monitor_v2_watermarks
SET backfill_cursor = GREATEST(date_trunc('minute', NOW()), request_type_rebuild_from), updated_at = NOW()
WHERE id = 1;
END;
$migration$;
