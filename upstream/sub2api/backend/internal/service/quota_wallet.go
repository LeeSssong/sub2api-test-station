package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

var (
	ErrQuotaInvalidAmount       = errors.New("quota amount must be greater than zero")
	ErrQuotaNegativeAmount      = errors.New("quota amount must not be negative")
	ErrQuotaInsufficient        = errors.New("insufficient quota balance")
	ErrQuotaRefundExceedsCash   = errors.New("refund exceeds cash balance")
	ErrQuotaRefundExceedsPaid   = errors.New("refund exceeds paid quota balance")
	ErrQuotaLegacySetBelowGift  = errors.New("legacy target balance cannot be below gift quota balance")
	ErrQuotaIdempotencyConflict = errors.New("idempotency key was already used with a different request")
	ErrQuotaWalletNotFound      = errors.New("quota wallet user not found")
)

const (
	QuotaRecordRecharge            = "recharge"
	QuotaRecordRefund              = "refund"
	QuotaRecordUsageConsumption    = "usage_consumption"
	QuotaRecordLegacyBalanceAdjust = "legacy_balance_adjustment"
)

type QuotaSummary struct {
	UserID               int64
	CashBalanceCNY       decimal.Decimal
	PaidQuotaBalanceUSD  decimal.Decimal
	GiftQuotaBalanceUSD  decimal.Decimal
	TotalQuotaBalanceUSD decimal.Decimal
	WalletVersion        int64
	UpdatedAt            time.Time
}

type QuotaMutationResult struct {
	Summary            QuotaSummary
	CashDeltaCNY       decimal.Decimal
	PaidDeltaUSD       decimal.Decimal
	GiftDeltaUSD       decimal.Decimal
	PaidConsumedUSD    decimal.Decimal
	GiftConsumedUSD    decimal.Decimal
	LedgerEntryID      int64
	Idempotent         bool
	RequestFingerprint string
}

type RechargeInput struct {
	UserID         int64
	AmountCNY      decimal.Decimal
	GiftQuotaUSD   decimal.Decimal
	IdempotencyKey string
	ReferenceType  string
	ReferenceID    string
	Note           string
	OperatorID     *int64
}

type RefundInput struct {
	UserID         int64
	AmountCNY      decimal.Decimal
	IdempotencyKey string
	ReferenceType  string
	ReferenceID    string
	Note           string
	OperatorID     *int64
}

type UsageConsumptionInput struct {
	UserID         int64
	AmountUSD      decimal.Decimal
	IdempotencyKey string
	ReferenceType  string
	ReferenceID    string
	Note           string
}

type LegacyBalanceAdjustmentInput struct {
	UserID         int64
	Mode           string // add, subtract, set
	AmountUSD      decimal.Decimal
	TargetUSD      decimal.Decimal
	IdempotencyKey string
	ReferenceType  string
	ReferenceID    string
	Note           string
	OperatorID     *int64
}

type QuotaWallet struct {
	ID                  int64
	UserID              int64
	CashBalanceCNY      decimal.Decimal
	PaidQuotaBalanceUSD decimal.Decimal
	GiftQuotaBalanceUSD decimal.Decimal
	Version             int64
	UpdatedAt           time.Time
}

type QuotaLedgerEntry struct {
	ID                int64
	UserID            int64
	RecordType        string
	CashDeltaCNY      decimal.Decimal
	PaidQuotaDeltaUSD decimal.Decimal
	GiftQuotaDeltaUSD decimal.Decimal
	CashBeforeCNY     decimal.Decimal
	CashAfterCNY      decimal.Decimal
	PaidBeforeUSD     decimal.Decimal
	PaidAfterUSD      decimal.Decimal
	GiftBeforeUSD     decimal.Decimal
	GiftAfterUSD      decimal.Decimal
	ReferenceType     string
	ReferenceID       string
	IdempotencyKey    string
	Note              string
	OperatorID        *int64
	Status            string
	CreatedAt         time.Time
}

