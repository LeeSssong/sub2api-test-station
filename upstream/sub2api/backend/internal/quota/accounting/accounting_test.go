package accounting

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestApplyLegacyDebtOffsetAcrossOrdinaryGrants(t *testing.T) {
	opening := decimal.NewFromInt(-150)
	grants := []Grant{{ID: 1, Paid: decimal.NewFromInt(100)}, {ID: 2, Paid: decimal.NewFromInt(100)}}
	first := ApplyLegacyDebtOffset(opening, grants[0].Paid, decimal.Zero)
	second := ApplyLegacyDebtOffset(opening, grants[1].Paid, first.Offset)
	if !first.Offset.Equal(decimal.NewFromInt(100)) || !second.Offset.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("unexpected offsets: first=%s second=%s", first.Offset, second.Offset)
	}
}

func TestAllocateFIFOUsesPaidThenGiftAndSkipsOpening(t *testing.T) {
	grants := []Grant{
		{ID: 1, Type: "migration_opening", Paid: decimal.NewFromInt(-20)},
		{ID: 2, Type: "payment_order", Paid: decimal.NewFromInt(3), ConsumedPaid: decimal.NewFromInt(1)},
		{ID: 3, Type: "payment_order", Gift: decimal.NewFromInt(5)},
	}
	result, err := Allocate(grants, decimal.NewFromInt(6))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Paid.Equal(decimal.NewFromInt(2)) || !result.Gift.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("unexpected split: paid=%s gift=%s", result.Paid, result.Gift)
	}
	if len(result.Allocations) != 2 || result.Allocations[0].GrantID != 2 || result.Allocations[1].GrantID != 3 {
		t.Fatalf("unexpected allocations: %#v", result.Allocations)
	}
}

func TestAllocateRejectsInsufficientWithoutPartialMutation(t *testing.T) {
	grants := []Grant{{ID: 1, Type: "payment_order", Paid: decimal.NewFromInt(2)}}
	result, err := Allocate(grants, decimal.NewFromInt(3))
	if err != ErrInsufficientQuota {
		t.Fatalf("expected insufficient quota, got %v", err)
	}
	if len(result.Allocations) != 0 || !result.Paid.IsZero() || !result.Gift.IsZero() {
		t.Fatalf("insufficient allocation must be all-or-nothing: %#v", result)
	}
}

func TestDeductGiftFIFOExcludesOpeningAndIsAllOrNothing(t *testing.T) {
	grants := []Grant{
		{ID: 1, Type: "migration_opening", Gift: decimal.NewFromInt(99)},
		{ID: 2, Type: "admin_gift", Gift: decimal.NewFromInt(3), ConsumedGift: decimal.NewFromInt(1)},
		{ID: 3, Type: "promo_bonus", Gift: decimal.NewFromInt(4), DeductedGift: decimal.NewFromInt(1)},
	}
	result, err := DeductGift(grants, decimal.NewFromInt(5))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Gift.Equal(decimal.NewFromInt(5)) || len(result.Allocations) != 2 {
		t.Fatalf("unexpected deduction: %#v", result)
	}
	if result.Allocations[0].GrantID != 2 || result.Allocations[1].GrantID != 3 {
		t.Fatalf("expected FIFO allocations, got %#v", result.Allocations)
	}
	if _, err := DeductGift(grants, decimal.NewFromInt(6)); err != ErrInsufficientQuota {
		t.Fatalf("expected all-or-nothing insufficient result, got %v", err)
	}
}

func TestRefundablePaidQuotaExcludesOpeningAndAllConsumedReservations(t *testing.T) {
	grants := []Grant{
		{ID: 1, Type: "migration_opening", Paid: decimal.NewFromInt(-50)},
		{ID: 2, Type: "payment_order", Paid: decimal.NewFromInt(100), ConsumedPaid: decimal.NewFromInt(20), RefundedPaid: decimal.NewFromInt(10), ReservedPaid: decimal.NewFromInt(5), DebtOffsetPaid: decimal.NewFromInt(15)},
		{ID: 3, Type: "payment_order", Paid: decimal.NewFromInt(40), ConsumedPaid: decimal.NewFromInt(40)},
	}
	got := RefundablePaidQuota(grants)
	if !got.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("refundable paid quota=%s, want 50", got)
	}
}

func TestAllocatePaidRefundUsesFIFOAndNeverTouchesGift(t *testing.T) {
	grants := []Grant{
		{ID: 1, Type: "payment_order", Paid: decimal.NewFromInt(30), ConsumedPaid: decimal.NewFromInt(5)},
		{ID: 2, Type: "payment_order", Paid: decimal.NewFromInt(40), ConsumedPaid: decimal.NewFromInt(10)},
	}
	result, err := AllocatePaidRefund(grants, decimal.NewFromInt(50))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Paid.Equal(decimal.NewFromInt(50)) || result.Gift.Sign() != 0 || len(result.Allocations) != 2 {
		t.Fatalf("unexpected refund allocation: %#v", result)
	}
	if result.Allocations[0].GrantID != 1 || !result.Allocations[0].Quota.Equal(decimal.NewFromInt(25)) || result.Allocations[1].GrantID != 2 || !result.Allocations[1].Quota.Equal(decimal.NewFromInt(25)) {
		t.Fatalf("unexpected FIFO allocations: %#v", result.Allocations)
	}
}

func TestRefundLimitUsesLowerOfRemainingCashAndPaidQuota(t *testing.T) {
	if got := RefundLimit(decimal.NewFromInt(90), decimal.NewFromInt(50)); !got.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("got %s", got)
	}
	if got := RefundLimit(decimal.NewFromInt(20), decimal.NewFromInt(50)); !got.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("got %s", got)
	}
	if got := RefundLimit(decimal.NewFromInt(-1), decimal.NewFromInt(50)); !got.IsZero() {
		t.Fatalf("negative cash must clamp to zero: %s", got)
	}
}

func TestValidateGrantInputRequiresPositiveTotalAndKnownSource(t *testing.T) {
	if err := (GrantInput{Type: "payment_order", Paid: decimal.NewFromInt(1)}).Validate(); err == nil {
		t.Fatal("payment grant without source must be rejected")
	}
	if err := (GrantInput{UserID: 1, Type: "admin_gift", Gift: decimal.NewFromInt(1), IdempotencyKey: "k"}).Validate(); err != nil {
		t.Fatalf("admin gift should be valid: %v", err)
	}
}
