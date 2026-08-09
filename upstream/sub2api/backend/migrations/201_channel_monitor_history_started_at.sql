ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS history_started_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Every existing monitor starts a new statistics epoch when this repair is deployed.
UPDATE channel_monitors
SET history_started_at = NOW();
