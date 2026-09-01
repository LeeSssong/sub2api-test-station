package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

// paymentOrderQuotaSnapshot captures the immutable quota interpretation used
// when an order is created. It intentionally preserves the legacy 1:1 balance
// rule while recording the provider currency for later reconciliation.
func paymentOrderQuotaSnapshot(req CreateOrderRequest, sel *payment.InstanceSelection, orderAmount, payAmount, feeRate float64) (decimal.Decimal, string, map[string]any) {
	currency := payment.DefaultPaymentCurrency
	if sel != nil {
		currency = paymentProviderConfigCurrency(sel.ProviderKey, sel.Config)
	}
	paid := decimal.NewFromFloat(orderAmount)
	status := "confirmed"
	if orderAmount <= 0 {
		status = "legacy_unknown"
		paid = decimal.Zero
	}
	snapshot := map[string]any{
		"currency":              currency,
		"quota_conversion_rule": "legacy_balance_amount_1_to_1",
		"order_type":            req.OrderType,
		"order_amount":          decimal.NewFromFloat(orderAmount).StringFixed(8),
		"pay_amount":            decimal.NewFromFloat(payAmount).StringFixed(8),
		"fee_rate":              decimal.NewFromFloat(feeRate).StringFixed(8),
	}
	return paid, status, snapshot
}

func refundQuotaLimitForOrder(attribution, currency string, orderAmount, consumedPaid, refundedPaid decimal.Decimal) decimal.Decimal {
	if strings.EqualFold(strings.TrimSpace(attribution), "legacy_unknown") {
		return decimal.Zero
	}
	limit := orderAmount.Sub(consumedPaid).Sub(refundedPaid)
	if limit.IsNegative() {
		return decimal.Zero
	}
	return limit
}

func validateRefundAgainstQuota(requested, limit decimal.Decimal, force bool) error {
	if requested.LessThanOrEqual(decimal.Zero) || limit.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("refund quota unavailable")
	}
	if requested.GreaterThan(limit) {
		return fmt.Errorf("refund exceeds refundable paid quota")
	}
	return nil
}
