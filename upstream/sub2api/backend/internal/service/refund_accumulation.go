package service

import (
	"errors"

	"github.com/shopspring/decimal"
)

var ErrRefundTotalExceeded = errors.New("refund total exceeds order amount")

func accumulateRefundAmount(previous, orderAmount, current decimal.Decimal) (decimal.Decimal, error) {
	if previous.IsNegative() || current.IsNegative() || orderAmount.IsNegative() {
		return decimal.Zero, ErrRefundTotalExceeded
	}
	total := previous.Add(current)
	if total.GreaterThan(orderAmount) {
		return decimal.Zero, ErrRefundTotalExceeded
	}
	return total, nil
}
