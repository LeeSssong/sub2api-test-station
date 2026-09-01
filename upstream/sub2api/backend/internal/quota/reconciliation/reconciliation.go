package reconciliation

import (
	"time"

	"github.com/shopspring/decimal"
)

type IssueKind string

const (
	IssueWalletFormula        IssueKind = "wallet_formula_difference"
	IssueAllocationMismatch   IssueKind = "allocation_difference"
	IssueRefundMismatch       IssueKind = "refund_difference"
	IssueDuplicateIdempotency IssueKind = "duplicate_idempotency_key"
	IssueInvalidAllocation    IssueKind = "invalid_allocation_json"
	IssueCrossUserGrant       IssueKind = "cross_user_grant"
	IssueCrossOrderGrant      IssueKind = "cross_order_grant"
	IssueUnknownReservation   IssueKind = "unknown_reservation"
	IssueLegacyUnknown        IssueKind = "legacy_unknown_residual"
	IssueUsageDeltaMismatch   IssueKind = "usage_delta_mismatch"
)

type Issue struct {
	Kind    IssueKind `json:"kind"`
	UserID  int64     `json:"user_id,omitempty"`
	Ref     string    `json:"ref,omitempty"`
	Message string    `json:"message"`
}

type WalletSnapshot struct {
	UserID      int64           `json:"user_id"`
	PaidBalance decimal.Decimal `json:"paid_balance_usd"`
	GiftBalance decimal.Decimal `json:"gift_balance_usd"`
}

type GrantSnapshot struct {
	ID               int64           `json:"id"`
	UserID           int64           `json:"user_id"`
	PaidGranted      decimal.Decimal `json:"paid_granted_usd"`
	GiftGranted      decimal.Decimal `json:"gift_granted_usd"`
	PaidConsumed     decimal.Decimal `json:"paid_consumed_usd"`
	GiftConsumed     decimal.Decimal `json:"gift_consumed_usd"`
	PaidRefunded     decimal.Decimal `json:"paid_refunded_usd"`
	GiftDeducted     decimal.Decimal `json:"gift_deducted_usd"`
	PaidReserved     decimal.Decimal `json:"paid_reserved_usd"`
	LegacyDebtOffset decimal.Decimal `json:"legacy_debt_offset_usd"`
}

type UsageSnapshot struct {
	ID                string          `json:"id,omitempty"`
	UserID            int64           `json:"user_id"`
	GrantID           int64           `json:"grant_id,omitempty"`
	PaidDelta         decimal.Decimal `json:"paid_delta_usd"`
	GiftDelta         decimal.Decimal `json:"gift_delta_usd"`
	Delta             decimal.Decimal `json:"delta_usd"`
	AttributionStatus string          `json:"attribution_status"`
	AllocationValid   bool            `json:"allocation_valid"`
	Allocations       []Allocation    `json:"allocations,omitempty"`
}

type Allocation struct {
	GrantID int64           `json:"grant_id"`
	Bucket  string          `json:"bucket"`
	Quota   decimal.Decimal `json:"quota_usd"`
}

type RefundSnapshot struct {
	OrderID  string          `json:"order_id"`
	Refunded decimal.Decimal `json:"refunded_usd"`
	Adjusted decimal.Decimal `json:"adjusted_usd"`
}

type Snapshot struct {
	Wallets                  []WalletSnapshot `json:"wallets"`
	Grants                   []GrantSnapshot  `json:"grants"`
	Usage                    []UsageSnapshot  `json:"usage"`
	Refunds                  []RefundSnapshot `json:"refunds"`
	DuplicateIdempotencyKeys []string         `json:"duplicate_idempotency_keys"`
	InvalidAllocationRows    int              `json:"invalid_allocation_rows"`
	UnknownReservations      int              `json:"unknown_reservations"`
	CrossOrderGrantRows      int              `json:"cross_order_grant_rows"`
	LegacyUnknownResidual    decimal.Decimal  `json:"legacy_unknown_residual_usd"`
}

type Report struct {
	BatchID   string                `json:"batch_id"`
	QueriedAt time.Time             `json:"queried_at"`
	Database  string                `json:"database,omitempty"`
	Issues    []Issue               `json:"issues"`
	Users     map[int64]UserSummary `json:"users"`
	Global    GlobalSummary         `json:"global"`
}

type UserSummary struct {
	Issues int `json:"issues"`
	Grants int `json:"grants"`
	Usage  int `json:"usage"`
}
type GlobalSummary struct {
	Users  int `json:"users"`
	Grants int `json:"grants"`
	Usage  int `json:"usage"`
	Issues int `json:"issues"`
}

func (r Report) HasDifferences() bool { return len(r.Issues) > 0 }
func (r Report) HasKind(kind IssueKind) bool {
	for _, issue := range r.Issues {
		if issue.Kind == kind {
			return true
		}
	}
	return false
}

