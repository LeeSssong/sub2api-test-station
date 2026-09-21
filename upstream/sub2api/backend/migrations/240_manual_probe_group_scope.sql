-- A user initiated probe is evidence for its requested line, not every group
-- sharing the selected upstream account. Scheduled account probes remain NULL.
ALTER TABLE account_monitor_results ADD COLUMN IF NOT EXISTS group_id bigint;
CREATE INDEX IF NOT EXISTS idx_account_monitor_results_group_checked
 ON account_monitor_results(group_id, checked_at DESC) WHERE group_id IS NOT NULL;
