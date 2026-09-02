ALTER TABLE account_monitor_v4_snapshots
    DROP CONSTRAINT IF EXISTS account_monitor_v4_snapshots_window_check;

DELETE FROM account_monitor_v4_snapshots WHERE "window" = '30d';

ALTER TABLE account_monitor_v4_snapshots
    ADD CONSTRAINT account_monitor_v4_snapshots_window_check
    CHECK ("window" IN ('1h', '24h', '7d'));
