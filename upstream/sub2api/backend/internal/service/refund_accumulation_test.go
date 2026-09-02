package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestRefundAccumulationAddsPartialRefundsWithoutExceedingOrder(t *testing.T) {
	got, err := accumulateRefundAmount(decimal.NewFromInt(20), decimal.NewFromInt(100), decimal.NewFromInt(80))
	if err != nil || !got.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("got=%s err=%v, want 100", got, err)
	}
	if _, err := accumulateRefundAmount(decimal.NewFromInt(1), decimal.NewFromInt(100), decimal.NewFromInt(100)); err == nil {
		t.Fatal("refund total above order amount must be rejected")
	}
}