type QuotaWalletRepository interface {
	WithLockedWallet(ctx context.Context, userID int64, fn func(context.Context, *QuotaWallet) error) error
	GetSummary(ctx context.Context, userID int64) (QuotaSummary, error)
	ApplyMutation(ctx context.Context, wallet *QuotaWallet, result QuotaMutationResult, recordType, idemKey, referenceType, referenceID, note string, operatorID *int64) (QuotaMutationResult, error)
	ListLedger(ctx context.Context, userID int64, page, pageSize int, recordType string) ([]QuotaLedgerEntry, int, error)
}

type QuotaWalletService interface {
	GetSummary(context.Context, int64) (QuotaSummary, error)
	ListLedger(context.Context, int64, int, int, string) ([]QuotaLedgerEntry, int, error)
	Recharge(context.Context, RechargeInput) (QuotaMutationResult, error)
	Refund(context.Context, RefundInput) (QuotaMutationResult, error)
	ConsumeUsage(context.Context, UsageConsumptionInput) (QuotaMutationResult, error)
	LegacyAdjust(context.Context, LegacyBalanceAdjustmentInput) (QuotaMutationResult, error)
}

// RedeemBalanceAdjuster provides the historical floor-at-zero semantics for
// negative redeem codes without a read-then-write race.
type RedeemBalanceAdjuster interface {
	AdjustRedeemBalance(context.Context, int64, decimal.Decimal) (QuotaMutationResult, error)
}

type quotaWalletService struct{ repo QuotaWalletRepository }

func NewQuotaWalletService(repo QuotaWalletRepository) QuotaWalletService {
	return &quotaWalletService{repo: repo}
}

func (s *quotaWalletService) GetSummary(ctx context.Context, userID int64) (QuotaSummary, error) {
	if userID <= 0 {
		return QuotaSummary{}, ErrQuotaWalletNotFound
	}
	return s.repo.GetSummary(ctx, userID)
}

func (s *quotaWalletService) ListLedger(ctx context.Context, userID int64, page, pageSize int, recordType string) ([]QuotaLedgerEntry, int, error) {
	if userID <= 0 {
		return nil, 0, ErrQuotaWalletNotFound
	}
	return s.repo.ListLedger(ctx, userID, page, pageSize, strings.TrimSpace(recordType))
}

func (s *quotaWalletService) mutate(ctx context.Context, userID int64, key, fingerprint string, fn func(*QuotaWallet) (QuotaMutationResult, error), recordType, refType, refID, note string, operator *int64) (QuotaMutationResult, error) {
	if userID <= 0 {
		return QuotaMutationResult{}, ErrQuotaWalletNotFound
	}
	var out QuotaMutationResult
	err := s.repo.WithLockedWallet(ctx, userID, func(txctx context.Context, wallet *QuotaWallet) error {
		result, err := fn(wallet)
		if err != nil {
			return err
		}
		result.RequestFingerprint = fingerprint
		out, err = s.repo.ApplyMutation(txctx, wallet, result, recordType, key, refType, refID, note, operator)
		return err
	})
	return out, err
}

func (s *quotaWalletService) Recharge(ctx context.Context, in RechargeInput) (QuotaMutationResult, error) {
	if in.AmountCNY.IsNegative() || in.GiftQuotaUSD.IsNegative() {
		return QuotaMutationResult{}, ErrQuotaNegativeAmount
	}
	if in.AmountCNY.IsZero() && in.GiftQuotaUSD.IsZero() {
		return QuotaMutationResult{}, ErrQuotaInvalidAmount
	}
	fp := quotaRequestFingerprint(QuotaRecordRecharge, in.AmountCNY, in.GiftQuotaUSD, in.ReferenceType, in.ReferenceID, in.Note, in.OperatorID)
	return s.mutate(ctx, in.UserID, in.IdempotencyKey, fp, func(w *QuotaWallet) (QuotaMutationResult, error) {
		cash, paid, gift := in.AmountCNY, in.AmountCNY, in.GiftQuotaUSD
		return mutationResult(w, cash, paid, gift, decimal.Zero, decimal.Zero), nil
	}, QuotaRecordRecharge, in.ReferenceType, in.ReferenceID, in.Note, in.OperatorID)
}

