package service

import (
	"errors"
	"strings"

	"github.com/shopspring/decimal"
)

var ErrInvalidAdminRecharge = errors.New("invalid admin recharge")

// AdminRechargeInput is the server-side contract for a standard administrator
// recharge order. Amount is the order's currency amount; GiftQuota is always
// a separate station-credit bucket and is never inferred from a discount.
type AdminRechargeInput struct {
	UserID         int64
	OperatorUserID int64
	Amount         decimal.Decimal
	GiftQuota      decimal.Decimal
	PaymentType    string
	PaymentTradeNo string
	Note           string
}

func ValidateAdminRechargeInput(in *AdminRechargeInput) error {
	if in == nil || in.UserID <= 0 || in.OperatorUserID <= 0 || in.Amount.LessThanOrEqual(decimal.Zero) || in.GiftQuota.IsNegative() || strings.TrimSpace(in.PaymentTradeNo) == "" || strings.TrimSpace(in.Note) == "" {
		return ErrInvalidAdminRecharge
	}
	if strings.TrimSpace(in.PaymentType) == "" {
		in.PaymentType = "admin_recharge"
	}
	if in.PaymentType != "admin_recharge" {
		return ErrInvalidAdminRecharge
	}
	return nil
}
