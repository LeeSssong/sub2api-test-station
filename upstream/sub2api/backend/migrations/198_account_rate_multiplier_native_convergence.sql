UPDATE accounts
SET extra = (
    CASE
        WHEN extra ->> 'upstream_billing_rate_multiplier_policy' IN ('manual_override', 'upstream_managed') THEN
            jsonb_set(
                extra,
                '{upstream_billing_rate_sync_enabled}',
                CASE
                    WHEN extra ->> 'upstream_billing_rate_multiplier_policy' = 'manual_override' THEN 'false'::jsonb
                    WHEN extra ->> 'upstream_billing_rate_multiplier_policy' = 'upstream_managed' THEN 'true'::jsonb
                END,
                true
            )
        ELSE extra
    END
) - 'upstream_billing_rate_multiplier_policy' - 'account_monitor_multiplier_measurement',
updated_at = GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')
WHERE extra ? 'upstream_billing_rate_multiplier_policy'
   OR extra ? 'account_monitor_multiplier_measurement';
