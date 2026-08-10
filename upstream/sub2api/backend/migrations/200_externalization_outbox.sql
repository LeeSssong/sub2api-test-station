-- Externalization integration outbox.
-- Expand-only: this migration creates new tables and indexes only. Core
-- business tables remain the sole source of truth for business writes.
CREATE TABLE IF NOT EXISTS externalization_outbox (
    event_id         UUID PRIMARY KEY,
    event_type       TEXT NOT NULL,
    occurred_at      TIMESTAMPTZ NOT NULL,
    source_version   TEXT NOT NULL,
    contract_version INTEGER NOT NULL CHECK (contract_version > 0),
    payload          JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    status           TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending', 'processing', 'retry', 'published', 'dead')),
    available_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempts         INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    claimed_at       TIMESTAMPTZ,
    claimed_by       TEXT,
    last_error_class TEXT,
    last_error       TEXT,
    published_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_externalization_outbox_claimable
    ON externalization_outbox (available_at, occurred_at, event_id)
    WHERE status IN ('pending', 'retry');

CREATE INDEX IF NOT EXISTS idx_externalization_outbox_lease
    ON externalization_outbox (claimed_at)
    WHERE status = 'processing';

CREATE INDEX IF NOT EXISTS idx_externalization_outbox_type_occurred
    ON externalization_outbox (event_type, occurred_at, event_id);

COMMENT ON TABLE externalization_outbox IS
    'Credential-free integration events emitted by the official core for relay-ops projections';
