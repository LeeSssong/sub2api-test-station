package store

import (
	"testing"
	"time"

	"example.invalid/relay-ops-service/internal/accounting"
	"example.invalid/relay-ops-service/internal/domain"
	"github.com/shopspring/decimal"
)

func TestCompareCashEventTreatsEquivalentReplayAsIdempotent(t *testing.T) {
	accountID := int64(42)
	paidAt := time.Date(2026, 8, 2, 2, 0, 0, 0, time.UTC)
	input := accounting.CashEventInput{
		EventType:  accounting.EventTypeAccountPurchase,
		PaidAt:     paidAt,
		AmountCNY:  decimal.RequireFromString("68.00"),
		SourceKind: accounting.SourceKindOwnedOAuth,
		AccountID:  &accountID,
		Notes:      "plus account purchase",
	}
	stored := accounting.CashEvent{
		EventType:       input.EventType,
		PaidAt:          paidAt,
		AmountCNY:       decimal.RequireFromString("68"),
		SourceKind:      input.SourceKind,
		AccountID:       &accountID,
		Notes:           input.Notes,
		CreatedByUserID: 7,
	}
	if err := compareCashEvent(stored, input); err != nil {
		t.Fatalf("compareCashEvent equivalent replay: %v", err)
	}
	if err := compareCashEvent(stored, accounting.CashEventInput{
		EventType: input.EventType, PaidAt: paidAt, AmountCNY: decimal.RequireFromString("69"),
		SourceKind: input.SourceKind, AccountID: &accountID, Notes: input.Notes,
	}); err != ErrConflict {
		t.Fatalf("compareCashEvent changed amount error = %v, want ErrConflict", err)
	}
}

func TestCreateCashEventRejectsInvalidActorAndIdempotencyKey(t *testing.T) {
	st := &Store{}
	input := accounting.CashEventInput{
		EventType:  accounting.EventTypeFee,
		PaidAt:     time.Now(),
		AmountCNY:  decimal.NewFromInt(1),
		SourceKind: accounting.SourceKindUpstreamAPIKey,
	}
	if _, _, err := st.CreateCashEvent(nil, domain.AdminActor{}, input, "key"); err == nil {
		t.Fatal("CreateCashEvent accepted invalid actor")
	}
	if _, _, err := st.CreateCashEvent(nil, domain.AdminActor{UserID: 1}, input, " "); err == nil {
		t.Fatal("CreateCashEvent accepted blank idempotency key")
	}
}
