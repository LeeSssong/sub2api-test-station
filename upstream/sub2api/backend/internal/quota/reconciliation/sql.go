package reconciliation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// LoadSnapshot reads only the tables required by the reconciliation contract.
// The caller should pass a read-only transaction when connecting to production.
func LoadSnapshot(ctx context.Context, db Queryer) (Snapshot, error) {
	var s Snapshot
	if err := loadWallets(ctx, db, &s); err != nil {
		return s, err
	}
	if err := loadGrants(ctx, db, &s); err != nil {
		return s, err
	}
	if err := loadUsage(ctx, db, &s); err != nil {
		return s, err
	}
	if err := loadRefundsAndAnomalies(ctx, db, &s); err != nil {
		return s, err
	}
	return s, nil
}

func scanDecimal(value any) (decimal.Decimal, error) {
	switch v := value.(type) {
	case nil:
		return decimal.Zero, nil
	case string:
		return decimal.NewFromString(v)
	case []byte:
		return decimal.NewFromString(string(v))
	default:
		return decimal.Zero, fmt.Errorf("unsupported numeric scan type %T", value)
	}
}

func loadWallets(ctx context.Context, db Queryer, s *Snapshot) error {
	rows, err := db.QueryContext(ctx, `SELECT user_id, paid_quota_balance_usd, gift_quota_balance_usd FROM user_wallets ORDER BY user_id`)
	if err != nil {
		return fmt.Errorf("load wallets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var paidRaw, giftRaw any
		if err := rows.Scan(&id, &paidRaw, &giftRaw); err != nil {
			return fmt.Errorf("scan wallet: %w", err)
		}
		paid, err := scanDecimal(paidRaw)
		if err != nil {
			return err
		}
		gift, err := scanDecimal(giftRaw)
		if err != nil {
			return err
		}
		s.Wallets = append(s.Wallets, WalletSnapshot{UserID: id, PaidBalance: paid, GiftBalance: gift})
	}
	return rows.Err()
}

func loadGrants(ctx context.Context, db Queryer, s *Snapshot) error {
	rows, err := db.QueryContext(ctx, `SELECT id, user_id, paid_quota_usd, gift_quota_usd, consumed_paid_quota_usd, consumed_gift_quota_usd, refunded_paid_quota_usd, deducted_gift_quota_usd, reserved_paid_quota_usd, legacy_debt_offset_paid_quota_usd FROM user_quota_grants ORDER BY id`)
	if err != nil {
		return fmt.Errorf("load grants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var g GrantSnapshot
		var values [8]any
		if err := rows.Scan(&g.ID, &g.UserID, &values[0], &values[1], &values[2], &values[3], &values[4], &values[5], &values[6], &values[7]); err != nil {
			return fmt.Errorf("scan grant: %w", err)
		}
		parsed := make([]decimal.Decimal, 8)
		for i := range values {
			parsed[i], err = scanDecimal(values[i])
			if err != nil {
				return err
			}
		}
		g.PaidGranted, g.GiftGranted, g.PaidConsumed, g.GiftConsumed, g.PaidRefunded, g.GiftDeducted, g.PaidReserved, g.LegacyDebtOffset = parsed[0], parsed[1], parsed[2], parsed[3], parsed[4], parsed[5], parsed[6], parsed[7]
		s.Grants = append(s.Grants, g)
	}
	return rows.Err()
}

func loadUsage(ctx context.Context, db Queryer, s *Snapshot) error {
	rows, err := db.QueryContext(ctx, `SELECT id::text, user_id, delta_usd, paid_quota_delta_usd, gift_quota_delta_usd, attribution_status, paid_grant_allocations, gift_grant_allocations FROM billing_usage_entries ORDER BY id`)
	if err != nil {
		return fmt.Errorf("load usage: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var u UsageSnapshot
		var deltaRaw, paidRaw, giftRaw any
		var attributionStatus string
		var paidAlloc, giftAlloc []byte
		if err := rows.Scan(&u.ID, &u.UserID, &deltaRaw, &paidRaw, &giftRaw, &attributionStatus, &paidAlloc, &giftAlloc); err != nil {
			return fmt.Errorf("scan usage: %w", err)
		}
		u.Delta, err = scanDecimal(deltaRaw)
		if err != nil {
			return err
		}
		u.PaidDelta, err = scanDecimal(paidRaw)
		if err != nil {
			return err
		}
		u.GiftDelta, err = scanDecimal(giftRaw)
		if err != nil {
			return err
		}
		u.AttributionStatus = attributionStatus
		u.AllocationValid = validAllocationJSON(paidAlloc) && validAllocationJSON(giftAlloc)
		u.Allocations = append(u.Allocations, parseAllocations(paidAlloc, "paid")...)
		u.Allocations = append(u.Allocations, parseAllocations(giftAlloc, "gift")...)
		s.Usage = append(s.Usage, u)
	}
	return rows.Err()
}

func parseAllocations(raw []byte, bucket string) []Allocation {
	var values []struct {
		GrantID int64  `json:"grant_id"`
		Bucket  string `json:"bucket"`
		Quota   string `json:"quota_usd"`
	}
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	out := make([]Allocation, 0, len(values))
	for _, value := range values {
		if value.Bucket != bucket {
			continue
		}
		quota, err := decimal.NewFromString(value.Quota)
		if err == nil {
			out = append(out, Allocation{GrantID: value.GrantID, Bucket: bucket, Quota: quota})
		}
	}
	return out
}

func validAllocationJSON(raw []byte) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var values []map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return false
	}
	for _, item := range values {
		if len(item) != 3 || item["grant_id"] == nil || item["bucket"] == nil || item["quota_usd"] == nil {
			return false
		}
		bucket, ok := item["bucket"].(string)
		if !ok || (bucket != "paid" && bucket != "gift") {
			return false
		}
		quota, ok := item["quota_usd"].(string)
		if !ok {
			return false
		}
		if _, err := decimal.NewFromString(quota); err != nil {
			return false
		}
	}
	return true
}

func loadRefundsAndAnomalies(ctx context.Context, db Queryer, s *Snapshot) error {
	rows, err := db.QueryContext(ctx, `SELECT po.id::text, po.refunded_paid_quota_usd, COALESCE(SUM(CASE WHEN a.status = 'completed' THEN a.applied_paid_quota_usd ELSE 0 END), 0) FROM payment_orders po LEFT JOIN user_quota_adjustments a ON a.payment_order_id = po.id GROUP BY po.id, po.refunded_paid_quota_usd ORDER BY po.id`)
	if err != nil {
		return fmt.Errorf("load refunds: %w", err)
	}
	for rows.Next() {
		var f RefundSnapshot
		var refunded, adjusted any
		if err := rows.Scan(&f.OrderID, &refunded, &adjusted); err != nil {
			rows.Close()
			return fmt.Errorf("scan refund: %w", err)
		}
		f.Refunded, err = scanDecimal(refunded)
		if err != nil {
			rows.Close()
			return err
		}
		f.Adjusted, err = scanDecimal(adjusted)
		if err != nil {
			rows.Close()
			return err
		}
		s.Refunds = append(s.Refunds, f)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT user_id::text || ':' || idempotency_key FROM (SELECT user_id, idempotency_key FROM user_quota_grants WHERE idempotency_key IS NOT NULL UNION ALL SELECT user_id, idempotency_key FROM user_quota_adjustments) keys GROUP BY user_id, idempotency_key HAVING COUNT(*) > 1 ORDER BY 1`)
	if err != nil {
		return fmt.Errorf("load duplicate idempotency keys: %w", err)
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return err
		}
		s.DuplicateIdempotencyKeys = append(s.DuplicateIdempotencyKeys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM billing_usage_entries WHERE attribution_status = 'exact' AND (paid_grant_allocations IS NULL OR gift_grant_allocations IS NULL OR jsonb_typeof(paid_grant_allocations) <> 'array' OR jsonb_typeof(gift_grant_allocations) <> 'array')`).Scan(&count); err != nil {
		return fmt.Errorf("load invalid allocations: %w", err)
	}
	s.InvalidAllocationRows = count
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_quota_adjustments WHERE status IN ('pending','unknown','reconciling') AND jsonb_array_length(reserved_allocations) > 0`).Scan(&s.UnknownReservations); err != nil {
		return fmt.Errorf("load unknown reservations: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_quota_adjustments a CROSS JOIN LATERAL jsonb_array_elements(COALESCE(a.applied_allocations, '[]'::jsonb)) item JOIN user_quota_grants g ON g.id = (item->>'grant_id')::bigint WHERE a.adjustment_type = 'refund_recovery' AND g.payment_order_id IS DISTINCT FROM a.payment_order_id`).Scan(&s.CrossOrderGrantRows); err != nil {
		return fmt.Errorf("load cross-order allocations: %w", err)
	}
	var residualRaw any
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN attribution_status = 'legacy_unknown' THEN delta_usd - COALESCE(paid_quota_delta_usd, 0) - COALESCE(gift_quota_delta_usd, 0) ELSE 0 END), 0) FROM billing_usage_entries`).Scan(&residualRaw); err != nil {
		return fmt.Errorf("load legacy unknown residual: %w", err)
	}
	s.LegacyUnknownResidual, err = scanDecimal(residualRaw)
	if err != nil {
		return err
	}
	return nil
}