func (s *quotaWalletService) Refund(ctx context.Context, in RefundInput) (QuotaMutationResult, error) {
	if in.AmountCNY.IsNegative() {
		return QuotaMutationResult{}, ErrQuotaNegativeAmount
	}
	if in.AmountCNY.IsZero() {
		return QuotaMutationResult{}, ErrQuotaInvalidAmount
	}
	fp := quotaRequestFingerprint(QuotaRecordRefund, in.AmountCNY, in.ReferenceType, in.ReferenceID, in.Note, in.OperatorID)
	return s.mutate(ctx, in.UserID, in.IdempotencyKey, fp, func(w *QuotaWallet) (QuotaMutationResult, error) {
		if in.AmountCNY.GreaterThan(w.CashBalanceCNY) {
			return QuotaMutationResult{}, ErrQuotaRefundExceedsCash
		}
		if in.AmountCNY.GreaterThan(w.PaidQuotaBalanceUSD) {
			return QuotaMutationResult{}, ErrQuotaRefundExceedsPaid
		}
		return mutationResult(w, in.AmountCNY.Neg(), in.AmountCNY.Neg(), w.GiftQuotaBalanceUSD.Neg(), decimal.Zero, decimal.Zero), nil
	}, QuotaRecordRefund, in.ReferenceType, in.ReferenceID, in.Note, in.OperatorID)
}

func (s *quotaWalletService) ConsumeUsage(ctx context.Context, in UsageConsumptionInput) (QuotaMutationResult, error) {
	if in.AmountUSD.IsNegative() {
		return QuotaMutationResult{}, ErrQuotaNegativeAmount
	}
	if in.AmountUSD.IsZero() {
		return QuotaMutationResult{}, ErrQuotaInvalidAmount
	}
	fp := quotaRequestFingerprint(QuotaRecordUsageConsumption, in.AmountUSD, in.ReferenceType, in.ReferenceID, in.Note)
	return s.mutate(ctx, in.UserID, in.IdempotencyKey, fp, func(w *QuotaWallet) (QuotaMutationResult, error) {
		if in.AmountUSD.GreaterThan(w.PaidQuotaBalanceUSD.Add(w.GiftQuotaBalanceUSD)) {
			return QuotaMutationResult{}, ErrQuotaInsufficient
		}
		paid := decimal.Min(in.AmountUSD, w.PaidQuotaBalanceUSD)
		gift := in.AmountUSD.Sub(paid)
		return mutationResult(w, decimal.Zero, paid.Neg(), gift.Neg(), paid, gift), nil
	}, QuotaRecordUsageConsumption, in.ReferenceType, in.ReferenceID, in.Note, nil)
}

