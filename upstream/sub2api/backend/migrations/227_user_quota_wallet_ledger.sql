-- T55 native quota wallet, auditable ledger, and quota mutation idempotency.
-- Expand-only: preserve users.balance and do not backfill historical ledger rows.

CREATE TABLE IF NOT EXISTS user_wallets (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    cash_balance_cny DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (cash_balance_cny >= 0),
    paid_quota_balance_usd DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (paid_quota_balance_usd >= 0),
    gift_quota_balance_usd DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (gift_quota_balance_usd >= 0),
    version                BIGINT NOT NULL DEFAULT 1,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_quota_ledger_entries (
    id                    BIGSERIAL PRIMARY KEY,
    user_id               BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    record_type           VARCHAR(40) NOT NULL CHECK (record_type IN ('recharge', 'refund', 'usage_consumption', 'legacy_balance_adjustment', 'payment_fulfillment', 'redeem_credit', 'affiliate_credit', 'migration_projection')),
    cash_delta_cny        DECIMAL(20,8) NOT NULL DEFAULT 0,
    paid_quota_delta_usd  DECIMAL(20,8) NOT NULL DEFAULT 0,
    gift_quota_delta_usd  DECIMAL(20,8) NOT NULL DEFAULT 0,
    cash_before_cny       DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (cash_before_cny >= 0),
    cash_after_cny        DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (cash_after_cny >= 0),
    paid_before_usd       DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (paid_before_usd >= 0),
    paid_after_usd        DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (paid_after_usd >= 0),
    gift_before_usd       DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (gift_before_usd >= 0),
    gift_after_usd        DECIMAL(20,8) NOT NULL DEFAULT 0 CHECK (gift_after_usd >= 0),
    reference_type        VARCHAR(64),
    reference_id          VARCHAR(255),
    idempotency_key       VARCHAR(255),
    note                  TEXT NOT NULL DEFAULT '',
    operator_id           BIGINT REFERENCES users(id) ON DELETE SET NULL,
    status                VARCHAR(24) NOT NULL DEFAULT 'confirmed' CHECK (status = 'confirmed'),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_quota_ledger_entries_user_id_idempotency_key_key UNIQUE (user_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_user_quota_ledger_entries_user_created_at
    ON user_quota_ledger_entries (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_quota_ledger_entries_reference
    ON user_quota_ledger_entries (reference_type, reference_id);

CREATE TABLE IF NOT EXISTS quota_idempotency_records (
    id                 BIGSERIAL PRIMARY KEY,
    user_id            BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key    VARCHAR(255) NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    status             VARCHAR(24) NOT NULL DEFAULT 'processing',
    ledger_entry_id    BIGINT REFERENCES user_quota_ledger_entries(id) ON DELETE SET NULL,
    response_status    INTEGER,
    response_body      TEXT,
    expires_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT quota_idempotency_records_user_id_key_key UNIQUE (user_id, idempotency_key),
    CONSTRAINT quota_idempotency_records_ledger_entry_id_key UNIQUE (ledger_entry_id)
);

CREATE INDEX IF NOT EXISTS idx_quota_idempotency_records_expires_at
    ON quota_idempotency_records (expires_at);

-- Initialize only active users. Re-running this statement never overwrites a wallet
-- created by a concurrent request and never creates historical ledger entries.
INSERT INTO user_wallets (
    user_id,
    cash_balance_cny,
    paid_quota_balance_usd,
    gift_quota_balance_usd,
    version,
    created_at,
    updated_at
)
SELECT users.id, 0, users.balance, 0, 1, NOW(), NOW()
FROM users
WHERE users.deleted_at IS NULL
ON CONFLICT (user_id) DO NOTHING;
