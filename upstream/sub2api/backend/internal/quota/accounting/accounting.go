package accounting

import (
	"errors"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	ErrInsufficientQuota = errors.New("insufficient quota")
	ErrInvalidGrant      = errors.New("invalid quota grant")
)

type Grant struct {
	ID             int64
	Type           string
	Paid           decimal.Decimal
	Gift           decimal.Decimal
	ConsumedPaid   decimal.Decimal
	ConsumedGift   decimal.Decimal
	RefundedPaid   decimal.Decimal
	DeductedGift   decimal.Decimal
	ReservedPaid   decimal.Decimal
	DebtOffsetPaid decimal.Decimal
}

type GrantInput struct {
	Type              string
	UserID            int64
	PaymentOrderID    *int64
	RedeemCodeID      *int64
	PromoUsageID      *int64
	AffiliateLedgerID *int64
	Paid              decimal.Decimal
	Gift              decimal.Decimal
	IdempotencyKey    string
}

func (in GrantInput) Validate() error {
	if in.UserID <= 0 || strings.TrimSpace(in.Type) == "" || strings.TrimSpace(in.IdempotencyKey) == "" {
		return ErrInvalidGrant
	}
	if in.Paid.IsNegative() || in.Gift.IsNegative() || in.Paid.Add(in.Gift).LessThanOrEqual(decimal.Zero) {
		return ErrInvalidGrant
	}
	sources := 0
	if in.PaymentOrderID != nil {
		sources++
	}
	if in.RedeemCodeID != nil {
		sources++
	}
	if in.PromoUsageID != nil {
		sources++
	}
	if in.AffiliateLedgerID != nil {
		sources++
	}
	switch in.Type {
	case "payment_order":
		if sources != 1 || in.PaymentOrderID == nil {
			return ErrInvalidGrant
		}
	case "redeem_code":
		if sources != 1 || in.RedeemCodeID == nil {
			return ErrInvalidGrant
		}
	case "promo_bonus":
		if sources != 1 || in.PromoUsageID == nil {
			return ErrInvalidGrant
		}
	case "affiliate_rebate":
		if sources != 1 || in.AffiliateLedgerID == nil {
			return ErrInvalidGrant
		}
	case "admin_gift":
		if sources != 0 || in.Gift.IsZero() {
			return ErrInvalidGrant
		}
	default:
		return ErrInvalidGrant
	}
	return nil
}

type DebtOffsetResult struct{ Offset, RemainingDebt decimal.Decimal }

func ApplyLegacyDebtOffset(opening, paid, priorOffset decimal.Decimal) DebtOffsetResult {
	debt := decimal.Zero
	if opening.IsNegative() {
		debt = opening.Abs()
	}
	if priorOffset.GreaterThan(debt) {
		priorOffset = debt
	}
	debt = debt.Sub(priorOffset)
	if paid.IsNegative() {
		paid = decimal.Zero
	}
	offset := decimal.Min(paid, debt)
	return DebtOffsetResult{Offset: offset, RemainingDebt: debt.Sub(offset)}
}

type Allocation struct {
	GrantID int64
	Bucket  string
	Quota   decimal.Decimal
}
type AllocationResult struct {
	Paid, Gift  decimal.Decimal
	Allocations []Allocation
}