func (s *quotaWalletService) LegacyAdjust(ctx context.Context, in LegacyBalanceAdjustmentInput) (QuotaMutationResult, error) {
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode != "add" && mode != "subtract" && mode != "set" {
		return QuotaMutationResult{}, fmt.Errorf("unsupported legacy balance adjustment mode %q", in.Mode)
	}
	if mode == "set" {
		if in.TargetUSD.IsNegative() {
			return QuotaMutationResult{}, ErrQuotaNegativeAmount
		}
		fp := quotaRequestFingerprint(QuotaRecordLegacyBalanceAdjust, mode, in.TargetUSD, in.ReferenceType, in.ReferenceID, in.Note, in.OperatorID)
		return s.mutate(ctx, in.UserID, in.IdempotencyKey, fp, func(w *QuotaWallet) (QuotaMutationResult, error) {
			if in.TargetUSD.LessThan(w.GiftQuotaBalanceUSD) {
				return QuotaMutationResult{}, ErrQuotaLegacySetBelowGift
			}
			return mutationResult(w, decimal.Zero, in.TargetUSD.Sub(w.PaidQuotaBalanceUSD.Add(w.GiftQuotaBalanceUSD)), decimal.Zero, decimal.Zero, decimal.Zero), nil
		}, QuotaRecordLegacyBalanceAdjust, in.ReferenceType, in.ReferenceID, in.Note, in.OperatorID)
	}
	if in.AmountUSD.IsNegative() {
		return QuotaMutationResult{}, ErrQuotaNegativeAmount
	}
	if in.AmountUSD.IsZero() {
		return QuotaMutationResult{}, ErrQuotaInvalidAmount
	}
	if mode == "subtract" {
		in.AmountUSD = in.AmountUSD.Neg()
	}
	fp := quotaRequestFingerprint(QuotaRecordLegacyBalanceAdjust, mode, in.AmountUSD, in.ReferenceType, in.ReferenceID, in.Note, in.OperatorID)
	return s.mutate(ctx, in.UserID, in.IdempotencyKey, fp, func(w *QuotaWallet) (QuotaMutationResult, error) {
		if in.AmountUSD.IsNegative() && in.AmountUSD.Abs().GreaterThan(w.PaidQuotaBalanceUSD) {
			return QuotaMutationResult{}, ErrQuotaInsufficient
		}
		return mutationResult(w, decimal.Zero, in.AmountUSD, decimal.Zero, decimal.Zero, decimal.Zero), nil
	}, QuotaRecordLegacyBalanceAdjust, in.ReferenceType, in.ReferenceID, in.Note, in.OperatorID)
}

// AdjustRedeemBalance applies a redeem balance delta while holding the wallet
// row lock. Negative deltas only reduce paid quota and clamp at zero; gift
// quota remains untouched to preserve the legacy redeem contract.
func (s *quotaWalletService) AdjustRedeemBalance(ctx context.Context, userID int64, delta decimal.Decimal) (QuotaMutationResult, error) {
	if delta.IsZero() {
		return QuotaMutationResult{}, ErrQuotaInvalidAmount
	}
	return s.mutate(ctx, userID, "", quotaRequestFingerprint(QuotaRecordLegacyBalanceAdjust, "redeem_floor", delta), func(w *QuotaWallet) (QuotaMutationResult, error) {
		paid := w.PaidQuotaBalanceUSD.Add(delta)
		if paid.IsNegative() {
			paid = decimal.Zero
		}
		return mutationResult(w, decimal.Zero, paid.Sub(w.PaidQuotaBalanceUSD), decimal.Zero, decimal.Zero, decimal.Zero), nil
	}, QuotaRecordLegacyBalanceAdjust, "redeem", "", "", nil)
}

func quotaRequestFingerprint(parts ...any) string {
	h := sha256.New()
	for _, part := range parts {
		// Operator IDs are passed as pointers by the handler. Hash the stable
		// value rather than the process-local pointer address so retries with
		// the same Idempotency-Key replay successfully.
		if operator, ok := part.(*int64); ok {
			if operator == nil {
				fmt.Fprint(h, "<nil>\x00")
			} else {
				fmt.Fprintf(h, "%d\x00", *operator)
			}
			continue
		}
		fmt.Fprintf(h, "%v\x00", part)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func mutationResult(w *QuotaWallet, cashDelta, paidDelta, giftDelta, paidConsumed, giftConsumed decimal.Decimal) QuotaMutationResult {
	return QuotaMutationResult{Summary: QuotaSummary{UserID: w.UserID, CashBalanceCNY: w.CashBalanceCNY.Add(cashDelta), PaidQuotaBalanceUSD: w.PaidQuotaBalanceUSD.Add(paidDelta), GiftQuotaBalanceUSD: w.GiftQuotaBalanceUSD.Add(giftDelta), TotalQuotaBalanceUSD: w.PaidQuotaBalanceUSD.Add(paidDelta).Add(w.GiftQuotaBalanceUSD.Add(giftDelta)), WalletVersion: w.Version + 1, UpdatedAt: time.Now().UTC()}, CashDeltaCNY: cashDelta, PaidDeltaUSD: paidDelta, GiftDeltaUSD: giftDelta, PaidConsumedUSD: paidConsumed, GiftConsumedUSD: giftConsumed}
}
