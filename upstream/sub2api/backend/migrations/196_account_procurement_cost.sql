ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS procurement_cost_cny NUMERIC(14,2),
    ADD COLUMN IF NOT EXISTS procurement_cost_effective_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'accounts_procurement_cost_cny_nonnegative'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_procurement_cost_cny_nonnegative
            CHECK (procurement_cost_cny >= 0);
    END IF;
END $$;
