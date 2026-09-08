-- The user confirmed that legacy administrator balance recharge/deduction
-- history must not be retained. This removes only the old redeem-code rows;
-- quota accounting facts, wallets, payment orders, and concurrency history
-- remain untouched.
DELETE FROM redeem_codes
WHERE type = 'admin_balance';
