ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS estimated_usable_quota_usd NUMERIC(14,2);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'accounts_estimated_usable_quota_usd_positive'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_estimated_usable_quota_usd_positive
            CHECK (
                estimated_usable_quota_usd > 0
                AND estimated_usable_quota_usd <> 'NaN'::numeric
                AND estimated_usable_quota_usd <> 'Infinity'::numeric
            );
    END IF;
END $$;
