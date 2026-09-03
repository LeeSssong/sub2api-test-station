-- T91-A quota accounting foundation. Expand-only; no facts are backfilled here.

CREATE TABLE IF NOT EXISTS user_quota_grants (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    grant_type VARCHAR(32) NOT NULL CHECK (grant_type IN ('payment_order','redeem_code','promo_bonus','admin_gift','affiliate_rebate','migration_opening')),
    payment_order_id BIGINT REFERENCES payment_orders(id) ON DELETE RESTRICT,
    redeem_code_id BIGINT REFERENCES redeem_codes(id) ON DELETE RESTRICT,
    promo_code_usage_id BIGINT,
    affiliate_ledger_id BIGINT,
    paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0,
    gift_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0,
    total_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0,
    consumed_paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (consumed_paid_quota_usd >= 0),
    consumed_gift_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (consumed_gift_quota_usd >= 0),
    refunded_paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (refunded_paid_quota_usd >= 0),
    deducted_gift_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (deducted_gift_quota_usd >= 0),
    reserved_paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (reserved_paid_quota_usd >= 0),
    legacy_debt_offset_paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (legacy_debt_offset_paid_quota_usd >= 0),
    operator_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    idempotency_key VARCHAR(128),
    rule_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    note TEXT NOT NULL DEFAULT '',
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_quota_grants_amounts_check CHECK (
        (grant_type = 'migration_opening' AND gift_quota_usd = 0 AND total_quota_usd = paid_quota_usd
         AND consumed_paid_quota_usd = 0 AND consumed_gift_quota_usd = 0 AND refunded_paid_quota_usd = 0
         AND deducted_gift_quota_usd = 0 AND reserved_paid_quota_usd = 0 AND legacy_debt_offset_paid_quota_usd = 0)
        OR
        (grant_type <> 'migration_opening' AND paid_quota_usd >= 0 AND gift_quota_usd >= 0
         AND total_quota_usd = paid_quota_usd + gift_quota_usd AND total_quota_usd > 0 AND idempotency_key IS NOT NULL)
    ),
    CONSTRAINT user_quota_grants_source_check CHECK (
      (grant_type = 'payment_order' AND payment_order_id IS NOT NULL AND redeem_code_id IS NULL AND promo_code_usage_id IS NULL AND affiliate_ledger_id IS NULL)
      OR (grant_type = 'redeem_code' AND redeem_code_id IS NOT NULL AND payment_order_id IS NULL AND promo_code_usage_id IS NULL AND affiliate_ledger_id IS NULL)
      OR (grant_type = 'promo_bonus' AND promo_code_usage_id IS NOT NULL AND payment_order_id IS NULL AND redeem_code_id IS NULL AND affiliate_ledger_id IS NULL)
      OR (grant_type = 'affiliate_rebate' AND affiliate_ledger_id IS NOT NULL AND payment_order_id IS NULL AND redeem_code_id IS NULL AND promo_code_usage_id IS NULL)
      OR (grant_type IN ('admin_gift','migration_opening') AND payment_order_id IS NULL AND redeem_code_id IS NULL AND promo_code_usage_id IS NULL AND affiliate_ledger_id IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS user_quota_grants_payment_order_unique ON user_quota_grants(payment_order_id) WHERE grant_type = 'payment_order';
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_grants_redeem_code_unique ON user_quota_grants(redeem_code_id) WHERE grant_type = 'redeem_code';
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_grants_promo_usage_unique ON user_quota_grants(promo_code_usage_id) WHERE grant_type = 'promo_bonus';
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_grants_affiliate_ledger_unique ON user_quota_grants(affiliate_ledger_id) WHERE grant_type = 'affiliate_rebate';
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_grants_user_idempotency_unique ON user_quota_grants(user_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_grants_opening_unique ON user_quota_grants(user_id) WHERE grant_type = 'migration_opening';
CREATE INDEX IF NOT EXISTS user_quota_grants_fifo ON user_quota_grants(user_id, granted_at, id);

CREATE TABLE IF NOT EXISTS user_quota_adjustments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    adjustment_type VARCHAR(32) NOT NULL CHECK (adjustment_type IN ('refund_recovery','admin_gift_deduction')),
    payment_order_id BIGINT REFERENCES payment_orders(id) ON DELETE RESTRICT,
    reserved_allocations JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(reserved_allocations) = 'array'),
    applied_allocations JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(applied_allocations) = 'array'),
    refund_amount NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (refund_amount >= 0),
    refund_currency VARCHAR(8), refund_method VARCHAR(32), refund_trade_no VARCHAR(128),
    refund_provider_instance_id VARCHAR(128), provider_refund_id VARCHAR(128), provider_request_key VARCHAR(128),
    provider_state VARCHAR(20) NOT NULL DEFAULT 'not_started' CHECK (provider_state IN ('not_started','requested','succeeded','failed','unknown','manual_verified')),
    provider_response_snapshot JSONB, provider_error_code VARCHAR(64), provider_error_message TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0), last_attempt_at TIMESTAMPTZ, next_retry_at TIMESTAMPTZ,
    reconciliation_note TEXT, reconciled_by BIGINT REFERENCES users(id) ON DELETE RESTRICT, reconciled_at TIMESTAMPTZ,
    requested_paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (requested_paid_quota_usd >= 0),
    applied_paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (applied_paid_quota_usd >= 0),
    applied_gift_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (applied_gift_quota_usd >= 0),
    shortfall_paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (shortfall_paid_quota_usd >= 0),
    force_refund BOOLEAN NOT NULL DEFAULT FALSE, approval_reason TEXT, approved_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    approved_at TIMESTAMPTZ, financial_exception_ref VARCHAR(128), operator_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    actor_type VARCHAR(16) NOT NULL CHECK (actor_type IN ('system','user','admin')),
    reason TEXT NOT NULL CHECK (reason <> ''),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','completed','failed','rejected','unknown','reconciling')),
    idempotency_key VARCHAR(128) NOT NULL,
    adjusted_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_quota_adjustments_type_check CHECK (
      (adjustment_type = 'refund_recovery' AND payment_order_id IS NOT NULL AND refund_amount > 0 AND refund_currency IS NOT NULL AND applied_gift_quota_usd = 0)
      OR (adjustment_type = 'admin_gift_deduction' AND payment_order_id IS NULL AND refund_amount = 0 AND applied_paid_quota_usd = 0 AND applied_gift_quota_usd > 0)
    ),
    CONSTRAINT user_quota_adjustments_refund_method_check CHECK (
      (adjustment_type = 'admin_gift_deduction' AND refund_method IS NULL AND refund_trade_no IS NULL)
      OR (adjustment_type = 'refund_recovery' AND refund_method IS NOT NULL AND refund_method IN ('original_channel','manual_transfer')
          AND (refund_method <> 'manual_transfer' OR refund_trade_no IS NOT NULL))
    ),
    CONSTRAINT user_quota_adjustments_actor_check CHECK (actor_type <> 'admin' OR operator_user_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS user_quota_adjustments_user_created ON user_quota_adjustments(user_id, created_at);
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_adjustments_user_idempotency_unique ON user_quota_adjustments(user_id, idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_adjustments_provider_request_unique ON user_quota_adjustments(refund_provider_instance_id, provider_request_key) WHERE provider_request_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_adjustments_provider_refund_unique ON user_quota_adjustments(refund_provider_instance_id, provider_refund_id) WHERE provider_refund_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_quota_adjustments_manual_trade_unique ON user_quota_adjustments(refund_method, refund_trade_no) WHERE refund_trade_no IS NOT NULL;

ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS gift_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS total_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS quota_rule_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS refunded_paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS quota_accounting_status VARCHAR(24) NOT NULL DEFAULT 'legacy_unknown';
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS operator_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS operator_note TEXT;
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS operator_recharged_at TIMESTAMPTZ;
CREATE UNIQUE INDEX IF NOT EXISTS payment_orders_provider_trade_unique
    ON payment_orders(provider_instance_id, payment_trade_no)
    WHERE provider_instance_id IS NOT NULL AND payment_trade_no <> '';
CREATE UNIQUE INDEX IF NOT EXISTS payment_orders_admin_recharge_trade_unique
    ON payment_orders(payment_type, payment_trade_no)
    WHERE payment_type = 'admin_recharge' AND payment_trade_no <> '';

ALTER TABLE payment_audit_logs ADD COLUMN IF NOT EXISTS operator_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT;
ALTER TABLE payment_audit_logs ADD COLUMN IF NOT EXISTS payment_order_id BIGINT REFERENCES payment_orders(id) ON DELETE RESTRICT;

ALTER TABLE billing_usage_entries ADD COLUMN IF NOT EXISTS paid_quota_delta_usd NUMERIC(20,8);
ALTER TABLE billing_usage_entries ADD COLUMN IF NOT EXISTS gift_quota_delta_usd NUMERIC(20,8);
ALTER TABLE billing_usage_entries ADD COLUMN IF NOT EXISTS attempted_quota_usd NUMERIC(20,8);
ALTER TABLE billing_usage_entries ADD COLUMN IF NOT EXISTS attribution_status VARCHAR(20) NOT NULL DEFAULT 'legacy_unknown';
ALTER TABLE billing_usage_entries ADD COLUMN IF NOT EXISTS paid_grant_allocations JSONB;
ALTER TABLE billing_usage_entries ADD COLUMN IF NOT EXISTS gift_grant_allocations JSONB;
CREATE UNIQUE INDEX IF NOT EXISTS billing_usage_entries_usage_log_id_unique ON billing_usage_entries(usage_log_id);

ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS code_kind VARCHAR(20) NOT NULL DEFAULT 'legacy_auto';
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS quota_accounting_status VARCHAR(24) NOT NULL DEFAULT 'legacy_unknown';
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS paid_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS gift_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS total_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS source_order_id BIGINT REFERENCES payment_orders(id) ON DELETE RESTRICT;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS voided_at TIMESTAMPTZ;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS voided_by BIGINT REFERENCES users(id) ON DELETE RESTRICT;

ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS benefit_mode VARCHAR(20) NOT NULL DEFAULT 'legacy_unknown';
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS gift_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;
ALTER TABLE promo_code_usages ADD COLUMN IF NOT EXISTS payment_order_id BIGINT REFERENCES payment_orders(id) ON DELETE RESTRICT;
ALTER TABLE promo_code_usages ADD COLUMN IF NOT EXISTS grant_id BIGINT;
ALTER TABLE promo_code_usages ADD COLUMN IF NOT EXISTS applied_gift_quota_usd NUMERIC(20,8) NOT NULL DEFAULT 0;

DO $$ BEGIN
  ALTER TABLE user_quota_grants ADD CONSTRAINT user_quota_grants_promo_usage_fk FOREIGN KEY (promo_code_usage_id) REFERENCES promo_code_usages(id) ON DELETE RESTRICT;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
  ALTER TABLE promo_code_usages ADD CONSTRAINT promo_code_usages_grant_fk FOREIGN KEY (grant_id) REFERENCES user_quota_grants(id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED;
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
