package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/userquotagrant"
	"github.com/Wei-Shaw/sub2api/internal/quota/accounting"
	"github.com/shopspring/decimal"
)

var (
	ErrQuotaGrantIdempotencyConflict = errors.New("quota grant idempotency conflict")
	ErrQuotaAccountingUnavailable    = errors.New("quota accounting service unavailable")
)

type QuotaGrantInput struct {
	UserID            int64
	GrantType         string
	PaymentOrderID    *int64
	RedeemCodeID      *int64
	PromoCodeUsageID  *int64
	AffiliateLedgerID *int64
	PaidQuotaUSD      decimal.Decimal
	GiftQuotaUSD      decimal.Decimal
	IdempotencyKey    string
	RuleSnapshot      map[string]any
	Note              string
	OperatorUserID    *int64
}

type QuotaGrantResult struct {
	GrantID                 int64
	PaidQuotaUSD            decimal.Decimal
	GiftQuotaUSD            decimal.Decimal
	LegacyDebtOffsetPaidUSD decimal.Decimal
	Idempotent              bool
}

type GiftDeductionInput struct {
	UserID         int64
	AmountUSD      decimal.Decimal
	IdempotencyKey string
	Reason         string
	OperatorUserID int64
}

type GiftDeductionResult struct {
	AdjustmentID int64
	AppliedUSD   decimal.Decimal
	Idempotent   bool
}

