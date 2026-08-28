-- T84: explicit group-level gate for automatic active probes.
-- Account-level state remains backward-compatible in accounts.extra.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS active_probe_enabled BOOLEAN NOT NULL DEFAULT TRUE;
