package reconciliation

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestValidateAttempt(t *testing.T) {
	input, err := ValidateAttempt(AttemptInput{
		AttemptID: " attempt-1 ", LocalRequestID: " request-1 ", AccountID: 7,
		AdapterType: AdapterNewAPI, UserCharge: decimal.RequireFromString("0.12"),
		Currency: "usd", RequestStatus: "success", CompletedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("ValidateAttempt: %v", err)
	}
	if input.AttemptID != "attempt-1" || input.LocalRequestID != "request-1" || input.Currency != "USD" {
		t.Fatalf("normalized input = %#v", input)
	}
}

func TestValidateManualAdjustmentOnlyRequiresAmountAndRowContext(t *testing.T) {
	input, err := ValidateManualAdjustment(ManualAdjustmentInput{
		AttemptID: 19, Amount: decimal.RequireFromString("0.0821"), ActorUserID: 3,
		IdempotencyKey: "manual:19:1",
	})
	if err != nil {
		t.Fatalf("ValidateManualAdjustment: %v", err)
	}
	if input.Notes != "" {
		t.Fatalf("notes = %q, want empty", input.Notes)
	}
}

func TestCoverageRatio(t *testing.T) {
	if got := CoverageRatio(998, 1000).StringFixed(3); got != "0.998" {
		t.Fatalf("CoverageRatio = %s", got)
	}
	if got := CoverageRatio(0, 0); !got.IsZero() {
		t.Fatalf("empty CoverageRatio = %s", got)
	}
}

func TestValidateAutomaticTransaction(t *testing.T) {
	input, err := ValidateAutomaticTransaction(AutomaticTransactionInput{
		AttemptID: 1, AccountID: 7, SourceType: SourceAutomaticCharge,
		SourceRecordID: " log-9 ", Amount: decimal.RequireFromString("0.031"), Currency: "usd",
		OccurredAt: time.Now(), IdempotencyKey: "newapi:log-9",
	})
	if err != nil {
		t.Fatalf("ValidateAutomaticTransaction: %v", err)
	}
	if input.SourceRecordID != "log-9" || input.Currency != "USD" {
		t.Fatalf("normalized input = %#v", input)
	}
	input.SourceType = SourceAutomaticRefund
	if _, err := ValidateAutomaticTransaction(input); err == nil {
		t.Fatal("positive automatic refund accepted")
	}
}
