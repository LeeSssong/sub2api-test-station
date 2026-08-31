BEGIN;

DROP TABLE IF EXISTS relay_ops.agent_analyses;
DROP TABLE IF EXISTS relay_ops.notification_deliveries;
DROP TABLE IF EXISTS relay_ops.incidents;
DROP TABLE IF EXISTS relay_ops.notification_messages;
DROP TABLE IF EXISTS relay_ops.notification_decisions;
DROP TABLE IF EXISTS relay_ops.group_impact_signals;
DROP TABLE IF EXISTS relay_ops.operational_baselines;
DROP TABLE IF EXISTS relay_ops.native_ops_alert_events;
DROP TABLE IF EXISTS relay_ops.native_ops_alert_sync_state;

COMMIT;
