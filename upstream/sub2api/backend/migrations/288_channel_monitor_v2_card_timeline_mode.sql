-- One saved display mode for passive monitoring cards and tool line details.
-- Candidate only: applying this migration is a separate release operation.
ALTER TABLE channel_monitor_v2_config
    ADD COLUMN IF NOT EXISTS card_timeline_mode TEXT NOT NULL DEFAULT 'samples'
    CHECK (card_timeline_mode IN ('windows', 'samples'));
