CREATE TABLE IF NOT EXISTS relay_ops.balance_snapshots (
 account_id BIGINT NOT NULL, observed_at TIMESTAMPTZ NOT NULL, amount NUMERIC NOT NULL,
 currency TEXT NOT NULL, fresh_until TIMESTAMPTZ NOT NULL, source TEXT NOT NULL,
 PRIMARY KEY (account_id, observed_at)
);
CREATE TABLE IF NOT EXISTS relay_ops.externalization_commands (
 command_id TEXT PRIMARY KEY, actor_id BIGINT NOT NULL, account_id BIGINT NOT NULL,
 idempotency_key TEXT NOT NULL UNIQUE, payload JSONB NOT NULL, status TEXT NOT NULL DEFAULT 'accepted',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), completed_at TIMESTAMPTZ
);
ALTER TABLE relay_ops.externalization_commands ADD COLUMN IF NOT EXISTS result TEXT NOT NULL DEFAULT 'accepted';
ALTER TABLE relay_ops.externalization_commands ADD COLUMN IF NOT EXISTS contract_version INTEGER NOT NULL DEFAULT 1;
