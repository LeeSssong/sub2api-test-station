package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestRefundQuotaLimitForConfirmedCNYOrder(t *testing.T) {
	limit := refundQuotaLimitForOrder("confirmed", "CNY", decimal.NewFromInt(100), decimal.NewFromInt(20), decimal.NewFromInt(15))
	if !limit.Equal(decimal.NewFromInt(65)) {
		t.Fatalf("limit=%s, want 65", limit)
	}
}

func TestRefundQuotaLimitLeavesLegacyUnknownUnconstrained(t *testing.T) {
	limit := refundQuotaLimitForOrder("legacy_unknown", "CNY", decimal.NewFromInt(100), decimal.NewFromInt(20), decimal.NewFromInt(15))
	if !limit.IsZero() {
		t.Fatalf("legacy unknown should not invent a quota limit: %s", limit)
	}
}

func TestValidateRefundAgainstQuotaRejectsOverLimitEvenWhenForced(t *testing.T) {
	err := validateRefundAgainstQuota(decimal.NewFromInt(60), decimal.NewFromInt(50), true)
	if err == nil { t.Fatal("force_refund must not bypass paid quota limit") }
}