// ApplyRefundAdjustment records a completed paid-quota recovery after the
// external provider has confirmed the refund. It is idempotent and updates
// grant counters, wallet projection, and the payment-order quota summary in
// one transaction.
func (s *QuotaAccountingService) ApplyRefundAdjustment(ctx context.Context, userID, orderID int64, refundAmount, paidQuota decimal.Decimal, idempotencyKey, reason string) error {
	if s == nil || s.client == nil {
		return ErrQuotaAccountingUnavailable
	}
	if userID <= 0 || orderID <= 0 || paidQuota.LessThanOrEqual(decimal.Zero) || strings.TrimSpace(idempotencyKey) == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("invalid refund adjustment")
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var existingID int64
	err = scanQuotaOne(ctx, tx.Client(), `SELECT id FROM user_quota_adjustments WHERE user_id=$1 AND idempotency_key=$2`, []any{userID, strings.TrimSpace(idempotencyKey)}, &existingID)
	if err == nil {
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.Client().ExecContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, userID); err != nil {
		return err
	}
	if _, err := tx.Client().ExecContext(ctx, `SELECT id FROM payment_orders WHERE id=$1 AND user_id=$2 FOR UPDATE`, orderID, userID); err != nil {
		return err
	}
	rows, err := tx.Client().QueryContext(ctx, `SELECT id,grant_type,paid_quota_usd::text,consumed_paid_quota_usd::text,refunded_paid_quota_usd::text,reserved_paid_quota_usd::text,legacy_debt_offset_paid_quota_usd::text FROM user_quota_grants WHERE user_id=$1 AND payment_order_id=$2 ORDER BY granted_at ASC,id ASC FOR UPDATE`, userID, orderID)
	if err != nil {
		return err
	}
	grants := make([]accounting.Grant, 0)
	for rows.Next() {
		var id int64
		var typ, paid, consumed, refunded, reserved, offset string
		if err := rows.Scan(&id, &typ, &paid, &consumed, &refunded, &reserved, &offset); err != nil {
			rows.Close()
			return err
		}
		p, _ := decimal.NewFromString(paid)
		c, _ := decimal.NewFromString(consumed)
		r, _ := decimal.NewFromString(refunded)
		rs, _ := decimal.NewFromString(reserved)
		o, _ := decimal.NewFromString(offset)
		grants = append(grants, accounting.Grant{ID: id, Type: typ, Paid: p, ConsumedPaid: c, RefundedPaid: r, ReservedPaid: rs, DebtOffsetPaid: o})
	}
	rows.Close()
	allocation, err := accounting.AllocatePaidRefund(grants, paidQuota)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(allocation.Allocations)
	if _, err := tx.Client().ExecContext(ctx, `INSERT INTO user_quota_adjustments (user_id,adjustment_type,payment_order_id,reserved_allocations,applied_allocations,refund_amount,refund_currency,refund_method,applied_paid_quota_usd,actor_type,reason,status,idempotency_key,adjusted_at) VALUES ($1,'refund_recovery',$2,'[]'::jsonb,$3,$4,'CNY','original_channel',$5,'system',$6,'completed',$7,NOW())`, userID, orderID, raw, refundAmount.StringFixed(8), paidQuota.StringFixed(8), strings.TrimSpace(reason), strings.TrimSpace(idempotencyKey)); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"user_id": userID, "payment_order_id": orderID, "idempotency_key": strings.TrimSpace(idempotencyKey), "applied_paid_quota_usd": paidQuota.StringFixed(8)})
	if _, err := tx.Client().ExecContext(ctx, `INSERT INTO scheduler_outbox (event_type,payload) VALUES ($1,$2::jsonb)`, SchedulerOutboxEventQuotaRefundRequested, payload); err != nil {
		return err
	}
	for _, a := range allocation.Allocations {
		if _, err := tx.Client().ExecContext(ctx, `UPDATE user_quota_grants SET refunded_paid_quota_usd=refunded_paid_quota_usd+$1 WHERE id=$2`, a.Quota.StringFixed(8), a.GrantID); err != nil {
			return err
		}
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE payment_orders SET refunded_paid_quota_usd=refunded_paid_quota_usd+$1 WHERE id=$2`, paidQuota.StringFixed(8), orderID); err != nil {
		return err
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE user_wallets SET paid_quota_balance_usd=paid_quota_balance_usd-$1,version=version+1,updated_at=NOW() WHERE user_id=$2`, paidQuota.StringFixed(8), userID); err != nil {
		return err
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE users SET balance=(SELECT paid_quota_balance_usd+gift_quota_balance_usd FROM user_wallets WHERE user_id=$1),updated_at=NOW() WHERE id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// QuotaAccountingService owns v2 positive quota facts and their wallet projection.
// All writes are performed in one Ent transaction; provider calls must happen outside it.
type QuotaAccountingService struct{ client *dbent.Client }

func NewQuotaAccountingService(client *dbent.Client) *QuotaAccountingService {
	return &QuotaAccountingService{client: client}
}

func (s *QuotaAccountingService) CreatePaymentOrderGrant(ctx context.Context, userID, orderID int64, paid, gift decimal.Decimal, key, note string) (QuotaGrantResult, error) {
	return s.CreateQuotaGrant(ctx, QuotaGrantInput{UserID: userID, GrantType: "payment_order", PaymentOrderID: &orderID, PaidQuotaUSD: paid, GiftQuotaUSD: gift, IdempotencyKey: key, Note: note})
}

func (s *QuotaAccountingService) CreateRedeemCodeGrant(ctx context.Context, userID, redeemCodeID int64, paid, gift decimal.Decimal, key, note string) (QuotaGrantResult, error) {
	return s.CreateQuotaGrant(ctx, QuotaGrantInput{UserID: userID, GrantType: "redeem_code", RedeemCodeID: &redeemCodeID, PaidQuotaUSD: paid, GiftQuotaUSD: gift, IdempotencyKey: key, Note: note})
}

func (s *QuotaAccountingService) CreatePromoBonusGrant(ctx context.Context, userID, usageID int64, gift decimal.Decimal, key, note string) (QuotaGrantResult, error) {
	return s.CreateQuotaGrant(ctx, QuotaGrantInput{UserID: userID, GrantType: "promo_bonus", PromoCodeUsageID: &usageID, GiftQuotaUSD: gift, IdempotencyKey: key, Note: note})
}

func (s *QuotaAccountingService) CreateAffiliateRebateGrant(ctx context.Context, userID, ledgerID int64, gift decimal.Decimal, key, note string) (QuotaGrantResult, error) {
	return s.CreateQuotaGrant(ctx, QuotaGrantInput{UserID: userID, GrantType: "affiliate_rebate", AffiliateLedgerID: &ledgerID, GiftQuotaUSD: gift, IdempotencyKey: key, Note: note})
}

func (s *QuotaAccountingService) CreateAdminGiftGrant(ctx context.Context, userID, operatorID int64, gift decimal.Decimal, key, note string) (QuotaGrantResult, error) {
	return s.CreateQuotaGrant(ctx, QuotaGrantInput{UserID: userID, GrantType: "admin_gift", GiftQuotaUSD: gift, IdempotencyKey: key, Note: note, OperatorUserID: &operatorID})
}

func (s *QuotaAccountingService) CreateQuotaGrant(ctx context.Context, in QuotaGrantInput) (QuotaGrantResult, error) {
	if s == nil || s.client == nil {
		return QuotaGrantResult{}, ErrQuotaAccountingUnavailable
	}
	domainInput := accounting.GrantInput{UserID: in.UserID, Type: in.GrantType, PaymentOrderID: in.PaymentOrderID, RedeemCodeID: in.RedeemCodeID, PromoUsageID: in.PromoCodeUsageID, AffiliateLedgerID: in.AffiliateLedgerID, Paid: in.PaidQuotaUSD, Gift: in.GiftQuotaUSD, IdempotencyKey: in.IdempotencyKey}
	if err := domainInput.Validate(); err != nil {
		return QuotaGrantResult{}, fmt.Errorf("validate quota grant: %w", err)
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return QuotaGrantResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, err := tx.UserQuotaGrant.Query().Where(userquotagrant.UserIDEQ(in.UserID), userquotagrant.IdempotencyKeyEQ(key)).Only(ctx); err == nil {
		if !existing.PaidQuotaUsd.Equal(in.PaidQuotaUSD) || !existing.GiftQuotaUsd.Equal(in.GiftQuotaUSD) || existing.GrantType != in.GrantType {
			return QuotaGrantResult{}, ErrQuotaGrantIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return QuotaGrantResult{}, err
		}
		return QuotaGrantResult{GrantID: existing.ID, PaidQuotaUSD: existing.PaidQuotaUsd, GiftQuotaUSD: existing.GiftQuotaUsd, LegacyDebtOffsetPaidUSD: existing.LegacyDebtOffsetPaidQuotaUsd, Idempotent: true}, nil
	} else if !dbent.IsNotFound(err) {
		return QuotaGrantResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, in.UserID); err != nil {
		return QuotaGrantResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `INSERT INTO user_wallets (user_id,cash_balance_cny,paid_quota_balance_usd,gift_quota_balance_usd,version,created_at,updated_at) SELECT id,0,balance,0,1,NOW(),NOW() FROM users WHERE id=$1 ON CONFLICT (user_id) DO NOTHING`, in.UserID); err != nil {
		return QuotaGrantResult{}, err
	}
	var paidBefore, giftBefore string
	if err := scanQuotaOne(ctx, tx.Client(), `SELECT paid_quota_balance_usd::text,gift_quota_balance_usd::text FROM user_wallets WHERE user_id=$1 FOR UPDATE`, []any{in.UserID}, &paidBefore, &giftBefore); err != nil {
		return QuotaGrantResult{}, err
	}
	paidBalance, err := decimal.NewFromString(paidBefore)
	if err != nil {
		return QuotaGrantResult{}, err
	}
	giftBalance, err := decimal.NewFromString(giftBefore)
	if err != nil {
		return QuotaGrantResult{}, err
	}
	var opening, priorOffset string
	if err := scanQuotaOne(ctx, tx.Client(), `SELECT COALESCE((-paid_quota_usd)::text,'0') FROM user_quota_grants WHERE user_id=$1 AND grant_type='migration_opening' AND paid_quota_usd < 0 LIMIT 1`, []any{in.UserID}, &opening); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return QuotaGrantResult{}, err
	}
	if opening == "" {
		opening = "0"
	}
	if err := scanQuotaOne(ctx, tx.Client(), `SELECT COALESCE(SUM(legacy_debt_offset_paid_quota_usd),'0')::text FROM user_quota_grants WHERE user_id=$1 AND grant_type <> 'migration_opening'`, []any{in.UserID}, &priorOffset); err != nil {
		return QuotaGrantResult{}, err
	}
	openingD, err := decimal.NewFromString(opening)
	if err != nil {
		return QuotaGrantResult{}, err
	}
	priorD, err := decimal.NewFromString(priorOffset)
	if err != nil {
		return QuotaGrantResult{}, err
	}
	offset := accounting.ApplyLegacyDebtOffset(openingD.Neg(), in.PaidQuotaUSD, priorD).Offset
	now := time.Now().UTC()
	b := tx.UserQuotaGrant.Create().SetUserID(in.UserID).SetGrantType(in.GrantType).SetPaidQuotaUsd(in.PaidQuotaUSD).SetGiftQuotaUsd(in.GiftQuotaUSD).SetTotalQuotaUsd(in.PaidQuotaUSD.Add(in.GiftQuotaUSD)).SetConsumedPaidQuotaUsd(decimal.Zero).SetConsumedGiftQuotaUsd(decimal.Zero).SetRefundedPaidQuotaUsd(decimal.Zero).SetDeductedGiftQuotaUsd(decimal.Zero).SetReservedPaidQuotaUsd(decimal.Zero).SetLegacyDebtOffsetPaidQuotaUsd(offset).SetIdempotencyKey(key).SetRuleSnapshot(in.RuleSnapshot).SetNote(in.Note).SetGrantedAt(now)
	if in.PaymentOrderID != nil {
		b.SetPaymentOrderID(*in.PaymentOrderID)
	}
	if in.RedeemCodeID != nil {
		b.SetRedeemCodeID(*in.RedeemCodeID)
	}
	if in.PromoCodeUsageID != nil {
		b.SetPromoCodeUsageID(*in.PromoCodeUsageID)
	}
	if in.AffiliateLedgerID != nil {
		b.SetAffiliateLedgerID(*in.AffiliateLedgerID)
	}
	if in.OperatorUserID != nil {
		b.SetOperatorUserID(*in.OperatorUserID)
	}
	grant, err := b.Save(ctx)
	if err != nil {
		if dbent.IsConstraintError(err) {
			return s.retryGrantIdempotency(ctx, tx, in)
		}
		return QuotaGrantResult{}, err
	}
	paidAfter, giftAfter := paidBalance.Add(in.PaidQuotaUSD), giftBalance.Add(in.GiftQuotaUSD)
	if _, err := tx.Client().ExecContext(ctx, `UPDATE user_wallets SET paid_quota_balance_usd=$1,gift_quota_balance_usd=$2,version=version+1,updated_at=NOW() WHERE user_id=$3`, paidAfter.StringFixed(8), giftAfter.StringFixed(8), in.UserID); err != nil {
		return QuotaGrantResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE users SET balance=$1,updated_at=NOW() WHERE id=$2 AND deleted_at IS NULL`, paidAfter.Add(giftAfter).StringFixed(8), in.UserID); err != nil {
		return QuotaGrantResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return QuotaGrantResult{}, err
	}
	return QuotaGrantResult{GrantID: grant.ID, PaidQuotaUSD: in.PaidQuotaUSD, GiftQuotaUSD: in.GiftQuotaUSD, LegacyDebtOffsetPaidUSD: offset}, nil
}

// CreateAdminGiftDeduction atomically reserves and applies a FIFO gift-only
// deduction. It never touches payment orders or the paid bucket.
func (s *QuotaAccountingService) CreateAdminGiftDeduction(ctx context.Context, in GiftDeductionInput) (GiftDeductionResult, error) {
	if s == nil || s.client == nil {
		return GiftDeductionResult{}, ErrQuotaAccountingUnavailable
	}
	if in.UserID <= 0 || in.OperatorUserID <= 0 || in.AmountUSD.LessThanOrEqual(decimal.Zero) || strings.TrimSpace(in.IdempotencyKey) == "" || strings.TrimSpace(in.Reason) == "" {
		return GiftDeductionResult{}, fmt.Errorf("invalid gift deduction")
	}
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return GiftDeductionResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var existingID int64
	var existingAmount string
	err = scanQuotaOne(ctx, tx.Client(), `SELECT id, applied_gift_quota_usd::text FROM user_quota_adjustments WHERE user_id=$1 AND idempotency_key=$2`, []any{in.UserID, strings.TrimSpace(in.IdempotencyKey)}, &existingID, &existingAmount)
	if err == nil {
		a, parseErr := decimal.NewFromString(existingAmount)
		if parseErr != nil {
			return GiftDeductionResult{}, parseErr
		}
		if !a.Equal(in.AmountUSD) {
			return GiftDeductionResult{}, ErrQuotaGrantIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return GiftDeductionResult{}, err
		}
		return GiftDeductionResult{AdjustmentID: existingID, AppliedUSD: a, Idempotent: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return GiftDeductionResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `SELECT id FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, in.UserID); err != nil {
		return GiftDeductionResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `INSERT INTO user_wallets (user_id,cash_balance_cny,paid_quota_balance_usd,gift_quota_balance_usd,version,created_at,updated_at) SELECT id,0,balance,0,1,NOW(),NOW() FROM users WHERE id=$1 ON CONFLICT (user_id) DO NOTHING`, in.UserID); err != nil {
		return GiftDeductionResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `SELECT user_id FROM user_wallets WHERE user_id=$1 FOR UPDATE`, in.UserID); err != nil {
		return GiftDeductionResult{}, err
	}
	rows, err := tx.Client().QueryContext(ctx, `SELECT id,gift_quota_usd::text,consumed_gift_quota_usd::text,deducted_gift_quota_usd::text FROM user_quota_grants WHERE user_id=$1 AND grant_type <> 'migration_opening' ORDER BY granted_at ASC,id ASC FOR UPDATE`, in.UserID)
	if err != nil {
		return GiftDeductionResult{}, err
	}
	type alloc struct {
		GrantID int64  `json:"grant_id"`
		Bucket  string `json:"bucket"`
		Quota   string `json:"quota_usd"`
	}
	allocs := make([]alloc, 0)
	remaining := in.AmountUSD
	for rows.Next() {
		var id int64
		var total, consumed, deducted string
		if err := rows.Scan(&id, &total, &consumed, &deducted); err != nil {
			rows.Close()
			return GiftDeductionResult{}, err
		}
		t, _ := decimal.NewFromString(total)
		c, _ := decimal.NewFromString(consumed)
		d, _ := decimal.NewFromString(deducted)
		available := t.Sub(c).Sub(d)
		if available.LessThanOrEqual(decimal.Zero) {
			continue
		}
		x := decimal.Min(available, remaining)
		if x.IsZero() {
			continue
		}
		allocs = append(allocs, alloc{GrantID: id, Bucket: "gift", Quota: x.StringFixed(8)})
		remaining = remaining.Sub(x)
		if remaining.IsZero() {
			break
		}
	}
	rows.Close()
	if !remaining.IsZero() {
		return GiftDeductionResult{}, accounting.ErrInsufficientQuota
	}
	raw, _ := json.Marshal(allocs)
	var adjustmentID int64
	err = scanQuotaOne(ctx, tx.Client(), `INSERT INTO user_quota_adjustments (user_id,adjustment_type,reserved_allocations,applied_allocations,refund_amount,applied_gift_quota_usd,actor_type,reason,status,idempotency_key,operator_user_id,adjusted_at) VALUES ($1,'admin_gift_deduction','[]'::jsonb,$2,0,$3,'admin',$4,'completed',$5,$6,NOW()) RETURNING id`, []any{in.UserID, raw, in.AmountUSD.StringFixed(8), strings.TrimSpace(in.Reason), strings.TrimSpace(in.IdempotencyKey), in.OperatorUserID}, &adjustmentID)
	if err != nil {
		return GiftDeductionResult{}, err
	}
	for _, a := range allocs {
		q, _ := decimal.NewFromString(a.Quota)
		if _, err := tx.Client().ExecContext(ctx, `UPDATE user_quota_grants SET deducted_gift_quota_usd=deducted_gift_quota_usd+$1 WHERE id=$2`, q.StringFixed(8), a.GrantID); err != nil {
			return GiftDeductionResult{}, err
		}
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE user_wallets SET gift_quota_balance_usd=gift_quota_balance_usd-$1,version=version+1,updated_at=NOW() WHERE user_id=$2`, in.AmountUSD.StringFixed(8), in.UserID); err != nil {
		return GiftDeductionResult{}, err
	}
	if _, err := tx.Client().ExecContext(ctx, `UPDATE users SET balance=(SELECT paid_quota_balance_usd+gift_quota_balance_usd FROM user_wallets WHERE user_id=$1),updated_at=NOW() WHERE id=$1`, in.UserID); err != nil {
		return GiftDeductionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return GiftDeductionResult{}, err
	}
	return GiftDeductionResult{AdjustmentID: adjustmentID, AppliedUSD: in.AmountUSD}, nil
}

func scanQuotaOne(ctx context.Context, c *dbent.Client, query string, args []any, dest ...any) error {
	rows, err := c.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return rows.Scan(dest...)
}

func (s *QuotaAccountingService) retryGrantIdempotency(ctx context.Context, tx *dbent.Tx, in QuotaGrantInput) (QuotaGrantResult, error) {
	existing, err := tx.UserQuotaGrant.Query().Where(userquotagrant.UserIDEQ(in.UserID), userquotagrant.IdempotencyKeyEQ(strings.TrimSpace(in.IdempotencyKey))).Only(ctx)
	if err != nil {
		return QuotaGrantResult{}, err
	}
	if !existing.PaidQuotaUsd.Equal(in.PaidQuotaUSD) || !existing.GiftQuotaUsd.Equal(in.GiftQuotaUSD) || existing.GrantType != in.GrantType {
		return QuotaGrantResult{}, ErrQuotaGrantIdempotencyConflict
	}
	if err := tx.Commit(); err != nil {
		return QuotaGrantResult{}, err
	}
	return QuotaGrantResult{GrantID: existing.ID, PaidQuotaUSD: existing.PaidQuotaUsd, GiftQuotaUSD: existing.GiftQuotaUsd, LegacyDebtOffsetPaidUSD: existing.LegacyDebtOffsetPaidQuotaUsd, Idempotent: true}, nil
}
