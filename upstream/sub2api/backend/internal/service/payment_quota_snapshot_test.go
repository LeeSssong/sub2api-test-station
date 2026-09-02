package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

func TestPaymentOrderQuotaSnapshotKeepsExistingForeignCurrencyAmountRule(t *testing.T) {
	paid, status, snapshot := paymentOrderQuotaSnapshot(CreateOrderRequest{OrderType: payment.OrderTypeBalance}, &payment.InstanceSelection{ProviderKey: payment.TypeStripe, Config: map[string]string{"currency": "USD"}}, 25, 23, 0)
	if status != "confirmed" {
		t.Fatalf("status=%q, want confirmed", status)
	}
	if !paid.Equal(decimal.NewFromInt(25)) {
		t.Fatalf("paid=%s, want 25", paid)
	}
	if snapshot["currency"] != "USD" || snapshot["quota_conversion_rule"] != "legacy_balance_amount_1_to_1" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}
