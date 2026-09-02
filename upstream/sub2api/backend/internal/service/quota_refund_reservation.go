package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/quota/accounting"
	"github.com/shopspring/decimal"
)

type RefundReservationInput struct {
	UserID                int64
	PaymentOrderID        int64
	RefundAmount          decimal.Decimal
	RequestedPaidQuotaUSD decimal.Decimal
	RefundCurrency        string
	RefundMethod          string
	ProviderInstanceID    string
	ProviderRequestKey    string
	IdempotencyKey        string
	Reason                string
	OperatorUserID        *int64
	ForceRefund           bool
}

type RefundReservationResult struct {
	AdjustmentID int64
	Allocations  []accounting.Allocation
	Idempotent   bool
}

type refundReservationAllocation struct {
	GrantID  int64  `json:"grant_id"`
	Bucket   string `json:"bucket"`
	QuotaUSD string `json:"quota_usd"`
}

// FinalizeRefundReservation applies a provider-confirmed refund in a second
// transaction. It consumes the reservation, increments refunded counters and
// projects the wallet; repeated calls with the same adjustment are idempotent.
func (s *QuotaAccountingService) FinalizeRefundReservation(ctx context.Context, adjustmentID int64, providerRefundID, tradeNo string, snapshot map[string]any) error {
	if s == nil || s.client == nil || adjustmentID <= 0 {
		return ErrQuotaAccountingUnavailable
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var userID, orderID int64
	var status, raw string
	var appliedPaid string
	err = scanQuotaOne(ctx, tx.Client(), `SELECT user_id,COALESCE(payment_order_id,0),status,reserved_allocations::text,applied_paid_quota_usd::text FROM user_quota_adjustments WHERE id=$1 FOR UPDATE`, []any{adjustmentID}, &userID, &orderID, &status, &raw, &appliedPaid)
	if err != nil {
		return err
	}
	if status == "completed" {
		return tx.Commit()
	}
	if status != "pending" && status != "unknown" && status != "reconciling" {
		return errors.New("refund reservation is not finalizable")
	}
	var allocs []refundReservationAllocation
	if err := json.Unmarshal([]byte(raw), &allocs); err != nil {
		return err
	}
	for _, a := range allocs {
		if a.Bucket != "paid" {
			continue
		}
		q, err := decimal.NewFromString(a.QuotaUSD)
		if err != nil {
			return err
		}
		if _, err := tx.Client().ExecContext(ctx, `UPDATE user_quota_grants SET reserved_paid_quota_usd=GREATEST(reserved_paid_quota_usd-$1,0),refunded_paid_quota_usd=refunded_paid_quota_usd+$1 WHERE id=$2 AND user_id=$3`, q.StringFixed(8), a.GrantID, userID); err != nil {
			return err
		}
	}
	var snap any = nil
	if snapshot != nil {
		snap = snapshot
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE user_quota_adjustments SET applied_allocations=reserved_allocations,applied_paid_quota_usd=requested_paid_quota_usd,provider_refund_id=NULLIF($1,''),refund_trade_no=NULLIF($2,''),provider_state='succeeded',provider_response_snapshot=$3,status='completed',adjusted_at=NOW(),updated_at=NOW() WHERE id=$4`, providerRefundID, tradeNo, snap, adjustmentID); err != nil {
		return err
	}
	if orderID > 0 {
		if _, err := tx.Client().ExecContext(ctx, `UPDATE payment_orders SET refunded_paid_quota_usd=refunded_paid_quota_usd+$1,quota_accounting_status='confirmed' WHERE id=$2`, appliedPaid, orderID); err != nil {
			return err
		}
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE user_wallets SET paid_quota_balance_usd=paid_quota_balance_usd-$1,version=version+1,updated_at=NOW() WHERE user_id=$2`, appliedPaid, userID); err != nil {
		return err
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE users SET balance=(SELECT paid_quota_balance_usd+gift_quota_balance_usd FROM user_wallets WHERE user_id=$1),updated_at=NOW() WHERE id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// ReleaseRefundReservation returns reserved paid quota after a provider
// operation is explicitly rejected or cancelled before completion.
func (s *QuotaAccountingService) ReleaseRefundReservation(ctx context.Context, adjustmentID int64, status string, note string) error {
	if s == nil || s.client == nil || adjustmentID <= 0 {
		return ErrQuotaAccountingUnavailable
	}
	if status == "" {
		status = "reconciling"
	}
	if status != "reconciling" && status != "rejected" && status != "unknown" {
		return errors.New("invalid reservation release status")
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var userID int64
	var raw, current string
	if err := scanQuotaOne(ctx, tx.Client(), `SELECT user_id,reserved_allocations::text,status FROM user_quota_adjustments WHERE id=$1 FOR UPDATE`, []any{adjustmentID}, &userID, &raw, &current); err != nil {
		return err
	}
	if current == "completed" || current == "rejected" {
		return tx.Commit()
	}
	var allocs []refundReservationAllocation
	if err := json.Unmarshal([]byte(raw), &allocs); err != nil {
		return err
	}
	for _, a := range allocs {
		if a.Bucket == "paid" {
			q, e := decimal.NewFromString(a.QuotaUSD)
			if e != nil {
				return e
			}
			if _, e = tx.Client().ExecContext(ctx, `UPDATE user_quota_grants SET reserved_paid_quota_usd=GREATEST(reserved_paid_quota_usd-$1,0) WHERE id=$2 AND user_id=$3`, q.StringFixed(8), a.GrantID, userID); e != nil {
				return e
			}
		}
	}
	_, err = tx.Client().ExecContext(ctx, `UPDATE user_quota_adjustments SET reserved_allocations='[]'::jsonb,status=$1,reconciliation_note=NULLIF($2,''),updated_at=NOW() WHERE id=$3`, status, note, adjustmentID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ReserveRefundPaidQuota creates the durable pending refund fact and reserves
// paid grant capacity atomically. It intentionally performs no provider call.
func (s *QuotaAccountingService) ReserveRefundPaidQuota(ctx context.Context, in RefundReservationInput) (RefundReservationResult, error) {
	if s == nil || s.client == nil {
		return RefundReservationResult{}, ErrQuotaAccountingUnavailable
	}
	if in.UserID <= 0 || in.PaymentOrderID <= 0 || in.RefundAmount.LessThanOrEqual(decimal.Zero) || in.RequestedPaidQuotaUSD.LessThanOrEqual(decimal.Zero) || strings.TrimSpace(in.RefundCurrency) == "" || strings.TrimSpace(in.RefundMethod) == "" || strings.TrimSpace(in.IdempotencyKey) == "" || strings.TrimSpace(in.Reason) == "" {
		return RefundReservationResult{}, errors.New("invalid refund reservation")
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return RefundReservationResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var existingID int64
	if err := scanQuotaOne(ctx, tx.Client(), `SELECT id FROM user_quota_adjustments WHERE user_id=$1 AND idempotency_key=$2`, []any{in.UserID, strings.TrimSpace(in.IdempotencyKey)}, &existingID); err == nil {
		if err := tx.Commit(); err != nil {
			return RefundReservationResult{}, err
		}
		return RefundReservationResult{AdjustmentID: existingID, Idempotent: true}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return RefundReservationResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, in.UserID); err != nil {
		return RefundReservationResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `SELECT id FROM payment_orders WHERE id=$1 AND user_id=$2 FOR UPDATE`, in.PaymentOrderID, in.UserID); err != nil {
		return RefundReservationResult{}, err
	}
	grants, err := loadRefundGrantRows(ctx, tx.Client(), in.UserID, in.PaymentOrderID)
	if err != nil {
		return RefundReservationResult{}, err
	}
	allocation, err := accounting.ReservePaidRefund(grants, in.RequestedPaidQuotaUSD)
	if err != nil {
		return RefundReservationResult{}, err
	}
	encoded := make([]refundReservationAllocation, 0, len(allocation.Allocations))
	for _, item := range allocation.Allocations {
		encoded = append(encoded, refundReservationAllocation{GrantID: item.GrantID, Bucket: item.Bucket, QuotaUSD: item.Quota.StringFixed(8)})
	}
	raw, err := json.Marshal(encoded)
	if err != nil {
		return RefundReservationResult{}, err
	}
	var adjustmentID int64
	err = scanQuotaOne(ctx, tx.Client(), `INSERT INTO user_quota_adjustments (user_id,adjustment_type,payment_order_id,reserved_allocations,applied_allocations,refund_amount,refund_currency,refund_method,refund_provider_instance_id,provider_request_key,provider_state,requested_paid_quota_usd,applied_paid_quota_usd,applied_gift_quota_usd,shortfall_paid_quota_usd,force_refund,actor_type,reason,status,idempotency_key,operator_user_id,adjusted_at) VALUES ($1,'refund_recovery',$2,$3,'[]'::jsonb,$4,$5,$6,$7,$8,'requested',$9,0,0,0,$10,'admin',$11,'pending',$12,$13,NOW()) RETURNING id`, []any{in.UserID, in.PaymentOrderID, raw, in.RefundAmount.StringFixed(8), strings.TrimSpace(in.RefundCurrency), strings.TrimSpace(in.RefundMethod), nullString(in.ProviderInstanceID), nullString(in.ProviderRequestKey), in.RequestedPaidQuotaUSD.StringFixed(8), in.ForceRefund, strings.TrimSpace(in.Reason), strings.TrimSpace(in.IdempotencyKey), in.OperatorUserID}, &adjustmentID)
	if err != nil {
		return RefundReservationResult{}, err
	}
	for _, item := range allocation.Allocations {
		if _, err := tx.Client().ExecContext(ctx, `UPDATE user_quota_grants SET reserved_paid_quota_usd=reserved_paid_quota_usd+$1 WHERE id=$2`, item.Quota.StringFixed(8), item.GrantID); err != nil {
			return RefundReservationResult{}, err
		}
	}
	payload, _ := json.Marshal(map[string]any{"adjustment_id": adjustmentID, "user_id": in.UserID, "payment_order_id": in.PaymentOrderID, "idempotency_key": strings.TrimSpace(in.IdempotencyKey)})
	if _, err := tx.Client().ExecContext(ctx, `INSERT INTO scheduler_outbox (event_type,payload) VALUES ($1,$2::jsonb)`, SchedulerOutboxEventQuotaRefundRequested, payload); err != nil {
		return RefundReservationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RefundReservationResult{}, err
	}
	return RefundReservationResult{AdjustmentID: adjustmentID, Allocations: allocation.Allocations}, nil
}

func loadRefundGrantRows(ctx context.Context, client *dbent.Client, userID, orderID int64) ([]accounting.Grant, error) {
	rows, err := client.QueryContext(ctx, `SELECT id,grant_type,paid_quota_usd::text,consumed_paid_quota_usd::text,refunded_paid_quota_usd::text,reserved_paid_quota_usd::text,legacy_debt_offset_paid_quota_usd::text FROM user_quota_grants WHERE user_id=$1 AND payment_order_id=$2 AND grant_type <> 'migration_opening' ORDER BY granted_at ASC,id ASC FOR UPDATE`, userID, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := make([]accounting.Grant, 0)
	for rows.Next() {
		var id int64
		var typ, paid, consumed, refunded, reserved, offset string
		if err := rows.Scan(&id, &typ, &paid, &consumed, &refunded, &reserved, &offset); err != nil {
			return nil, err
		}
		parsed := make([]decimal.Decimal, 5)
		for i, text := range []string{paid, consumed, refunded, reserved, offset} {
			parsed[i], err = decimal.NewFromString(text)
			if err != nil {
				return nil, err
			}
		}
		grants = append(grants, accounting.Grant{ID: id, Type: typ, Paid: parsed[0], ConsumedPaid: parsed[1], RefundedPaid: parsed[2], ReservedPaid: parsed[3], DebtOffsetPaid: parsed[4]})
	}
	return grants, rows.Err()
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}