// DeductGift allocates an administrative gift deduction in grant FIFO order.
// Opening snapshots are never eligible and insufficient requests are atomic.
func DeductGift(grants []Grant, requested decimal.Decimal) (AllocationResult, error) {
	if requested.IsNegative() || requested.IsZero() {
		return AllocationResult{}, ErrInsufficientQuota
	}
	ordered := append([]Grant(nil), grants...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	remaining := requested
	result := AllocationResult{Allocations: make([]Allocation, 0)}
	for _, g := range ordered {
		if g.Type == "migration_opening" {
			continue
		}
		available := g.Gift.Sub(g.ConsumedGift).Sub(g.DeductedGift)
		if available.LessThanOrEqual(decimal.Zero) {
			continue
		}
		amount := decimal.Min(available, remaining)
		if amount.IsZero() {
			continue
		}
		result.Gift = result.Gift.Add(amount)
		result.Allocations = append(result.Allocations, Allocation{GrantID: g.ID, Bucket: "gift", Quota: amount})
		remaining = remaining.Sub(amount)
		if remaining.IsZero() {
			return result, nil
		}
	}
	return AllocationResult{}, ErrInsufficientQuota
}

func RefundablePaidQuota(grants []Grant) decimal.Decimal {
	total := decimal.Zero
	for _, g := range grants {
		if g.Type == "migration_opening" {
			continue
		}
		available := g.Paid.Sub(g.ConsumedPaid).Sub(g.RefundedPaid).Sub(g.ReservedPaid).Sub(g.DebtOffsetPaid)
		if available.IsPositive() {
			total = total.Add(available)
		}
	}
	return total
}

func AllocatePaidRefund(grants []Grant, requested decimal.Decimal) (AllocationResult, error) {
	if requested.LessThanOrEqual(decimal.Zero) {
		return AllocationResult{}, ErrInsufficientQuota
	}
	ordered := append([]Grant(nil), grants...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	remaining := requested
	result := AllocationResult{Allocations: make([]Allocation, 0)}
	for _, g := range ordered {
		if g.Type == "migration_opening" {
			continue
		}
		available := g.Paid.Sub(g.ConsumedPaid).Sub(g.RefundedPaid).Sub(g.ReservedPaid).Sub(g.DebtOffsetPaid)
		if available.LessThanOrEqual(decimal.Zero) {
			continue
		}
		amount := decimal.Min(available, remaining)
		result.Paid = result.Paid.Add(amount)
		result.Allocations = append(result.Allocations, Allocation{GrantID: g.ID, Bucket: "paid", Quota: amount})
		remaining = remaining.Sub(amount)
		if remaining.IsZero() {
			return result, nil
		}
	}
	return AllocationResult{}, ErrInsufficientQuota
}

// ReservePaidRefund uses the same FIFO eligibility rules as refund allocation.
// The service layer persists the resulting allocation as a reservation before
// invoking an external provider.
func ReservePaidRefund(grants []Grant, requested decimal.Decimal) (AllocationResult, error) {
	return AllocatePaidRefund(grants, requested)
}

// RefundLimit returns the maximum refundable amount in the same quota unit,
// bounded by both remaining cash and remaining paid quota.
func RefundLimit(remainingCash, remainingPaid decimal.Decimal) decimal.Decimal {
	if remainingCash.IsNegative() {
		remainingCash = decimal.Zero
	}
	if remainingPaid.IsNegative() {
		remainingPaid = decimal.Zero
	}
	return decimal.Min(remainingCash, remainingPaid)
}

func Allocate(grants []Grant, requested decimal.Decimal) (AllocationResult, error) {
	if requested.IsNegative() || requested.IsZero() {
		return AllocationResult{}, ErrInsufficientQuota
	}
	ordered := append([]Grant(nil), grants...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	remaining := requested
	result := AllocationResult{Allocations: make([]Allocation, 0)}
	for _, bucket := range []string{"paid", "gift"} {
		for _, g := range ordered {
			if g.Type == "migration_opening" {
				continue
			}
			available := decimal.Zero
			if bucket == "paid" {
				available = g.Paid.Sub(g.ConsumedPaid).Sub(g.RefundedPaid).Sub(g.ReservedPaid).Sub(g.DebtOffsetPaid)
			} else {
				available = g.Gift.Sub(g.ConsumedGift).Sub(g.DeductedGift)
			}
			if available.LessThanOrEqual(decimal.Zero) {
				continue
			}
			amount := decimal.Min(available, remaining)
			if amount.IsZero() {
				continue
			}
			result.Allocations = append(result.Allocations, Allocation{GrantID: g.ID, Bucket: bucket, Quota: amount})
			if bucket == "paid" {
				result.Paid = result.Paid.Add(amount)
			} else {
				result.Gift = result.Gift.Add(amount)
			}
			remaining = remaining.Sub(amount)
			if remaining.IsZero() {
				return result, nil
			}
		}
	}
	return AllocationResult{}, ErrInsufficientQuota
}