func Evaluate(s Snapshot) Report {
	r := Report{QueriedAt: time.Now().UTC(), Issues: make([]Issue, 0), Users: make(map[int64]UserSummary)}
	for _, w := range s.Wallets {
		u := r.Users[w.UserID]
		r.Users[w.UserID] = u
	}
	for _, g := range s.Grants {
		u := r.Users[g.UserID]
		u.Grants++
		r.Users[g.UserID] = u
	}
	for _, u := range s.Usage {
		x := r.Users[u.UserID]
		x.Usage++
		r.Users[u.UserID] = x
	}
	r.Global.Users = len(r.Users)
	r.Global.Grants = len(s.Grants)
	r.Global.Usage = len(s.Usage)
	grantByID := make(map[int64]GrantSnapshot, len(s.Grants))
	for _, g := range s.Grants {
		grantByID[g.ID] = g
	}
	usagePaid := make(map[int64]decimal.Decimal)
	usageGift := make(map[int64]decimal.Decimal)
	for _, u := range s.Usage {
		if !u.AllocationValid {
			continue
		}
		if len(u.Allocations) > 0 {
			var allocatedPaid, allocatedGift decimal.Decimal
			for _, allocation := range u.Allocations {
				g, ok := grantByID[allocation.GrantID]
				if !ok || (g.UserID != 0 && u.UserID != g.UserID) {
					r.Issues = append(r.Issues, Issue{Kind: IssueCrossUserGrant, UserID: u.UserID, Ref: u.ID, Message: "usage allocation references a grant owned by another user or missing grant"})
					continue
				}
				if allocation.Bucket == "gift" {
					usageGift[allocation.GrantID] = usageGift[allocation.GrantID].Add(allocation.Quota)
					allocatedGift = allocatedGift.Add(allocation.Quota)
				} else {
					usagePaid[allocation.GrantID] = usagePaid[allocation.GrantID].Add(allocation.Quota)
					allocatedPaid = allocatedPaid.Add(allocation.Quota)
				}
			}
			if u.AttributionStatus == "exact" && (!allocatedPaid.Equal(u.PaidDelta.Abs()) || !allocatedGift.Equal(u.GiftDelta.Abs())) {
				r.Issues = append(r.Issues, Issue{Kind: IssueUsageDeltaMismatch, UserID: u.UserID, Ref: u.ID, Message: "exact usage allocations differ from signed usage deltas"})
			}
		} else if u.AttributionStatus == "exact" && (!u.PaidDelta.IsZero() || !u.GiftDelta.IsZero()) {
			r.Issues = append(r.Issues, Issue{Kind: IssueUsageDeltaMismatch, UserID: u.UserID, Ref: u.ID, Message: "exact usage entry has no grant allocations for a non-zero delta"})
		} else if u.GrantID != 0 {
			g, ok := grantByID[u.GrantID]
			if !ok || (g.UserID != 0 && u.UserID != g.UserID) {
				r.Issues = append(r.Issues, Issue{Kind: IssueCrossUserGrant, UserID: u.UserID, Ref: u.ID, Message: "usage allocation references a grant owned by another user or missing grant"})
				continue
			}
			usagePaid[u.GrantID] = usagePaid[u.GrantID].Add(u.PaidDelta)
			usageGift[u.GrantID] = usageGift[u.GrantID].Add(u.GiftDelta)
		}
	}
	for _, g := range s.Grants {
		if !usagePaid[g.ID].Equal(g.PaidConsumed) || !usageGift[g.ID].Equal(g.GiftConsumed) {
			r.Issues = append(r.Issues, Issue{Kind: IssueAllocationMismatch, UserID: g.UserID, Ref: decimal.NewFromInt(g.ID).String(), Message: "grant consumed totals differ from usage allocations"})
		}
	}
	for _, w := range s.Wallets {
		var paid, gift decimal.Decimal
		for _, g := range s.Grants {
			if g.UserID == w.UserID {
				paid = paid.Add(g.PaidGranted).Sub(g.PaidConsumed).Sub(g.PaidRefunded).Sub(g.PaidReserved).Sub(g.LegacyDebtOffset)
				gift = gift.Add(g.GiftGranted).Sub(g.GiftConsumed).Sub(g.GiftDeducted)
			}
		}
		if !paid.Equal(w.PaidBalance) || !gift.Equal(w.GiftBalance) {
			r.Issues = append(r.Issues, Issue{Kind: IssueWalletFormula, UserID: w.UserID, Message: "wallet balance differs from grant formula"})
		}
	}
	for _, f := range s.Refunds {
		if !f.Refunded.Equal(f.Adjusted) {
			r.Issues = append(r.Issues, Issue{Kind: IssueRefundMismatch, Ref: f.OrderID, Message: "order refund total differs from adjustment total"})
		}
	}
	if len(s.DuplicateIdempotencyKeys) > 0 {
		r.Issues = append(r.Issues, Issue{Kind: IssueDuplicateIdempotency, Message: "duplicate idempotency keys detected"})
	}
	if s.InvalidAllocationRows > 0 {
		r.Issues = append(r.Issues, Issue{Kind: IssueInvalidAllocation, Message: "invalid allocation JSON rows detected"})
	}
	if s.UnknownReservations > 0 {
		r.Issues = append(r.Issues, Issue{Kind: IssueUnknownReservation, Message: "pending or unknown reservations detected"})
	}
	if s.CrossOrderGrantRows > 0 {
		r.Issues = append(r.Issues, Issue{Kind: IssueCrossOrderGrant, Message: "allocation references a grant from another order"})
	}
	if !s.LegacyUnknownResidual.IsZero() {
		r.Issues = append(r.Issues, Issue{Kind: IssueLegacyUnknown, Message: "legacy unknown residual is non-zero"})
	}
	r.Global.Issues = len(r.Issues)
	for _, issue := range r.Issues {
		if issue.UserID != 0 {
			u := r.Users[issue.UserID]
			u.Issues++
			r.Users[issue.UserID] = u
		}
	}
	return r
}
